package handlers

import (
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
//	PUT    /scrape-bookmarks/:id    → { ok: true }   （重命名）
//	DELETE /scrape-bookmarks/:id    → { ok: true }
// ─────────────────────────────────────────────────────────────

type ScrapeBookmarkHandler struct {
	db *gorm.DB
}

func NewScrapeBookmarkHandler(db *gorm.DB) *ScrapeBookmarkHandler {
	return &ScrapeBookmarkHandler{db: db}
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

// Rename 重命名书签
func (h *ScrapeBookmarkHandler) Rename(c *gin.Context) {
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
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数不合法"})
		return
	}
	if err := services.RenameScrapeBookmark(h.db, user.ID, uint(id), req.Name); err != nil {
		if err == services.ErrBookmarkNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "重命名失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
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
