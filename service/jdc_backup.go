package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

const (
	JDCBackupTypeManual    = "manual"
	JDCBackupTypeScheduled = "scheduled"
	JDCBackupTypeImported  = "imported"

	JDCBackupStatusRunning = "running"
	JDCBackupStatusSuccess = "success"
	JDCBackupStatusFailed  = "failed"
)

var jdcBackupMu sync.Mutex

type JDCBackupCleanupResult struct {
	DeletedLogs      int64 `json:"deleted_logs"`
	DeletedQuotaData int64 `json:"deleted_quota_data"`
}

type snapshotTable struct {
	Model any
	Name  string
}

func getSnapshotTables(includeLogs bool) []snapshotTable {
	tables := []snapshotTable{
		{Model: &model.Channel{}},
		{Model: &model.Token{}},
		{Model: &model.User{}},
		{Model: &model.PasskeyCredential{}},
		{Model: &model.Option{}},
		{Model: &model.Redemption{}},
		{Model: &model.Ability{}},
		{Model: &model.Midjourney{}},
		{Model: &model.TopUp{}},
		{Model: &model.Task{}},
		{Model: &model.Model{}},
		{Model: &model.Vendor{}},
		{Model: &model.PrefillGroup{}},
		{Model: &model.Setup{}},
		{Model: &model.TwoFA{}},
		{Model: &model.TwoFABackupCode{}},
		{Model: &model.Checkin{}},
		{Model: &model.SubscriptionOrder{}},
		{Model: &model.SubscriptionPlan{}},
		{Model: &model.UserSubscription{}},
		{Model: &model.SubscriptionPreConsumeRecord{}},
		{Model: &model.CustomOAuthProvider{}},
		{Model: &model.UserOAuthBinding{}},
	}
	if includeLogs {
		tables = append(tables, snapshotTable{Model: &model.Log{}})
		tables = append(tables, snapshotTable{Model: &model.QuotaData{}})
	}
	for i := range tables {
		tables[i].Name = getTableName(model.DB, tables[i].Model)
	}
	return tables
}

func getTableName(db *gorm.DB, value any) string {
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(value); err != nil || stmt.Schema == nil {
		return ""
	}
	return stmt.Schema.Table
}

