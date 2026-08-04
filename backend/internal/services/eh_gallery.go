package services

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"SakuHentai/internal/models"

	"github.com/PuerkitoBio/goquery"
)

// FetchGalleryList 发起网络请求并解析画廊列表
func (s *EHService) FetchGalleryList(account *models.AccountSetting, params SearchParams) (*OnlineComicResult, error) {
	client, err := s.BuildClient(account)
	if err != nil {
		return nil, err
	}

	baseURL := "https://e-hentai.org/"
	if account.IsEx {
		baseURL = "https://exhentai.org/"
	}

	reqURL, _ := url.Parse(baseURL)
	q := reqURL.Query()

	// 1. 搜索关键词
	if params.Keyword != "" {
		q.Set("f_search", params.Keyword)
	}

	// 2. 游标与日期跳转优先级逻辑
	if params.Next != "" {
		// 向下滑动：向下游标
		q.Set("next", params.Next)
	} else if params.Prev != "" {
		// 向上滑动：向上游标
		q.Set("prev", params.Prev)
	} else if params.Seek != "" {
		// 按日期跳转：填入 seek 参数 (YYYY-MM-DD)
		q.Set("seek", params.Seek)
	} else if params.Page > 1 {
		// 兜底兼容传统页码
		q.Set("p", strconv.Itoa(params.Page-1))
	}

	// 3. 分类掩码
	fCats := CalculateFCats(params.ActiveCategories)
	if fCats > 0 {
		q.Set("f_cats", strconv.Itoa(fCats))
	}

	reqURL.RawQuery = q.Encode()

	// 发起请求
	req, _ := http.NewRequest("GET", reqURL.String(), nil)
	req.Close = true

	// 补全伪装浏览器请求头
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")

	resp, err := client.Do(req)
	if err != nil && strings.Contains(err.Error(), "EOF") {
		reqRetry, _ := http.NewRequest("GET", reqURL.String(), nil)
		reqRetry.Close = true
		reqRetry.Header = req.Header
		resp, err = client.Do(reqRetry)
	}
	if err != nil {
		return nil, fmt.Errorf("请求 E 站失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("E 站响应状态异常: %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("解析 HTML 失败: %v", err)
	}

	var comics []OnlineComicDTO

	// 解析画廊卡片
	doc.Find("table.itg tr, div.gl1t, div.gl2t, div.gl1e, div.gl2e").Each(func(i int, s *goquery.Selection) {
		linkNode := s.Find("a[href*='/g/']").First()
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

		title := s.Find(".glink, .gltitle").First().Text()
		if title == "" {
			return
		}

		rawCoverURL := extractCoverURL(s)
		proxiedCoverURL := ""
		if rawCoverURL != "" {
			proxiedCoverURL = "/api/v1/comics/cover-proxy?url=" + url.QueryEscape(rawCoverURL)
		}

		category := strings.TrimSpace(s.Find(".cs, .cn").First().Text())

		rating := 0.0
		if style, ok := s.Find(".ir").Attr("style"); ok {
			rating = parseRatingFromStyle(style)
		}

		var tags []string
		s.Find("div.gt, div.gtl, div.gtw, div.gtd, div[title*=':']").Each(func(_ int, tagNode *goquery.Selection) {
			tagStr := ""
			if t, ok := tagNode.Attr("title"); ok && strings.Contains(t, ":") {
				tagStr = t
			} else if id, ok := tagNode.Attr("id"); ok && strings.HasPrefix(id, "ta_") {
				tagStr = strings.TrimPrefix(id, "ta_")
				tagStr = strings.ReplaceAll(tagStr, "_", " ")
			} else {
				tagStr = strings.TrimSpace(tagNode.Text())
			}

			if tagStr != "" {
				tags = append(tags, strings.ToLower(tagStr))
			}
		})

		pageCount := 0
		itemText := s.Text()
		if matches := pageCountRegex.FindStringSubmatch(itemText); len(matches) > 1 {
			pageCount, _ = strconv.Atoi(matches[1])
		}

		updatedAt := ""
		if match := dateRegex.FindString(itemText); match != "" {
			updatedAt = match
		}

		uploader := ""
		s.Find("a[href*='/uploader/'], .gl5m a, .gl3e a").EachWithBreak(func(_ int, uNode *goquery.Selection) bool {
			txt := strings.TrimSpace(uNode.Text())
			if txt != "" {
				uploader = txt
				return false
			}
			return true
		})

		comics = append(comics, OnlineComicDTO{
			ID:           gid,
			Token:        token,
			Title:        title,
			CoverURL:     proxiedCoverURL,
			Source:       "online",
			Category:     category,
			Rating:       rating,
			Tags:         tags,
			PageCount:    pageCount,
			UpdatedAt:    updatedAt,
			Uploader:     uploader,
			IsDownloaded: false,
		})
	})

	totalPages := parseTotalPagesByCount(doc)

	// 🟢 提取 Next 游标 (节点 ID 为 #dnext)
	nextCursor := ""
	if dnext := doc.Find("#dnext"); dnext.Length() > 0 {
		if href, ok := dnext.Attr("href"); ok {
			if u, err := url.Parse(href); err == nil {
				nextCursor = u.Query().Get("next")
				if nextCursor == "" {
					nextCursor = u.Query().Get("from")
				}
			}
		}
	}

	// 🟢 提取 Prev 游标 (节点 ID 为 #dprev)
	prevCursor := ""
	if dprev := doc.Find("#dprev"); dprev.Length() > 0 {
		if href, ok := dprev.Attr("href"); ok {
			if u, err := url.Parse(href); err == nil {
				prevCursor = u.Query().Get("prev")
				if prevCursor == "" {
					prevCursor = u.Query().Get("from")
				}
			}
		}
	}

	return &OnlineComicResult{
		Comics:      comics,
		TotalPages:  totalPages,
		CurrentPage: params.Page,
		Next:        nextCursor,
		Prev:        prevCursor,
		HasMore:     nextCursor != "", // 如果 Next 游标不为空，则认为还有下一页
	}, nil
}

// FetchPopularList 维持原样...
func (s *EHService) FetchPopularList(account *models.AccountSetting) ([]OnlineComicDTO, error) {
	client, err := s.BuildClient(account)
	if err != nil {
		return nil, err
	}

	baseURL := "https://e-hentai.org/"
	if account.IsEx {
		baseURL = "https://exhentai.org/"
	}

	reqURL := baseURL + "popular"

	req, _ := http.NewRequest("GET", reqURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求热门列表失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("E 站热门响应状态异常: %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("解析 HTML 失败: %v", err)
	}

	var comics []OnlineComicDTO

	doc.Find("table.itg tr, div.gl1t, div.gl2t, div.gl1e").Each(func(i int, s *goquery.Selection) {
		linkNode := s.Find("a[href*='/g/']").First()
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

		title := s.Find(".glink, .gltitle").First().Text()
		if title == "" {
			return
		}

		rawCoverURL := extractCoverURL(s)
		proxiedCoverURL := ""
		if rawCoverURL != "" {
			proxiedCoverURL = "/api/v1/comics/cover-proxy?url=" + url.QueryEscape(rawCoverURL)
		}

		category := strings.TrimSpace(s.Find(".cs, .cn").First().Text())

		rating := 0.0
		if style, ok := s.Find(".ir").Attr("style"); ok {
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
			IsDownloaded: false,
		})
	})

	return comics, nil
}