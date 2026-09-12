package services

import (
	"encoding/json"
	"errors"
	"log"
	"math"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"SakuManga/internal/models"

	"gorm.io/gorm"
)

// ─────────────────────────────────────────────────────────────
// Round32 阶段一：XP 词云统计服务
//
// 职责：
//   1. 维护 xp_tag_stats / xp_comic_stats 两张统计表（增量差分 + 全量重建）
//   2. 对外提供词云查询（分组 / 库藏·阅读双视图 / 画师 Top）
//   3. 向阶段二推荐输出统一权重表（w'(t) = 融合权重 × 泛化抑制）
//
// 口径（与 plans/round32-xp-cloud-recommend-plan.md 一致）：
//   有效 tag = MergeTags(onlineTags, offlineAddTags, offlineRemoveTags)（三态全空回退 Tags），
//              剔除 language / reclass，namespace 归一分组
//   库藏侧   lib(c) = 1
//   阅读侧   r(c)   = 0.60·log1p(readCount) + 0.25·exp(-Δdays/90) + 0.15·(rating-1)/9
//   聚合     libWeight(t)/readWeight(t) = Σ_c signal(c) / |tags(c)|（稀释，防泛化标签淹没核心 tag）
//
// 增量策略（决策 D6）：单本变更走 RecomposeComic 差分（事务内 ± 幂等）；
// 公式版本变更 / 脏标记 / 管理员手动触发走 RebuildAll 全量重建。
// ─────────────────────────────────────────────────────────────

const (
	// XpSchemaVersion 统计数据结构版本（表结构变更时递增 → 触发全量重建）
	XpSchemaVersion = 1
	// XpFormulaVersion 权重公式版本（公式调整时递增 → 触发全量重建）
	XpFormulaVersion = 1

	// xpWeightCacheTTL 权重表内存缓存有效期（推荐抽卡高频读取，避免每次重扫统计表）
	xpWeightCacheTTL = 60 * time.Second

	// 阅读侧多信号融合系数（三项各自量纲对齐后加权）
	xpReadCountWeight = 0.60 // 阅读次数（log1p 压缩长尾）
	xpHistoryWeight   = 0.25 // 阅读历史近期性
	xpRatingWeight    = 0.15 // 个人评分
	xpHistoryHalfLife = 90.0 // 阅读历史半衰期（天）：越久远的历史权重越低，反映 XP 漂移

	// xpPruneEpsilon 聚合行的清理阈值（差分归零判定，容忍浮点累积误差）
	xpPruneEpsilon = 1e-6
)

// XP 词云命名空间分组
var (
	xpCoreNamespaces   = map[string]bool{"female": true, "male": true, "mixed": true, "other": true}
	xpIPNamespaces     = map[string]bool{"character": true, "parody": true}
	xpArtistNamespaces = map[string]bool{"artist": true, "group": true}
	xpSkipNamespaces   = map[string]bool{"language": true, "reclass": true}
)

// xpGroupNamespaces 分组 → 参与统计的命名空间集合（SQL IN 过滤用）
var xpGroupNamespaces = map[string][]string{
	"core":   {"female", "male", "mixed", "other"},
	"ip":     {"character", "parody"},
	"artist": {"artist", "group"},
}

// xpMiscExcludeNamespaces 「其他」分组排除的已知命名空间（其余未知命名空间一律归入 misc）
var xpMiscExcludeNamespaces = []string{
	"female", "male", "mixed", "other",
	"character", "parody",
	"artist", "group",
	"language", "reclass",
}

// XpGroupOf 返回 tag 的命名空间分组：core | ip | artist | misc | skip
func XpGroupOf(namespace string) string {
	ns := strings.ToLower(strings.TrimSpace(namespace))
	switch {
	case ns == "":
		return "skip"
	case xpSkipNamespaces[ns]:
		return "skip"
	case xpCoreNamespaces[ns]:
		return "core"
	case xpIPNamespaces[ns]:
		return "ip"
	case xpArtistNamespaces[ns]:
		return "artist"
	default:
		return "misc"
	}
}

// SplitXpTag 拆分并归一化 tag 为 (namespace, key)。
//
// 归一规则与 TagEngine.TranslateTags / 前端 tagRaws 保持一致：
// namespace 小写（无冒号时归为 other），key 去空白、下划线转空格、小写。
func SplitXpTag(raw string) (namespace, key string) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", ""
	}
	ns := "other"
	k := s
	if idx := strings.Index(s, ":"); idx > 0 {
		ns = strings.ToLower(strings.TrimSpace(s[:idx]))
		k = s[idx+1:]
	}
	k = strings.ToLower(strings.TrimSpace(strings.ReplaceAll(k, "_", " ")))
	if ns == "" || k == "" {
		return "", ""
	}
	return ns, k
}

