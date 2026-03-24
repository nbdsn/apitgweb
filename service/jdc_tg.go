package service

import (
	"bytes"
	"encoding/json"
	"fmt"
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

type telegramUpdate struct {
	UpdateID int             `json:"update_id"`
	Message  *telegramMessage `json:"message"`
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

var telegramPollState struct {
	sync.Mutex
	offset int
	token  string
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
	if !cfg.BotEnabled || cfg.BotToken == "" {
		return nil
	}
	for _, adminID := range cfg.AdminIDs {
		adminID = strings.TrimSpace(adminID)
		if adminID == "" {
			continue
		}
		if err := sendTelegramMessage(cfg.BotToken, adminID, message); err != nil {
			return err
		}
	}
	return nil
}

func sendTelegramMessage(token, chatID, message string) error {
	chunks := splitTelegramMessage(message, 3500)
	for _, chunk := range chunks {
		payload := map[string]any{
			"chat_id": chatID,
			"text":    chunk,
		}
		body, _ := json.Marshal(payload)
		resp, err := http.Post("https://api.telegram.org/bot"+token+"/sendMessage", "application/json", bytes.NewReader(body))
		if err != nil {
			return err
		}
		_ = resp.Body.Close()
		if resp.StatusCode >= 300 {
			return fmt.Errorf("telegram send failed with status %d", resp.StatusCode)
		}
	}
	return nil
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
	response := executeTelegramCommand(strings.TrimSpace(msg.Text))
	if err := sendTelegramMessage(token, strconv.FormatInt(msg.Chat.ID, 10), response); err != nil {
		common.SysLog("[jdc-tg] send command response failed: " + err.Error())
	}
}

func executeTelegramCommand(text string) string {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return telegramHelpText()
	}
	cmd := strings.ToLower(strings.SplitN(fields[0], "@", 2)[0])
	switch cmd {
	case "/start", "/help":
		return telegramHelpText()
	case "/stats":
		return buildTelegramStats()
	case "/users":
		return buildTelegramUsersSummary()
	case "/user":
		if len(fields) < 2 {
			return "用法: /user 用户名或ID"
		}
		return buildTelegramUserDetail(fields[1])
	case "/addquota":
		if len(fields) < 3 {
			return "用法: /addquota 用户名或ID 额度"
		}
		return applyTelegramQuotaChange(fields[1], fields[2], true)
	case "/subquota":
		if len(fields) < 3 {
			return "用法: /subquota 用户名或ID 额度"
		}
		return applyTelegramQuotaChange(fields[1], fields[2], false)
	case "/redeem":
		return createTelegramRedemption(fields[1:])
	default:
		return telegramHelpText()
	}
}

func telegramHelpText() string {
	return strings.Join([]string{
		"JDC TG 管理命令",
		"/stats 查看使用统计、资源消耗、性能指标",
		"/users 查看用户总览",
		"/user 用户名或ID 查看用户详情",
		"/addquota 用户名或ID 100 给用户增加 100 展示额度",
		"/subquota 用户名或ID 100 给用户减少 100 展示额度",
		"/redeem 100 1 名称 24 生成 1 个 100 额度、24 小时有效兑换码",
	}, "\n")
}

func buildTelegramStats() string {
	var totalUsers int64
	var enabledUsers int64
	var totalChannels int64
	var totalTokens int64
	_ = model.DB.Model(&model.User{}).Count(&totalUsers).Error
	_ = model.DB.Model(&model.User{}).Where("status = ?", common.UserStatusEnabled).Count(&enabledUsers).Error
	_ = model.DB.Model(&model.Channel{}).Count(&totalChannels).Error
	_ = model.DB.Model(&model.Token{}).Count(&totalTokens).Error
	since := time.Now().Add(-24 * time.Hour).Unix()
	stat, _ := model.SumUsedQuota(model.LogTypeConsume, since, time.Now().Unix(), "", "", "", 0, "")
	sys := common.GetSystemStatus()
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	return strings.Join([]string{
		"系统概览",
		fmt.Sprintf("用户总数: %d", totalUsers),
		fmt.Sprintf("启用用户: %d", enabledUsers),
		fmt.Sprintf("渠道数: %d", totalChannels),
		fmt.Sprintf("令牌数: %d", totalTokens),
		fmt.Sprintf("近24h消耗额度: %.2f", common.RawQuotaToDisplay(stat.Quota)),
		fmt.Sprintf("近24h RPM: %d", stat.Rpm),
		fmt.Sprintf("近24h TPM: %d", stat.Tpm),
		fmt.Sprintf("CPU: %.2f%%", sys.CPUUsage),
		fmt.Sprintf("内存: %.2f%%", sys.MemoryUsage),
		fmt.Sprintf("磁盘: %.2f%%", sys.DiskUsage),
		fmt.Sprintf("Goroutine: %d", runtime.NumGoroutine()),
		fmt.Sprintf("Heap: %.2f MB", float64(mem.Alloc)/1024/1024),
	}, "\n")
}

func buildTelegramUsersSummary() string {
	var users []*model.User
	if err := model.DB.Order("quota asc").Limit(10).Find(&users).Error; err != nil {
		return "查询用户失败: " + err.Error()
	}
	lines := []string{"用户概览（额度最低前 10）"}
	for _, user := range users {
		lines = append(lines, fmt.Sprintf("%s | 余额 %.2f | 已用 %.2f", user.Username, common.RawQuotaToDisplay(user.Quota), common.RawQuotaToDisplay(user.UsedQuota)))
	}
	return strings.Join(lines, "\n")
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
