package handlers

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"SakuHentai/internal/models"
	"SakuHentai/internal/services"
)

type OnlineComicHandler struct {
	db        *gorm.DB
	ehService *services.EHService
}

func NewOnlineComicHandler(db *gorm.DB, ehService *services.EHService) *OnlineComicHandler {
	return &OnlineComicHandler{db: db, ehService: ehService}
}

// 获取或初始化 EHSetting 配置
func getEHSetting(db *gorm.DB) *models.EHSetting {
	var setting models.EHSetting
	// 1. 仅通过主键 ID=1 查找
	if err := db.First(&setting, 1).Error; err != nil {
		// 2. 只有查不到记录时，才插入默认配置
		setting = models.EHSetting{
			ID:             1,
			Site:           "e-hentai",
			PreferRedirect: true,
		}
		db.Create(&setting)
	}
	return &setting
}

// GetOnlineComics 抓取线上画廊列表 (首页 /)
func (h *OnlineComicHandler) GetOnlineComics(c *gin.Context) {
	var account models.AccountSetting
	if err := h.db.First(&account, 1).Error; err != nil || account.IPBMemberID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "请先绑定并保存 E 站账户凭证"})
		return
	}

	var params services.SearchParams
	if err := c.ShouldBindQuery(&params); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "非法请求参数"})
		return
	}

	if params.Page <= 0 {
		params.Page = 1
	}

	ehSetting := getEHSetting(h.db)
	result, err := h.ehService.FetchGalleryList(&account, params, ehSetting)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 🟢 解构 result.Comics 并挂载本地 SQLite 收藏状态
	comics := services.AttachFavoriteStates(h.db, result.Comics)

	c.JSON(http.StatusOK, gin.H{
		"comics":      comics,
		"totalPages":  result.TotalPages,
		"currentPage": result.CurrentPage,
		"next":        result.Next,    // 向下游标
		"prev":        result.Prev,    // 向上游标
		"hasMore":     result.HasMore, // 是否有更多标记
	})
}

// 🟢 新增：GetWatchedComics 抓取线上订阅列表 (/watched)
func (h *OnlineComicHandler) GetWatchedComics(c *gin.Context) {
	var account models.AccountSetting
	if err := h.db.First(&account, 1).Error; err != nil || account.IPBMemberID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "请先绑定并保存 E 站账户凭证"})
		return
	}

	var params services.SearchParams
	if err := c.ShouldBindQuery(&params); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "非法请求参数"})
		return
	}

	if params.Page <= 0 {
		params.Page = 1
	}

	// 调用 services/eh_sub.go 的 FetchWatchedList
	result, err := h.ehService.FetchWatchedList(&account, params)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 挂载本地 SQLite 收藏状态
	comics := services.AttachFavoriteStates(h.db, result.Comics)

	c.JSON(http.StatusOK, gin.H{
		"comics":      comics,
		"totalPages":  result.TotalPages,
		"currentPage": result.CurrentPage,
		"next":        result.Next,    // 向下游标
		"prev":        result.Prev,    // 向上游标
		"hasMore":     result.HasMore, // 是否有更多标记
	})
}

// ProxyCover 代理转发 ExHentai / E-Hentai 的封面图片
func (h *OnlineComicHandler) ProxyCover(c *gin.Context) {
	targetURL := c.Query("url")
	if targetURL == "" {
		c.Status(http.StatusBadRequest)
		return
	}

	var account models.AccountSetting
	if err := h.db.First(&account, 1).Error; err != nil {
		c.Status(http.StatusUnauthorized)
		return
	}

	client, err := h.ehService.BuildClient(&account)
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}

	req, err := http.NewRequest("GET", targetURL, nil)
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://exhentai.org/")

	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		c.Status(http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	c.Header("Content-Type", resp.Header.Get("Content-Type"))
	c.Header("Cache-Control", "public, max-age=86400")

	_, _ = io.Copy(c.Writer, resp.Body)
}

// GetOnlineComicDetail 获取画廊详情
func (h *OnlineComicHandler) GetOnlineComicDetail(c *gin.Context) {
	gid := c.Query("id")
	token := c.Query("token")

	if gid == "" || token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数缺失，必需传递 id (GID) 和 token"})
		return
	}

	var account models.AccountSetting
	if err := h.db.First(&account, 1).Error; err != nil || account.IPBMemberID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "请先绑定并保存 E 站账户凭证"})
		return
	}

	ehSetting := getEHSetting(h.db)
	detail, err := h.ehService.FetchGalleryDetail(&account, gid, token, ehSetting)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 1. 如果 E 站网页端成功解析出了收藏状态，同步刷新到 SQLite 本地库
	if detail.IsFavorite && detail.FavIndex != nil {
		favState := models.FavoriteState{
			GID:    gid,
			Token:  token,
			FavCat: *detail.FavIndex,
		}
		h.db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "g_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"fav_cat", "token", "updated_at"}),
		}).Create(&favState)
	} else {
		// 2. 如果网页端未解析出，使用 SQLite 中的本地数据兜底
		detail = services.AttachDetailFavoriteState(h.db, detail)
	}

	c.JSON(http.StatusOK, detail)
}

// GetOnlinePopular 获取线上热门画廊列表
func (h *OnlineComicHandler) GetOnlinePopular(c *gin.Context) {
	var account models.AccountSetting
	if err := h.db.First(&account, 1).Error; err != nil || account.IPBMemberID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "请先绑定并保存 E 站账户凭证"})
		return
	}

	ehSetting := getEHSetting(h.db)
	comics, err := h.ehService.FetchPopularList(&account, ehSetting)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	comics = services.AttachFavoriteStates(h.db, comics)

	c.JSON(http.StatusOK, gin.H{
		"comics": comics,
	})
}

// GetOnlineComicPreviews 独立获取指定页码的预览图列表
func (h *OnlineComicHandler) GetOnlineComicPreviews(c *gin.Context) {
	gid := c.Query("id")
	token := c.Query("token")
	pageStr := c.DefaultQuery("page", "1")

	if gid == "" || token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数缺失，必需传递 id (GID) 和 token"})
		return
	}

	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "页码格式不正确"})
		return
	}

	var account models.AccountSetting
	if err := h.db.First(&account, 1).Error; err != nil || account.IPBMemberID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "请先绑定并保存 E 站账户凭证"})
		return
	}

	ehSetting := getEHSetting(h.db)
	previews, err := h.ehService.FetchGalleryPreviews(&account, gid, token, page, ehSetting)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, previews)
}

// 辅助方法：同时获取账号凭证与 EH 设置
func (h *OnlineComicHandler) getAccountAndSetting() (*models.AccountSetting, *models.EHSetting, error) {
	var account models.AccountSetting
	if err := h.db.First(&account, 1).Error; err != nil || account.IPBMemberID == "" {
		return nil, nil, errors.New("请先绑定并保存 E 站账户凭证")
	}

	var setting models.EHSetting
	h.db.FirstOrCreate(&setting, models.EHSetting{
		ID:             1,
		Site:           "e-hentai",
		PreferRedirect: true,
	})

	return &account, &setting, nil
}

