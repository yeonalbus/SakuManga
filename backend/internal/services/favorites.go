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

// FetchFavoritesList 抓取指定收藏夹分类 (0~9) 及游标的在线画廊列表
func (s *FavoritesService) FetchFavoritesList(db *gorm.DB, userID uint, account *models.AccountSetting, favCat int, next, prev, seek, sortMode string, setting *models.EHSetting) (*OnlineComicResult, error) {
	client, err := s.ehService.BuildClient(account)
	currentFav := favCat
	if err != nil {
		return nil, err
	}

	baseURL := GetBaseURL(account, setting)
	reqURL, _ := url.Parse(baseURL + "favorites.php")
	q := reqURL.Query()
	q.Set("favcat", strconv.Itoa(favCat))

	// 🟢 核心修复：只有在没有游标 (next/prev) 的初始请求时，才发送 inline_set 参数！
	// 避免翻页时携带有冲突的排序设置导致 E 站重置 pagination DOM 结构
	if next == "" && prev == "" && seek == "" {
		if sortMode == "published" {
			q.Set("inline_set", "fs_p") // 按发布时间排序
		} else if sortMode == "favorited" {
			q.Set("inline_set", "fs_f") // 按收藏时间排序
		}
	}

	if next != "" {
		q.Set("next", next)
	} else if prev != "" {
		q.Set("prev", prev)
	} else if seek != "" {
		q.Set("seek", seek)
	}
	reqURL.RawQuery = q.Encode()

	req, _ := http.NewRequest("GET", reqURL.String(), nil)
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

	debugPrintPaginationDOM(doc, reqURL.String())

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
			proxiedCoverURL = "/api/v1/comics/cover-proxy?url=" + url.QueryEscape(rawCoverURL)
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
			FavIndex:     &currentFav,
			IsDownloaded: false,
		})
	})

	// 落盘本地 SQLite 状态
	if len(comics) > 0 {
		var batchFavs []models.FavoriteState
		for _, c := range comics {
			batchFavs = append(batchFavs, models.FavoriteState{
				UserID: userID,
				GID:    c.ID,
				Token:  c.Token,
				FavCat: favCat,
			})
		}
		db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_id"}, {Name: "g_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"fav_cat", "token", "updated_at"}),
		}).Create(&batchFavs)
	}

	// 全排版兼容游标解析
	nextCursor, prevCursor := extractCursors(doc)

	return &OnlineComicResult{
		Comics:  comics,
		Next:    nextCursor,
		Prev:    prevCursor,
		HasMore: nextCursor != "",
	}, nil
}

// 🟢 多模式游标解析器（修复节点选择器与游标清洗逻辑）
func extractCursors(doc *goquery.Document) (string, string) {
	nextCursor := ""
	prevCursor := ""

	// 1. 提取 Next 游标 (优先匹配 #unext 顶部栏, #dnext 底部栏, #next 通用)
	doc.Find("#unext, #dnext, #next").Each(func(_ int, s *goquery.Selection) {
		if nextCursor != "" {
			return // 已经提取到有效游标则退出
		}
		// 排除 span 或被禁用的节点
		if s.Is("span") || s.HasClass("disabled") {
			return
		}
		// 节点本身是 <a>，或者从其子代提取 <a>
		aSel := s
		if !s.Is("a") {
			aSel = s.Find("a").First()
		}
		if href, ok := aSel.Attr("href"); ok {
			if u, err := url.Parse(href); err == nil {
				nextCursor = u.Query().Get("next")
				if nextCursor == "" {
					nextCursor = u.Query().Get("from")
				}
			}
		}
	})

	// 2. 提取 Prev 游标 (优先匹配 #uprev 顶部栏, #dprev 底部栏, #prev 通用)
	doc.Find("#uprev, #dprev, #prev").Each(func(_ int, s *goquery.Selection) {
		if prevCursor != "" {
			return // 已经提取到有效游标则退出
		}
		// 排除 span 或被禁用的节点
		if s.Is("span") || s.HasClass("disabled") {
			return
		}
		// 节点本身是 <a>，或者从其子代提取 <a>
		aSel := s
		if !s.Is("a") {
			aSel = s.Find("a").First()
		}
		if href, ok := aSel.Attr("href"); ok {
			if u, err := url.Parse(href); err == nil {
				prevCursor = u.Query().Get("prev")
				if prevCursor == "" {
					prevCursor = u.Query().Get("from")
				}
			}
		}
	})

	// 3. 清洗哨兵边界值 (1-0, 0-0, 0)
	if isBoundaryCursor(nextCursor) {
		nextCursor = ""
	}
	if isBoundaryCursor(prevCursor) {
		prevCursor = ""
	}

	return nextCursor, prevCursor
}

