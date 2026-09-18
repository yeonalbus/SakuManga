package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"SakuManga/internal/middleware"
	"SakuManga/internal/models"
	"SakuManga/internal/services"
)

// RandomComicItem 随机抽卡统一返回项（在线/离线混合 DTO）
type RandomComicItem struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	CoverURL     string   `json:"coverUrl"`
	Source       string   `json:"source"`
	Category     string   `json:"category,omitempty"`
	Rating       float64  `json:"rating,omitempty"`
	Tags         []string `json:"tags,omitempty"`
	PageCount    int      `json:"pageCount,omitempty"`
	ReadCount    int      `json:"readCount,omitempty"`
	UpdatedAt    string   `json:"updatedAt"`
	IsDownloaded bool     `json:"isDownloaded"`
	Token        string   `json:"token,omitempty"`
	Uploader     string   `json:"uploader,omitempty"`
	IsFavorite   bool     `json:"isFavorite"`
	LocalPath    string   `json:"localPath,omitempty"`
	FileSize     int64    `json:"fileSize,omitempty"`
	HasError     bool     `json:"hasError,omitempty"`

	// ─── Round32 阶段二：偏好推荐（仅 mode=recommend 的离线结果填充）───
	MatchedTags []string `json:"matchedTags,omitempty"` // 命中理由（贡献最高的前 3 个 tag）
	Score       float64  `json:"score,omitempty"`       // 推荐得分（调试/说明用）
}

// fromOnlineDTO 在线 DTO → 抽卡统一项
func fromOnlineDTO(c services.OnlineComicDTO) RandomComicItem {
	return RandomComicItem{
		ID:           c.ID,
		Title:        c.Title,
		CoverURL:     c.CoverURL,
		Source:       c.Source,
		Category:     c.Category,
		Rating:       c.Rating,
		Tags:         c.Tags,
		PageCount:    c.PageCount,
		UpdatedAt:    c.UpdatedAt,
		IsDownloaded: c.IsDownloaded,
		Token:        c.Token,
		Uploader:     c.Uploader,
		IsFavorite:   c.IsFavorite,
	}
}

// offlineDisplayTags 离线漫画的展示/匹配 tag：双轨三态合并 (online ∪ add) − remove，
// 三态全空时回退旧 Tags 字段（与 GetOfflineComics / GetOfflineComicDetail 口径一致）。
//
// Round32 一致性修复：随机抽卡此前直接使用旧 Tags 字段，与列表/详情的合并口径不一致
// （实测隔离库 3418 本中约 42% 两字段不同），会导致卡片标签与推荐命中理由对不上，
// 也会让负向排除漏掉「本地新增/已删除」的 tag。
func offlineDisplayTags(c *models.OfflineComic) []string {
	online := services.UnmarshalTagSlice(c.OnlineTags)
	add := services.UnmarshalTagSlice(c.OfflineAddTags)
	remove := services.UnmarshalTagSlice(c.OfflineRemoveTags)
	merged := services.MergeTags(online, add, remove)
	if len(merged) == 0 && strings.TrimSpace(c.OnlineTags) == "" {
		return parseRawTags(c.Tags)
	}
	return merged
}

// fromOfflineModel 离线模型 → 抽卡统一项
func fromOfflineModel(c models.OfflineComic) RandomComicItem {
	return RandomComicItem{
		ID:           c.ID,
		Title:        c.Title,
		CoverURL:     c.CoverURL,
		Source:       string(c.Source),
		Category:     c.Category,
		Rating:       c.Rating,
		Tags:         offlineDisplayTags(&c),
		PageCount:    c.PageCount,
		ReadCount:    c.ReadCount,
		UpdatedAt:    c.UpdatedAt.Format(time.RFC3339),
		IsDownloaded: c.IsDownloaded,
		LocalPath:    c.LocalPath,
		FileSize:     c.FileSize,
		HasError:     c.NeedsUpdate,
	}
}

