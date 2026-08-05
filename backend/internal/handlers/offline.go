package handlers

import (
	"errors"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"SakuHentai/internal/middleware"
	"SakuHentai/internal/models"
	"SakuHentai/internal/services"
)

// 下载/更新日志统一前缀（与 services 包保持一致，便于验收 grep）
const (
	dlLogTag  = "[DOWNLOAD]"
	dlWarnTag = "[DOWNLOAD-WARN]"
	dlErrTag  = "[DOWNLOAD-ERROR]"
)

// OfflineHandler 离线更新检测 + 维护查重 HTTP 层
type OfflineHandler struct {
	db        *gorm.DB
	ehService *services.EHService
	manager   *services.DownloadManager
}

// NewOfflineHandler 构造离线 handler
func NewOfflineHandler(db *gorm.DB, ehService *services.EHService, manager *services.DownloadManager) *OfflineHandler {
	return &OfflineHandler{db: db, ehService: ehService, manager: manager}
}

// ─────────────────────────────────────────────────────────────
// 更新检测
// ─────────────────────────────────────────────────────────────

// CheckOfflineUpdates 运行一次更新检测 POST /api/v1/offline/updates/check
//
// 联网逐画廊核对在线详情（限流退避内置），可能耗时较长，由前端异步调用。
// 检测内部固定使用管理员账号（后台维护任务），用户仅作为触发入口。
func (h *OfflineHandler) CheckOfflineUpdates(c *gin.Context) {
	result, err := services.CheckUpdates(h.db, h.ehService)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 自动更新画廊（autoUpdateGallery）：检测到新版后立即按所选方案入队下载
	if h.manager.GetSettings().AutoUpdateGallery {
		enqueued, skipped := h.autoEnqueueUpdates(result)
		log.Printf("%s [update] 检测完成：需要更新 %d 个，自动入队 %d 个，跳过 %d 个（autoUpdateGallery=true）",
			dlLogTag, len(result.NeedsUpdate), enqueued, skipped)
	}

	c.JSON(http.StatusOK, result)
}

// ListOfflineUpdates 列出需要更新的漫画 GET /api/v1/offline/updates
func (h *OfflineHandler) ListOfflineUpdates(c *gin.Context) {
	var comics []models.OfflineComic
	if err := h.db.Where("needs_update = ?", true).Order("updated_at desc").Find(&comics).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": comics, "total": len(comics)})
}

// downloadUpdateReq 更新下载请求体
type downloadUpdateReq struct {
	ComicID string `json:"comicId"`
	Mode    string `json:"mode"` // 可选：gallery | archive（覆盖自动更新方案）
}

// autoEnqueueUpdates 自动为所有待更新漫画入队下载（autoUpdateGallery=true 时调用）
func (h *OfflineHandler) autoEnqueueUpdates(result *services.UpdateCheckResult) (enqueued, skipped int) {
	userID := services.LoadAdminUserID(h.db)
	for i := range result.NeedsUpdate {
		comic := result.NeedsUpdate[i]
		params, err := h.buildUpdateParams(&comic, "", userID)
		if err != nil {
			log.Printf("%s [update] 自动更新跳过漫画 %s（%s）: %v", dlWarnTag, comic.ID, comic.Title, err)
			skipped++
			continue
		}
		if _, err := h.manager.CreateTask(params); err != nil {
			// 已存在进行中任务或创建失败：跳过但不算致命错误
			log.Printf("%s [update] 自动更新漫画 %s 入队失败: %v", dlWarnTag, comic.ID, err)
			skipped++
			continue
		}
		enqueued++
	}
	return enqueued, skipped
}

// buildUpdateParams 根据漫画的更新信息构造下载参数（手动更新与自动更新共用同一套方案选择逻辑）
func (h *OfflineHandler) buildUpdateParams(comic *models.OfflineComic, modeOverride string, userID uint) (services.CreateDownloadParams, error) {
	// 优先使用检测到的新版 gid/token；同 gid 扩充时 token 可能已变更
	gid := comic.NewGID
	if gid == "" {
		gid = comic.GID
	}
	token := comic.NewToken
	if token == "" {
		token = comic.Token
	}
	if gid == "" || token == "" {
		return services.CreateDownloadParams{}, errors.New("该漫画缺少新版 gid/token，无法下载更新")
	}

	// 选择更新下载方案：请求覆盖 > 自动更新方案 > 归档
	setting := h.manager.GetSettings()
	mode := modeOverride
	if mode == "" {
		mode = setting.AutoUpdateScheme
	}
	if mode != string(models.DownloadModeGallery) && mode != string(models.DownloadModeArchive) {
		mode = string(models.DownloadModeArchive)
	}

	archiveType := ""
	if mode == string(models.DownloadModeArchive) {
		if setting.DefaultDownloadOriginal {
			archiveType = string(models.ArchiveTypeOriginal)
		} else {
			archiveType = string(models.ArchiveTypeResample)
		}
	}

	return services.CreateDownloadParams{
		UserID:           userID,
		GID:              gid,
		Token:            token,
		Title:            comic.Title,
		CoverURL:         comic.CoverURL,
		Mode:             models.DownloadMode(mode),
		ArchiveType:      models.ArchiveType(archiveType),
		UpdateForComicID: comic.ID,
	}, nil
}

// DownloadUpdate 为需要更新的漫画启动新版下载 POST /api/v1/offline/updates/download
func (h *OfflineHandler) DownloadUpdate(c *gin.Context) {
	user := middleware.CurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}
	if !requireDownloadPermission(c) {
		return
	}

	var req downloadUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil || req.ComicID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求体格式错误，必需传递 comicId"})
		return
	}

	var comic models.OfflineComic
	if err := h.db.First(&comic, "id = ?", req.ComicID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "未找到漫画记录"})
		return
	}
	if !comic.NeedsUpdate {
		c.JSON(http.StatusBadRequest, gin.H{"error": "该漫画当前无需更新（needsUpdate=false）"})
		return
	}

	// 构造更新下载参数（方案选择与自动更新共用同一逻辑）
	params, err := h.buildUpdateParams(&comic, req.Mode, user.ID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	task, err := h.manager.CreateTask(params)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"task": task})
}

// ─────────────────────────────────────────────────────────────
// 维护查重
// ─────────────────────────────────────────────────────────────

// GetMaintainDedup 运行维护查重 GET /api/v1/offline/maintain
func (h *OfflineHandler) GetMaintainDedup(c *gin.Context) {
	result, err := services.MaintainDedup(h.db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

// removeDedupReq 删除重复项请求体
type removeDedupReq struct {
	ComicID    string `json:"comicId"`
	DeleteFile bool   `json:"deleteFile"` // 是否同时物理删除本地文件
}

// RemoveDedup 删除重复漫画 POST /api/v1/offline/maintain/remove
func (h *OfflineHandler) RemoveDedup(c *gin.Context) {
	var req removeDedupReq
	if err := c.ShouldBindJSON(&req); err != nil || req.ComicID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求体格式错误，必需传递 comicId"})
		return
	}
	if err := services.RemoveDedupComic(h.db, req.ComicID, req.DeleteFile); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
