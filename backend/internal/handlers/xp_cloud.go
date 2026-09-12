package handlers

import (
	"net/http"
	"strconv"

	"SakuManga/internal/middleware"
	"SakuManga/internal/services"

	"github.com/gin-gonic/gin"
)

// ─────────────────────────────────────────────────────────────
// Round30 阶段一：XP 词云接口
//
//   GET  /api/v1/offline/xp-cloud          词云查询（登录可用）
//   POST /api/v1/offline/xp-cloud/rebuild  全量重算（仅管理员）
//
// 统计口径与增量维护见 services/xp_cloud.go 与 plans/round30-xp-cloud-recommend-plan.md。
// ─────────────────────────────────────────────────────────────

// XpCloudHandler XP 词云处理器
type XpCloudHandler struct {
	svc *services.XpCloudService
}

// NewXpCloudHandler 构造 XP 词云处理器（接收共享服务实例，与增量触发点共用同一权重缓存）
func NewXpCloudHandler(svc *services.XpCloudService) *XpCloudHandler {
	return &XpCloudHandler{svc: svc}
}

// GetXpCloud 词云查询 GET /api/v1/offline/xp-cloud?group=core|ip|misc|all&view=library|reading&limit=120
//
// 阅读侧按当前登录用户隔离（历史/评分），库藏侧为全库共享。
func (h *XpCloudHandler) GetXpCloud(c *gin.Context) {
	user := middleware.CurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	group := c.DefaultQuery("group", "core")
	switch group {
	case "core", "ip", "misc", "all":
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "group 仅支持 core | ip | misc | all"})
		return
	}

	view := c.DefaultQuery("view", "library")
	if view != "reading" {
		view = "library"
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "120"))

	result, err := h.svc.Query(user.ID, group, view, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "词云统计失败：" + err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

// RebuildXpCloud 全量重算统计表 POST /api/v1/offline/xp-cloud/rebuild（仅管理员）
//
// 用于公式调整、数据异常或增量链路漏接后的手动兜底；重算后立即返回最新元信息。
func (h *XpCloudHandler) RebuildXpCloud(c *gin.Context) {
	user := middleware.CurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	if err := h.svc.ForceRebuild(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "重算失败：" + err.Error()})
		return
	}

	result, err := h.svc.Query(user.ID, "core", "library", 1)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "统计已重算"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "统计已重算", "meta": result.Meta})
}
