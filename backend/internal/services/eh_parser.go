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
	// Round11-Bug2：页数正则收紧 —— (?:pages?|P\b|页) 带单词边界，
	// 排除标题/标签中「数字 + 大写P开头单词」（如 "223 Piece"、"(959539 Piece)"）被误匹配；
	// (?i:pages?) 兼容 E 站 "pages"/"page"/"Pages" 大小写变体；P\b 仅匹配独立大写 P（如 "39P"）。
	pageCountRegex   = regexp.MustCompile(`(\d+)\s*(?:(?i:pages?)|P\b|页)`)
	dateRegex        = regexp.MustCompile(`\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}`)

	// listPageFullRegex 匹配列表页中「独立页数文案」节点（整串锚定）。
	// 与排行榜 toplistPageFullRegex 同思路：只认叶子节点整段文本，拒绝行级拼接。
	// (?i) 使 pages?/p\b 兼容 "Pages"/"P"/"p" 变体；「页」匹配中文单位。
	listPageFullRegex = regexp.MustCompile(`(?i)^\s*(\d+)\s*(?:pages?|p\b|页)\s*$`)
)

// extractListPageCount 从画廊行节点提取页数（Round11-Bug3 修复）。
//
// ⚠️ 旧实现直接对整行 s.Text() 跑 pageCountRegex：行文本是各子节点文本的拼接，
// 会把上传者名字末尾的数字与紧随其后的 "N pages" 接成更大的数，
// 实测 gid=4136008（上传者 "Nid135" + "29 pages" → "Nid13529 pages"）被误取为 13529。
// 正确做法：只扫描行内「叶子级」小节点（自身不嵌套 div/a/p/span/table/ul），
// 整段文本形如 "NN pages/page/P/页" 才采纳，取文档序第一个命中
// （页数节点位于 .gl3e 容器，先于 .gl4e 的标题/标签，不会被标签文本抢跑）。
func extractListPageCount(s *goquery.Selection) int {
	var result int
	s.Find("div, span, td").EachWithBreak(func(_ int, n *goquery.Selection) bool {
		// 跳过聚合节点（如 .gl3e 容器，其 Text() 仍会拼接子节点文本）
		if n.Find("div, a, p, span, table, ul").Length() > 0 {
			return true
		}
		txt := strings.TrimSpace(n.Text())
		if txt == "" {
			return true
		}
		if m := listPageFullRegex.FindStringSubmatch(txt); len(m) > 1 {
			if v, err := strconv.Atoi(m[1]); err == nil && v > 0 && v <= 100000 {
				result = v
				return false
			}
		}
		return true
	})
	return result
}

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

// parseRatingFromStyle 解析 E 站列表卡片评分雪碧图 background-position（Round11-Bug1）。
//
// E 站 .ir 评分雪碧与排行榜同构（X 横向整星 + Y 行偏移半星），统一委托
// parseToplistRatingFromStyle 处理：
//   - 满星行（Y=-1px）：0px→5.0, -16px→4.0, -32px→3.0, -48px→2.0, -64px→1.0
//   - 半星行（Y=-21px，E 站列表大量使用）：0px→4.5, -16px→3.5, -32px→2.5, -48px→1.5, -64px→0.5
// 旧实现只按 X 偏移判整星，导致 0px -21px（4.5 星）解析为 0（卡片显示 ⭐ —）、
// -16px -21px（3.5 星）误判为 4.0。修复后评分精度与 E 站一致。
func parseRatingFromStyle(style string) float64 {
	return parseToplistRatingFromStyle(style)
}

// toplistRatingPosRegex 匹配排行榜评分雪碧图 background-position 的 X/Y 偏移
// （形如 "background-position:0px -21px"）
var toplistRatingPosRegex = regexp.MustCompile(`background-position:\s*(-?\d+(?:\.\d+)?)px\s+(-?\d+(?:\.\d+)?)px`)

// parseToplistRatingFromStyle 解析 E 站排行榜评分雪碧图（横向布局）。
//
// E 站 .ir 评分雪碧：background-position:Xpx Ypx
//   X（整星横向位置）：0px→5.0, -16px→4.0, -32px→3.0, -48px→2.0, -64px→1.0
//     即每向左移 16px 降 1 星（base = 5.0 + x/16.0）
//   Y（行偏移）：-1px=满星行, -21px=半星行（-0.5）
// 实测样本：0px -1px→5.0, 0px -21px→4.5, -32px -21px→2.5, -48px -1px→2.0
func parseToplistRatingFromStyle(style string) float64 {
	m := toplistRatingPosRegex.FindStringSubmatch(style)
	if len(m) < 3 {
		return 0.0
	}
	x, _ := strconv.ParseFloat(m[1], 64)
	y, _ := strconv.ParseFloat(m[2], 64)

	base := 5.0 + x/16.0
	half := 0.0
	if y <= -20 {
		half = 0.5
	}
	r := base - half
	if r < 0 {
		return 0.0
	}
	if r > 5 {
		return 5.0
	}
	return r
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

// parseTotalPagesFromPager 从列表页底部翻页器提取总页数（问题2 根因修复）。
//
// E 站翻页器 (.ptt) 的每个页码链接形如 "?p=N"（N 为 0 基页索引），
// 取其中最大的 N+1 即为总页数。首页（无关键词）不显示 "N results" 文案，
// 此时这是唯一可靠的总页数来源——缺了它随机翻页永远落回第 1 页。
func parseTotalPagesFromPager(doc *goquery.Document) int {
	maxP := -1
	doc.Find(".ptt a[href], table.ptt a[href]").Each(func(i int, s *goquery.Selection) {
		href, _ := s.Attr("href")
		u, err := url.Parse(href)
		if err != nil {
			return
		}
		p := u.Query().Get("p")
		if p == "" {
			return
		}
		if n, err := strconv.Atoi(p); err == nil && n > maxP {
			maxP = n
		}
	})
	if maxP >= 0 {
		total := maxP + 1
		log.Printf("[EH-DEBUG] 从底部翻页器提取总页数: %d 页", total)
		return total
	}
	return 0
}

// parseTotalPages 综合判定总页数：优先翻页器（对首页/无结果文案页可靠），
// 其次 "N results" 文案，最后回退默认第 1 页。
func parseTotalPages(doc *goquery.Document) int {
	if n := parseTotalPagesFromPager(doc); n > 0 {
		return n
	}
	return parseTotalPagesByCount(doc) // 未提取到文案时其内部回退 1
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