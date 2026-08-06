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
	if !services.StartOfflineTask(services.OfflineTaskUpdate) {
		c.JSON(http.StatusConflict, gin.H{"error": "已有离线维护任务正在运行，请稍后再试"})
		return
	}

	go func() {
		result, err := services.CheckUpdatesWithProgress(h.db, h.ehService, services.OfflineUpdateProgressSink)
		if err != nil {
			services.FinishOfflineTask(err)
			return
		}
		services.StoreUpdateCheckResult(result)

		// 自动更新画廊（autoUpdateGallery）：检测到新版后立即按所选方案入队下载
		if h.manager.GetSettings().AutoUpdateGallery {
			enqueued, skipped := h.autoEnqueueUpdates(result)
			log.Printf("%s [update] 检测完成：需要更新 %d 个，自动入队 %d 个，跳过 %d 个（autoUpdateGallery=true）",
				dlLogTag, len(result.NeedsUpdate), enqueued, skipped)
		}
		services.FinishOfflineTask(nil)
	}()

	c.JSON(http.StatusAccepted, gin.H{"started": true})
}

// GetCheckUpdatesProgress 更新检测进度轮询 GET /api/v1/offline/updates/check/progress
func (h *OfflineHandler) GetCheckUpdatesProgress(c *gin.Context) {
	c.JSON(http.StatusOK, services.GetOfflineTaskProgress())
}

// GetCheckUpdatesResult 更新检测结果读取 GET /api/v1/offline/updates/check/result
func (h *OfflineHandler) GetCheckUpdatesResult(c *gin.Context) {
	c.JSON(http.StatusOK, services.GetUpdateCheckResult())
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
		if setting.DefaultDownloadScheme == models.DefaultSchemeArchiveResample {
			archiveType = string(models.ArchiveTypeResample)
		} else {
			archiveType = string(models.ArchiveTypeOriginal)
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

// GetMaintainDedup 异步启动维护查重 GET /api/v1/offline/maintain
//
// 维护查重逐画廊联网核对，可能耗时数十分钟，改为异步任务 + 进度轮询：
//   1. 本接口立即返回 202（启动结果）；
//   2. 前端轮询 GET /offline/maintain/progress 获取进度；
//   3. 完成后前端读取 GET /offline/maintain/result 获取结果。
func (h *OfflineHandler) GetMaintainDedup(c *gin.Context) {
	if !services.StartOfflineTask(services.OfflineTaskMaintain) {
		c.JSON(http.StatusConflict, gin.H{"error": "已有离线维护任务正在运行，请稍后再试"})
		return
	}
	// 需求1：?full=true 强制全量在线核对（忽略 parent_checked_at 增量标记，联网逐本重抓）
	full := c.DefaultQuery("full", "") == "true"

	go func() {
		result, err := services.MaintainDedupWithProgress(h.db, h.ehService, services.OfflineMaintainProgressSink, full)
		if err != nil {
			services.FinishOfflineTask(err)
			return
		}
		services.StoreMaintainDedupResult(result)
		services.FinishOfflineTask(nil)
	}()

	c.JSON(http.StatusAccepted, gin.H{"started": true})
}

// GetMaintainProgress 维护查重进度轮询 GET /api/v1/offline/maintain/progress
func (h *OfflineHandler) GetMaintainProgress(c *gin.Context) {
	c.JSON(http.StatusOK, services.GetOfflineTaskProgress())
}

// GetMaintainResult 维护查重结果读取 GET /api/v1/offline/maintain/result
func (h *OfflineHandler) GetMaintainResult(c *gin.Context) {
	c.JSON(http.StatusOK, services.GetMaintainDedupResult())
}

// removeDedupReq 删除重复项请求体
type removeDedupReq struct {
	ComicID    string   `json:"comicId"`
	ComicIDs   []string `json:"comicIds"`   // 批量删除：传多个 comicId，一次删除
	DeleteFile bool     `json:"deleteFile"` // 是否同时物理删除本地文件
}

// RemoveDedup 删除重复漫画 POST /api/v1/offline/maintain/remove
//
// 支持两种删除方式：
//   - 单个删除：传 comicId（兼容旧版）
//   - 批量删除：传 comicIds 数组，一次提交多个，避免反复“删除→刷新”
func (h *OfflineHandler) RemoveDedup(c *gin.Context) {
	var req removeDedupReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求体格式错误"})
		return
	}
	// 批量删除：comicIds 非空则优先批量
	if len(req.ComicIDs) > 0 {
		deleted, err := services.RemoveDedupComics(h.db, req.ComicIDs, req.DeleteFile)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "deleted": deleted})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true, "deleted": deleted})
		return
	}
	if req.ComicID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求体格式错误，必需传递 comicId 或 comicIds"})
		return
	}
	if err := services.RemoveDedupComic(h.db, req.ComicID, req.DeleteFile); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "deleted": 1})
}
