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
func (s *EHService) FetchGalleryDetail(account *models.AccountSetting, gid, token string) (*GalleryDetailResult, error) {
	client, err := s.BuildClient(account)
	if err != nil {
		return nil, err
	}

	baseURL := "https://e-hentai.org"
	if account.IsEx {
		baseURL = "https://exhentai.org"
	}

	tags := make([]string, 0)
	previewPages := make([]PreviewPageDTO, 0)
	comments := make([]CommentDTO, 0)

	detailURL := fmt.Sprintf("%s/g/%s/%s/?p=0", baseURL, gid, token)
	req, _ := http.NewRequest("GET", detailURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	
	// 🟢 关键：通过 Cookie 显式强制 ExHentai 返回大缩略图模式 (ts_l)
	req.AddCookie(&http.Cookie{Name: "inline_set", Value: "ts_l"})

	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != 200 {
		return nil, fmt.Errorf("获取画廊详情失败")
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
		if strings.Contains(favText, "Favorited") {
			isFav = true
			defaultIdx := 0
			favIdx = &defaultIdx

			for idx := 0; idx <= 9; idx++ {
				if strings.Contains(favText, fmt.Sprintf("Favorite %d", idx)) {
					i := idx
					favIdx = &i
					break
				}
			}
		}
	}

	return &GalleryDetailResult{
		ID:             gid,
		Token:          token,
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

// FetchGalleryPreviews 抓取指定页码 (p=0, p=1...) 的预览图切片
func (s *EHService) FetchGalleryPreviews(account *models.AccountSetting, gid, token string, page int) ([]PreviewPageDTO, error) {
	client, err := s.BuildClient(account)
	if err != nil {
		return nil, err
	}

	baseURL := "https://e-hentai.org"
	if account.IsEx {
		baseURL = "https://exhentai.org"
	}

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