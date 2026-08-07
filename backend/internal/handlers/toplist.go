package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"SakuHentai/internal/middleware"
	"SakuHentai/internal/models"
	"SakuHentai/internal/services"
)

type ToplistHandler struct {
	db             *gorm.DB
	toplistService *services.ToplistService
}

func NewToplistHandler(db *gorm.DB, toplistService *services.ToplistService) *ToplistHandler {
	return &ToplistHandler{
		db:             db,
		toplistService: toplistService,
	}
}

// GetToplist 读取指定类型 + 页码的排行榜，并自动挂载本地 SQLite 收藏状态
// 参数：tl=11|12|13|15（默认 15 Yesterday），page=1~200（默认 1，每页 50 条）
func (h *ToplistHandler) GetToplist(c *gin.Context) {
	// 榜单对所有登录用户开放；未绑定凭证时以空账号读缓存（无收藏状态）
	account := middleware.CurrentAccount(c)
	if account == nil {
		account = &models.AccountSetting{}
	}

	// 排行榜类型（默认 Yesterday tl=15）
	tl := c.DefaultQuery("tl", services.ToplistYesterday)
	if !services.IsValidToplistTL(tl) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "不支持的排行榜类型 tl=" + tl})
		return
	}

	// 页码（默认 1，固定 1~200）
	page := 1
	if p := c.Query("page"); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > services.ToplistMaxPage {
			c.JSON(http.StatusBadRequest, gin.H{"error": "页码必须在 1~200 之间"})
			return
		}
		page = n
	}

	list, err := h.toplistService.GetToplist(account, tl, page)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	// 🟢 比对 SQLite 本地数据库，挂载收藏状态
	if len(list) > 0 {
		baseComics := make([]services.OnlineComicDTO, len(list))
		for i, item := range list {
			baseComics[i] = item.OnlineComicDTO
		}
		baseComics = services.AttachFavoriteStates(h.db, account.ID, baseComics)
		baseComics = services.AttachDownloadStates(h.db, baseComics)
		for i := range list {
			list[i].OnlineComicDTO = baseComics[i]
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"comics":      list,
		"totalPages":  services.ToplistMaxPage,
		"currentPage": page,
	})
}
