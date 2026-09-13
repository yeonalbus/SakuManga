package services

import (
	"encoding/json"
	"log"
	"regexp"
	"strconv"
	"strings"

	"SakuManga/internal/models"

	"gorm.io/gorm"
)

// ─────────────────────────────────────────────────────────────
// 搜刮书签失效判定（Round35 重写：语义 = 书签级可见性）
//
// 判定语义（与用户对齐的最终口径）：
//   书签是「位置快照」；只要在**书签自带的搜索&筛选条件**下仍能看到锚定画廊，
//   锚定就成立（列表位置漂移不算失效）；一旦在该条件下看不到，书签即失效——
//   与画廊自身状态无关：Expunged（已移除）画廊页面仍 200、内容仍可访问，
//   甚至在「仅搜索移除了的画廊」下可见，但在书签原条件下不可见 → 仍判失效。
//
// 因此判定必须**按书签 config 复刻一次真实检索**（与前端定位循环同一判据）：
//   1. 首屏 seek 到锚定发布时间所在日期（E 站 seek 只吃日粒度），此后用 next 游标向下翻页；
//   2. 每页先查锚定 gid：命中 → 有效（位置漂移无关紧要）；
//   3. 未命中且本页最旧一条发布时间 ≤ 锚定时间 → 后面只会更旧 → 该条件下不存在 → 失效；
//   4. 列表到底（无 next 游标）仍未出现 → 失效；
//   5. 扫描超过硬上限 / 请求失败 / 缺少锚定时间 → 未判定（不写失效标记，避免误杀）。
//
// 与旧实现（Round33）的区别：旧实现用「详情页 HTTP 200 + 标题非空」判有效，
// 判的其实是画廊可访问性，与书签语义无关 —— Expunged 画廊详情页完全正常，
// 于是「首页看不到 + 检测说有效」的误报必然发生（本次 bug 根因）。
// ─────────────────────────────────────────────────────────────

// 书签判定状态
const (
	BookmarkStatusOK          = "ok"          // 原检索条件下仍可见 → 书签有效
	BookmarkStatusUnreachable = "unreachable" // 原检索条件下已不可见 → 书签失效
	BookmarkStatusError       = "error"       // 未判定（请求失败 / 信息不足 / 超硬上限）
)

// 历史状态常量（Round33 旧实现产出；新判定不再产出，保留供前端与历史数据兼容）
const (
	BookmarkStatusRemoved   = "removed"
	BookmarkStatusCopyright = "copyright"
	BookmarkStatusInvalid   = "invalid"
	BookmarkStatusReplaced  = "replaced"
)

// bookmarkScanHardCapPages 扫描硬上限（仅防异常：next 游标循环、列表时间戳异常）。
// 正常终止条件是「本页最旧一条已早于锚定时间」，与当日画廊密度无关：
// 越密集的日子多翻几页，稀疏的日子可能首屏即停。
const bookmarkScanHardCapPages = 100

// BookmarkAnchorInfo 锚点信息（探测结果中回传的刷新后元信息）
type BookmarkAnchorInfo struct {
	GID      string `json:"gid"`
	Token    string `json:"token,omitempty"`
	Title    string `json:"title,omitempty"`
	PostedAt string `json:"postedAt,omitempty"`
}

// BookmarkCheckResult 单条书签判定结果
type BookmarkCheckResult struct {
	ID         uint                `json:"id"`
	Status     string              `json:"status"`
	Message    string              `json:"message,omitempty"`
	NewVersion *BookmarkAnchorInfo `json:"newVersion,omitempty"` // 历史字段（Round33 replaced 用；新判定不再产出）
	Refreshed  *BookmarkAnchorInfo `json:"refreshed,omitempty"`  // ok：命中的列表条目元信息（刷新标题/时间）
}

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
		log.Printf("[BOOKMARK] config JSON 解析失败，按空筛选条件判定: %v", err)
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

// toSearchParams 复刻前端 buildOnlineSearchParams：保证检测与「书签定位循环」使用同一检索上下文。
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

// bookmarkPageFetcher 单页列表抓取（抽出便于单测注入假数据）
type bookmarkPageFetcher func(params SearchParams) (*OnlineComicResult, error)