// XpEffectiveTags 计算参与统计的有效 tag（namespace:key 归一形式）。
//
// 与详情页展示口径完全一致（双轨三态），再剔除 language/reclass 这类无语义命名空间。
func XpEffectiveTags(comic *models.OfflineComic) []string {
	online := UnmarshalTagSlice(comic.OnlineTags)
	add := UnmarshalTagSlice(comic.OfflineAddTags)
	remove := UnmarshalTagSlice(comic.OfflineRemoveTags)

	merged := MergeTags(online, add, remove)
	// 兼容旧数据：三态全空时回退旧 Tags 字段（与 GetOfflineComicDetail 一致）
	if len(merged) == 0 && strings.TrimSpace(comic.OnlineTags) == "" {
		merged = UnmarshalTagSlice(comic.Tags)
	}

	out := make([]string, 0, len(merged))
	for _, raw := range merged {
		ns, key := SplitXpTag(raw)
		if ns == "" || xpSkipNamespaces[ns] {
			continue
		}
		out = append(out, ns+":"+key)
	}
	return normalizeTags(out)
}

// xpTagsJSON 有效 tag 集合的稳定序列化（排序后输出，便于逐字节比较判断是否变化）
func xpTagsJSON(tags []string) string {
	sorted := make([]string, len(tags))
	copy(sorted, tags)
	sort.Strings(sorted)
	data, err := json.Marshal(sorted)
	if err != nil {
		return "[]"
	}
	return string(data)
}

// xpTagsFromJSON 反序列化快照中的 tag 集合（脏数据回退空集合）
func xpTagsFromJSON(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" {
		return nil
	}
	var tags []string
	if err := json.Unmarshal([]byte(raw), &tags); err != nil {
		return nil
	}
	return tags
}

// ─── DTO ───

// XpCloudTag 词云词条
type XpCloudTag struct {
	Namespace  string  `json:"namespace"`
	Key        string  `json:"key"`
	Name       string  `json:"name"`       // 中文翻译（词典缺失时回退 key）
	Group      string  `json:"group"`      // core | ip | misc
	Weight     float64 `json:"weight"`     // 当前视图的归一权重（0~1，字号依据）
	LibWeight  float64 `json:"libWeight"`  // 库藏视图归一权重
	ReadWeight float64 `json:"readWeight"` // 阅读视图归一权重
	ComicCount int     `json:"comicCount"` // 含该 tag 的本数
}

// XpCloudArtist 画师/社团条目（不进词云，独立 Top 列表）
type XpCloudArtist struct {
	Namespace  string  `json:"namespace"`
	Key        string  `json:"key"`
	Name       string  `json:"name"`
	ComicCount int     `json:"comicCount"`
	ReadWeight float64 `json:"readWeight"` // 阅读侧热度（Σ r(c)/|tags(c)|）
}

// XpCloudMeta 统计元信息（前端用于覆盖率提示与重算入口）
type XpCloudMeta struct {
	TotalComics      int   `json:"totalComics"`      // 本地库总本数
	TaggedComics     int   `json:"taggedComics"`     // 参与统计（有有效 tag）的本数
	ReadSignalComics int   `json:"readSignalComics"` // 阅读信号非零的本数（覆盖率提示）
	LastRebuildAt    int64 `json:"lastRebuildAt"`    // 上次全量重建时间戳(ms)
	FormulaVersion   int   `json:"formulaVersion"`
}

// XpCloudResult 词云查询结果
type XpCloudResult struct {
	Tags    []XpCloudTag    `json:"tags"`
	Artists []XpCloudArtist `json:"artists"`
	Meta    XpCloudMeta     `json:"meta"`
}

// XpWeightTable 推荐用权重表（阶段二复用；Lib/Read 均已 0~1 归一）
type XpWeightTable struct {
	Lib        map[string]float64 `json:"-"` // "namespace:key" → 库藏归一权重
	Read       map[string]float64 `json:"-"` // "namespace:key" → 阅读归一权重
	IDF        map[string]float64 `json:"-"` // "namespace:key" → 泛化抑制系数
	LibMax     float64            `json:"-"`
	ReadMax    float64            `json:"-"`
	Tagged     int                `json:"-"` // 参与统计的本数（IDF 的 N）
	ComputedAt time.Time          `json:"-"`
}

// xpWeightCacheEntry 权重表缓存项
type xpWeightCacheEntry struct {
	table *XpWeightTable
	at    time.Time
}

// XpCloudService XP 词云统计服务
type XpCloudService struct {
	db *gorm.DB

	rebuildMu sync.Mutex
	cacheMu   sync.Mutex
	cache     map[uint]*xpWeightCacheEntry
}

// NewXpCloudService 构造 XP 词云统计服务
func NewXpCloudService(db *gorm.DB) *XpCloudService {
	return &XpCloudService{db: db, cache: map[uint]*xpWeightCacheEntry{}}
}