// 辅助校验函数
func isBoundaryCursor(cursor string) bool {
	return cursor == "" || cursor == "1-0" || cursor == "0-0" || cursor == "0"
}

// AddFavorite 添加/修改在线收藏
func (s *FavoritesService) AddFavorite(db *gorm.DB, userID uint, account *models.AccountSetting, gid, token string, favCat int, note string, setting *models.EHSetting) error {
	client, err := s.ehService.BuildClient(account)
	if err != nil {
		return err
	}

	baseURL := GetBaseURL(account, setting)
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
		UserID: userID,
		GID:    gid,
		Token:  token,
		FavCat: favCat,
	}
	db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "g_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"fav_cat", "token", "updated_at"}),
	}).Create(&favState)

	return nil
}

// RemoveFavorite 取消在线收藏
func (s *FavoritesService) RemoveFavorite(db *gorm.DB, userID uint, account *models.AccountSetting, gid, token string, setting *models.EHSetting) error {
	client, err := s.ehService.BuildClient(account)
	if err != nil {
		return err
	}

	baseURL := GetBaseURL(account, setting)

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

	db.Where("user_id = ? AND g_id = ?", userID, gid).Delete(&models.FavoriteState{})

	return nil
}

// AttachFavoriteStates 挂载 SQLite 中的本地收藏状态 (仅补充，绝不误杀原状态)
func AttachFavoriteStates(db *gorm.DB, userID uint, comics []OnlineComicDTO) []OnlineComicDTO {
	if len(comics) == 0 {
		// 无结果时返回非 nil 空切片，避免 JSON 序列化为 null
		// （前端 filteredComics 会对 comics 直接 .filter，null 会导致渲染崩溃）
		return make([]OnlineComicDTO, 0)
	}

	gids := make([]string, 0, len(comics))
	for _, c := range comics {
		gids = append(gids, c.ID)
	}

	var favs []models.FavoriteState
	db.Where("user_id = ? AND g_id IN ? AND fav_cat >= 0", userID, gids).Find(&favs)

	favMap := make(map[string]int)
	for _, f := range favs {
		favMap[f.GID] = f.FavCat
	}

	for i := range comics {
		if cat, exists := favMap[comics[i].ID]; exists {
			comics[i].IsFavorite = true
			
			// 🟢 修复指针共享：重新分配独立的内存地址
			idx := cat
			comics[i].FavIndex = &idx
		}
		// 🟢 关键修复：如果 SQLite 里没有，保持 HTML 解析原样！严禁强行改为 false！
	}

	return comics
}

// 🟢 新增：AttachDownloadStates 挂载本地"已下载"状态
// 通过比对离线漫画库 (OfflineComic) 的 GID 字段，判断哪些在线画廊已下载到本地，
// 从而让首页/热门/订阅/收藏等在线列表统一显示"已下载"角标。
// 注意：本地漫画库为全局共享，无需按用户隔离；只补 true，绝不误改 false。
func AttachDownloadStates(db *gorm.DB, comics []OnlineComicDTO) []OnlineComicDTO {
	if len(comics) == 0 {
		// 无结果时返回非 nil 空切片，避免 JSON 序列化为 null 破坏前端渲染
		return make([]OnlineComicDTO, 0)
	}

	gids := make([]string, 0, len(comics))
	for _, c := range comics {
		if c.ID != "" {
			gids = append(gids, c.ID)
		}
	}
	if len(gids) == 0 {
		return comics
	}

	var rows []models.OfflineComic
	db.Select("g_id").Where("g_id IN ?", gids).Find(&rows)

	downloaded := make(map[string]bool, len(rows))
	for _, r := range rows {
		if r.GID != "" {
			downloaded[r.GID] = true
		}
	}

	for i := range comics {
		if downloaded[comics[i].ID] {
			comics[i].IsDownloaded = true
		}
	}

	return comics
}

// 🟢 新增：专为详情页 (GalleryDetailResult) 提供本地 SQLite 状态挂载
func AttachDetailFavoriteState(db *gorm.DB, userID uint, detail *GalleryDetailResult) *GalleryDetailResult {
	if detail == nil || detail.ID == "" {
		return detail
	}

	var fav models.FavoriteState
	err := db.Where("user_id = ? AND g_id = ? AND fav_cat >= 0", userID, detail.ID).First(&fav).Error
	if err == nil {
		detail.IsFavorite = true
		idx := fav.FavCat
		detail.FavIndex = &idx
	}

	return detail
}