// trimKeywords 过滤掉多关键词队列中的空串与纯空白项。
func trimKeywords(kws []string) []string {
	out := make([]string, 0, len(kws))
	for _, k := range kws {
		if t := strings.TrimSpace(k); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// parseFloatOr 解析浮点参数，缺失/非法时回退默认值
func parseFloatOr(raw string, def float64) float64 {
	if strings.TrimSpace(raw) == "" {
		return def
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return def
	}
	return v
}

// ─────────────────────────────────────────────────────────────
// 离线候选硬约束（随机与推荐共用）
//
// Round32：把原先内联在随机闭包里的 where 构造抽出，
// 保证「纯随机」与「偏好推荐」的筛选语义完全一致（推荐只替换采样方式，不改变卡池边界）。
// ─────────────────────────────────────────────────────────────

// offlineFilter 离线抽卡/推荐的公共硬约束
type offlineFilter struct {
	keyword         string
	keywords        []string
	excludeTags     []string
	excludeKeywords []string
	categories      []string
	minRating       float64
	minPages        int
	maxPages        int
	language        string
	onlyDownloaded  bool

	// ─── 推荐专属 ───
	excludeRead   bool     // 排除读过的（read_count > 0）
	shelfComicIDs []string // 排除已在书架的本子（用户维度，由调用方解析后传入）
}

// buildOfflineQuery 按硬约束构造离线候选查询（不含排序与分页）
func buildOfflineQuery(db *gorm.DB, f offlineFilter) *gorm.DB {
	q := db.Model(&models.OfflineComic{})

	if kw := strings.TrimSpace(f.keyword); kw != "" {
		like := "%" + services.EscapeLike(kw) + "%"
		// Round41-Bug：标题池 = 主标题(title) + 日文原名(title_jpn)。metadata 入库时 title 常为
		// 罗马音、title_jpn 才是日文原名，而前端卡片主标题展示的是 titleJpn（displayTitle =
		// titleJpn || title）——只匹配 title 会让「按看得见的标题」抽不到卡。
		// 与 OfflineHome.vue 的 comicTitleTexts 保持同语义。
		q = q.Where("title LIKE ? OR title_jpn LIKE ? OR tags LIKE ?", like, like, like)
	}
	// 问题1：多关键词队列按 AND 语义匹配（须全部命中标题或标签，与 OfflineHome 一致）
	// Round20-Bug3：tag 形关键词（含命名空间，如 female:"magical girl$" / female:yuri$）按
	// E 站语义匹配 tags JSON 元素（$ 精确、无 $ 前缀）；裸词维持标题/标签子串。
	for _, raw := range f.keywords {
		kw := strings.TrimSpace(raw)
		if kw == "" {
			continue
		}
		tag := services.ParseFSearchTag(kw)
		if tag.IsTag {
			// Round41-Bug：标题兜底同样覆盖日文原名（title_jpn），与前端标题池一致
			like := "%" + services.EscapeLike(kw) + "%"
			orParts := []string{"title LIKE ? ESCAPE '\\'", "title_jpn LIKE ? ESCAPE '\\'"}
			orArgs := []interface{}{like, like}
			for _, p := range tag.TagJSONMatchPatterns() {
				orParts = append(orParts, "tags LIKE ? ESCAPE '\\'")
				orArgs = append(orArgs, p)
			}
			q = q.Where("("+strings.Join(orParts, " OR ")+")", orArgs...)
			continue
		}
		like := "%" + services.EscapeLike(kw) + "%"
		q = q.Where("(title LIKE ? OR title_jpn LIKE ? OR tags LIKE ?)", like, like, like)
	}
	// Round3-任务6：离线随机负向排除（与前端 matchExcludes 语义一致）
	for _, raw := range f.excludeTags {
		tag := strings.TrimSpace(raw)
		if tag == "" {
			continue
		}
		// Round20-Bug3：负向 tag 支持 f_search 语法（引号/锚点/下划线归一）
		pt := services.ParseFSearchTag(tag)
		if pt.IsTag {
			notParts := make([]string, 0, 2)
			notArgs := make([]interface{}, 0, 2)
			for _, p := range pt.TagJSONMatchPatterns() {
				notParts = append(notParts, "tags NOT LIKE ? ESCAPE '\\'")
				notArgs = append(notArgs, p)
			}
			q = q.Where("("+strings.Join(notParts, " AND ")+")", notArgs...)
			continue
		}
		// 离线 tags 为 JSON 字符串数组（namespace:key），负向 tag 对整条目精确匹配
		q = q.Where("tags NOT LIKE ?", "%\""+services.EscapeLike(tag)+"\"%")
	}
	for _, raw := range f.excludeKeywords {
		kw := strings.TrimSpace(raw)
		if kw == "" {
			continue
		}
		like := "%" + services.EscapeLike(kw) + "%"
		// Round41-Bug：负向关键词的文本池同样含日文原名（与前端 matchExcludes 的
		// collectSearchTexts 一致），否则 titleJpn 命中负向词的条目会被前端兜底剔除却不补位。
		// title_jpn 可空：NULL 时 NOT LIKE 结果为 NULL 会误排整行，故显式 IS NULL 兜底。
		q = q.Where(
			"(title NOT LIKE ? AND tags NOT LIKE ? AND (title_jpn IS NULL OR title_jpn NOT LIKE ?))",
			like, like, like,
		)
	}
	if f.minRating > 0 {
		q = q.Where("rating >= ?", f.minRating)
	}
	if f.minPages > 0 {
		q = q.Where("page_count >= ?", f.minPages)
	}
	if f.maxPages > 0 {
		q = q.Where("page_count <= ?", f.maxPages)
	}
	// 问题6：离线随机继承全局筛选
	if len(f.categories) > 0 {
		q = q.Where("category IN ?", f.categories)
	}
	if lang := strings.TrimSpace(f.language); lang != "" && lang != "All" {
		// 离线 tags 为 JSON 字符串数组，语言以 "language:xx" 形式存储
		langTag := "language:" + strings.ToLower(lang)
		q = q.Where("tags LIKE ?", "%\""+langTag+"\"%")
	}
	if f.onlyDownloaded {
		q = q.Where("is_downloaded = ?", true)
	}
	// Round32 推荐专属：排除读过的 / 已在书架的
	if f.excludeRead {
		q = q.Where("read_count <= 0")
	}
	if len(f.shelfComicIDs) > 0 {
		q = q.Where("id NOT IN ?", f.shelfComicIDs)
	}
	return q
}

// GetRandomComics 随机抽卡接口
//
// 查询参数:
//   - count:       抽卡数量（默认 8，上限 50）
//   - source:      范围 all | online | offline（默认 all）
//   - mode:        卡池模式 random | recommend（Round32；默认 random，行为与旧版一致）
//   - keyword:     搜索关键词（在线走 f_search，离线匹配标题（含日文原名 title_jpn）/标签）
//   - keywords:    筛选抽屉的多关键词队列（在线与 keyword 合并进 f_search；离线须全部命中标题（含日文原名）/标签）
//   - excludeTags: 负向 tag（namespace:key 精确匹配，在线采样池丢弃+补位/离线 SQL 排除，多次传递）
//   - excludeKeywords: 负向关键词（标题/标签/上传者子串匹配，在线采样池丢弃+补位/离线 SQL 排除，多次传递）
//   - categories:  分类过滤（在线/离线均生效，多次传递）
//   - minRating:   最低评分（仅离线生效）
//   - minPages:    最少页数（仅离线生效）
//   - maxPages:    最多页数（仅离线生效）
//   - language:    语言过滤（仅离线生效，All|Chinese|Japanese|English）
//   - onlyDownloaded: 仅已下载（仅离线生效）
//
// 推荐模式专属（mode=recommend，仅作用于本地库；在线部分仍为纯随机，决策 D1）:
//   - recoTheta:     偏好侧重（0=纯库藏，1=纯阅读，默认 0.6）
//   - recoExplore:   探索率 ε（默认 0.15）
//   - recoTemp:      采样温度 T（默认 0.5）
//   - recoExcludeRead:  排除读过的（read_count > 0）
//   - recoExcludeShelf: 排除已在书架的本子
func (h *OnlineComicHandler) GetRandomComics(c *gin.Context) {
	account := middleware.CurrentAccount(c)
	user := middleware.CurrentUser(c)

	// 1. 解析通用参数
	count := 8
	if v := c.Query("count"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			if n > 50 {
				n = 50
			}
			count = n
		}
	}

	source := c.DefaultQuery("source", "all")
	if source != "online" && source != "offline" {
		source = "all"
	}

	// Round32：卡池模式（默认 random = 旧行为；recommend 只改变本地库采样方式）
	mode := c.DefaultQuery("mode", "random")
	if mode != "recommend" {
		mode = "random"
	}
	recoOpts := services.NormalizeRecommendOptions(services.RecommendOptions{
		Theta:       parseFloatOr(c.Query("recoTheta"), services.RecoDefaultTheta),
		Explore:     parseFloatOr(c.Query("recoExplore"), services.RecoDefaultExplore),
		Temperature: parseFloatOr(c.Query("recoTemp"), services.RecoDefaultTemperature),
		Gamma:       parseFloatOr(c.Query("recoGamma"), services.RecoDefaultGamma),
	})
	recoExcludeRead := c.DefaultQuery("recoExcludeRead", "false") == "true"
	recoExcludeShelf := c.DefaultQuery("recoExcludeShelf", "false") == "true"

	keyword := c.Query("keyword")
	keywords := c.QueryArray("keywords") // 问题1：筛选抽屉的多关键词队列
	// Round3-任务6：负向排除（- 前缀解析后的 excludeTags / excludeKeywords，前后端语义一致）
	excludeTags := c.QueryArray("excludeTags")
	excludeKeywords := c.QueryArray("excludeKeywords")
	activeCategories := c.QueryArray("categories")
	minRating, _ := strconv.ParseFloat(c.DefaultQuery("minRating", "0"), 64)
	minPages, _ := strconv.Atoi(c.DefaultQuery("minPages", "0"))
	maxPages, _ := strconv.Atoi(c.DefaultQuery("maxPages", "0"))
	language := c.DefaultQuery("language", "All")
	onlyDownloaded := c.DefaultQuery("onlyDownloaded", "false") == "true"

	// 抽卡专用过滤器：在线高级筛选（E-Hentai f_* 参数，仅在线/全库生效）
	onlyRemoved := c.DefaultQuery("onlyRemoved", "false") == "true"
	onlyTorrents := c.DefaultQuery("onlyTorrents", "false") == "true"
	disableLangFilter := c.DefaultQuery("disableLangFilter", "false") == "true"
	disableUploaderFilter := c.DefaultQuery("disableUploaderFilter", "false") == "true"
	disableTagFilter := c.DefaultQuery("disableTagFilter", "false") == "true"

	// 离线硬约束（随机/推荐共用同一卡池边界）
	filter := offlineFilter{
		keyword:         keyword,
		keywords:        keywords,
		excludeTags:     excludeTags,
		excludeKeywords: excludeKeywords,
		categories:      activeCategories,
		minRating:       minRating,
		minPages:        minPages,
		maxPages:        maxPages,
		language:        language,
		onlyDownloaded:  onlyDownloaded,
		excludeRead:     recoExcludeRead,
	}
	if recoExcludeShelf && user != nil {
		filter.shelfComicIDs = loadUserShelfComicIDs(h.db, user.ID)
	}

	// 2. 离线随机：SQL ORDER BY RANDOM() 全库随机
	randomOffline := func(limit int) []RandomComicItem {
		var rows []models.OfflineComic
		if err := buildOfflineQuery(h.db, filter).Order("RANDOM()").Limit(limit).Find(&rows).Error; err != nil {
			return []RandomComicItem{}
		}
		items := make([]RandomComicItem, 0, len(rows))
		for _, r := range rows {
			items = append(items, fromOfflineModel(r))
		}
		return items
	}

	// 2.1 离线偏好推荐（Round32 阶段二）
	//     返回 (结果, 降级说明)：任何异常/数据不足都退回纯随机，不阻断抽卡。
	recommendOffline := func(limit int) ([]RandomComicItem, string) {
		fallback := func(reason string) ([]RandomComicItem, string) {
			return randomOffline(limit), reason
		}
		if user == nil {
			return fallback("偏好推荐需要登录后使用，已退回纯随机")
		}
		if services.GlobalXpCloud == nil {
			return fallback("偏好统计未就绪，已退回纯随机")
		}

		var candidates []models.OfflineComic
		// 上限保护：本地库规模通常在数千本，2 万本以上只取一部分做打分（避免极端库卡顿）
		if err := buildOfflineQuery(h.db, filter).Limit(20000).Find(&candidates).Error; err != nil {
			return fallback("推荐候选读取失败，已退回纯随机")
		}

		reco := services.NewRecommendService(services.GlobalXpCloud)
		ranked, err := reco.Recommend(user.ID, candidates, limit, recoOpts)
		if err != nil {
			if err == services.ErrRecommendNoWeights {
				return fallback("本地库暂无可用的偏好数据，已退回纯随机")
			}
			return fallback("推荐计算失败，已退回纯随机")
		}

		items := make([]RandomComicItem, 0, len(ranked))
		for _, r := range ranked {
			item := fromOfflineModel(r.Comic)
			item.MatchedTags = r.MatchedTags
			item.Score = r.Score
			items = append(items, item)
		}
		return items, ""
	}

	// 3. 在线随机：抓随机页 + 洗牌采样（推荐模式不作用于在线，决策 D1）
	randomOnline := func(limit int) ([]RandomComicItem, error) {
		if account == nil || account.IPBMemberID == "" {
			return nil, fmt.Errorf("请先绑定并保存 E 站账户凭证")
		}
		ehSetting := getEHSetting(h.db, account.ID)
		// 问题1：在线把顶栏主词与筛选抽屉多关键词队列合并为一条 f_search（与 OnlineHome 一致）
		mergedKw := strings.Join(
			append([]string{strings.TrimSpace(keyword)}, trimKeywords(keywords)...),
			" ",
		)
		mergedKw = strings.TrimSpace(mergedKw)
		// 抽卡专用过滤器：在线全量透传（语言并入 f_search，高级筛选走 f_* 参数）
		params := services.SearchParams{
			Keyword:               mergedKw,
			ActiveCategories:      activeCategories,
			Language:              language,
			OnlyRemoved:           onlyRemoved,
			OnlyTorrents:          onlyTorrents,
			DisableLangFilter:     disableLangFilter,
			DisableUploaderFilter: disableUploaderFilter,
			DisableTagFilter:      disableTagFilter,
		}
		if minRating > 0 {
			// 在线映射为 E 站 f_srdd 星级（与 OnlineHome 一致）
			params.MinRating = strconv.FormatFloat(minRating, 'f', -1, 64)
		}
		comics, err := h.ehService.FetchRandomGalleryList(account, params, ehSetting, limit, excludeTags, excludeKeywords)
		if err != nil {
			return nil, err
		}
		comics = services.AttachFavoriteStates(h.db, account.ID, comics)
		comics = services.AttachDownloadStates(h.db, comics)
		items := make([]RandomComicItem, 0, len(comics))
		for _, co := range comics {
			items = append(items, fromOnlineDTO(co))
		}
		return items, nil
	}

	// 抽取本地候选（随机或推荐）
	drawOffline := func(limit int) ([]RandomComicItem, string) {
		if mode == "recommend" {
			return recommendOffline(limit)
		}
		return randomOffline(limit), ""
	}

	var items []RandomComicItem
	warning := ""

	switch source {
	case "online":
		result, err := randomOnline(count)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		items = result
	case "offline":
		items, warning = drawOffline(count)
	default: // all：先随机抽本地约一半，再在线补齐剩余，比例接近 1:1；任一方不足时由另一方补齐
		offlineCount := count/2 + count%2
		onlineCount := count - offlineCount

		// 1. 先抽本地（推荐模式下为加权推荐）
		offlineItems, offlineWarning := drawOffline(offlineCount)
		warning = offlineWarning

		// 2. 本地不足 → 在线多抽补齐剩余
		if len(offlineItems) < offlineCount {
			onlineCount += offlineCount - len(offlineItems)
		}

		// 3. 再抽在线（补齐剩余数量）
		onlineItems, err := randomOnline(onlineCount)
		if err != nil {
			warning = "在线抽卡失败：" + err.Error() + "，已为你从本地库补充抽取"
			onlineItems = nil
			// 在线失败 → 本地补齐总数
			if missing := count - len(offlineItems); missing > 0 {
				extra, _ := drawOffline(missing)
				offlineItems = append(offlineItems, extra...)
			}
		}

		// 输出顺序：本地在前，在线在后
		items = append([]RandomComicItem{}, offlineItems...)
		items = append(items, onlineItems...)
	}

	c.JSON(http.StatusOK, gin.H{
		"comics":  items,
		"count":   len(items),
		"warning": warning,
		// Round32：前端据此标注「在线部分为纯随机」（推荐只作用于本地库）
		"mode":         mode,
		"onlineRandom": mode == "recommend",
	})
}

// loadUserShelfComicIDs 汇总该用户全部书架内的本子 ID（推荐模式「排除已在书架」用）
func loadUserShelfComicIDs(db *gorm.DB, userID uint) []string {
	var shelves []models.Bookshelf
	if err := db.Where("user_id = ?", userID).Find(&shelves).Error; err != nil {
		return nil
	}
	seen := make(map[string]bool, 256)
	out := make([]string, 0, 256)
	for i := range shelves {
		for _, id := range parseRawTags(shelves[i].ComicIDs) {
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}