// ─────────────────────────────────────────────────────────────
// 全局实例与增量触发便捷入口
//
// 各业务触发点（阅读次数 / 离线历史 / 评分 / 扫描 / Tag 维护）通过下方便捷函数调用：
// 一律**不阻断主流程**（失败仅记日志 + 置脏标记，由下次查询兜底全量重建）。
// ─────────────────────────────────────────────────────────────

// GlobalXpCloud XP 词云统计服务全局实例（由 router 装配，与 handler 共用同一实例以共享缓存）
var GlobalXpCloud *XpCloudService

// XpRecomposeComic 系统级触发：该本新增/更新/删除/Tag 变化后对所有用户重算
func XpRecomposeComic(comicID string) {
	if GlobalXpCloud == nil || strings.TrimSpace(comicID) == "" {
		return
	}
	if err := GlobalXpCloud.RecomposeComicAllUsers(comicID); err != nil {
		log.Printf("[xp] 增量重算失败 comic=%s：%v（已置脏，下次查询自动全量重建）", comicID, err)
	}
}

// XpRecomposeComics 系统级批量触发（扫描收尾 / Tag 刷新收尾）
func XpRecomposeComics(comicIDs []string) {
	if GlobalXpCloud == nil || len(comicIDs) == 0 {
		return
	}
	if err := GlobalXpCloud.RecomposeComicsAllUsers(comicIDs); err != nil {
		log.Printf("[xp] 批量增量重算失败（%d 本）：%v（已置脏，下次查询自动全量重建）", len(comicIDs), err)
	}
}

// XpRecomposeComicForUser 用户级触发：该用户对某本的阅读侧信号变化（阅读次数 / 历史 / 评分）
func XpRecomposeComicForUser(userID uint, comicID string) {
	if GlobalXpCloud == nil || userID == 0 || strings.TrimSpace(comicID) == "" {
		return
	}
	if err := GlobalXpCloud.RecomposeComic(comicID, userID); err != nil {
		log.Printf("[xp] 增量重算失败 user=%d comic=%s：%v（已置脏，下次查询自动全量重建）", userID, comicID, err)
	}
}

// XpRebuildAsync 异步全量重建（低频系统级事件收尾：扫描 / 下载入库 / Tag 刷新 / Tag 写回）。
//
// 这类事件影响面广（可能成批新增、更新、删除），逐本差分收益低而易漏，统一走全量重建；
// 放后台执行不阻塞主流程，并做去抖合并：已有重建在跑时仅置「待补一轮」标记，
// 保证「最后一次变更之后一定还有一次重建」，同时避免批量下载时反复重建。
func XpRebuildAsync(reason string) {
	if GlobalXpCloud == nil {
		return
	}
	if xpRebuildRunning.CompareAndSwap(false, true) {
		go xpRebuildLoop(reason)
		return
	}
	xpRebuildAgain.Store(true)
}

var (
	xpRebuildRunning atomic.Bool
	xpRebuildAgain   atomic.Bool
)

// xpRebuildLoop 重建循环：跑完一轮后若期间有新事件，再补一轮
func xpRebuildLoop(reason string) {
	defer xpRebuildRunning.Store(false)
	for {
		if err := GlobalXpCloud.ForceRebuild(); err != nil {
			log.Printf("[xp] %s 后全量重建失败：%v", reason, err)
		} else {
			log.Printf("[xp] %s 后统计已全量重建", reason)
		}
		if !xpRebuildAgain.CompareAndSwap(true, false) {
			return
		}
	}
}

// ─────────────────────────────────────────────────────────────
// 信号计算
// ─────────────────────────────────────────────────────────────

// xpSignalCtx 阅读侧信号的批量上下文（一次性加载历史/评分，避免逐本查库）
type xpSignalCtx struct {
	history map[string]time.Time // comicID → 最后阅读时间
	rating  map[string]int       // comicID → 个人评分（1-10）
}

// loadSignalCtx 加载指定用户的阅读侧信号（历史取离线来源；同 comic 多条取最新）
func (s *XpCloudService) loadSignalCtx(tx *gorm.DB, userID uint) *xpSignalCtx {
	ctx := &xpSignalCtx{history: map[string]time.Time{}, rating: map[string]int{}}

	var records []models.HistoryRecord
	if err := tx.Where("user_id = ? AND source = ?", userID, models.SourceOffline).Find(&records).Error; err == nil {
		for i := range records {
			h := &records[i]
			if h.ComicID == "" {
				continue
			}
			if prev, ok := ctx.history[h.ComicID]; !ok || h.LastReadAt.After(prev) {
				ctx.history[h.ComicID] = h.LastReadAt
			}
		}
	}

	var ratings []models.ComicRating
	if err := tx.Where("user_id = ?", userID).Find(&ratings).Error; err == nil {
		for i := range ratings {
			ctx.rating[ratings[i].ComicID] = ratings[i].Score
		}
	}
	return ctx
}

