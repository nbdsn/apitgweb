package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
)

type JDCQuotaAdjustmentAction struct {
	UserID      int    `json:"user_id"`
	Username    string `json:"username"`
	Action      string `json:"action"`
	Reason      string `json:"reason"`
	BeforeQuota int    `json:"before_quota"`
	AfterQuota  int    `json:"after_quota"`
	DeltaQuota  int    `json:"delta_quota"`
}

type telegramUpdateResponse struct {
	OK     bool             `json:"ok"`
	Result []telegramUpdate `json:"result"`
}

type telegramAPIResponse struct {
	OK          bool   `json:"ok"`
	Description string `json:"description"`
}

type telegramUpdate struct {
	UpdateID       int                   `json:"update_id"`
	Message        *telegramMessage      `json:"message"`
	CallbackQuery  *telegramCallbackData `json:"callback_query"`
}

type telegramMessage struct {
	MessageID int           `json:"message_id"`
	Text      string        `json:"text"`
	Chat      telegramChat  `json:"chat"`
	From      *telegramUser `json:"from"`
}

type telegramChat struct {
	ID int64 `json:"id"`
}

type telegramUser struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

type telegramCallbackData struct {
	ID      string           `json:"id"`
	From    *telegramUser    `json:"from"`
	Message *telegramMessage `json:"message"`
	Data    string           `json:"data"`
}

var telegramPollState struct {
	sync.Mutex
	offset int
	token  string
}

var telegramCommandState struct {
	sync.Mutex
	token      string
	lastSynced int64
}

func StartJDCTelegramTasks() {
	if !common.IsMasterNode {
		return
	}
	runDailyTask(
		"jdc-backup",
		func() string { return setting.GetJDCBackupSetting().BackupTime },
		func() bool { return setting.GetJDCBackupSetting().BackupEnabled },
		func() {
			record, err := CreateJDCBackup(JDCBackupTypeScheduled)
			if err != nil {
				common.SysLog("[jdc-backup] scheduled backup failed: " + err.Error())
				return
			}
			common.SysLog("[jdc-backup] scheduled backup created: " + record.Filepath)
			if setting.GetJDCBackupSetting().CleanupEnabled {
				if _, err := CleanupJDCDatabaseLogs(); err != nil {
					common.SysLog("[jdc-backup] cleanup failed: " + err.Error())
				}
			}
		},
	)

	runDailyTask(
		"jdc-tg-adjust",
		func() string { return setting.GetJDCTGSetting().AutoAdjustTime },
		func() bool { return setting.GetJDCTGSetting().AutoAdjustEnabled },
		func() {
			actions, err := RunJDCQuotaAdjustment()
			if err != nil {
				common.SysLog("[jdc-tg] auto adjust failed: " + err.Error())
				if setting.GetJDCTGSetting().DailyReportEnabled {
					_ = SendJDCTelegramAdminMessage("每日额度处理失败: " + err.Error())
				}
				return
			}
			if setting.GetJDCTGSetting().DailyReportEnabled {
				_ = SendJDCTelegramAdminMessage(BuildJDCQuotaAdjustmentReport(actions))
			}
		},
	)

	go telegramPollingLoop()
}

