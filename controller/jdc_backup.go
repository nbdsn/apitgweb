package controller

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

func GetJDCBackups(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	records, err := service.ListJDCBackups(limit)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, records)
}

func CreateJDCBackup(c *gin.Context) {
	record, err := service.CreateJDCBackup(service.JDCBackupTypeManual)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, record)
}

func DownloadJDCBackup(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	record, err := service.GetJDCBackupFile(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.FileAttachment(record.Filepath, record.Filename)
}

func ImportJDCBackup(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "请上传备份文件"})
		return
	}
	src, err := file.Open()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	defer src.Close()
	path, err := service.SaveUploadedJDCBackup(src, file.Filename)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	record, err := service.RegisterImportedBackup(path)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := service.ImportJDCBackupFromPath(path); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, record)
}

func RestoreJDCBackup(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := service.RestoreJDCBackupRecord(id); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"id": id})
}

func CleanupJDCDatabaseLogs(c *gin.Context) {
	result, err := service.CleanupJDCDatabaseLogs()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}
