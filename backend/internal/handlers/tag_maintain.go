package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"SakuHentai/internal/services"
)

// TagMaintainHandler Tag 维护（双轨三态）HTTP 层
type TagMaintainHandler struct {
	db  *gorm.DB
	svc *services.TagMaintainService
}

// NewTagMaintainHandler 构造 Tag 维护 handler
func NewTagMaintainHandler(db *gorm.DB, svc *services.TagMaintainService) *TagMaintainHandler {
	return &TagMaintainHandler{db: db, svc: svc}
}

// GetSetting 读取 Tag 维护设置 GET /api/v1/offline/tags/setting
func (h *TagMaintainHandler) GetSetting(c *gin.Context) {
	c.JSON(http.StatusOK, services.LoadTagMaintainSetting(h.db))
}

// saveTagSettingReq 保存设置的请求体（指针字段以区分“未传”与“传 false”）
type saveTagSettingReq struct {
	EnableDailyRefresh      *bool `json:"enableDailyRefresh"`
	EnableWeeklyWriteback   *bool `json:"enableWeeklyWriteback"`
	EnableFSearchAutoCorrect *bool `json:"enableFSearchAutoCorrect"`
	RefreshHour             *int  `json:"refreshHour"`
	WritebackWeekday        *int  `json:"writebackWeekday"`
	WritebackHour           *int  `json:"writebackHour"`
}

// SaveSetting 保存 Tag 维护设置 POST /api/v1/offline/tags/setting
func (h *TagMaintainHandler) SaveSetting(c *gin.Context) {
	var req saveTagSettingReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求体格式错误"})
		return
	}

	setting := services.LoadTagMaintainSetting(h.db)
	if req.EnableDailyRefresh != nil {
		setting.EnableDailyRefresh = *req.EnableDailyRefresh
	}
	if req.EnableWeeklyWriteback != nil {
		setting.EnableWeeklyWriteback = *req.EnableWeeklyWriteback
	}
	// 修复：请求体缺该字段导致「在线搜索 Tag 语法自动修正」开关无法关闭/开启
	if req.EnableFSearchAutoCorrect != nil {
		setting.EnableFSearchAutoCorrect = *req.EnableFSearchAutoCorrect
	}
	if req.RefreshHour != nil {
		setting.RefreshHour = *req.RefreshHour
	}
	if req.WritebackWeekday != nil {
		setting.WritebackWeekday = *req.WritebackWeekday
	}
	if req.WritebackHour != nil {
		setting.WritebackHour = *req.WritebackHour
	}

	saved, err := services.SaveTagMaintainSetting(h.db, setting)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, saved)
}

// RefreshTags 手动立即刷新 Tag POST /api/v1/offline/tags/refresh
//
// 联网逐画廊核对可能耗时较长，采用异步执行并立即返回 202，
// 前端通过 GET /progress 轮询进度（返回 running 期间再次触发返回 409）。
func (h *TagMaintainHandler) RefreshTags(c *gin.Context) {
	if h.svc.IsRunning() {
		c.JSON(http.StatusConflict, gin.H{"error": "已有 Tag 维护任务正在执行，请稍后再试"})
		return
	}
	go func() {
		_, _ = h.svc.RefreshAllTags()
	}()
	c.JSON(http.StatusAccepted, gin.H{"ok": true, "type": "refresh"})
}

// Writeback 手动立即反向写回 ComicInfo POST /api/v1/offline/tags/writeback
func (h *TagMaintainHandler) Writeback(c *gin.Context) {
	if h.svc.IsRunning() {
		c.JSON(http.StatusConflict, gin.H{"error": "已有 Tag 维护任务正在执行，请稍后再试"})
		return
	}
	go func() {
		_, _ = h.svc.WritebackComicInfo()
	}()
	c.JSON(http.StatusAccepted, gin.H{"ok": true, "type": "writeback"})
}

// GetProgress 读取刷新/写回任务进度 GET /api/v1/offline/tags/progress
func (h *TagMaintainHandler) GetProgress(c *gin.Context) {
	c.JSON(http.StatusOK, h.svc.GetProgress())
}

// editComicTagsReq 单本 tag 编辑请求体
type editComicTagsReq struct {
	AddTags    []string `json:"addTags"`    // 本地新增 tag
	RemoveTags []string `json:"removeTags"` // 删除 tag（属于 online 则记入 remove，属于 add 则移除）
}

// EditComicTags 单本 tag 增删落库 PUT /api/v1/comics/:id/tags
func (h *TagMaintainHandler) EditComicTags(c *gin.Context) {
	id := c.Param("id")
	var req editComicTagsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求体格式错误"})
		return
	}
	if err := h.svc.EditComicTags(id, req.AddTags, req.RemoveTags); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
