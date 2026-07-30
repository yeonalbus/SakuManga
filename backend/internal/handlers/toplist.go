package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

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
	var account models.AccountSetting
	_ = h.db.First(&account, 1)

	list := h.toplistService.GetCachedToplist(&account)

	// 🟢 比对 SQLite 本地数据库，挂载收藏状态
	if len(list) > 0 {
		baseComics := make([]services.OnlineComicDTO, len(list))
		for i, item := range list {
			baseComics[i] = item.OnlineComicDTO
		}
		baseComics = services.AttachFavoriteStates(h.db, baseComics)
		for i := range list {
			list[i].OnlineComicDTO = baseComics[i]
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"comics": list,
	})
}