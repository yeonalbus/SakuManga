package services

import (
	"log"
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
	if strings.Contains(u, "clear.ad.png") || strings.Contains(u, "blank.gif") {
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

// 用于匹配 style="background: url('https://...') ..." 中的图片链接
var cssURLRegex = regexp.MustCompile(`url\(['"]?(.*?)['"]?\)`)

// extractPreviewImage 兼容提取：大图优先抓 img src，小图兜底抓 style url()
func extractPreviewImage(s *goquery.Selection) string {
	// 1. 优先抓取 <img> 的 src 或 data-src (大图 gdtl 模式)
	if img := s.Find("img"); img.Length() > 0 {
		if src, ok := img.Attr("src"); ok && isValidCoverURL(src) {
			return src
		}
		if dataSrc, ok := img.Attr("data-src"); ok && isValidCoverURL(dataSrc) {
			return dataSrc
		}
	}

	// 2. 若当前节点本身就是 img
	if s.Is("img") {
		if src, ok := s.Attr("src"); ok && isValidCoverURL(src) {
			return src
		}
	}

	// 3. 兜底提取 inline style 中的 background: url(...)
	style, exists := s.Attr("style")
	if !exists {
		style, exists = s.Find("div[style]").Attr("style")
	}
	if exists {
		if matches := cssURLRegex.FindStringSubmatch(style); len(matches) > 1 {
			return matches[1]
		}
	}

	return ""
}