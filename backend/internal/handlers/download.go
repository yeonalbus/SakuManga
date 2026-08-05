package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"SakuHentai/internal/models"
	"SakuHentai/internal/services"
)

// DownloadHandler 下载任务 HTTP 层
type DownloadHandler struct {
	db        *gorm.DB
	ehService *services.EHService
	manager   *services.DownloadManager
}

// NewDownloadHandler 构造下载 handler
func NewDownloadHandler(db *gorm.DB, ehService *services.EHService, manager *services.DownloadManager) *DownloadHandler {
	return &DownloadHandler{db: db, ehService: ehService, manager: manager}
}

// requireAccount 校验 E 站账号凭证
func (h *DownloadHandler) requireAccount(c *gin.Context) *models.AccountSetting {
	var account models.AccountSetting
	if err := h.db.First(&account, 1).Error; err != nil || account.IPBMemberID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "请先绑定并保存 E 站账户凭证"})
		return nil
	}
	return &account
}

// ─────────────────────────────────────────────────────────────
// 任务 CRUD
// ─────────────────────────────────────────────────────────────

// CreateDownload 创建下载任务 POST /api/v1/downloads
func (h *DownloadHandler) CreateDownload(c *gin.Context) {
	if h.requireAccount(c) == nil {
		return
	}

	var p services.CreateDownloadParams
	if err := c.ShouldBindJSON(&p); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求体格式错误: " + err.Error()})
		return
	}

	task, err := h.manager.CreateTask(p)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, task)
}

// ListDownloads 下载任务列表 GET /api/v1/downloads
func (h *DownloadHandler) ListDownloads(c *gin.Context) {
	var p services.DownloadListParams
	if err := c.ShouldBindQuery(&p); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "非法请求参数"})
		return
	}
	tasks, total, err := h.manager.ListTasks(p)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"tasks": tasks, "total": total, "page": p.Page, "size": p.Size})
}

// GetDownload 单个任务详情 GET /api/v1/downloads/:id
func (h *DownloadHandler) GetDownload(c *gin.Context) {
	task, err := h.manager.GetTask(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, task)
}

// ─────────────────────────────────────────────────────────────
// 任务操作
// ─────────────────────────────────────────────────────────────

// PauseDownload 暂停 POST /api/v1/downloads/:id/pause
func (h *DownloadHandler) PauseDownload(c *gin.Context) {
	task, err := h.manager.PauseTask(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, task)
}

// ResumeDownload 恢复 POST /api/v1/downloads/:id/resume
func (h *DownloadHandler) ResumeDownload(c *gin.Context) {
	task, err := h.manager.ResumeTask(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, task)
}

// CancelDownload 取消 POST /api/v1/downloads/:id/cancel
func (h *DownloadHandler) CancelDownload(c *gin.Context) {
	task, err := h.manager.CancelTask(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, task)
}

// RetryDownload 重试 POST /api/v1/downloads/:id/retry
func (h *DownloadHandler) RetryDownload(c *gin.Context) {
	task, err := h.manager.RetryTask(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, task)
}

// UnlockDownload GP 解锁 POST /api/v1/downloads/:id/unlock
func (h *DownloadHandler) UnlockDownload(c *gin.Context) {
	task, err := h.manager.UnlockTask(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, task)
}

// RestoreDownloads 恢复历史任务 POST /api/v1/downloads/restore
func (h *DownloadHandler) RestoreDownloads(c *gin.Context) {
	count, err := h.manager.RestoreTasks()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"restored": count})
}

// ─────────────────────────────────────────────────────────────
// GP 面板
// ─────────────────────────────────────────────────────────────

// GetGPInfo 获取 GP 面板信息 GET /api/v1/downloads/gp-info?gid=..&token=..
func (h *DownloadHandler) GetGPInfo(c *gin.Context) {
	account := h.requireAccount(c)
	if account == nil {
		return
	}

	gid := c.Query("gid")
	token := c.Query("token")
	if gid == "" || token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数缺失，必需传递 gid 和 token"})
		return
	}

	setting := getEHSetting(h.db)
	info, err := h.manager.GetGPInfo(account, setting, gid, token)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, info)
}

// ─────────────────────────────────────────────────────────────
// 下载设置
// ─────────────────────────────────────────────────────────────

// GetDownloadSettings 获取下载设置 GET /api/v1/downloads/settings
func (h *DownloadHandler) GetDownloadSettings(c *gin.Context) {
	c.JSON(http.StatusOK, h.manager.GetSettings())
}

// SaveDownloadSettings 保存下载设置 POST /api/v1/downloads/settings
func (h *DownloadHandler) SaveDownloadSettings(c *gin.Context) {
	var s models.DownloadSetting
	if err := c.ShouldBindJSON(&s); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求体格式错误: " + err.Error()})
		return
	}
	saved, err := h.manager.SaveSettings(&s)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, saved)
}