// readWeightOf 单本阅读侧信号（多信号融合）
func (c *xpSignalCtx) readWeightOf(comicID string, readCount int) float64 {
	if readCount < 0 {
		readCount = 0
	}
	w := xpReadCountWeight * math.Log1p(float64(readCount))

	if t, ok := c.history[comicID]; ok && !t.IsZero() {
		days := time.Since(t).Hours() / 24
		if days < 0 {
			days = 0
		}
		w += xpHistoryWeight * math.Exp(-days/xpHistoryHalfLife)
	}

	if score, ok := c.rating[comicID]; ok && score > 0 {
		if score > 10 {
			score = 10
		}
		w += xpRatingWeight * float64(score-1) / 9.0
	}
	return w
}

// ─────────────────────────────────────────────────────────────
// 差分更新与全量重建
// ─────────────────────────────────────────────────────────────

// RecomposeComic 重算单本漫画对该用户统计表的贡献（差分、幂等）。
//
// 漫画已不存在时退化为「只做减法」（清掉残留贡献）。
// 调用方无需关心是否变化：无变化时内部直接跳过。
func (s *XpCloudService) RecomposeComic(comicID string, userID uint) error {
	return s.recomposeBatch([]string{comicID}, []uint{userID})
}

// RecomposeComics 批量重算（同一用户下的多本：Tag 刷新/批量导入后调用）
func (s *XpCloudService) RecomposeComics(comicIDs []string, userID uint) error {
	return s.recomposeBatch(comicIDs, []uint{userID})
}

// RecomposeComicAllUsers 系统级变更（扫描/删除/Tag 写回）后对所有用户重算该本
func (s *XpCloudService) RecomposeComicAllUsers(comicID string) error {
	userIDs, err := s.allUserIDs()
	if err != nil {
		return err
	}
	return s.recomposeBatch([]string{comicID}, userIDs)
}

// RecomposeComicsAllUsers 系统级批量变更后对所有用户重算
func (s *XpCloudService) RecomposeComicsAllUsers(comicIDs []string) error {
	userIDs, err := s.allUserIDs()
	if err != nil {
		return err
	}
	return s.recomposeBatch(comicIDs, userIDs)
}