func ensureJDCBackupDir() (string, error) {
	dir := setting.GetJDCBackupSetting().GetResolvedBackupDir()
	if dir == "" {
		return "", errors.New("backup dir is empty")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

func openSnapshotDB(path string) (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{PrepareStmt: true})
	if err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(
		&model.Channel{},
		&model.Token{},
		&model.User{},
		&model.PasskeyCredential{},
		&model.Option{},
		&model.Redemption{},
		&model.Ability{},
		&model.Log{},
		&model.JDCBackupRecord{},
		&model.JDCQuotaAdjustmentLog{},
		&model.Midjourney{},
		&model.TopUp{},
		&model.QuotaData{},
		&model.Task{},
		&model.Model{},
		&model.Vendor{},
		&model.PrefillGroup{},
		&model.Setup{},
		&model.TwoFA{},
		&model.TwoFABackupCode{},
		&model.Checkin{},
		&model.SubscriptionOrder{},
		&model.UserSubscription{},
		&model.SubscriptionPreConsumeRecord{},
		&model.CustomOAuthProvider{},
		&model.UserOAuthBinding{},
	); err != nil {
		return nil, err
	}
	if err := model.EnsureSubscriptionPlanTableSQLiteWithDB(db); err != nil {
		return nil, err
	}
	return db, nil
}

func CreateJDCBackup(backupType string) (*model.JDCBackupRecord, error) {
	jdcBackupMu.Lock()
	defer jdcBackupMu.Unlock()

	dir, err := ensureJDCBackupDir()
	if err != nil {
		return nil, err
	}
	filename := fmt.Sprintf("newapi-snapshot-%s.sqlite", time.Now().Format("20060102-150405"))
	path := filepath.Join(dir, filename)
	record := &model.JDCBackupRecord{
		Filename:   filename,
		Filepath:   path,
		BackupType: backupType,
		Status:     JDCBackupStatusRunning,
		CreatedAt:  common.GetTimestamp(),
	}
	_ = model.CreateJDCBackupRecord(record)

	if err := createSnapshotToPath(path, setting.GetJDCBackupSetting().IncludeLogsInExport); err != nil {
		record.Status = JDCBackupStatusFailed
		record.Remark = err.Error()
		record.FinishedAt = common.GetTimestamp()
		_ = model.UpdateJDCBackupRecord(record)
		return nil, err
	}
	if info, statErr := os.Stat(path); statErr == nil {
		record.SizeBytes = info.Size()
	}
	record.Status = JDCBackupStatusSuccess
	record.FinishedAt = common.GetTimestamp()
	_ = model.UpdateJDCBackupRecord(record)
	_ = CleanupOldJDCBackups()
	return record, nil
}

func createSnapshotToPath(path string, includeLogs bool) error {
	if err := os.RemoveAll(path); err != nil {
		return err
	}
	targetDB, err := openSnapshotDB(path)
	if err != nil {
		return err
	}
	sqlDB, err := targetDB.DB()
	if err == nil {
		defer sqlDB.Close()
	}
	for _, table := range getSnapshotTables(includeLogs) {
		if table.Name == "" {
			continue
		}
		sourceDB := model.DB
		if table.Name == getTableName(model.LOG_DB, &model.Log{}) && includeLogs {
			sourceDB = model.LOG_DB
		}
		if err := copyTableData(sourceDB, targetDB, table.Name); err != nil {
			return fmt.Errorf("copy table %s failed: %w", table.Name, err)
		}
	}
	return nil
}

func copyTableData(sourceDB, targetDB *gorm.DB, tableName string) error {
	rows, err := sourceDB.Table(tableName).Rows()
	if err != nil {
		return err
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return err
	}
	batch := make([]map[string]any, 0, 100)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		if err := targetDB.Table(tableName).CreateInBatches(batch, 100).Error; err != nil {
			return err
		}
		batch = make([]map[string]any, 0, 100)
		return nil
	}

	for rows.Next() {
		values := make([]any, len(columns))
		scanArgs := make([]any, len(columns))
		for i := range values {
			scanArgs[i] = &values[i]
		}
		if err := rows.Scan(scanArgs...); err != nil {
			return err
		}
		rowMap := make(map[string]any, len(columns))
		for i, col := range columns {
			rowMap[col] = normalizeDBValue(values[i])
		}
		batch = append(batch, rowMap)
		if len(batch) >= 100 {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return flush()
}

func normalizeDBValue(v any) any {
	switch val := v.(type) {
	case []byte:
		return string(val)
	default:
		return val
	}
}

func ImportJDCBackupFromPath(path string) error {
	jdcBackupMu.Lock()
	defer jdcBackupMu.Unlock()

	sourceDB, err := gorm.Open(sqlite.Open(path), &gorm.Config{PrepareStmt: true})
	if err != nil {
		return err
	}
	sqlDB, err := sourceDB.DB()
	if err == nil {
		defer sqlDB.Close()
	}
	includeLogs := hasTable(sourceDB, getTableName(sourceDB, &model.Log{}))
	if err := replaceDataFromSnapshot(sourceDB, includeLogs); err != nil {
		return err
	}
	model.InitOptionMap()
	model.CheckSetup()
	if common.MemoryCacheEnabled {
		model.InitChannelCache()
	}
	return nil
}

func RestoreJDCBackupRecord(id int) error {
	record, err := model.GetJDCBackupRecordByID(id)
	if err != nil {
		return err
	}
	return ImportJDCBackupFromPath(record.Filepath)
}

func replaceDataFromSnapshot(sourceDB *gorm.DB, includeLogs bool) error {
	tables := getSnapshotTables(includeLogs)
	return model.DB.Transaction(func(tx *gorm.DB) error {
		if common.UsingSQLite {
			if err := tx.Exec("PRAGMA foreign_keys = OFF").Error; err != nil {
				return err
			}
			defer tx.Exec("PRAGMA foreign_keys = ON")
		}
		for i := len(tables) - 1; i >= 0; i-- {
			table := tables[i]
			if table.Name == "" {
				continue
			}
			if !hasTable(tx, table.Name) {
				continue
			}
			if err := tx.Exec(fmt.Sprintf("DELETE FROM %s", quoteTable(table.Name))).Error; err != nil {
				return fmt.Errorf("clear table %s failed: %w", table.Name, err)
			}
		}
		for _, table := range tables {
			if table.Name == "" || !hasTable(sourceDB, table.Name) {
				continue
			}
			if err := copyTableData(sourceDB, tx, table.Name); err != nil {
				return err
			}
		}
		return nil
	})
}

func hasTable(db *gorm.DB, tableName string) bool {
	if tableName == "" {
		return false
	}
	return db.Migrator().HasTable(tableName)
}

func quoteTable(tableName string) string {
	if common.UsingPostgreSQL {
		return fmt.Sprintf("\"%s\"", tableName)
	}
	return fmt.Sprintf("`%s`", tableName)
}

func CleanupJDCDatabaseLogs() (*JDCBackupCleanupResult, error) {
	cfg := setting.GetJDCBackupSetting()
	result := &JDCBackupCleanupResult{}
	if cfg.LogRetentionDays > 0 {
		targetTs := time.Now().Add(-time.Duration(cfg.LogRetentionDays) * 24 * time.Hour).Unix()
		count, err := model.DeleteOldLog(context.Background(), targetTs, 1000)
		if err != nil {
			return nil, err
		}
		result.DeletedLogs = count
	}
	if cfg.QuotaDataRetentionDays > 0 {
		targetTs := time.Now().Add(-time.Duration(cfg.QuotaDataRetentionDays) * 24 * time.Hour).Unix()
		res := model.DB.Exec("DELETE FROM quota_data WHERE created_at < ?", targetTs)
		if res.Error != nil {
			return nil, res.Error
		}
		result.DeletedQuotaData = res.RowsAffected
	}
	return result, nil
}

func CleanupOldJDCBackups() error {
	cfg := setting.GetJDCBackupSetting()
	if cfg.RetainCount <= 0 {
		return nil
	}
	records, err := model.ListJDCBackupRecords(500)
	if err != nil {
		return err
	}
	var successRecords []*model.JDCBackupRecord
	for _, record := range records {
		if record.Status == JDCBackupStatusSuccess {
			successRecords = append(successRecords, record)
		}
	}
	if len(successRecords) <= cfg.RetainCount {
		return nil
	}
	sort.Slice(successRecords, func(i, j int) bool {
		return successRecords[i].ID > successRecords[j].ID
	})
	for _, record := range successRecords[cfg.RetainCount:] {
		_ = os.Remove(record.Filepath)
		_ = model.DB.Delete(record).Error
	}
	return nil
}

func SaveUploadedJDCBackup(src io.Reader, originalName string) (string, error) {
	dir, err := ensureJDCBackupDir()
	if err != nil {
		return "", err
	}
	ext := filepath.Ext(originalName)
	if ext == "" {
		ext = ".sqlite"
	}
	filename := fmt.Sprintf("imported-%s%s", time.Now().Format("20060102-150405"), ext)
	path := filepath.Join(dir, filename)
	file, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	if _, err := io.Copy(file, src); err != nil {
		return "", err
	}
	return path, nil
}

func RegisterImportedBackup(path string) (*model.JDCBackupRecord, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	record := &model.JDCBackupRecord{
		Filename:   filepath.Base(path),
		Filepath:   path,
		BackupType: JDCBackupTypeImported,
		Status:     JDCBackupStatusSuccess,
		SizeBytes:  info.Size(),
		CreatedAt:  common.GetTimestamp(),
		FinishedAt: common.GetTimestamp(),
	}
	return record, model.CreateJDCBackupRecord(record)
}

func GetJDCBackupFile(id int) (*model.JDCBackupRecord, error) {
	return model.GetJDCBackupRecordByID(id)
}

func ListJDCBackups(limit int) ([]*model.JDCBackupRecord, error) {
	return model.ListJDCBackupRecords(limit)
}

func BuildJDCBackupSummary(record *model.JDCBackupRecord) string {
	parts := []string{record.Filename, record.Status}
	if record.SizeBytes > 0 {
		parts = append(parts, fmt.Sprintf("%d bytes", record.SizeBytes))
	}
	if record.Remark != "" {
		parts = append(parts, record.Remark)
	}
	return strings.Join(parts, " | ")
}
