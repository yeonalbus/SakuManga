package services

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"SakuHentai/internal/models"

	"github.com/PuerkitoBio/goquery"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type FavoritesService struct {
	ehService *EHService
}

func NewFavoritesService(ehService *EHService) *FavoritesService {
	return &FavoritesService{ehService: ehService}
}

// FetchFavoritesList 抓取指定收藏夹分类 (0~9) 及页码的在线画廊列表
// 🟢 修复：添加 db *gorm.DB 参数，以便落盘 SQLite
func (s *FavoritesService) FetchFavoritesList(db *gorm.DB, account *models.AccountSetting, favCat int, page int) (*OnlineComicResult, error) {
	client, err := s.ehService.BuildClient(account)
	if err != nil {
		return nil, err
	}

	baseURL := "https://e-hentai.org/"
	if account.IsEx {
		baseURL = "https://exhentai.org/"
	}

	ehPage := page - 1
	if ehPage < 0 {
		ehPage = 0
	}

	reqURL := fmt.Sprintf("%sfavorites.php?favcat=%d&p=%d", baseURL, favCat, ehPage)

	req, _ := http.NewRequest("GET", reqURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求收藏夹失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("E 站收藏夹响应状态异常: %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("解析收藏夹 HTML 失败: %v", err)
	}

	var comics []OnlineComicDTO

	doc.Find("table.itg tr, div.gl1t, div.gl2t, div.gl1e").Each(func(i int, sel *goquery.Selection) {
		linkNode := sel.Find("a[href*='/g/']").First()
		href, exists := linkNode.Attr("href")
		if !exists {
			return
		}

		parts := strings.Split(strings.Trim(href, "/"), "/")
		if len(parts) < 2 {
			return
		}
		gid := parts[len(parts)-2]
		token := parts[len(parts)-1]

		title := sel.Find(".glink, .gltitle").First().Text()
		if title == "" {
			return
		}

		rawCoverURL := extractCoverURL(sel)
		proxiedCoverURL := ""
		if rawCoverURL != "" {
			proxiedCoverURL = "http://localhost:8080/api/v1/comics/cover-proxy?url=" + url.QueryEscape(rawCoverURL)
		}

		category := strings.TrimSpace(sel.Find(".cs, .cn").First().Text())

		rating := 0.0
		if style, ok := sel.Find(".ir").Attr("style"); ok {
			rating = parseRatingFromStyle(style)
		}

		comics = append(comics, OnlineComicDTO{
			ID:           gid,
			Token:        token,
			Title:        title,
			CoverURL:     proxiedCoverURL,
			Source:       "online",
			Category:     category,
			Rating:       rating,
			IsFavorite:   true,
			FavIndex:     &favCat,
			IsDownloaded: false,
		})
	})

	totalPages := 1
	doc.Find("table.ptd td, table.ptt td").Each(func(i int, s *goquery.Selection) {
		text := strings.TrimSpace(s.Text())
		if p, err := strconv.Atoi(text); err == nil && p > totalPages {
			totalPages = p
		}
	})

	// 🟢 落盘本地 SQLite 状态
	if len(comics) > 0 {
		var batchFavs []models.FavoriteState
		for _, c := range comics {
			batchFavs = append(batchFavs, models.FavoriteState{
				GID:    c.ID,
				Token:  c.Token,
				FavCat: favCat,
			})
		}
		db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "gid"}},
			DoUpdates: clause.AssignmentColumns([]string{"fav_cat", "token", "updated_at"}),
		}).Create(&batchFavs)
	}

	return &OnlineComicResult{
		Comics:      comics,
		TotalPages:  totalPages,
		CurrentPage: page,
	}, nil
}

// AddFavorite 添加/修改在线收藏
func (s *FavoritesService) AddFavorite(db *gorm.DB, account *models.AccountSetting, gid, token string, favCat int, note string) error {
	client, err := s.ehService.BuildClient(account)
	if err != nil {
		return err
	}

	baseURL := "https://e-hentai.org/"
	if account.IsEx {
		baseURL = "https://exhentai.org/"
	}

	popURL := fmt.Sprintf("%sgallerypopups.php?gid=%s&t=%s&act=addfav", baseURL, gid, token)

	formData := url.Values{}
	formData.Set("favcat", strconv.Itoa(favCat))
	formData.Set("favnote", note)
	formData.Set("apply", "Apply Changes")
	formData.Set("update", "1")

	req, _ := http.NewRequest("POST", popURL, strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != 200 {
		return fmt.Errorf("远端提交收藏失败")
	}
	defer resp.Body.Close()

	favState := models.FavoriteState{
		GID:    gid,
		Token:  token,
		FavCat: favCat,
	}
	db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "gid"}},
		DoUpdates: clause.AssignmentColumns([]string{"fav_cat", "token", "updated_at"}),
	}).Create(&favState)

	return nil
}

// RemoveFavorite 取消在线收藏
func (s *FavoritesService) RemoveFavorite(db *gorm.DB, account *models.AccountSetting, gid, token string) error {
	client, err := s.ehService.BuildClient(account)
	if err != nil {
		return err
	}

	baseURL := "https://e-hentai.org/"
	if account.IsEx {
		baseURL = "https://exhentai.org/"
	}

	popURL := fmt.Sprintf("%sgallerypopups.php?gid=%s&t=%s&act=addfav", baseURL, gid, token)

	formData := url.Values{}
	formData.Set("favcat", "favdel")
	formData.Set("apply", "Apply Changes")
	formData.Set("update", "1")

	req, _ := http.NewRequest("POST", popURL, strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != 200 {
		return fmt.Errorf("远端取消收藏失败")
	}
	defer resp.Body.Close()

	db.Where("gid = ?", gid).Delete(&models.FavoriteState{})

	return nil
}

// AttachFavoriteStates 挂载 SQLite 中的本地收藏状态
func AttachFavoriteStates(db *gorm.DB, comics []OnlineComicDTO) []OnlineComicDTO {
	if len(comics) == 0 {
		return comics
	}

	gids := make([]string, 0, len(comics))
	for _, c := range comics {
		gids = append(gids, c.ID)
	}

	var favs []models.FavoriteState
	db.Where("gid IN ? AND fav_cat >= 0", gids).Find(&favs)

	favMap := make(map[string]int)
	for _, f := range favs {
		favMap[f.GID] = f.FavCat
	}

	for i := range comics {
		if cat, exists := favMap[comics[i].ID]; exists {
			comics[i].IsFavorite = true
			comics[i].FavIndex = &cat
		} else {
			comics[i].IsFavorite = false
			comics[i].FavIndex = nil
		}
	}

	return comics
}