func RunJDCQuotaAdjustment() ([]*JDCQuotaAdjustmentAction, error) {
	cfg := setting.GetJDCTGSetting()
	lowThresholdRaw := common.DisplayQuotaToRaw(cfg.LowQuotaThreshold)
	lowTargetRaw := common.DisplayQuotaToRaw(cfg.LowQuotaTarget)
	highThresholdRaw := common.DisplayQuotaToRaw(cfg.HighQuotaThreshold)
	highTargetRaw := common.DisplayQuotaToRaw(cfg.HighQuotaTarget)
	whitelist := make(map[string]struct{}, len(cfg.AdjustWhitelist))
	for _, item := range cfg.AdjustWhitelist {
		trimmed := strings.ToLower(strings.TrimSpace(item))
		if trimmed != "" {
			whitelist[trimmed] = struct{}{}
		}
	}

	var users []*model.User
	if err := model.DB.Where("status = ? AND role = ?", common.UserStatusEnabled, common.RoleCommonUser).Find(&users).Error; err != nil {
		return nil, err
	}

	actions := make([]*JDCQuotaAdjustmentAction, 0)
	logs := make([]*model.JDCQuotaAdjustmentLog, 0)
	taskDate := time.Now().Format("2006-01-02")
	for _, user := range users {
		if _, ok := whitelist[strings.ToLower(strings.TrimSpace(user.Username))]; ok {
			continue
		}
		beforeQuota := user.Quota
		afterQuota := beforeQuota
		reason := ""
		actionType := ""
		if beforeQuota < lowThresholdRaw {
			afterQuota = lowTargetRaw
			reason = fmt.Sprintf("低于 %.2f，补到 %.2f", cfg.LowQuotaThreshold, cfg.LowQuotaTarget)
			actionType = "increase"
		} else if beforeQuota > highThresholdRaw {
			afterQuota = highTargetRaw
			reason = fmt.Sprintf("高于 %.2f，改为 %.2f", cfg.HighQuotaThreshold, cfg.HighQuotaTarget)
			actionType = "decrease"
		}
		if afterQuota == beforeQuota {
			continue
		}
		delta := afterQuota - beforeQuota
		if err := model.DeltaUpdateUserQuota(user.Id, delta); err != nil {
			return actions, err
		}
		logText := fmt.Sprintf("JDC 每日额度处理：原额度 %s，现额度 %s，变动 %s，规则：%s", logger.LogQuota(beforeQuota), logger.LogQuota(afterQuota), logger.LogQuota(absInt(delta)), reason)
		model.RecordLog(user.Id, model.LogTypeManage, logText)
		action := &JDCQuotaAdjustmentAction{
			UserID:      user.Id,
			Username:    user.Username,
			Action:      actionType,
			Reason:      reason,
			BeforeQuota: beforeQuota,
			AfterQuota:  afterQuota,
			DeltaQuota:  delta,
		}
		actions = append(actions, action)
		logs = append(logs, &model.JDCQuotaAdjustmentLog{
			TaskDate:    taskDate,
			UserID:      user.Id,
			Username:    user.Username,
			Action:      actionType,
			Reason:      reason,
			BeforeQuota: beforeQuota,
			AfterQuota:  afterQuota,
			DeltaQuota:  delta,
			CreatedAt:   common.GetTimestamp(),
		})
	}
	if err := model.CreateJDCQuotaAdjustmentLogs(logs); err != nil {
		return actions, err
	}
	return actions, nil
}

func BuildJDCQuotaAdjustmentReport(actions []*JDCQuotaAdjustmentAction) string {
	if len(actions) == 0 {
		return fmt.Sprintf("每日额度处理完成\n时间：%s\n本次没有需要调整的用户。", time.Now().Format("2006-01-02 15:04:05"))
	}
	sort.Slice(actions, func(i, j int) bool {
		if actions[i].Action == actions[j].Action {
			return actions[i].Username < actions[j].Username
		}
		return actions[i].Action < actions[j].Action
	})
	lines := []string{fmt.Sprintf("每日额度处理完成\n时间：%s\n共处理 %d 个用户", time.Now().Format("2006-01-02 15:04:05"), len(actions))}
	for _, action := range actions {
		verb := "增加"
		if action.DeltaQuota < 0 {
			verb = "减少"
		}
		lines = append(lines,
			fmt.Sprintf(
				"用户 %s 当前剩余 %.2f，%s %.2f，现有 %.2f；规则：%s",
				action.Username,
				common.RawQuotaToDisplay(action.BeforeQuota),
				verb,
				common.RawQuotaToDisplay(absInt(action.DeltaQuota)),
				common.RawQuotaToDisplay(action.AfterQuota),
				action.Reason,
			),
		)
	}
	return strings.Join(lines, "\n")
}

func ListJDCQuotaAdjustmentLogs(limit int) ([]*model.JDCQuotaAdjustmentLog, error) {
	return model.ListJDCQuotaAdjustmentLogs(limit)
}

func SendJDCTelegramAdminMessage(message string) error {
	cfg := setting.GetJDCTGSetting()
	if !cfg.BotEnabled {
		return fmt.Errorf("TG Bot 未启用")
	}
	if strings.TrimSpace(cfg.BotToken) == "" {
		return fmt.Errorf("TG Bot Token 未配置")
	}
	validAdmins := 0
	var sendErrors []string
	for _, adminID := range cfg.AdminIDs {
		adminID = strings.TrimSpace(adminID)
		if adminID == "" {
			continue
		}
		validAdmins++
		if err := sendTelegramMessage(cfg.BotToken, adminID, message); err != nil {
			sendErrors = append(sendErrors, fmt.Sprintf("%s: %s", adminID, err.Error()))
		}
	}
	if validAdmins == 0 {
		return fmt.Errorf("管理员 ID 未配置，必须填写数字 Telegram 用户 ID，不是用户名")
	}
	if len(sendErrors) == validAdmins {
		return fmt.Errorf("发送失败：%s", strings.Join(sendErrors, " | "))
	}
	return nil
}

