package services

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"SakuHentai/internal/models"

	"github.com/PuerkitoBio/goquery"
)

// FetchWatchedList 抓取 E 站订阅列表 (/watched)
func (s *EHService) FetchWatchedList(account *models.AccountSetting, params SearchParams) (*OnlineComicResult, error) {
	// 1. 订阅（Watched）页面强依赖 E 站 Cookie，必须先校验账号登录状态
	if account == nil || account.IPBMemberID == "" || account.IPBPassHash == "" {
		return nil, errors.New("未登录 E 站账号或 Cookie 无效，无法获取订阅列表")
	}

	client, err := s.BuildClient(account)
	if err != nil {
		return nil, err
	}

	// 2. 指向 /watched 端点
	// eh_sub.go 构造 URL 逻辑
	baseURL := "https://e-hentai.org/watched"
	// 仅当明确开启 Ex 且配有 igneous 时才走 Ex 站
	if account.IsEx && account.Igneous != "" {
		baseURL = "https://exhentai.org/watched"
	}

	reqURL, _ := url.Parse(baseURL)
	q := reqURL.Query()

	// 3. 拼接搜索关键词
	if params.Keyword != "" {
		q.Set("f_search", params.Keyword)
	}

	// 4. 游标与日期跳转逻辑 (与首页一致)
	if params.Next != "" {
		q.Set("next", params.Next)
	} else if params.Prev != "" {
		q.Set("prev", params.Prev)
	} else if params.Seek != "" {
		q.Set("seek", params.Seek)
	} else if params.Page > 1 {
		q.Set("p", strconv.Itoa(params.Page-1))
	}

	// 5. 分类掩码计算
	fCats := CalculateFCats(params.ActiveCategories)
	if fCats > 0 {
		q.Set("f_cats", strconv.Itoa(fCats))
	}

	reqURL.RawQuery = q.Encode()

	// 6. 发起 HTTP 请求
	req, _ := http.NewRequest("GET", reqURL.String(), nil)
	req.Close = true

	// 补全 HTTP 伪装头信息
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
		return nil, fmt.Errorf("请求 E 站订阅页失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("E 站订阅页响应状态异常: %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("解析订阅页 HTML 失败: %v", err)
	}

	var comics []OnlineComicDTO

	// 7. 解析画廊卡片 (完全复用画廊卡片的选择器)
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

	// 8. 提取 Next 游标 (#dnext)
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

	// 9. 提取 Prev 游标 (#dprev)
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
		HasMore:     nextCursor != "",
	}, nil
}