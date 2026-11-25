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

// ReclassifyRecovery 异常任务重分类 POST /api/v1/offline/recovery/reclassify
// 把已取消/出错的恢复任务按失败文本重新归类（404 等确定性失败直接转入阵亡名单），
// 用于修复前旧逻辑失败的存量任务，无需重新下载。
func (h *RecoveryHandler) ReclassifyRecovery(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}
	res, err := h.manager.ReclassifyRecoveryTasks()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}

// ClearCandidates 清空全部恢复候选 POST /api/v1/offline/recovery/clear-candidates
// 全部转为阵亡（reason=abandoned）并物理删除对应的失效 offline_comics 记录及引用。
func (h *RecoveryHandler) ClearCandidates(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}
	res, err := services.ClearRecoveryCandidates(h.db)
	if err != nil {
		if errors.Is(err, services.ErrRecoveryInProgress) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}

// ReconcileDefault 默认下载目录失效检测与清理 POST /api/v1/offline/recovery/reconcile-default
func (h *RecoveryHandler) ReconcileDefault(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}
	extractPath := ""
	if h.manager != nil {
		extractPath = h.manager.GetSettings().ExtractPath
	}
	res, err := services.ReconcileDefaultDownloadComics(h.db, extractPath)
	if err != nil {
		if errors.Is(err, services.ErrRecoveryInProgress) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}

// GetFallen 阵亡名单 GET /api/v1/offline/fallen?page=&size=
// 排序：未 dismissed 在前（创建时间升序），dismissed 划线置底（dismissed_at 升序）。
func (h *RecoveryHandler) GetFallen(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "50"))
	items, total, err := services.ListFallenComics(h.db, services.FallenListParams{Page: page, Size: size})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": total, "page": page, "size": size})
}

// DismissFallen 阵亡条目手动 x POST /api/v1/offline/fallen/dismiss
// body: { id: 123, dismissed: true }（按条目 id：no-ehentai 条目没有 g_id）
// dismissed=true → 划线 + 置底（保留显示）；false → 撤销划线回到原位置。
func (h *RecoveryHandler) DismissFallen(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}
	var req struct {
		ID        uint `json:"id"`
		Dismissed bool `json:"dismissed"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.ID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求体格式错误，需传 id 与 dismissed"})
		return
	}
	if err := services.SetFallenDismissed(h.db, req.ID, req.Dismissed); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "阵亡条目不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}