// allUserIDs 全部用户 ID（阅读侧信号按用户隔离，每个用户各有一份贡献快照）
func (s *XpCloudService) allUserIDs() ([]uint, error) {
	var ids []uint
	if err := s.db.Model(&models.User{}).Pluck("id", &ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}

// recomposeBatch 批量差分重算：单事务内完成「多本 × 多用户」，任一失败整体回滚并置脏
func (s *XpCloudService) recomposeBatch(comicIDs []string, userIDs []uint) error {
	if len(comicIDs) == 0 || len(userIDs) == 0 {
		return nil
	}
	// 信号上下文按用户加载一次复用（避免逐本重复查历史/评分）
	ctxCache := make(map[uint]*xpSignalCtx, len(userIDs))

	err := s.db.Transaction(func(tx *gorm.DB) error {
		for _, userID := range userIDs {
			if err := s.recomposeForUserTx(tx, comicIDs, userID, ctxCache); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		// 增量失败不阻断主流程，仅置脏标记由后续查询兜底重建
		s.markDirty()
		return err
	}
	s.invalidateCache()
	return nil
}

// recomposeForUserTx 单用户 × 多本差分（须在事务内调用）
//
// 信号上下文按用户复用：首本由 recomposeComicTx 按需加载并回填 ctxCache，后续本直接命中。
func (s *XpCloudService) recomposeForUserTx(tx *gorm.DB, comicIDs []string, userID uint, ctxCache map[uint]*xpSignalCtx) error {
	for _, comicID := range comicIDs {
		if strings.TrimSpace(comicID) == "" {
			continue
		}
		if err := s.recomposeComicTx(tx, comicID, userID, ctxCache[userID], ctxCache); err != nil {
			return err
		}
	}
	return nil
}

// recomposeComicTx 单本差分（须在事务内调用）
//
// ctx 为阅读侧信号上下文；传 nil 时按需加载并写入 ctxCache 供同用户后续本复用。
func (s *XpCloudService) recomposeComicTx(tx *gorm.DB, comicID string, userID uint, ctx *xpSignalCtx, ctxCache map[uint]*xpSignalCtx) error {
	var comic models.OfflineComic
	err := tx.First(&comic, "id = ?", comicID).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	comicMissing := errors.Is(err, gorm.ErrRecordNotFound)

	var old models.XpComicStat
	oldErr := tx.Where("user_id = ? AND comic_id = ?", userID, comicID).First(&old).Error
	hasOld := oldErr == nil
	if oldErr != nil && !errors.Is(oldErr, gorm.ErrRecordNotFound) {
		return oldErr
	}

	// 1. 漫画已删除：减掉旧贡献并清除快照
	if comicMissing {
		if !hasOld {
			return nil
		}
		if err := s.applyTagDelta(tx, userID, xpTagsFromJSON(old.TagsJSON), old.LibWeight, old.ReadWeight, -1); err != nil {
			return err
		}
		if err := tx.Where("user_id = ? AND comic_id = ?", userID, comicID).Delete(&models.XpComicStat{}).Error; err != nil {
			return err
		}
		return s.pruneTags(tx, userID)
	}

	// 2. 计算新贡献
	tags := XpEffectiveTags(&comic)
	tagsJSON := xpTagsJSON(tags)
	if ctx == nil {
		if cached, ok := ctxCache[userID]; ok {
			ctx = cached
		} else {
			ctx = s.loadSignalCtx(tx, userID)
			if ctxCache != nil {
				ctxCache[userID] = ctx
			}
		}
	}
	readWeight := ctx.readWeightOf(comic.ID, comic.ReadCount)
	const libWeight = 1.0

	// 3. 无变化直接跳过（扫描/刷新高频触发时的核心优化）
	if hasOld && old.TagsJSON == tagsJSON && old.LibWeight == libWeight && old.ReadWeight == readWeight {
		return nil
	}

	// 4. 先减旧、再加新（同一事务内，保证聚合表始终自洽）
	if hasOld {
		if err := s.applyTagDelta(tx, userID, xpTagsFromJSON(old.TagsJSON), old.LibWeight, old.ReadWeight, -1); err != nil {
			return err
		}
	}
	if len(tags) > 0 {
		if err := s.applyTagDelta(tx, userID, tags, libWeight, readWeight, 1); err != nil {
			return err
		}
	}

	// 5. 写回快照（无 tag 的本子保留空快照：下次 tag 出现时仍能命中「只做加法」路径）
	stat := models.XpComicStat{
		UserID:     userID,
		ComicID:    comic.ID,
		LibWeight:  libWeight,
		ReadWeight: readWeight,
		TagCount:   len(tags),
		TagsJSON:   tagsJSON,
		UpdatedAt:  time.Now(),
	}
	if hasOld {
		if err := tx.Model(&models.XpComicStat{}).
			Where("user_id = ? AND comic_id = ?", userID, comic.ID).
			Updates(map[string]interface{}{
				"lib_weight":  stat.LibWeight,
				"read_weight": stat.ReadWeight,
				"tag_count":   stat.TagCount,
				"tags_json":   stat.TagsJSON,
				"updated_at":  stat.UpdatedAt,
			}).Error; err != nil {
			return err
		}
	} else if err := tx.Create(&stat).Error; err != nil {
		return err
	}

	return s.pruneTags(tx, userID)
}

// applyTagDelta 把单本贡献按稀释规则累加到聚合表（sign = +1 加 / -1 减）
func (s *XpCloudService) applyTagDelta(tx *gorm.DB, userID uint, tags []string, libWeight, readWeight, sign float64) error {
	if len(tags) == 0 {
		return nil
	}
	n := float64(len(tags))
	now := time.Now()

	for _, tag := range tags {
		ns, key := SplitXpTag(tag)
		if ns == "" || xpSkipNamespaces[ns] {
			continue
		}
		if err := tx.Exec(`INSERT INTO xp_tag_stats
				(user_id, namespace, tag_key, lib_weight, read_weight, comic_count, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(user_id, namespace, tag_key) DO UPDATE SET
				lib_weight  = xp_tag_stats.lib_weight  + excluded.lib_weight,
				read_weight = xp_tag_stats.read_weight + excluded.read_weight,
				comic_count = xp_tag_stats.comic_count + excluded.comic_count,
				updated_at  = excluded.updated_at`,
			userID, ns, key, sign*libWeight/n, sign*readWeight/n, int(sign), now).Error; err != nil {
			return err
		}
	}
	return nil
}

// pruneTags 清理差分归零的聚合行（本数归零且两侧权重都归零）
func (s *XpCloudService) pruneTags(tx *gorm.DB, userID uint) error {
	return tx.Exec(`DELETE FROM xp_tag_stats
		WHERE user_id = ? AND comic_count <= 0 AND lib_weight <= ? AND read_weight <= ?`,
		userID, xpPruneEpsilon, xpPruneEpsilon).Error
}

// RebuildAll 全量重建所有用户的统计表（新鲜度满足时短路返回）
func (s *XpCloudService) RebuildAll() error { return s.rebuild(false) }

// rebuild 全量重建实现；force=true 跳过「版本一致且非脏」的新鲜度短路
func (s *XpCloudService) rebuild(force bool) error {
	s.rebuildMu.Lock()
	defer s.rebuildMu.Unlock()

	// 双重检查：拿到锁后若已被其他协程重建完成则直接返回（低频事件并发收尾时避免重复重建）
	if !force {
		var meta models.XpMeta
		if err := s.db.First(&meta, 1).Error; err == nil &&
			meta.FormulaVersion == XpFormulaVersion &&
			meta.SchemaVersion == XpSchemaVersion && !meta.Dirty {
			return nil
		}
	}

	var users []models.User
	if err := s.db.Find(&users).Error; err != nil {
		return err
	}
	var comics []models.OfflineComic
	if err := s.db.Find(&comics).Error; err != nil {
		return err
	}

	now := time.Now()
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("DELETE FROM xp_tag_stats").Error; err != nil {
			return err
		}
		if err := tx.Exec("DELETE FROM xp_comic_stats").Error; err != nil {
			return err
		}

		for _, u := range users {
			signalCtx := s.loadSignalCtx(tx, u.ID)
			agg := make(map[string]*models.XpTagStat, 4096)
			stats := make([]models.XpComicStat, 0, len(comics))

			for i := range comics {
				comic := &comics[i]
				tags := XpEffectiveTags(comic)
				if len(tags) == 0 {
					continue
				}
				readWeight := signalCtx.readWeightOf(comic.ID, comic.ReadCount)
				n := float64(len(tags))

				for _, tag := range tags {
					ns, key := SplitXpTag(tag)
					if ns == "" || xpSkipNamespaces[ns] {
						continue
					}
					k := ns + ":" + key
					row := agg[k]
					if row == nil {
						row = &models.XpTagStat{UserID: u.ID, Namespace: ns, TagKey: key, UpdatedAt: now}
						agg[k] = row
					}
					row.LibWeight += 1.0 / n
					row.ReadWeight += readWeight / n
					row.ComicCount++
				}

				stats = append(stats, models.XpComicStat{
					UserID:     u.ID,
					ComicID:    comic.ID,
					LibWeight:  1,
					ReadWeight: readWeight,
					TagCount:   len(tags),
					TagsJSON:   xpTagsJSON(tags),
					UpdatedAt:  now,
				})
			}

			tagRows := make([]models.XpTagStat, 0, len(agg))
			for _, row := range agg {
				tagRows = append(tagRows, *row)
			}
			if len(tagRows) > 0 {
				if err := tx.CreateInBatches(tagRows, 500).Error; err != nil {
					return err
				}
			}
			if len(stats) > 0 {
				if err := tx.CreateInBatches(stats, 500).Error; err != nil {
					return err
				}
			}
		}
		return s.writeMeta(tx, false)
	})
	if err != nil {
		return err
	}
	s.invalidateCache()
	return nil
}

// ForceRebuild 强制全量重算（管理员手动触发 / 低频系统级事件收尾）
//
// 与 RebuildAll 的区别：跳过新鲜度短路，一定执行重建。
func (s *XpCloudService) ForceRebuild() error {
	return s.rebuild(true)
}

// EnsureFresh 查询前的保鲜检查：缺失/版本不符/脏标记 → 全量重建
func (s *XpCloudService) EnsureFresh() error {
	var meta models.XpMeta
	err := s.db.First(&meta, 1).Error
	if err == nil &&
		meta.FormulaVersion == XpFormulaVersion &&
		meta.SchemaVersion == XpSchemaVersion && !meta.Dirty {
		return nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return s.RebuildAll()
}

// writeMeta 写入统计元信息（单例 ID=1，upsert）
func (s *XpCloudService) writeMeta(tx *gorm.DB, dirty bool) error {
	return tx.Exec(`INSERT INTO xp_meta (id, schema_version, formula_version, last_rebuild_at, dirty)
		VALUES (1, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			schema_version  = excluded.schema_version,
			formula_version = excluded.formula_version,
			last_rebuild_at = excluded.last_rebuild_at,
			dirty           = excluded.dirty`,
		XpSchemaVersion, XpFormulaVersion, time.Now().UnixMilli(), dirty).Error
}

// markDirty 置脏标记（增量失败兜底，下次查询自动全量重建）
func (s *XpCloudService) markDirty() {
	_ = s.db.Exec(`UPDATE xp_meta SET dirty = 1 WHERE id = 1`).Error
}

// invalidateCache 清除权重表内存缓存
func (s *XpCloudService) invalidateCache() {
	s.cacheMu.Lock()
	s.cache = map[uint]*xpWeightCacheEntry{}
	s.cacheMu.Unlock()
}

// ─────────────────────────────────────────────────────────────
// 查询
// ─────────────────────────────────────────────────────────────

// Query 词云查询：view = library|reading，group = core|ip|misc|all
func (s *XpCloudService) Query(userID uint, group, view string, limit int) (*XpCloudResult, error) {
	if err := s.EnsureFresh(); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 300 {
		limit = 120
	}
	if view != "reading" {
		view = "library"
	}
	if group == "" {
		group = "core"
	}

	weightCol := "lib_weight"
	if view == "reading" {
		weightCol = "read_weight"
	}

	// 1. 归一基准（全量 max；分开取，保证两个视图各自满量程）
	var libMax, readMax float64
	if err := s.db.Raw("SELECT COALESCE(MAX(lib_weight), 0) FROM xp_tag_stats WHERE user_id = ?", userID).Scan(&libMax).Error; err != nil {
		return nil, err
	}
	if err := s.db.Raw("SELECT COALESCE(MAX(read_weight), 0) FROM xp_tag_stats WHERE user_id = ?", userID).Scan(&readMax).Error; err != nil {
		return nil, err
	}

	// 2. 取词条（按分组过滤 + 当前视图权重排序 + 截断，避免加载全量聚合行）
	var rows []models.XpTagStat
	q := s.db.Where("user_id = ?", userID)
	switch group {
	case "core", "ip", "artist":
		q = q.Where("namespace IN ?", xpGroupNamespaces[group])
	case "all":
		// 全部（含画师/社团），仅排除 skip 类命名空间
	default: // misc：未知命名空间统一归入「其他」
		q = q.Where("namespace NOT IN ?", xpMiscExcludeNamespaces)
	}
	if err := q.Order(weightCol + " DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}

	// 3. 翻译（复用既有词典引擎，词典缺失时 Name 回退 key）
	raws := make([]string, 0, len(rows))
	for i := range rows {
		raws = append(raws, rows[i].Namespace+":"+rows[i].TagKey)
	}
	translated := GlobalTagEngine.TranslateTags(raws)

	tags := make([]XpCloudTag, 0, len(rows))
	for i := range rows {
		row := &rows[i]
		name := row.TagKey
		if i < len(translated) && translated[i] != nil {
			if cleaned := CleanTagDisplayName(translated[i].Name); cleaned != "" {
				name = cleaned
			}
		}
		libNorm := xpNormalize(row.LibWeight, libMax)
		readNorm := xpNormalize(row.ReadWeight, readMax)
		weight := libNorm
		if view == "reading" {
			weight = readNorm
		}
		tags = append(tags, XpCloudTag{
			Namespace:  row.Namespace,
			Key:        row.TagKey,
			Name:       name,
			Group:      XpGroupOf(row.Namespace),
			Weight:     weight,
			LibWeight:  libNorm,
			ReadWeight: readNorm,
			ComicCount: row.ComicCount,
		})
	}
	// 阅读视图下剔除零信号条目（SQL 已排序，此处仅兜底）
	if view == "reading" {
		filtered := tags[:0]
		for _, t := range tags {
			if t.ReadWeight > 0 {
				filtered = append(filtered, t)
			}
		}
		tags = filtered
	}

	// 4. 画师/社团 Top 列表（不进词云）
	var artistRows []models.XpTagStat
	if err := s.db.Where("user_id = ? AND namespace IN ?", userID, xpGroupNamespaces["artist"]).
		Order("comic_count DESC").Limit(20).Find(&artistRows).Error; err != nil {
		return nil, err
	}
	artistRaws := make([]string, 0, len(artistRows))
	for i := range artistRows {
		artistRaws = append(artistRaws, artistRows[i].Namespace+":"+artistRows[i].TagKey)
	}
	artistTranslated := GlobalTagEngine.TranslateTags(artistRaws)
	artists := make([]XpCloudArtist, 0, len(artistRows))
	for i := range artistRows {
		row := &artistRows[i]
		name := row.TagKey
		if i < len(artistTranslated) && artistTranslated[i] != nil {
			if cleaned := CleanTagDisplayName(artistTranslated[i].Name); cleaned != "" {
				name = cleaned
			}
		}
		artists = append(artists, XpCloudArtist{
			Namespace:  row.Namespace,
			Key:        row.TagKey,
			Name:       name,
			ComicCount: row.ComicCount,
			ReadWeight: xpNormalize(row.ReadWeight, readMax),
		})
	}

	// 5. 元信息（覆盖率提示）
	meta, err := s.meta(userID)
	if err != nil {
		return nil, err
	}

	return &XpCloudResult{Tags: tags, Artists: artists, Meta: meta}, nil
}

// meta 汇总统计元信息
func (s *XpCloudService) meta(userID uint) (XpCloudMeta, error) {
	out := XpCloudMeta{FormulaVersion: XpFormulaVersion}

	var total, tagged, readSignal int64
	if err := s.db.Model(&models.OfflineComic{}).Count(&total).Error; err != nil {
		return out, err
	}
	if err := s.db.Model(&models.XpComicStat{}).Where("user_id = ?", userID).Count(&tagged).Error; err != nil {
		return out, err
	}
	if err := s.db.Model(&models.XpComicStat{}).
		Where("user_id = ? AND read_weight > 0", userID).Count(&readSignal).Error; err != nil {
		return out, err
	}
	var meta models.XpMeta
	if err := s.db.First(&meta, 1).Error; err == nil {
		out.LastRebuildAt = meta.LastRebuildAt
	}

	out.TotalComics = int(total)
	out.TaggedComics = int(tagged)
	out.ReadSignalComics = int(readSignal)
	return out, nil
}

// ─── 标签展示名清洗 ───

// EH Tag Translation 词典的 name 字段含 markdown 图标语法
// （如 `![长筒袜图标](https://.../stockings.webp)长筒袜`），直接展示会出现整段 URL。
var (
	tagIconFullRe  = regexp.MustCompile(`!\[.*?\]\(.*?\)`)
	tagIconLooseRe = regexp.MustCompile(`!\[[^\]]*\]`)
	// alt 提取：兼容「完整图标 ![alt](url)」与「脏数据残片 ![alt]」（URL 部分可选）
	tagIconAltRe = regexp.MustCompile(`!\[(.*?)\](?:\([^)]*\))?`)
)

// CleanTagDisplayName 清洗标签展示名：① 完整图标 `![alt](url)` 整体剥离；
// ② 脏数据残片 `![alt]`（无 url）同样剥离；③ 纯图标（剥离后无文本）时退回取 alt 文本。
//
// 第 ③ 步比前端 comicStore.cleanTagName 更宽松（前端对无 url 残片会返回空串并回退 tag key），
// 但两者对"正常含文本"的词典数据结果一致；调用方仍应对空结果回退到 key。
func CleanTagDisplayName(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	cleaned := strings.TrimSpace(
		tagIconLooseRe.ReplaceAllString(tagIconFullRe.ReplaceAllString(s, ""), ""),
	)
	if cleaned != "" {
		return cleaned
	}
	if m := tagIconAltRe.FindStringSubmatch(s); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

// xpNormalize 归一化到 0~1（max 为 0 时返回 0）
func xpNormalize(v, max float64) float64 {
	if max <= 0 || v <= 0 {
		return 0
	}
	n := v / max
	if n > 1 {
		return 1
	}
	return n
}

// ─────────────────────────────────────────────────────────────
// 权重表（阶段二推荐复用）
// ─────────────────────────────────────────────────────────────

// WeightTable 输出推荐用权重表（含 IDF 泛化抑制；带内存缓存）
func (s *XpCloudService) WeightTable(userID uint) (*XpWeightTable, error) {
	s.cacheMu.Lock()
	if entry, ok := s.cache[userID]; ok && time.Since(entry.at) < xpWeightCacheTTL {
		table := entry.table
		s.cacheMu.Unlock()
		return table, nil
	}
	s.cacheMu.Unlock()

	if err := s.EnsureFresh(); err != nil {
		return nil, err
	}

	var rows []models.XpTagStat
	if err := s.db.Where("user_id = ?", userID).Find(&rows).Error; err != nil {
		return nil, err
	}

	table := &XpWeightTable{
		Lib:        make(map[string]float64, len(rows)),
		Read:       make(map[string]float64, len(rows)),
		IDF:        make(map[string]float64, len(rows)),
		ComputedAt: time.Now(),
	}
	if len(rows) == 0 {
		return table, nil
	}

	var tagged int64
	if err := s.db.Model(&models.XpComicStat{}).Where("user_id = ?", userID).Count(&tagged).Error; err != nil {
		return nil, err
	}
	table.Tagged = int(tagged)

	for i := range rows {
		row := &rows[i]
		if row.LibWeight > table.LibMax {
			table.LibMax = row.LibWeight
		}
		if row.ReadWeight > table.ReadMax {
			table.ReadMax = row.ReadWeight
		}
	}

	n := float64(table.Tagged)
	if n < 1 {
		n = 1
	}
	for i := range rows {
		row := &rows[i]
		key := row.Namespace + ":" + row.TagKey
		table.Lib[key] = xpNormalize(row.LibWeight, table.LibMax)
		table.Read[key] = xpNormalize(row.ReadWeight, table.ReadMax)
		// 泛化抑制：库内出现本数越多，越接近通用标签，权重被压低
		table.IDF[key] = math.Log(1 + n/(1+float64(row.ComicCount)))
	}

	s.cacheMu.Lock()
	s.cache[userID] = &xpWeightCacheEntry{table: table, at: time.Now()}
	s.cacheMu.Unlock()
	return table, nil
}
