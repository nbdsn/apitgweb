package setting

import (
	"path/filepath"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
)

type JDCBackupSetting struct {
	BackupEnabled        bool   `json:"backup_enabled"`
	BackupTime           string `json:"backup_time"`
	BackupDir            string `json:"backup_dir"`
	RetainCount          int    `json:"retain_count"`
	CleanupEnabled       bool   `json:"cleanup_enabled"`
	LogRetentionDays     int    `json:"log_retention_days"`
	QuotaDataRetentionDays int  `json:"quota_data_retention_days"`
	IncludeLogsInExport  bool   `json:"include_logs_in_export"`
}

var jdcBackupSetting = JDCBackupSetting{
	BackupEnabled:          true,
	BackupTime:             "00:00",
	BackupDir:              "",
	RetainCount:            7,
	CleanupEnabled:         true,
	LogRetentionDays:       30,
	QuotaDataRetentionDays: 30,
	IncludeLogsInExport:    false,
}

func init() {
	config.GlobalConfig.Register("jdc_backup", &jdcBackupSetting)
}

func GetJDCBackupSetting() *JDCBackupSetting {
	return &jdcBackupSetting
}

func (s *JDCBackupSetting) GetResolvedBackupDir() string {
	if s.BackupDir != "" {
		return s.BackupDir
	}
	if common.LogDir != nil && *common.LogDir != "" {
		return filepath.Join(*common.LogDir, "jdc-backups")
	}
	return "./logs/jdc-backups"
}
