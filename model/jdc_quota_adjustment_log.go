package model

import "gorm.io/gorm"

type JDCQuotaAdjustmentLog struct {
	ID            int            `json:"id"`
	TaskDate      string         `json:"task_date" gorm:"size:32;index"`
	UserID        int            `json:"user_id" gorm:"index"`
	Username      string         `json:"username" gorm:"size:128;index"`
	Action        string         `json:"action" gorm:"size:32;index"`
	Reason        string         `json:"reason" gorm:"size:255"`
	BeforeQuota   int            `json:"before_quota"`
	AfterQuota    int            `json:"after_quota"`
	DeltaQuota    int            `json:"delta_quota"`
	CreatedAt     int64          `json:"created_at" gorm:"bigint;index"`
	DeletedAt     gorm.DeletedAt `json:"-" gorm:"index"`
}

func (JDCQuotaAdjustmentLog) TableName() string {
	return "jdc_quota_adjustment_logs"
}

func CreateJDCQuotaAdjustmentLogs(logs []*JDCQuotaAdjustmentLog) error {
	if len(logs) == 0 {
		return nil
	}
	return DB.Create(&logs).Error
}

func ListJDCQuotaAdjustmentLogs(limit int) ([]*JDCQuotaAdjustmentLog, error) {
	if limit <= 0 {
		limit = 100
	}
	var logs []*JDCQuotaAdjustmentLog
	err := DB.Order("id desc").Limit(limit).Find(&logs).Error
	return logs, err
}
