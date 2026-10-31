package services

import (
	"encoding/json"
	"log"
	"regexp"
	"strconv"
	"strings"
)

// ─────────────────────────────────────────────────────────────
// Round49：书签检索上下文解析（从已剪枝的失效检测文件搬出）
//
// 失效检测（Round35 的书签级可见性扫描）已按方案剪枝，但以下工具仍被
// 更新探测（scrape_bookmark_probe.go）使用：
//   - parseAnchorJSON：读锚定画廊（gid / postedAt）
//   - parseBookmarkConfig / toSearchParams：复刻书签创建时的检索条件
//     （与前端 buildOnlineSearchParams 对齐，保证探测与书签跳转同一上下文）
// ─────────────────────────────────────────────────────────────

// bookmarkAnchorJSON 锚点 JSON（与前端 ScrapeBookmark['anchor'] 对齐）
type bookmarkAnchorJSON struct {
	GID      string `json:"gid"`
	Token    string `json:"token,omitempty"`
	Title    string `json:"title,omitempty"`
	PostedAt string `json:"postedAt,omitempty"`
}

// parseAnchorJSON 解析锚点（null / 空 / 非法一律返回 nil）
func parseAnchorJSON(raw string) (*bookmarkAnchorJSON, error) {
	s := strings.TrimSpace(raw)
	if s == "" || s == "null" {
		return nil, nil
	}
	var a bookmarkAnchorJSON
	if err := json.Unmarshal([]byte(s), &a); err != nil {
		log.Printf("[BOOKMARK] 锚点 JSON 解析失败: %v", err)
		return nil, err
	}
	if a.GID == "" {
		return nil, nil
	}
	return &a, nil
}

// bookmarkSearchConfig 书签 config 快照（与前端 SearchConfig 对齐，只取在线检索相关字段）
type bookmarkSearchConfig struct {
	Keyword               string   `json:"keyword"`
	Keywords              []string `json:"keywords"`
	ActiveCategories      []string `json:"activeCategories"`
	MinRating             float64  `json:"minRating"`
	Language              string   `json:"language"`
	OnlyRemoved           bool     `json:"onlyRemoved"`
	OnlyTorrents          bool     `json:"onlyTorrents"`
	DisableLangFilter     bool     `json:"disableLangFilter"`
	DisableUploaderFilter bool     `json:"disableUploaderFilter"`
	DisableTagFilter      bool     `json:"disableTagFilter"`
}

// parseBookmarkConfig 解析 config JSON（坏 JSON 退化为空筛选条件，不拒绝整条书签）
func parseBookmarkConfig(raw string) bookmarkSearchConfig {
	var cfg bookmarkSearchConfig
	s := strings.TrimSpace(raw)
	if s == "" || s == "null" {
		return cfg
	}
	if err := json.Unmarshal([]byte(s), &cfg); err != nil {
		log.Printf("[BOOKMARK] config JSON 解析失败，按空筛选条件处理: %v", err)
	}
	return cfg
}

// positiveKeywordTokens 复刻前端 parseKeywordQueue 的「正向项」拆分：
// `- ` 前缀为负向排除（E 站 f_search 不支持排除语法，前端本就只本地剔除，不下发）。
func positiveKeywordTokens(items []string) []string {
	out := make([]string, 0, len(items))
	for _, raw := range items {
		item := strings.TrimSpace(raw)
		if item == "" || strings.HasPrefix(item, "-") {
			continue
		}
		out = append(out, item)
	}
	return out
}

// toSearchParams 复刻前端 buildOnlineSearchParams：保证探测与书签跳转使用同一检索上下文。
func (c bookmarkSearchConfig) toSearchParams() SearchParams {
	tokens := make([]string, 0, len(c.Keywords)+1)
	if kw := strings.TrimSpace(c.Keyword); kw != "" {
		tokens = append(tokens, positiveKeywordTokens([]string{kw})...)
	}
	tokens = append(tokens, positiveKeywordTokens(c.Keywords)...)

	minRating := ""
	if c.MinRating > 0 {
		// 与前端 String(4) → "4" / String(4.5) → "4.5" 等价
		minRating = strconv.FormatFloat(c.MinRating, 'f', -1, 64)
	}
	return SearchParams{
		Keyword:               strings.Join(tokens, " "),
		ActiveCategories:      c.ActiveCategories,
		MinRating:             minRating,
		Language:              c.Language,
		OnlyRemoved:           c.OnlyRemoved,
		OnlyTorrents:          c.OnlyTorrents,
		DisableLangFilter:     c.DisableLangFilter,
		DisableUploaderFilter: c.DisableUploaderFilter,
		DisableTagFilter:      c.DisableTagFilter,
	}
}

// seekDateRegex 从锚定发布时间取 seek 日期（E 站 seek 只接受日粒度）
var seekDateRegex = regexp.MustCompile(`^\s*(\d{4}-\d{2}-\d{2})`)

// seekDateOf 取 "2026-09-13 05:09" → "2026-09-13"；无法解析返回空串
func seekDateOf(postedAt string) string {
	if m := seekDateRegex.FindStringSubmatch(postedAt); len(m) > 1 {
		return m[1]
	}
	return ""
}
