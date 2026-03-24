package model

import "gorm.io/gorm"

type JDCBackupRecord struct {
	ID          int            `json:"id"`
	Filename    string         `json:"filename" gorm:"size:255;index"`
	Filepath    string         `json:"filepath" gorm:"size:1024"`
	BackupType  string         `json:"backup_type" gorm:"size:32;index"`
	Status      string         `json:"status" gorm:"size:32;index"`
	SizeBytes   int64          `json:"size_bytes" gorm:"default:0"`
	Remark      string         `json:"remark" gorm:"size:1024"`
	CreatedAt   int64          `json:"created_at" gorm:"bigint;index"`
	FinishedAt  int64          `json:"finished_at" gorm:"bigint"`
	DeletedAt   gorm.DeletedAt `json:"-" gorm:"index"`
}

func (JDCBackupRecord) TableName() string {
	return "jdc_backup_records"
}

func CreateJDCBackupRecord(record *JDCBackupRecord) error {
	return DB.Create(record).Error
}

func UpdateJDCBackupRecord(record *JDCBackupRecord) error {
	return DB.Save(record).Error
}

func ListJDCBackupRecords(limit int) ([]*JDCBackupRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	var records []*JDCBackupRecord
	err := DB.Order("id desc").Limit(limit).Find(&records).Error
	return records, err
}

func GetJDCBackupRecordByID(id int) (*JDCBackupRecord, error) {
	var record JDCBackupRecord
	err := DB.First(&record, id).Error
	return &record, err
}
