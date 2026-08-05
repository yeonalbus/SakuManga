package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"SakuHentai/internal/middleware"
	"SakuHentai/internal/models"
	"SakuHentai/internal/services"
)

// RandomComicItem 随机抽卡统一返回项（在线/离线混合 DTO）
type RandomComicItem struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	CoverURL     string   `json:"coverUrl"`
	Source       string   `json:"source"`
	Category     string   `json:"category,omitempty"`
	Rating       float64  `json:"rating,omitempty"`
	Tags         []string `json:"tags,omitempty"`
	PageCount    int      `json:"pageCount,omitempty"`
	ReadCount    int      `json:"readCount,omitempty"`
	UpdatedAt    string   `json:"updatedAt"`
	IsDownloaded bool     `json:"isDownloaded"`
	Token        string   `json:"token,omitempty"`
	Uploader     string   `json:"uploader,omitempty"`
	IsFavorite   bool     `json:"isFavorite"`
	LocalPath    string   `json:"localPath,omitempty"`
	FileSize     int64    `json:"fileSize,omitempty"`
	HasError     bool     `json:"hasError,omitempty"`
}

// fromOnlineDTO 在线 DTO → 抽卡统一项
func fromOnlineDTO(c services.OnlineComicDTO) RandomComicItem {
	return RandomComicItem{
		ID:           c.ID,
		Title:        c.Title,
		CoverURL:     c.CoverURL,
		Source:       c.Source,
		Category:     c.Category,
		Rating:       c.Rating,
		Tags:         c.Tags,
		PageCount:    c.PageCount,
		UpdatedAt:    c.UpdatedAt,
		IsDownloaded: c.IsDownloaded,
		Token:        c.Token,
		Uploader:     c.Uploader,
		IsFavorite:   c.IsFavorite,
	}
}

// fromOfflineModel 离线模型 → 抽卡统一项
func fromOfflineModel(c models.OfflineComic) RandomComicItem {
	return RandomComicItem{
		ID:           c.ID,
		Title:        c.Title,
		CoverURL:     c.CoverURL,
		Source:       string(c.Source),
		Category:     c.Category,
		Rating:       c.Rating,
		Tags:         parseRawTags(c.Tags),
		PageCount:    c.PageCount,
		ReadCount:    c.ReadCount,
		UpdatedAt:    c.UpdatedAt.Format(time.RFC3339),
		IsDownloaded: c.IsDownloaded,
		LocalPath:    c.LocalPath,
		FileSize:     c.FileSize,
		HasError:     c.NeedsUpdate,
	}
}

// GetRandomComics 随机抽卡接口
//
// 查询参数:
//   - count:      抽卡数量（默认 8，上限 50）
//   - source:     范围 all | online | offline（默认 all）
//   - keyword:    搜索关键词（在线走 f_search，离线匹配标题/标签）
//   - categories: 分类过滤（仅在线生效，多次传递）
//   - minRating:  最低评分（仅离线生效）
//   - minPages:   最少页数（仅离线生效）
//   - maxPages:   最多页数（仅离线生效）
func (h *OnlineComicHandler) GetRandomComics(c *gin.Context) {
	account := middleware.CurrentAccount(c)

	// 1. 解析通用参数
	count := 8
	if v := c.Query("count"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			if n > 50 {
				n = 50
			}
			count = n
		}
	}

	source := c.DefaultQuery("source", "all")
	if source != "online" && source != "offline" {
		source = "all"
	}

	keyword := c.Query("keyword")
	activeCategories := c.QueryArray("categories")
	minRating, _ := strconv.ParseFloat(c.DefaultQuery("minRating", "0"), 64)
	minPages, _ := strconv.Atoi(c.DefaultQuery("minPages", "0"))
	maxPages, _ := strconv.Atoi(c.DefaultQuery("maxPages", "0"))

	// 2. 离线随机：SQL ORDER BY RANDOM() 全库随机
	randomOffline := func(limit int) []RandomComicItem {
		q := h.db.Model(&models.OfflineComic{}).Order("RANDOM()").Limit(limit)
		if kw := strings.TrimSpace(keyword); kw != "" {
			like := "%" + kw + "%"
			q = q.Where("title LIKE ? OR tags LIKE ?", like, like)
		}
		if minRating > 0 {
			q = q.Where("rating >= ?", minRating)
		}
		if minPages > 0 {
			q = q.Where("page_count >= ?", minPages)
		}
		if maxPages > 0 {
			q = q.Where("page_count <= ?", maxPages)
		}

		var rows []models.OfflineComic
		if err := q.Find(&rows).Error; err != nil {
			return []RandomComicItem{}
		}
		items := make([]RandomComicItem, 0, len(rows))
		for _, r := range rows {
			items = append(items, fromOfflineModel(r))
		}
		return items
	}

	// 3. 在线随机：抓随机页 + 洗牌采样
	randomOnline := func(limit int) ([]RandomComicItem, error) {
		if account == nil || account.IPBMemberID == "" {
			return nil, fmt.Errorf("请先绑定并保存 E 站账户凭证")
		}
		ehSetting := getEHSetting(h.db, account.ID)
		params := services.SearchParams{
			Keyword:          keyword,
			ActiveCategories: activeCategories,
		}
		comics, err := h.ehService.FetchRandomGalleryList(account, params, ehSetting, limit)
		if err != nil {
			return nil, err
		}
		comics = services.AttachFavoriteStates(h.db, account.ID, comics)
		comics = services.AttachDownloadStates(h.db, comics)
		items := make([]RandomComicItem, 0, len(comics))
		for _, co := range comics {
			items = append(items, fromOnlineDTO(co))
		}
		return items, nil
	}

	var items []RandomComicItem
	warning := ""

	switch source {
	case "online":
		result, err := randomOnline(count)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		items = result
	case "offline":
		items = randomOffline(count)
	default: // all：在线/离线各占一半，在线失败时降级为全量本地补充
		onlineCount := count/2 + count%2
		offlineCount := count - onlineCount

		onlineItems, err := randomOnline(onlineCount)
		if err != nil {
			warning = "在线抽卡失败：" + err.Error() + "，已为你从本地库补充抽取"
			onlineItems = nil
			offlineCount = count
		}
		offlineItems := randomOffline(offlineCount)

		items = make([]RandomComicItem, 0, count)
		items = append(items, onlineItems...)
		items = append(items, offlineItems...)
	}

	c.JSON(http.StatusOK, gin.H{
		"comics":  items,
		"count":   len(items),
		"warning": warning,
	})
}
