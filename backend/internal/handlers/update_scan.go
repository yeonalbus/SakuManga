package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"SakuHentai/internal/services"
)

// UpdateScanHandler 每周自动更新扫描设置 HTTP 层（Round4 任务四）
//
// 设置读写走本 handler；手动立即扫描复用既有 POST /api/v1/offline/updates/check
// （该接口现同时执行常规更新检测与老化判定）。
type UpdateScanHandler struct {
	db *gorm.DB
}

// NewUpdateScanHandler 构造更新扫描设置 handler
func NewUpdateScanHandler(db *gorm.DB) *UpdateScanHandler {
	return &UpdateScanHandler{db: db}
}

// GetSetting 读取更新扫描设置 GET /api/v1/offline/update-scan/setting
func (h *UpdateScanHandler) GetSetting(c *gin.Context) {
	c.JSON(http.StatusOK, services.LoadUpdateScanSetting(h.db))
}

// saveUpdateScanSettingReq 保存设置的请求体（指针字段以区分“未传”与“传 false”）
type saveUpdateScanSettingReq struct {
	EnableWeeklyScan *bool `json:"enableWeeklyScan"`
	ScanWeekday      *int  `json:"scanWeekday"`
	ScanHour         *int  `json:"scanHour"`
}

// SaveSetting 保存更新扫描设置 POST /api/v1/offline/update-scan/setting
func (h *UpdateScanHandler) SaveSetting(c *gin.Context) {
	var req saveUpdateScanSettingReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求体格式错误"})
		return
	}

	setting := services.LoadUpdateScanSetting(h.db)
	if req.EnableWeeklyScan != nil {
		setting.EnableWeeklyScan = *req.EnableWeeklyScan
	}
	if req.ScanWeekday != nil {
		setting.ScanWeekday = *req.ScanWeekday
	}
	if req.ScanHour != nil {
		setting.ScanHour = *req.ScanHour
	}

	saved, err := services.SaveUpdateScanSetting(h.db, setting)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, saved)
}