func telegramAPIRequest(token, method string, payload map[string]any) ([]byte, error) {
	body, _ := json.Marshal(payload)
	resp, err := http.Post("https://api.telegram.org/bot"+token+"/"+method, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	respBody, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode >= 300 {
		var apiResp telegramAPIResponse
		if err := json.Unmarshal(respBody, &apiResp); err == nil && apiResp.Description != "" {
			return nil, fmt.Errorf("telegram %s failed with status %d: %s", method, resp.StatusCode, apiResp.Description)
		}
		return nil, fmt.Errorf("telegram %s failed with status %d: %s", method, resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	var apiResp telegramAPIResponse
	if err := json.Unmarshal(respBody, &apiResp); err == nil && !apiResp.OK {
		if apiResp.Description != "" {
			return nil, fmt.Errorf("telegram %s failed: %s", method, apiResp.Description)
		}
		return nil, fmt.Errorf("telegram %s failed", method)
	}
	return respBody, nil
}

func sendTelegramMessageWithMarkup(token, chatID, message string, replyMarkup map[string]any) error {
	chunks := splitTelegramMessage(message, 3500)
	for idx, chunk := range chunks {
		payload := map[string]any{
			"chat_id": chatID,
			"text":    chunk,
		}
		if idx == len(chunks)-1 && replyMarkup != nil {
			payload["reply_markup"] = replyMarkup
		}
		if _, err := telegramAPIRequest(token, "sendMessage", payload); err != nil {
			return err
		}
	}
	return nil
}

func sendTelegramMessage(token, chatID, message string) error {
	return sendTelegramMessageWithMarkup(token, chatID, message, nil)
}

func answerTelegramCallback(token, callbackID, text string) {
	if strings.TrimSpace(callbackID) == "" {
		return
	}
	payload := map[string]any{
		"callback_query_id": callbackID,
		"text":              text,
		"show_alert":        false,
	}
	if _, err := telegramAPIRequest(token, "answerCallbackQuery", payload); err != nil {
		common.SysLog("[jdc-tg] answer callback failed: " + err.Error())
	}
}

func syncTelegramCommands(token string) {
	if strings.TrimSpace(token) == "" {
		return
	}
	now := time.Now().Unix()
	telegramCommandState.Lock()
	if telegramCommandState.token == token && now-telegramCommandState.lastSynced < 3600 {
		telegramCommandState.Unlock()
		return
	}
	telegramCommandState.token = token
	telegramCommandState.lastSynced = now
	telegramCommandState.Unlock()

	payload := map[string]any{
		"commands": []map[string]string{
			{"command": "start", "description": "打开 JDC TG 管理菜单"},
			{"command": "help", "description": "查看所有可用命令"},
			{"command": "stats", "description": "查看详细统计和性能指标"},
			{"command": "users", "description": "用户概览 + 交互管理"},
			{"command": "redeem", "description": "交互生成兑换码"},
			{"command": "user", "description": "查看单个用户详情"},
			{"command": "addquota", "description": "给用户增加额度"},
			{"command": "subquota", "description": "给用户减少额度"},
		},
	}
	if _, err := telegramAPIRequest(token, "setMyCommands", payload); err != nil {
		common.SysLog("[jdc-tg] setMyCommands failed: " + err.Error())
	}
}

func buildInlineKeyboard(rows ...[]map[string]string) map[string]any {
	keyboard := make([][]map[string]string, 0, len(rows))
	for _, row := range rows {
		if len(row) == 0 {
			continue
		}
		keyboard = append(keyboard, row)
	}
	return map[string]any{"inline_keyboard": keyboard}
}

func splitTelegramMessage(message string, maxLen int) []string {
	if len(message) <= maxLen {
		return []string{message}
	}
	parts := make([]string, 0)
	remaining := message
	for len(remaining) > maxLen {
		splitAt := strings.LastIndex(remaining[:maxLen], "\n")
		if splitAt <= 0 {
			splitAt = maxLen
		}
		parts = append(parts, remaining[:splitAt])
		remaining = remaining[splitAt:]
		remaining = strings.TrimLeft(remaining, "\n")
	}
	if remaining != "" {
		parts = append(parts, remaining)
	}
	return parts
}

func telegramPollingLoop() {
	for {
		cfg := setting.GetJDCTGSetting()
		if !cfg.BotEnabled || cfg.BotToken == "" {
			time.Sleep(10 * time.Second)
			continue
		}
		syncTelegramCommands(cfg.BotToken)
		if err := pollTelegramUpdates(cfg.BotToken); err != nil {
			common.SysLog("[jdc-tg] poll failed: " + err.Error())
			time.Sleep(5 * time.Second)
		}
	}
}

func pollTelegramUpdates(token string) error {
	telegramPollState.Lock()
	if telegramPollState.token != token {
		telegramPollState.token = token
		telegramPollState.offset = 0
	}
	offset := telegramPollState.offset
	telegramPollState.Unlock()

	url := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?timeout=50&offset=%d", token, offset)
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("telegram getUpdates status %d", resp.StatusCode)
	}
	var payload telegramUpdateResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return err
	}
	for _, update := range payload.Result {
		if update.Message != nil {
			handleTelegramMessage(token, update.Message)
		}
		if update.CallbackQuery != nil {
			handleTelegramCallback(token, update.CallbackQuery)
		}
		telegramPollState.Lock()
		if update.UpdateID >= telegramPollState.offset {
			telegramPollState.offset = update.UpdateID + 1
		}
		telegramPollState.Unlock()
	}
	return nil
}

func handleTelegramMessage(token string, msg *telegramMessage) {
	if msg == nil || msg.Text == "" || msg.From == nil {
		return
	}
	if !isTelegramAdmin(msg.From.ID) {
		return
	}
	text, markup := executeTelegramCommand(strings.TrimSpace(msg.Text))
	if err := sendTelegramMessageWithMarkup(token, strconv.FormatInt(msg.Chat.ID, 10), text, markup); err != nil {
		common.SysLog("[jdc-tg] send command response failed: " + err.Error())
	}
}

func handleTelegramCallback(token string, cb *telegramCallbackData) {
	if cb == nil || cb.From == nil {
		return
	}
	if !isTelegramAdmin(cb.From.ID) {
		answerTelegramCallback(token, cb.ID, "无权限")
		return
	}
	chatID := ""
	if cb.Message != nil {
		chatID = strconv.FormatInt(cb.Message.Chat.ID, 10)
	}
	text, markup, ack := executeTelegramCallback(cb.Data)
	if chatID != "" && text != "" {
		if err := sendTelegramMessageWithMarkup(token, chatID, text, markup); err != nil {
			common.SysLog("[jdc-tg] send callback response failed: " + err.Error())
			answerTelegramCallback(token, cb.ID, "处理失败")
			return
		}
	}
	if strings.TrimSpace(ack) == "" {
		ack = "已处理"
	}
	answerTelegramCallback(token, cb.ID, ack)
}

func executeTelegramCommand(text string) (string, map[string]any) {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return telegramHelpText(), nil
	}
	cmd := strings.ToLower(strings.SplitN(fields[0], "@", 2)[0])
	switch cmd {
	case "/start", "/help":
		return telegramHelpText(), nil
	case "/stats":
		return buildTelegramStats(), nil
	case "/users":
		return buildTelegramUsersSummaryWithMenu()
	case "/user":
		if len(fields) < 2 {
			return "用法: /user 用户名或ID", nil
		}
		return buildTelegramUserDetail(fields[1]), nil
	case "/addquota":
		if len(fields) < 3 {
			return "用法: /addquota 用户名或ID 额度", nil
		}
		return applyTelegramQuotaChange(fields[1], fields[2], true), nil
	case "/subquota":
		if len(fields) < 3 {
			return "用法: /subquota 用户名或ID 额度", nil
		}
		return applyTelegramQuotaChange(fields[1], fields[2], false), nil
	case "/redeem":
		if len(fields) > 1 {
			return createTelegramRedemption(fields[1:]), nil
		}
		return buildRedeemAmountMenu()
	default:
		return telegramHelpText(), nil
	}
}

