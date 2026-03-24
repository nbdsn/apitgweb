package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

func RunJDCQuotaAdjustment(c *gin.Context) {
	actions, err := service.RunJDCQuotaAdjustment()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	report := service.BuildJDCQuotaAdjustmentReport(actions)
	_ = service.SendJDCTelegramAdminMessage(report)
	common.ApiSuccess(c, gin.H{
		"actions": actions,
		"report":  report,
	})
}

func ListJDCQuotaAdjustmentLogs(c *gin.Context) {
	logs, err := service.ListJDCQuotaAdjustmentLogs(100)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, logs)
}

func SendJDCTelegramTest(c *gin.Context) {
	if err := service.SendJDCTelegramAdminMessage("JDC TG 测试消息：机器人配置已生效。\n时间：" + common.GetTimeString()); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"ok": true})
}
