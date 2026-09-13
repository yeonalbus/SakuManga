package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"SakuManga/internal/middleware"
	"SakuManga/internal/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ─────────────────────────────────────────────────────────────
// Round28：搜刮书签 API（按登录用户隔离）
//
//	GET    /scrape-bookmarks        → { items: [...] }
//	POST   /scrape-bookmarks        → 201 { item: {...} }
//	PUT    /scrape-bookmarks/:id    → { ok: true }   （重命名 / 锚点更新：Round33 扩展）
//	DELETE /scrape-bookmarks/:id    → { ok: true }
//	POST   /scrape-bookmarks/check  → { results: [...] }（Round33：失效检测）
// ─────────────────────────────────────────────────────────────

type ScrapeBookmarkHandler struct {
	db *gorm.DB
	eh *services.EHService
}

func NewScrapeBookmarkHandler(db *gorm.DB, eh *services.EHService) *ScrapeBookmarkHandler {
	return &ScrapeBookmarkHandler{db: db, eh: eh}
}

// List 当前用户全部书签
func (h *ScrapeBookmarkHandler) List(c *gin.Context) {
	user := middleware.CurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}
	items, err := services.ListScrapeBookmarks(h.db, user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取书签失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// Create 新增书签
func (h *ScrapeBookmarkHandler) Create(c *gin.Context) {
	user := middleware.CurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}
	var in services.ScrapeBookmarkInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数不合法"})
		return
	}
	item, err := services.CreateScrapeBookmark(h.db, user.ID, in)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建书签失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"item": item})
}

// Update 更新书签（名称 / 锚点，均为可选的部分更新）
func (h *ScrapeBookmarkHandler) Update(c *gin.Context) {
	user := middleware.CurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "书签 id 不合法"})
		return
	}
	var req struct {
		Name   *string         `json:"name"`
		Anchor json.RawMessage `json:"anchor"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数不合法"})
		return
	}
	// 名称：传了才更新（空名忽略，保持原名）
	if req.Name != nil {
		if err := services.RenameScrapeBookmark(h.db, user.ID, uint(id), *req.Name); err != nil {
			if errors.Is(err, services.ErrBookmarkNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "重命名失败: " + err.Error()})
			return
		}
	}
	// 锚点（Round33）：迁移写回 / 失效标记写回
	if len(req.Anchor) > 0 {
		if err := services.UpdateScrapeBookmarkAnchor(h.db, user.ID, uint(id), req.Anchor); err != nil {
			if errors.Is(err, services.ErrBookmarkNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
				return
			}
			if errors.Is(err, services.ErrBookmarkInvalidAnchor) {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "锚点更新失败: " + err.Error()})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Check 批量失效检测（Round33）：探测锚定画廊是否已被删除 / 下架 / 被新版本取代
func (h *ScrapeBookmarkHandler) Check(c *gin.Context) {
	user := middleware.CurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}
	account := middleware.CurrentAccount(c)
	if account == nil || account.IPBMemberID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请先绑定并保存 E 站账户凭证"})
		return
	}
	var req struct {
		IDs []uint `json:"ids"`
	}
	_ = c.ShouldBindJSON(&req) // 允许空 body（检测全部）

	ehSetting := getEHSetting(h.db, account.ID)
	results, err := services.CheckScrapeBookmarks(h.db, h.eh, user.ID, account, ehSetting, req.IDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "失效检测失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"results": results})
}

// Delete 删除书签
func (h *ScrapeBookmarkHandler) Delete(c *gin.Context) {
	user := middleware.CurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "书签 id 不合法"})
		return
	}
	if err := services.DeleteScrapeBookmark(h.db, user.ID, uint(id)); err != nil {
		if err == services.ErrBookmarkNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