func telegramHelpText() string {
	return strings.Join([]string{
		"JDC TG 管理命令",
		"/stats 查看详细统计、资源消耗、性能指标",
		"/users 用户概览（最低额度前10）+ 交互管理",
		"/user 用户名或ID 查看用户详情",
		"/addquota 用户名或ID 100 给用户增加 100 展示额度",
		"/subquota 用户名或ID 100 给用户减少 100 展示额度",
		"/redeem 交互生成兑换码（金额 -> 张数）",
	}, "\n")
}

func buildTelegramStats() string {
	var totalUsers int64
	var enabledUsers int64
	var totalChannels int64
	var totalTokens int64
	var totalRequestCount int64
	_ = model.DB.Model(&model.User{}).Count(&totalUsers).Error
	_ = model.DB.Model(&model.User{}).Where("status = ?", common.UserStatusEnabled).Count(&enabledUsers).Error
	_ = model.DB.Model(&model.Channel{}).Count(&totalChannels).Error
	_ = model.DB.Model(&model.Token{}).Count(&totalTokens).Error
	_ = model.DB.Model(&model.User{}).Select("coalesce(sum(request_count),0)").Scan(&totalRequestCount).Error

	since := time.Now().Add(-24 * time.Hour).Unix()
	now := time.Now().Unix()
	stat, _ := model.SumUsedQuota(model.LogTypeConsume, since, now, "", "", "", 0, "")
	var requestCount24h int64
	_ = model.LOG_DB.Model(&model.Log{}).Where("type = ? AND created_at >= ?", model.LogTypeConsume, since).Count(&requestCount24h).Error
	tokenCount24h := model.SumUsedToken(model.LogTypeConsume, since, now, "", "", "")
	avgRPM24h := float64(requestCount24h) / 1440.0
	sys := common.GetSystemStatus()
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	return strings.Join([]string{
		"系统概览（近24小时 + 实时）",
		fmt.Sprintf("用户总数: %d", totalUsers),
		fmt.Sprintf("启用用户: %d", enabledUsers),
		fmt.Sprintf("渠道数: %d", totalChannels),
		fmt.Sprintf("令牌数: %d", totalTokens),
		fmt.Sprintf("请求次数(累计): %d", totalRequestCount),
		fmt.Sprintf("统计次数(近24h): %d", requestCount24h),
		fmt.Sprintf("统计额度(近24h): %.2f", common.RawQuotaToDisplay(stat.Quota)),
		fmt.Sprintf("统计Tokens(近24h): %d", tokenCount24h),
		fmt.Sprintf("平均RPM(近24h): %.2f", avgRPM24h),
		fmt.Sprintf("实时RPM(近60s): %d", stat.Rpm),
		fmt.Sprintf("实时TPM(近60s): %d", stat.Tpm),
		fmt.Sprintf("CPU: %.2f%%", sys.CPUUsage),
		fmt.Sprintf("内存: %.2f%%", sys.MemoryUsage),
		fmt.Sprintf("磁盘: %.2f%%", sys.DiskUsage),
		fmt.Sprintf("Goroutine: %d", runtime.NumGoroutine()),
		fmt.Sprintf("Heap: %.2f MB", float64(mem.Alloc)/1024/1024),
	}, "\n")
}