// 🟢 新增：详情页挂载本地「已下载」状态（本地离线库存在同 GID 记录 → 拦截重复下载提示）
func AttachDetailDownloadState(db *gorm.DB, detail *GalleryDetailResult) *GalleryDetailResult {
	if detail == nil || detail.ID == "" {
		return detail
	}

	var count int64
	db.Model(&models.OfflineComic{}).Where("g_id = ?", detail.ID).Count(&count)
	if count > 0 {
		detail.IsDownloaded = true
	}

	return detail
}

// ChangeFavoriteSortOrder 独立触发 E 站排序状态切换，并保存返回的 Cookie
func (s *FavoritesService) ChangeFavoriteSortOrder(db *gorm.DB, userID uint, account *models.AccountSetting, sortMode string, setting *models.EHSetting) error {
	client, err := s.ehService.BuildClient(account)
	if err != nil {
		return err
	}

	baseURL := GetBaseURL(account, setting)
	inlineSet := "fs_f" // 默认按收藏时间
	if sortMode == "published" {
		inlineSet = "fs_p" // 按发布时间
	}

	// 🟢 对标 JHenTai: 纯净无污染请求，不带 favcat，不带 next/prev
	reqURL := fmt.Sprintf("%sfavorites.php?inline_set=%s", baseURL, inlineSet)

	req, _ := http.NewRequest("GET", reqURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("切换排序请求失败: %v", err)
	}
	defer resp.Body.Close()

	// 🟢 核心点：从 client.Jar 中提取 E 站下发的最新 Cookie (sk)
	u, _ := url.Parse(baseURL)
	if client.Jar != nil {
		for _, cookie := range client.Jar.Cookies(u) {
			if cookie.Name == "sk" && cookie.Value != "" {
				// 更新到本地账号模型并落盘 SQLite
				account.SK = cookie.Value
				db.Model(&models.User{}).Where("id = ?", userID).Update("sk", cookie.Value)
				break
			}
		}
	}

	return nil
}

// 🟢 Debug: 深度打印页面中的分页与游标 DOM 结构
func debugPrintPaginationDOM(doc *goquery.Document, reqURL string) {
	fmt.Printf("\n================ [EH Cursor Debug Start] ================\n")
	fmt.Printf("当前请求 URL: %s\n", reqURL)

	// 1. 检查 #dprev 与 #dnext 节点本身（及其父级/子级）
	fmt.Println("\n--- [1. ID 选择器匹配情况] ---")
	doc.Find("#dprev, #dnext, #prev, #next").Each(func(i int, s *goquery.Selection) {
		id, _ := s.Attr("id")
		class, _ := s.Attr("class")
		html, _ := goquery.OuterHtml(s)
		fmt.Printf("#%d ID: [%s] | Class: [%s] | OuterHTML: %s\n", i, id, class, strings.TrimSpace(html))
	})

	// 2. 扫描整页所有带 prev= 的 <a> 标签
	fmt.Println("\n--- [2. 所有包含 'prev=' 的 <a> 链接] ---")
	doc.Find("a[href*='prev=']").Each(func(i int, s *goquery.Selection) {
		href, _ := s.Attr("href")
		class, _ := s.Attr("class")
		parentClass, _ := s.Parent().Attr("class")
		text := strings.TrimSpace(s.Text())
		fmt.Printf("Index %d | ParentClass: [%s] | SelfClass: [%s] | Text: [%s] | Href: %s\n", 
			i, parentClass, class, text, href)
	})

	// 3. 扫描整页所有带 next= 的 <a> 标签
	fmt.Println("\n--- [3. 所有包含 'next=' 的 <a> 链接] ---")
	doc.Find("a[href*='next=']").Each(func(i int, s *goquery.Selection) {
		href, _ := s.Attr("href")
		class, _ := s.Attr("class")
		parentClass, _ := s.Parent().Attr("class")
		text := strings.TrimSpace(s.Text())
		fmt.Printf("Index %d | ParentClass: [%s] | SelfClass: [%s] | Text: [%s] | Href: %s\n", 
			i, parentClass, class, text, href)
	})

	fmt.Printf("================ [EH Cursor Debug End] ==================\n\n")
}