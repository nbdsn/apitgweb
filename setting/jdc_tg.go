package setting

import "github.com/QuantumNous/new-api/setting/config"

type JDCTGSetting struct {
	BotEnabled              bool     `json:"bot_enabled"`
	BotToken                string   `json:"bot_token"`
	AdminIDs                []string `json:"admin_ids"`
	AutoAdjustEnabled       bool     `json:"auto_adjust_enabled"`
	AutoAdjustTime          string   `json:"auto_adjust_time"`
	LowQuotaThreshold       float64  `json:"low_quota_threshold"`
	LowQuotaTarget          float64  `json:"low_quota_target"`
	HighQuotaThreshold      float64  `json:"high_quota_threshold"`
	HighQuotaTarget         float64  `json:"high_quota_target"`
	AdjustWhitelist         []string `json:"adjust_whitelist"`
	DailyReportEnabled      bool     `json:"daily_report_enabled"`
}

var jdcTGSetting = JDCTGSetting{
	BotEnabled:         false,
	BotToken:           "",
	AdminIDs:           []string{},
	AutoAdjustEnabled:  false,
	AutoAdjustTime:     "00:00",
	LowQuotaThreshold:  20,
	LowQuotaTarget:     100,
	HighQuotaThreshold: 200,
	HighQuotaTarget:    80,
	AdjustWhitelist:    []string{},
	DailyReportEnabled: true,
}

func init() {
	config.GlobalConfig.Register("jdc_tg", &jdcTGSetting)
}

func GetJDCTGSetting() *JDCTGSetting {
	return &jdcTGSetting
}