func buildTelegramUsersSummary() string {
	var users []*model.User
	if err := model.DB.Where("role = ?", common.RoleCommonUser).Order("quota asc").Limit(10).Find(&users).Error; err != nil {
		return "查询用户失败: " + err.Error()
	}
	lines := []string{"用户概览（额度最低前 10）"}
	for _, user := range users {
		statusText := "启用"
		if user.Status != common.UserStatusEnabled {
			statusText = "停用"
		}
		lines = append(lines, fmt.Sprintf("ID:%d | %s | 状态:%s | 余额 %.2f | 已用 %.2f", user.Id, user.Username, statusText, common.RawQuotaToDisplay(user.Quota), common.RawQuotaToDisplay(user.UsedQuota)))
	}
	return strings.Join(lines, "\n")
}

func buildTelegramUsersSummaryWithMenu() (string, map[string]any) {
	var users []*model.User
	if err := model.DB.Where("role = ?", common.RoleCommonUser).Order("quota asc").Limit(10).Find(&users).Error; err != nil {
		return "查询用户失败: " + err.Error(), nil
	}
	lines := []string{"用户概览（额度最低前 10）", "点击用户进入操作菜单："}
	rows := make([][]map[string]string, 0)
	for _, user := range users {
		statusText := "启用"
		if user.Status != common.UserStatusEnabled {
			statusText = "停用"
		}
		lines = append(lines, fmt.Sprintf("ID:%d | %s | 状态:%s | 余额 %.2f", user.Id, user.Username, statusText, common.RawQuotaToDisplay(user.Quota)))
		rows = append(rows, []map[string]string{
			{"text": fmt.Sprintf("%s(%d)", user.Username, user.Id), "callback_data": fmt.Sprintf("jdcu:view:%d", user.Id)},
		})
	}
	rows = append(rows, []map[string]string{{"text": "刷新列表", "callback_data": "jdcu:list"}})
	return strings.Join(lines, "\n"), buildInlineKeyboard(rows...)
}

func buildTelegramUserDetail(query string) string {
	user, err := findSingleUser(query)
	if err != nil {
		return "查询失败: " + err.Error()
	}
	return strings.Join([]string{
		fmt.Sprintf("用户: %s", user.Username),
		fmt.Sprintf("ID: %d", user.Id),
		fmt.Sprintf("邮箱: %s", user.Email),
		fmt.Sprintf("角色: %d", user.Role),
		fmt.Sprintf("状态: %d", user.Status),
		fmt.Sprintf("余额: %.2f", common.RawQuotaToDisplay(user.Quota)),
		fmt.Sprintf("已用: %.2f", common.RawQuotaToDisplay(user.UsedQuota)),
	}, "\n")
}

