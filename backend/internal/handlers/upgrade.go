package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"SakuHentai/internal/middleware"
	"SakuHentai/internal/models"
	"SakuHentai/internal/services"
)

// ─────────────────────────────────────────────────────────────
// 画质升级 HTTP 层（图片质量升级功能）
//
// 检测 + 确认后把「低清/非最优版本」升级为「归档原图（archiveOriginal）」。
// 范围：SakuHentai 下载导入（scan_path_id 空）+ 有 gid/token 元数据 + 非 archiveOriginal。
// 升级下载复用 DownloadManager.CreateTask（UpdateForComicID + ForceDeleteOriginal），
// 完成后由 finalizeUpdate 自动删除旧版。
// ─────────────────────────────────────────────────────────────

// UpgradeHandler 画质升级 HTTP 层
type UpgradeHandler struct {
	db      *gorm.DB
	manager *services.DownloadManager
}

// NewUpgradeHandler 构造画质升级 handler
func NewUpgradeHandler(db *gorm.DB, manager *services.DownloadManager) *UpgradeHandler {
	return &UpgradeHandler{db: db, manager: manager}
}

// ListUpgradeCandidates 列出升级候选 GET /api/v1/offline/upgrade/list
func (h *UpgradeHandler) ListUpgradeCandidates(c *gin.Context) {
	items, err := services.ListUpgradeCandidates(h.db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": len(items)})
}

// upgradeDownloadReq 升级下载请求体
type upgradeDownloadReq struct {
	ComicID string `json:"comicId"`
}

// UpgradeDownload 为指定漫画启动归档原图升级下载 POST /api/v1/offline/upgrade/download
func (h *UpgradeHandler) UpgradeDownload(c *gin.Context) {
	user := middleware.CurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}
	if !requireDownloadPermission(c) {
		return
	}

	var req upgradeDownloadReq
	if err := c.ShouldBindJSON(&req); err != nil || req.ComicID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求体格式错误，必需传递 comicId"})
		return
	}

	var comic models.OfflineComic
	if err := h.db.First(&comic, "id = ?", req.ComicID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "未找到漫画记录"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 升级资格校验（与 ListUpgradeCandidates 判定一致，防御性重复校验）
	switch {
	case comic.GID == "" || comic.Token == "":
		c.JSON(http.StatusBadRequest, gin.H{"error": "该漫画缺少 E 站元数据（gid/token），无法升级下载"})
		return
	case comic.RemovedStatus:
		c.JSON(http.StatusBadRequest, gin.H{"error": "该画廊已被删除/移除，无法升级下载"})
		return
	case services.ResolveEffectiveDownloadScheme(&comic) == string(models.DefaultSchemeArchiveOriginal):
		// 新数据直接判方案；存量（方案空）按形态推断：archive=归档原图（已达标），gallery=画廊下载（可升级）
		c.JSON(http.StatusBadRequest, gin.H{"error": "该漫画已是归档原图（archiveOriginal），无需升级"})
		return
	}

	// 构造升级下载参数：固定归档原图方案 + 关联旧版（完成后自动删除旧版）
	params := services.CreateDownloadParams{
		UserID:             user.ID,
		GID:                comic.GID,
		Token:              comic.Token,
		Title:              comic.Title,
		CoverURL:           comic.CoverURL,
		Mode:               models.DownloadModeArchive,
		ArchiveType:        models.ArchiveTypeOriginal,
		UpdateForComicID:   comic.ID,
		ForceDeleteOriginal: true,
	}

	task, err := h.manager.CreateTask(params)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"task": task})
}
