package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"SakuManga/internal/middleware"
	"SakuManga/internal/services"
)

// ─────────────────────────────────────────────────────────────
// Round54：恢复与阵亡 HTTP 层
//
// 权限：恢复涉及下载（消耗 E 站配额/GP）与删除旧记录，全部接口仅管理员可用。
// 路由注册见 router.go「Round54：恢复与阵亡」分组。
// ─────────────────────────────────────────────────────────────

// RecoveryHandler 恢复功能 HTTP 层
type RecoveryHandler struct {
	db      *gorm.DB
	manager *services.DownloadManager
}

// NewRecoveryHandler 构造恢复 handler
func NewRecoveryHandler(db *gorm.DB, manager *services.DownloadManager) *RecoveryHandler {
	return &RecoveryHandler{db: db, manager: manager}
}

// GetStatus 恢复状态 GET /api/v1/offline/recovery/status
// 返回 phase（idle | ready | recovering）、候选数、进行中任务数、锁定路径。
func (h *RecoveryHandler) GetStatus(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}
	status, err := services.GetRecoveryStatus(h.db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, status)
}

// ListCandidates 恢复候选列表 GET /api/v1/offline/recovery/candidates?page=&size=
func (h *RecoveryHandler) ListCandidates(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "50"))
	items, total, err := services.ListRecoveryCandidates(h.db, page, size)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": total, "page": page, "size": size})
}

// StartRecovery 启动恢复 POST /api/v1/offline/recovery/start
// body: { scheme: "archive-original" | "archive-resample" | "gallery" }
func (h *RecoveryHandler) StartRecovery(c *gin.Context) {
	user := middleware.CurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}
	if !requireAdmin(c) {
		return
	}

	var req services.RecoveryStartParams
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求体格式错误: " + err.Error()})
		return
	}
	req.UserID = user.ID

	res, err := h.manager.StartRecovery(req)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrRecoverySchemeInvalid),
			errors.Is(err, services.ErrRecoveryNoCandidates):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, services.ErrRecoveryInProgress):
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusOK, res)
}

// CancelRecovery 取消恢复 POST /api/v1/offline/recovery/cancel
// body: { gids: ["..."] }（gids 为空 = 全部取消；候选清单全部保留，可再次启动幂等重入）
func (h *RecoveryHandler) CancelRecovery(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	var req services.RecoveryCancelParams
	// body 可为空（全部取消）：EOF 不是错误，忽略绑定失败
	_ = c.ShouldBindJSON(&req)

	res, err := h.manager.CancelRecovery(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}