func applyTelegramQuotaChange(query, amountStr string, increase bool) string {
	user, err := findSingleUser(query)
	if err != nil {
		return "处理失败: " + err.Error()
	}
	amount, err := strconv.ParseFloat(amountStr, 64)
	if err != nil || amount <= 0 {
		return "额度必须是大于 0 的数字"
	}
	raw := common.DisplayQuotaToRaw(amount)
	delta := raw
	verb := "增加"
	if !increase {
		delta = -raw
		verb = "减少"
	}
	before := user.Quota
	if err := model.DeltaUpdateUserQuota(user.Id, delta); err != nil {
		return "处理失败: " + err.Error()
	}
	after := before + delta
	model.RecordLog(user.Id, model.LogTypeManage, fmt.Sprintf("Telegram 管理员%s额度 %s，处理后 %s", verb, logger.LogQuota(absInt(delta)), logger.LogQuota(after)))
	return fmt.Sprintf("已给用户 %s %s %.2f，原余额 %.2f，现余额 %.2f", user.Username, verb, amount, common.RawQuotaToDisplay(before), common.RawQuotaToDisplay(after))
}

func applyTelegramQuotaChangeByUserID(userID int, amount float64, increase bool) string {
	if amount <= 0 {
		return "额度必须大于 0"
	}
	user, err := model.GetUserById(userID, false)
	if err != nil {
		return "处理失败: " + err.Error()
	}
	raw := common.DisplayQuotaToRaw(amount)
	delta := raw
	verb := "增加"
	if !increase {
		delta = -raw
		verb = "减少"
	}
	before := user.Quota
	if err := model.DeltaUpdateUserQuota(user.Id, delta); err != nil {
		return "处理失败: " + err.Error()
	}
	after := before + delta
	model.RecordLog(user.Id, model.LogTypeManage, fmt.Sprintf("Telegram 交互菜单%s额度 %s，处理后 %s", verb, logger.LogQuota(absInt(delta)), logger.LogQuota(after)))
	return fmt.Sprintf("已给用户 %s %s %.2f，原余额 %.2f，现余额 %.2f", user.Username, verb, amount, common.RawQuotaToDisplay(before), common.RawQuotaToDisplay(after))
}

func updateTelegramUserStatus(userID int, enabled bool) string {
	user, err := model.GetUserById(userID, false)
	if err != nil {
		return "处理失败: " + err.Error()
	}
	if user.Role == common.RoleRootUser {
		return "根账号不允许在 TG 菜单中变更状态"
	}
	targetStatus := common.UserStatusDisabled
	action := "停用"
	if enabled {
		targetStatus = common.UserStatusEnabled
		action = "启用"
	}
	if user.Status == targetStatus {
		return fmt.Sprintf("用户 %s 已经是%s状态", user.Username, action)
	}
	user.Status = targetStatus
	if err := user.Update(false); err != nil {
		return "处理失败: " + err.Error()
	}
	model.RecordLog(user.Id, model.LogTypeManage, fmt.Sprintf("Telegram 交互菜单已%s账户", action))
	return fmt.Sprintf("已%s用户 %s (ID:%d)", action, user.Username, user.Id)
}

func buildUserActionMenu(userID int) (string, map[string]any, string) {
	user, err := model.GetUserById(userID, false)
	if err != nil {
		return "用户不存在或读取失败", nil, "用户不存在"
	}
	statusText := "启用"
	if user.Status != common.UserStatusEnabled {
		statusText = "停用"
	}
	text := strings.Join([]string{
		fmt.Sprintf("用户: %s (ID:%d)", user.Username, user.Id),
		fmt.Sprintf("状态: %s", statusText),
		fmt.Sprintf("余额: %.2f", common.RawQuotaToDisplay(user.Quota)),
		fmt.Sprintf("已用: %.2f", common.RawQuotaToDisplay(user.UsedQuota)),
		"请选择下一步：",
	}, "\n")
	keyboard := buildInlineKeyboard(
		[]map[string]string{
			{"text": "增加额度", "callback_data": fmt.Sprintf("jdcu:act:%d:add", user.Id)},
			{"text": "减少额度", "callback_data": fmt.Sprintf("jdcu:act:%d:sub", user.Id)},
		},
		[]map[string]string{
			{"text": "启用账户", "callback_data": fmt.Sprintf("jdcu:set:%d:enable", user.Id)},
			{"text": "停用账户", "callback_data": fmt.Sprintf("jdcu:set:%d:disable", user.Id)},
		},
		[]map[string]string{{"text": "返回用户列表", "callback_data": "jdcu:list"}},
	)
	return text, keyboard, "已打开用户菜单"
}