// scanBookmarkVisibility 在书签原始检索条件下扫描锚定 gid 是否可见。
// 返回 (status, message, refreshed)。
func scanBookmarkVisibility(
	fetch bookmarkPageFetcher,
	cfg bookmarkSearchConfig,
	gid string,
	postedAt string,
) (string, string, *BookmarkAnchorInfo) {
	target := parseGalleryAddedAt(postedAt)
	seekDate := seekDateOf(postedAt)
	if target == nil || seekDate == "" {
		return BookmarkStatusError, "锚定发布时间无法解析，未判定", nil
	}

	next := ""
	for page := 1; page <= bookmarkScanHardCapPages; page++ {
		params := cfg.toSearchParams()
		if page == 1 {
			params.Seek = seekDate // 从锚定当天末尾开始向下扫
		} else {
			params.Next = next
		}

		res, err := fetch(params)
		if err != nil {
			// 网络 / 限流 / 会话失效：不判定失效，交用户重试
			return BookmarkStatusError, "检索失败：" + err.Error(), nil
		}
		if res == nil || len(res.Comics) == 0 {
			// 该条件下检索结果为空 → 锚定画廊必然不可见
			return BookmarkStatusUnreachable, "在当前搜索&筛选条件下已检索不到该画廊（结果为空）", nil
		}

		// ① 先查命中（锚定画廊可能就是本页最旧一条，先判越界会漏）
		for _, c := range res.Comics {
			if c.ID == gid {
				return BookmarkStatusOK, "", &BookmarkAnchorInfo{
					GID:      c.ID,
					Token:    c.Token,
					Title:    c.Title,
					PostedAt: c.UpdatedAt,
				}
			}
		}

		// ② 越过判定：本页最旧一条已不晚于锚定时间 → 后续只会更旧 → 该条件下不存在
		if oldest := parseGalleryAddedAt(res.Comics[len(res.Comics)-1].UpdatedAt); oldest != nil && !oldest.After(*target) {
			return BookmarkStatusUnreachable, "在当前搜索&筛选条件下已看不到该画廊（本页最旧一条已早于锚定时间）", nil
		}

		// ③ 列表到底（无 next 游标）仍未出现 → 该条件下不存在
		if res.Next == "" {
			return BookmarkStatusUnreachable, "在当前搜索&筛选条件下已看不到该画廊（列表已到尽头）", nil
		}

		next = res.Next
	}

	// 硬上限兜底：既未命中也没越过时间点（next 游标异常 / 时间戳解析异常）→ 未判定
	return BookmarkStatusError,
		"扫描超过 " + strconv.Itoa(bookmarkScanHardCapPages) + " 页仍未越过锚定时间点，未判定",
		nil
}

// CheckScrapeBookmarks 批量判定书签有效性。
// ids 为空表示判定当前用户全部「有锚点」的书签；逐条串行（复用 E 站自适应限流）。
func CheckScrapeBookmarks(
	db *gorm.DB,
	eh *EHService,
	userID uint,
	account *models.AccountSetting,
	setting *models.EHSetting,
	ids []uint,
) ([]BookmarkCheckResult, error) {
	q := db.Where("user_id = ?", userID)
	if len(ids) > 0 {
		q = q.Where("id IN ?", ids)
	}
	var marks []models.ScrapeBookmark
	if err := q.Order("id ASC").Find(&marks).Error; err != nil {
		return nil, err
	}

	results := make([]BookmarkCheckResult, 0, len(marks))
	for i := range marks {
		bm := marks[i]
		res := BookmarkCheckResult{ID: bm.ID}

		// 无锚点 / 锚点无效：未锚定书签不参与失效判定
		anchor, err := parseAnchorJSON(bm.Anchor)
		if err != nil || anchor == nil {
			continue
		}

		// 老书签无锚定发布时间：信息不全不做适配（无 postedAt 就无法 seek 到「当天」，
		// 也就无法界定扫描范围）→ 未判定，不写失效标记
		if strings.TrimSpace(anchor.PostedAt) == "" {
			res.Status = BookmarkStatusError
			res.Message = "书签缺少锚定发布时间，未判定（老书签不参与失效判定）"
			results = append(results, res)
			continue
		}

		cfg := parseBookmarkConfig(bm.Config)
		fetch := func(p SearchParams) (*OnlineComicResult, error) {
			ehRateLimiter.Wait()
			r, ferr := eh.FetchGalleryList(account, p, setting)
			ehRateLimiter.Mark(ferr == nil)
			return r, ferr
		}

		status, msg, refreshed := scanBookmarkVisibility(fetch, cfg, anchor.GID, anchor.PostedAt)
		res.Status = status
		res.Message = msg
		res.Refreshed = refreshed
		results = append(results, res)

		log.Printf("[BOOKMARK] 判定书签 [%d] gid=%s status=%s %s", bm.ID, anchor.GID, status, msg)
	}
	return results, nil
}
