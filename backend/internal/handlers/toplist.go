package handlers

import (
	"net/http"

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

// GetToplist 读取内存排行榜缓存，并自动挂载本地 SQLite 收藏状态
func (h *ToplistHandler) GetToplist(c *gin.Context) {
	// 榜单对所有登录用户开放；未绑定凭证时以空账号读缓存（无收藏状态）
	account := middleware.CurrentAccount(c)
	if account == nil {
		account = &models.AccountSetting{}
	}

	list := h.toplistService.GetCachedToplist(account)

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
		"comics": list,
	})
}