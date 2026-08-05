package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"SakuHentai/internal/middleware"
	"SakuHentai/internal/services"
)

type FavoritesHandler struct {
	db         *gorm.DB
	favService *services.FavoritesService
}

func NewFavoritesHandler(db *gorm.DB, favService *services.FavoritesService) *FavoritesHandler {
	return &FavoritesHandler{
		db:         db,
		favService: favService,
	}
}

func (h *FavoritesHandler) GetOnlineFavorites(c *gin.Context) {
	favCatStr := c.DefaultQuery("favcat", "0")
	favCat, _ := strconv.Atoi(favCatStr)
	if favCat < 0 || favCat > 9 {
		favCat = 0
	}

	next := c.Query("next")
	prev := c.Query("prev")
	seek := c.Query("seek")
	sortMode := c.DefaultQuery("sort", "favorited") // 读取排序参数

	account := middleware.CurrentAccount(c)
	if account == nil || account.IPBMemberID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请先绑定并保存 E 站账户凭证"})
		return
	}

	// 🟢 获取 EHSetting 并作为最后一个参数传入
	ehSetting := getEHSetting(h.db, account.ID)
	result, err := h.favService.FetchFavoritesList(h.db, account.ID, account, favCat, next, prev, seek, sortMode, ehSetting)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

func (h *FavoritesHandler) AddFavorite(c *gin.Context) {
	var req struct {
		GID    string `json:"gid" binding:"required"`
		Token  string `json:"token" binding:"required"`
		FavCat int    `json:"favCat"`
		Note   string `json:"note"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}

	account := middleware.CurrentAccount(c)
	if account == nil || account.IPBMemberID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请先绑定并保存 E 站账户凭证"})
		return
	}

	// 🟢 获取 EHSetting 并作为最后一个参数传入
	ehSetting := getEHSetting(h.db, account.ID)
	if err := h.favService.AddFavorite(h.db, account.ID, account, req.GID, req.Token, req.FavCat, req.Note, ehSetting); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "收藏保存成功"})
}

func (h *FavoritesHandler) RemoveFavorite(c *gin.Context) {
	var req struct {
		GID   string `json:"gid" binding:"required"`
		Token string `json:"token" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}

	account := middleware.CurrentAccount(c)
	if account == nil || account.IPBMemberID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请先绑定并保存 E 站账户凭证"})
		return
	}

	// 🟢 获取 EHSetting 并作为最后一个参数传入
	ehSetting := getEHSetting(h.db, account.ID)
	if err := h.favService.RemoveFavorite(h.db, account.ID, account, req.GID, req.Token, ehSetting); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "已从收藏夹移除"})
}

func (h *FavoritesHandler) ChangeSortOrder(c *gin.Context) {
	var req struct {
		SortMode string `json:"sortMode" binding:"required"` // "favorited" 或 "published"
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}

	account := middleware.CurrentAccount(c)
	if account == nil || account.IPBMemberID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "未绑定账户"})
		return
	}

	// 🟢 获取 EHSetting 并作为最后一个参数传入
	ehSetting := getEHSetting(h.db, account.ID)
	if err := h.favService.ChangeFavoriteSortOrder(h.db, account.ID, account, req.SortMode, ehSetting); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "排序设置成功"})
}