func buildUserQuotaAmountMenu(userID int, op string) (string, map[string]any, string) {
	user, err := model.GetUserById(userID, false)
	if err != nil {
		return "用户不存在或读取失败", nil, "用户不存在"
	}
	actionText := "增加"
	if op == "sub" {
		actionText = "减少"
	}
	amounts := []int{10, 30, 50, 100, 150, 200, 500}
	rows := make([][]map[string]string, 0)
	row := make([]map[string]string, 0, 3)
	for _, amount := range amounts {
		row = append(row, map[string]string{
			"text":          fmt.Sprintf("%d", amount),
			"callback_data": fmt.Sprintf("jdcu:amt:%d:%s:%d", userID, op, amount),
		})
		if len(row) == 3 {
			rows = append(rows, row)
			row = make([]map[string]string, 0, 3)
		}
	}
	if len(row) > 0 {
		rows = append(rows, row)
	}
	rows = append(rows, []map[string]string{{"text": "返回用户菜单", "callback_data": fmt.Sprintf("jdcu:view:%d", userID)}})
	text := fmt.Sprintf("用户 %s (ID:%d)\n请选择%s额度档位：", user.Username, user.Id, actionText)
	return text, buildInlineKeyboard(rows...), "请选择额度"
}

func createTelegramRedemption(args []string) string {
	if len(args) < 1 {
		return "用法: /redeem 额度 [个数] [名称] [有效小时]"
	}
	amount, err := strconv.ParseFloat(args[0], 64)
	if err != nil || amount <= 0 {
		return "额度必须是大于 0 的数字"
	}
	count := 1
	if len(args) >= 2 {
		if parsed, cErr := strconv.Atoi(args[1]); cErr == nil && parsed > 0 && parsed <= 20 {
			count = parsed
		}
	}
	name := "TG兑换码"
	if len(args) >= 3 && strings.TrimSpace(args[2]) != "" {
		name = args[2]
	}
	expiredTime := int64(0)
	if len(args) >= 4 {
		if hours, hErr := strconv.Atoi(args[3]); hErr == nil && hours > 0 {
			expiredTime = time.Now().Add(time.Duration(hours) * time.Hour).Unix()
		}
	}
	codes := make([]string, 0, count)
	for i := 0; i < count; i++ {
		code := common.GetUUID()
		item := &model.Redemption{
			Name:        name,
			Key:         code,
			CreatedTime: common.GetTimestamp(),
			Quota:       common.DisplayQuotaToRaw(amount),
			ExpiredTime: expiredTime,
		}
		if err := item.Insert(); err != nil {
			return "生成兑换码失败: " + err.Error()
		}
		codes = append(codes, code)
	}
	lines := []string{fmt.Sprintf("已生成 %d 个兑换码，额度 %.2f", len(codes), amount)}
	lines = append(lines, codes...)
	return strings.Join(lines, "\n")
}

func buildRedeemAmountMenu() (string, map[string]any) {
	amounts := []int{20, 50, 100, 150, 200}
	rows := make([][]map[string]string, 0)
	row := make([]map[string]string, 0, 3)
	for _, amount := range amounts {
		row = append(row, map[string]string{
			"text":          fmt.Sprintf("%d", amount),
			"callback_data": fmt.Sprintf("jdcr:amt:%d", amount),
		})
		if len(row) == 3 {
			rows = append(rows, row)
			row = make([]map[string]string, 0, 3)
		}
	}
	if len(row) > 0 {
		rows = append(rows, row)
	}
	return "兑换码生成：请选择金额档位", buildInlineKeyboard(rows...)
}

func buildRedeemCountMenu(amount int) (string, map[string]any, string) {
	counts := []int{1, 5, 10, 20, 30, 50, 100}
	rows := make([][]map[string]string, 0)
	row := make([]map[string]string, 0, 3)
	for _, count := range counts {
		row = append(row, map[string]string{
			"text":          fmt.Sprintf("%d", count),
			"callback_data": fmt.Sprintf("jdcr:cnt:%d:%d", amount, count),
		})
		if len(row) == 3 {
			rows = append(rows, row)
			row = make([]map[string]string, 0, 3)
		}
	}
	if len(row) > 0 {
		rows = append(rows, row)
	}
	rows = append(rows, []map[string]string{{"text": "返回金额菜单", "callback_data": "jdcr:menu"}})
	return fmt.Sprintf("已选择金额 %d，请选择生成张数", amount), buildInlineKeyboard(rows...), "请选择张数"
}

