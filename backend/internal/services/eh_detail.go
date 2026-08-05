package services

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"SakuHentai/internal/models"

	"github.com/PuerkitoBio/goquery"
)

// FetchGalleryDetail 请求并解析画廊详情页 (仅抓取 p=0 初始预览图与元数据)
func (s *EHService) FetchGalleryDetail(account *models.AccountSetting, gid, token string,setting *models.EHSetting) (*GalleryDetailResult, error) {
	client, err := s.BuildClient(account)
	if err != nil {
		return nil, err
	}

	baseURL := GetBaseURL(account, setting)
	
	tags := make([]string, 0)
	previewPages := make([]PreviewPageDTO, 0)
	comments := make([]CommentDTO, 0)

	detailURL := fmt.Sprintf("%s/g/%s/%s/?p=0", baseURL, gid, token)
	req, _ := http.NewRequest("GET", detailURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	
	// 🟢 关键：通过 Cookie 显式强制 ExHentai 返回大缩略图模式 (ts_l)
	req.AddCookie(&http.Cookie{Name: "inline_set", Value: "ts_l"})

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("获取画廊详情失败")
	}
	if resp.StatusCode != 200 {
		body := readBodyLimited(resp)
		resp.Body.Close()
		if gErr := classifyGalleryUnavailable(body); gErr != nil {
			return nil, gErr
		}
		return nil, fmt.Errorf("获取画廊详情失败（E 站返回 %d）", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	// 🟢 校验当前解析页面究竟是大图模式还是小图模式
	if doc.Find(".gdtm").Length() > 0 {
		log.Printf("[EH-WARN] 警告：画廊 [%s] 依然处于 gdtm (CSS雪花图) 模式，请检查 CookiePersist 是否生效！", gid)
	}
	if doc.Find(".gdtl").Length() == 0 {
		log.Printf("[EH-WARN] 画廊 [%s] 仍处于 gdtm (小图模式)，请检查 CookiePersist 是否生效", gid)
	}

	resp.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("解析 HTML 失败")
	}
	title := strings.TrimSpace(doc.Find("#gj").Text())
	if title == "" {
		title = strings.TrimSpace(doc.Find("#gn").Text())
	}
	subTitle := strings.TrimSpace(doc.Find("#gn").Text())

	// 父画廊解析：详情页底部 "Parent gallery" 链接形如 <a id="parent_0" href="/g/1234567/abcdef/">…
	parentGID := ""
	doc.Find("a[id^='parent_']").Each(func(_ int, a *goquery.Selection) {
		if parentGID != "" {
			return
		}
		if href, exists := a.Attr("href"); exists {
			parentGID = extractParentGID(href)
		}
	})

	rawCover := extractCoverURL(doc.Find("#gd1"))
	proxiedCover := ""
	if rawCover != "" {
		proxiedCover = "/api/v1/comics/cover-proxy?url=" + url.QueryEscape(rawCover)
	}

	uploader := strings.TrimSpace(doc.Find("#gdn").Text())
	category := strings.TrimSpace(doc.Find("#gdc").Text())
	updatedAt := ""
	pageCount := 0

	doc.Find("#gdd tr").Each(func(i int, tr *goquery.Selection) {
		label := strings.TrimSpace(tr.Find("td.gdt1").Text())
		value := strings.TrimSpace(tr.Find("td.gdt2").Text())
		if strings.HasPrefix(label, "Posted:") {
			updatedAt = value
		} else if strings.HasPrefix(label, "Length:") {
			if matches := pageCountRegex.FindStringSubmatch(value); len(matches) > 1 {
				pageCount, _ = strconv.Atoi(matches[1])
			}
		}
	})

	rating := 0.0
	if ratingStr := strings.TrimSpace(doc.Find("#rating_label").Text()); ratingStr != "" {
		parts := strings.Split(ratingStr, ":")
		if len(parts) > 1 {
			rating, _ = strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		}
	}

	doc.Find("#taglist tr").Each(func(i int, tr *goquery.Selection) {
		ns := strings.TrimSuffix(strings.TrimSpace(tr.Find("td.tc").Text()), ":")
		if ns == "" {
			ns = "other"
		}
		tr.Find("div[id^='ta_'], div.gt, div.gtl").Each(func(_ int, tagNode *goquery.Selection) {
			rawText := strings.TrimSpace(tagNode.Text())
			if rawText != "" {
				cleanKey := strings.ReplaceAll(rawText, "_", " ")
				tags = append(tags, fmt.Sprintf("%s:%s", ns, cleanKey))
			}
		})
	})

	doc.Find(".c1").Each(func(i int, s *goquery.Selection) {
		user := strings.TrimSpace(s.Find(".c3 a").First().Text())
		if user == "" {
			user = "匿名用户"
		}
		date := strings.TrimSpace(s.Find(".c3").Text())
		if dateMatches := dateRegex.FindString(date); dateMatches != "" {
			date = dateMatches
		}
		content := strings.TrimSpace(s.Find(".c6").Text())
		if content != "" {
			comments = append(comments, CommentDTO{
				ID:      int64(i + 1),
				User:    user,
				Date:    date,
				Content: content,
			})
		}
	})

	// 🟢 2. 仅解析第一页（p=0）的预览图切片，彻底剔除原本的 for 循环
	pageIdx := 1
	doc.Find("#gdt > div, #gdt > a").Each(func(_ int, s *goquery.Selection) {
		if dto, ok := parsePreviewTile(s, pageIdx, baseURL); ok {
			previewPages = append(previewPages, dto)
			pageIdx++
		}
	})
	// previewLinks := doc.Find("#gdt a[href*='/s/']")
	// if previewLinks.Length() == 0 {
	// 	previewLinks = doc.Find("#gdt > div")
	// 

	// 计算总预览页数
	maxPreviewPage := 1
	doc.Find("table.ptd td, table.ptt td, div.gtb td").Each(func(_ int, td *goquery.Selection) {
		if text := strings.TrimSpace(td.Text()); text != "" {
			if p, err := strconv.Atoi(text); err == nil && p > maxPreviewPage {
				maxPreviewPage = p
			}
		}
	})

	log.Printf("[EH-DETAIL-DEBUG] 成功抓取画廊 [%s] 首页 | 初始预览图: %d 张 | 总预览页数: %d | 社区评论: %d 条", gid, len(previewPages), maxPreviewPage, len(comments))
	isFav := false
	var favIdx *int

	favLink := doc.Find("#favoritelink")
	if favLink.Length() > 0 {
		favText := strings.TrimSpace(favLink.Text())

		// 🟢 关键修复：只要文本不包含 "Add to Favorites" 且非空，说明已经加入收藏
		if favText != "" && !strings.Contains(favText, "Add to Favorites") {
			isFav = true
			defaultIdx := 0
			favIdx = &defaultIdx

			// 1. 特殊情况：E 站默认 Favorite 0 的名称为 "Favorites"
			if favText == "Favorites" {
				i := 0
				favIdx = &i
			} else {
				// 2. 匹配 "Favorite 0" ~ "Favorite 9"
				for idx := 0; idx <= 9; idx++ {
					if strings.Contains(favText, fmt.Sprintf("Favorite %d", idx)) {
						i := idx
						favIdx = &i
						break
					}
				}
			}

			// 🟢 3. 颜色样式兜底：解析 inline style 里的颜色（E 站给每个 Fav 赋予了专属 color）
			if styleAttr, ok := favLink.Attr("style"); ok {
				if colorIdx := parseFavColorStyle(styleAttr); colorIdx >= 0 {
					favIdx = &colorIdx
				}
			}
		}
	}

	return &GalleryDetailResult{
		ID:             gid,
		Token:          token,
		ParentGID:      parentGID,
		Title:          title,
		SubTitle:       subTitle,
		CoverURL:       proxiedCover,
		Source:         "online",
		Category:       category,
		Uploader:       uploader,
		Rating:         rating,
		PageCount:      pageCount,
		UpdatedAt:      updatedAt,
		Tags:           tags,
		PreviewPages:   previewPages,
		Comments:       comments,
		IsFavorite:     isFav,
		FavIndex:       favIdx,
		MaxPreviewPage: maxPreviewPage, // 💡 可以在 DTO 中新增此字段，方便前端感知总页数
	}, nil
}

// parseFavColorStyle 根据 #favoritelink 的 inline style 颜色识别 0 ~ 9 收藏夹
func parseFavColorStyle(style string) int {
	style = strings.ToLower(style)
	// E 站经典 0~9 收藏夹颜色映射
	colors := map[string]int{
		"#7f7f7f": 0, // Fav 0 (灰色)
		"#f00000": 1, // Fav 1 (红色)
		"#ff7800": 2, // Fav 2 (橙色)
		"#f0d000": 3, // Fav 3 (黄/金)
		"#cbb000": 3,
		"#00a000": 4, // Fav 4 (绿色)
		"#98e020": 5, // Fav 5 (浅绿/青)
		"#00a0c0": 5,
		"#00a0a0": 6, // Fav 6 (蓝色)
		"#0000f0": 6,
		"#a000a0": 7, // Fav 7 (紫色)
		"#505050": 8, // Fav 8 (深灰)
		"#f000a0": 9, // Fav 9 (粉红)
		"#000000": 9,
	}

	for hexColor, cat := range colors {
		if strings.Contains(style, hexColor) {
			return cat
		}
	}
	return -1
}

// extractParentGID 从 /g/{gid}/{token}/ 形式的 href 中提取父画廊 gid
func extractParentGID(href string) string {
	const prefix = "/g/"
	idx := strings.Index(href, prefix)
	if idx < 0 {
		return ""
	}
	rest := href[idx+len(prefix):]
	end := strings.Index(rest, "/")
	if end <= 0 {
		return ""
	}
	return rest[:end]
}

// FetchGalleryPreviews 抓取指定页码 (p=0, p=1...) 的预览图切片
func (s *EHService) FetchGalleryPreviews(account *models.AccountSetting, gid, token string, page int,setting *models.EHSetting) ([]PreviewPageDTO, error) {
	client, err := s.BuildClient(account)
	if err != nil {
		return nil, err
	}

	baseURL := GetBaseURL(account, setting)

	// 1. 处理页码对齐：E 站 URL 的 p 参数是 0-based 索引 (p=0 对应第 1 页, p=1 对应第 2 页)
	// 如果前端传入的 page 是从 1 开始的，这里转换为 ehPage = page - 1
	ehPage := page
	if ehPage > 0 {
		ehPage = page - 1
	}

	previewURL := fmt.Sprintf("%s/g/%s/%s/?p=%d", baseURL, gid, token, ehPage)
	req, _ := http.NewRequest("GET", previewURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != 200 {
		return nil, fmt.Errorf("获取第 %d 页预览图失败", page)
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("解析预览页 HTML 失败")
	}

	previewPages := make([]PreviewPageDTO, 0)

	// 2. 遍历 #gdt 下的元素，统一使用 parsePreviewTile 解析 (兼容 gdtl 大图与 gdtm 雪碧图)
	itemIdx := 1
	doc.Find("#gdt > div, #gdt > a").Each(func(_ int, s *goquery.Selection) {
		if dto, ok := parsePreviewTile(s, itemIdx, baseURL); ok {
			previewPages = append(previewPages, dto)
			itemIdx++
		}
	})

	log.Printf("[EH-PREVIEW-DEBUG] 画廊 [%s] 第 %d 页预览图抓取成功 | 包含切片: %d 张", gid, page, len(previewPages))

	return previewPages, nil
}