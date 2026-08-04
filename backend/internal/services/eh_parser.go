package services

import (
	"log"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

var (
	urlRegex         = regexp.MustCompile(`url\(['"]?(.*?)['"]?\)`)
	resultCountRegex = regexp.MustCompile(`(?i)(?:Found\s+(?:about\s+)?|of\s+)([\d,]+)\s+results`)
	pageCountRegex   = regexp.MustCompile(`(\d+)\s*(?:pages|P|页)`)
	dateRegex        = regexp.MustCompile(`\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}`)
)

// CalculateFCats 计算 E 站反向分类掩码
func CalculateFCats(activeCategories []string) int {
	if len(activeCategories) == 0 {
		return 0
	}
	selectedMask := 0
	for _, cat := range activeCategories {
		if bit, ok := categoryBitmaskMap[cat]; ok {
			selectedMask |= bit
		}
	}
	return 1023 ^ selectedMask
}

// 判断是否为有效的封面图片地址（过滤透明占位图）
func isValidCoverURL(u string) bool {
	if u == "" {
		return false
	}
	// 🟢 过滤 clear.png，防止在意外降级到 gdtm 时误判透明占位图
	if strings.Contains(u, "clear.ad.png") || 
	   strings.Contains(u, "clear.png") || 
	   strings.Contains(u, "blank.gif") {
		return false
	}
	return true
}

// 深度提取封面图逻辑
func extractCoverURL(s *goquery.Selection) string {
	var rawURL string

	// 1. 优先从节点内部带有 style="background:...url(...)" 的 div 或 img 中提取
	s.Find("div[style*='url'], img[style*='url']").EachWithBreak(func(i int, sel *goquery.Selection) bool {
		style, _ := sel.Attr("style")
		matches := urlRegex.FindStringSubmatch(style)
		if len(matches) > 1 && isValidCoverURL(matches[1]) {
			rawURL = matches[1]
			return false // 找到了，停止遍历
		}
		return true
	})

	if rawURL != "" {
		return rawURL
	}

	// 2. 若无 CSS 背景图，检查 <img> 的 data-src 或 src 属性
	s.Find("img").EachWithBreak(func(i int, sel *goquery.Selection) bool {
		if dataSrc, ok := sel.Attr("data-src"); ok && isValidCoverURL(dataSrc) {
			rawURL = dataSrc
			return false
		}
		if src, ok := sel.Attr("src"); ok && isValidCoverURL(src) {
			rawURL = src
			return false
		}
		return true
	})

	return rawURL
}

func parseRatingFromStyle(style string) float64 {
	if strings.Contains(style, "0px 0px") || strings.Contains(style, "0px -1") {
		return 5.0
	}
	if strings.Contains(style, "-16px") {
		return 4.0
	}
	if strings.Contains(style, "-32px") {
		return 3.0
	}
	if strings.Contains(style, "-48px") {
		return 2.0
	}
	if strings.Contains(style, "-64px") {
		return 1.0
	}
	return 0.0
}

func parseTotalPagesByCount(doc *goquery.Document) int {
	var targetText string
	doc.Find("p, div, span").EachWithBreak(func(i int, s *goquery.Selection) bool {
		txt := s.Text()
		if strings.Contains(txt, "results") || strings.Contains(txt, "Found") {
			targetText += " " + txt
		}
		return true
	})

	matches := resultCountRegex.FindStringSubmatch(targetText)
	if len(matches) > 1 {
		cleanNum := strings.ReplaceAll(matches[1], ",", "")
		if totalCount, err := strconv.Atoi(cleanNum); err == nil && totalCount > 0 {
			totalPages := (totalCount + 24) / 25
			log.Printf("[EH-DEBUG] 成功抓取到总结果数: %d 条 | 计算得出总页数: %d 页", totalCount, totalPages)
			return totalPages
		}
	}

	log.Printf("[EH-DEBUG] 未提取到总结果数文案，回退默认第 1 页")
	return 1
}

var (
	cssBgUrlRegex   = regexp.MustCompile(`url\(['"]?(.*?)['"]?\)`)
	// 🟢 兼容形如 "-100px 0", "-100px 0px", "-100px -130px" 的情况
	cssOffsetRegex  = regexp.MustCompile(`-(\d+)px\s+-?(\d+)(?:px)?`)
	cssWidthRegex   = regexp.MustCompile(`width:\s*(\d+)px`)
	cssHeightRegex  = regexp.MustCompile(`height:\s*(\d+)px`)
)

// parsePreviewTile 统一解析入口：自动识别 gdtl (大图) 与 gdtm (雪碧图)
func parsePreviewTile(s *goquery.Selection, index int, baseURL string) (PreviewPageDTO, bool) {
	dto := PreviewPageDTO{
		PageIndex: index,
		IsSprite:  false,
	}

	// 1. 优先提取 <img> 标签的 src (gdtl 大图模式)
	imgNode := s.Find("img")
	if imgNode.Length() > 0 {
		src, _ := imgNode.Attr("src")
		if dataSrc, ok := imgNode.Attr("data-src"); ok && dataSrc != "" {
			src = dataSrc
		}
		// 校验非透明占位图 (clear.png)
		if isValidCoverURL(src) {
			if strings.HasPrefix(src, "/") {
				src = baseURL + src
			}
			dto.ImageURL = "/api/v1/comics/cover-proxy?url=" + url.QueryEscape(src)
			return dto, true
		}
	}

	// 2. <img> 无效时，解析节点内联 style 属性 (gdtm 雪碧图模式)
	style, _ := s.Attr("style")
	if style == "" {
		style, _ = s.Find("div[style]").Attr("style")
	}

	if style != "" {
		if urlMatches := cssBgUrlRegex.FindStringSubmatch(style); len(urlMatches) > 1 {
			rawURL := urlMatches[1]
			if strings.HasPrefix(rawURL, "/") {
				rawURL = baseURL + rawURL
			}
			dto.ImageURL = "/api/v1/comics/cover-proxy?url=" + url.QueryEscape(rawURL)

			// 解析 X/Y 轴偏移量 (例如 style 中的 -200px 0)
			if offsetMatches := cssOffsetRegex.FindStringSubmatch(style); len(offsetMatches) > 1 {
				dto.IsSprite = true
				dto.OffsetX, _ = strconv.Atoi(offsetMatches[1])
				if len(offsetMatches) > 2 {
					dto.OffsetY, _ = strconv.Atoi(offsetMatches[2])
				}
			}

			// 解析单张预览图的剪裁宽高 (E 站默认小图通常为 100x130)
			dto.Width = 100
			dto.Height = 130
			if wMatches := cssWidthRegex.FindStringSubmatch(style); len(wMatches) > 1 {
				dto.Width, _ = strconv.Atoi(wMatches[1])
			}
			if hMatches := cssHeightRegex.FindStringSubmatch(style); len(hMatches) > 1 {
				dto.Height, _ = strconv.Atoi(hMatches[1])
			}

			return dto, true
		}
	}

	return dto, false
}