func createTelegramRedemptionInteractive(amount, count int) string {
	if amount <= 0 || count <= 0 {
		return "兑换码参数错误"
	}
	name := fmt.Sprintf("TG兑换码-%s", time.Now().Format("20060102"))
	expiredTime := int64(0)
	codes := make([]string, 0, count)
	for i := 0; i < count; i++ {
		code := common.GetUUID()
		item := &model.Redemption{
			Name:        name,
			Key:         code,
			CreatedTime: common.GetTimestamp(),
			Quota:       common.DisplayQuotaToRaw(float64(amount)),
			ExpiredTime: expiredTime,
		}
		if err := item.Insert(); err != nil {
			return "生成兑换码失败: " + err.Error()
		}
		codes = append(codes, code)
	}
	lines := []string{
		fmt.Sprintf("兑换码已生成：%d 张", count),
		fmt.Sprintf("金额: %d", amount),
		"有效期: 永久",
		"",
		"兑换码列表：",
	}
	lines = append(lines, codes...)
	return strings.Join(lines, "\n")
}

func executeTelegramCallback(data string) (string, map[string]any, string) {
	if strings.TrimSpace(data) == "" {
		return "无效操作", nil, "无效操作"
	}
	parts := strings.Split(data, ":")
	if len(parts) == 0 {
		return "无效操作", nil, "无效操作"
	}

	if data == "jdcu:list" {
		text, markup := buildTelegramUsersSummaryWithMenu()
		return text, markup, "已刷新用户列表"
	}
	if data == "jdcr:menu" {
		text, markup := buildRedeemAmountMenu()
		return text, markup, "请选择金额"
	}

	if len(parts) >= 3 && parts[0] == "jdcu" {
		switch parts[1] {
		case "view":
			userID, err := strconv.Atoi(parts[2])
			if err != nil {
				return "用户参数错误", nil, "参数错误"
			}
			return buildUserActionMenu(userID)
		case "act":
			if len(parts) < 4 {
				return "参数错误", nil, "参数错误"
			}
			userID, err := strconv.Atoi(parts[2])
			if err != nil {
				return "用户参数错误", nil, "参数错误"
			}
			return buildUserQuotaAmountMenu(userID, parts[3])
		case "set":
			if len(parts) < 4 {
				return "参数错误", nil, "参数错误"
			}
			userID, err := strconv.Atoi(parts[2])
			if err != nil {
				return "用户参数错误", nil, "参数错误"
			}
			enabled := strings.EqualFold(parts[3], "enable")
			result := updateTelegramUserStatus(userID, enabled)
			text, markup, _ := buildUserActionMenu(userID)
			return result + "\n\n" + text, markup, "状态已更新"
		case "amt":
			if len(parts) < 5 {
				return "参数错误", nil, "参数错误"
			}
			userID, err := strconv.Atoi(parts[2])
			if err != nil {
				return "用户参数错误", nil, "参数错误"
			}
			op := parts[3]
			amount, err := strconv.Atoi(parts[4])
			if err != nil || amount <= 0 {
				return "额度参数错误", nil, "参数错误"
			}
			result := applyTelegramQuotaChangeByUserID(userID, float64(amount), op == "add")
			text, markup, _ := buildUserActionMenu(userID)
			return result + "\n\n" + text, markup, "额度已更新"
		}
	}

	if len(parts) >= 3 && parts[0] == "jdcr" {
		switch parts[1] {
		case "amt":
			amount, err := strconv.Atoi(parts[2])
			if err != nil || amount <= 0 {
				return "金额参数错误", nil, "参数错误"
			}
			return buildRedeemCountMenu(amount)
		case "cnt":
			if len(parts) < 4 {
				return "参数错误", nil, "参数错误"
			}
			amount, aErr := strconv.Atoi(parts[2])
			count, cErr := strconv.Atoi(parts[3])
			if aErr != nil || cErr != nil || amount <= 0 || count <= 0 {
				return "参数错误", nil, "参数错误"
			}
			result := createTelegramRedemptionInteractive(amount, count)
			text, markup := buildRedeemAmountMenu()
			return result + "\n\n" + text, markup, "兑换码已生成"
		}
	}

	return "未识别的操作", nil, "未识别"
}

func findSingleUser(query string) (*model.User, error) {
	if id, err := strconv.Atoi(query); err == nil {
		return model.GetUserById(id, false)
	}
	users, _, err := model.SearchUsers(query, "", 0, 10)
	if err != nil {
		return nil, err
	}
	if len(users) == 0 {
		return nil, fmt.Errorf("未找到用户 %s", query)
	}
	if len(users) > 1 {
		for _, user := range users {
			if strings.EqualFold(user.Username, query) {
				return user, nil
			}
		}
		return nil, fmt.Errorf("找到多个用户，请改用 ID")
	}
	return users[0], nil
}

func isTelegramAdmin(userID int64) bool {
	for _, adminID := range setting.GetJDCTGSetting().AdminIDs {
		adminID = strings.TrimSpace(adminID)
		if adminID == "" {
			continue
		}
		if strconv.FormatInt(userID, 10) == adminID {
			return true
		}
	}
	return false
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
