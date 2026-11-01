package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"time"

	"SakuManga/internal/models"

	"gorm.io/gorm"
)

// ─────────────────────────────────────────────────────────────
// 忽略标记（Round26 O2 建立；Round44 成员快照；Round51 改为「按组成员」）
//
// 作用域：仅疑似重复判定（Tier1 + Tier2 名称级簇）+ 规则 3 父子画廊；
// 规则 1/2/4（同 GID/hash/内容签名）与更新检测不读取本表。
//
// Round51 语义（珱垣拍板）：
//   - 忽略 = 「这一组（维护页上的那个簇）」，不是「名字」。
//   - 忽略时记录该簇的全部成员 id（SeenComicIDs）；匹配按成员集合判定：
//     簇成员全在集合内 → 静默；
//     出现集合外成员 → 列出；其中「忽略之后新入库」的成员计入「新增 N 本」；
//     集合成员全被删除 → 条目失效（不匹配任何簇）。
//   - SnapshotVersion=2 为新语义；启动时清理旧语义条目（破坏性更新，用户需重新忽略一次）。
// ─────────────────────────────────────────────────────────────

// ErrIgnoreNotFound 忽略条目不存在（恢复时容错）
var ErrIgnoreNotFound = errors.New("忽略条目不存在")

const ignoreSnapshotVersion = 2

// CreateIgnore 新增忽略条目（幂等：同 type+key 已存在则把成员并入现有集合）
//
// title 型：memberIDs 为该簇的全部成员 id（前端传入）；TitleKey/Artist 仅作展示。
// gid 型：与旧逻辑一致。
func CreateIgnore(db *gorm.DB, typ, titleKey, artist, gid, note string, memberIDs []string) (*models.IgnoredIdentifier, error) {
	if db == nil {
		return nil, fmt.Errorf("非法参数：db 不能为空")
	}
	typ = strings.TrimSpace(typ)
	switch typ {
	case "title":
		titleKey = strings.TrimSpace(titleKey)
		if titleKey == "" {
			return nil, fmt.Errorf("非法参数：title 类型必须提供标题")
		}
		if len(memberIDs) == 0 {
			return nil, fmt.Errorf("非法参数：title 类型必须提供组成员")
		}
	case "gid":
		gid = strings.TrimSpace(gid)
		if gid == "" {
			return nil, fmt.Errorf("非法参数：gid 类型必须提供画廊 gid")
		}
	default:
		return nil, fmt.Errorf("非法参数：忽略类型仅支持 title / gid")
	}

	// 幂等去重：同类型同 key 不重复插入；已存在则把新成员并入集合（视为「确认新增」）
	var existing models.IgnoredIdentifier
	q := db.Where("type = ?", typ)
	switch typ {
	case "title":
		q = q.Where("title_key = ? AND artist = ?", titleKey, artist)
	case "gid":
		q = q.Where("g_id = ?", gid)
	}
	if err := q.First(&existing).Error; err == nil {
		if typ == "title" {
			merged := unionComicIDs(existing.SeenComicIDs, memberIDs)
			blob, err := json.Marshal(merged)
			if err != nil {
				return nil, fmt.Errorf("序列化成员集合失败: %v", err)
			}
			if err := db.Model(&models.IgnoredIdentifier{}).Where("id = ?", existing.ID).
				Updates(map[string]interface{}{
					"seen_comic_ids":   string(blob),
					"snapshot_version": ignoreSnapshotVersion,
				}).Error; err != nil {
				return nil, fmt.Errorf("更新忽略成员集合失败: %v", err)
			}
			existing.SeenComicIDs = string(blob)
			existing.SnapshotVersion = ignoreSnapshotVersion
		}
		return &existing, nil
	}

	rec := &models.IgnoredIdentifier{
		// 主键唯一性：UnixNano 在 Windows 上受系统时钟 tick 精度限制（约 15.6ms），
		// 同一 tick 内连续创建不同条目会生成相同 ID 触发 UNIQUE 冲突（既有 flaky 测试根因）。
		// 追加随机后缀保证同 tick 内多次创建也唯一（Go 1.20+ math/rand 自动随机种子）。
		ID:              fmt.Sprintf("ignore-%d-%d", time.Now().UnixNano(), rand.Int63()),
		Type:            typ,
		TitleKey:        titleKey,
		Artist:          artist,
		GID:             gid,
		Note:            strings.TrimSpace(note),
		CreatedAt:       time.Now(),
		SnapshotVersion: ignoreSnapshotVersion,
	}
	if typ == "title" {
		blob, err := json.Marshal(dedupeComicIDs(memberIDs))
		if err != nil {
			return nil, fmt.Errorf("序列化成员集合失败: %v", err)
		}
		rec.SeenComicIDs = string(blob)
	}
	if err := db.Create(rec).Error; err != nil {
		return nil, fmt.Errorf("写入忽略条目失败: %v", err)
	}
	return rec, nil
}

// ListIgnores 忽略清单（按创建时间倒序）
func ListIgnores(db *gorm.DB) ([]models.IgnoredIdentifier, error) {
	if db == nil {
		return nil, fmt.Errorf("非法参数：db 不能为空")
	}
	var out []models.IgnoredIdentifier
	if err := db.Order("created_at desc").Find(&out).Error; err != nil {
		return nil, fmt.Errorf("读取忽略清单失败: %v", err)
	}
	return out, nil
}

// ── 忽略清单成员视图（Round51：成员 = 忽略时该簇的成员集合） ──

// IgnoreMemberView 忽略清单里的一条成员（= 忽略集合中的现存成员）
type IgnoreMemberView struct {
	Comic     models.OfflineComic `json:"comic"`
	PageCount int                 `json:"pageCount"` // 物理页数（OriginalPageCount 优先）
	Lang      string              `json:"lang,omitempty"`
	Artist    string              `json:"artist,omitempty"`
	Grouped   bool                `json:"grouped"` // 与同条目下其他成员构成同组（真的会判为重复）
}

// IgnoreItemView 忽略清单条目视图
type IgnoreItemView struct {
	models.IgnoredIdentifier
	Members      []IgnoreMemberView `json:"members,omitempty"`    // 忽略集合中的现存成员
	MatchedCount int                `json:"matchedCount"`         // 现存成员数
	GroupCount   int                `json:"groupCount"`           // 其中构成重复的组数
	GroupedCount int                `json:"groupedCount"`         // 构成重复的本数
	NewCount     int                `json:"newCount"`             // 维护缓存里该组「忽略后新增」的本数（无缓存为 0）
}

// ListIgnoresWithMembers 忽略清单（成员卡片数据源）。
//
// 口径（Round51）：成员 = 忽略时记录的那批本子中**当前仍存在**的；已删除的成员不展示。
// 「同组」按 clusterKey（核心名+卷号+画师）分组、组内 ≥2 本即构成重复。
// newCount 取自维护结果缓存（与集合有交集的簇的「忽略后新入库」成员数），
// 缓存不存在（尚未扫描）时为 0。
func ListIgnoresWithMembers(db *gorm.DB) ([]IgnoreItemView, error) {
	list, err := ListIgnores(db)
	if err != nil {
		return nil, err
	}
	comics, cerr := loadDedupCandidates(db)
	if cerr != nil {
		comics = nil // 读库失败时降级为纯清单（不带成员），不阻断清单展示
	}
	byID := make(map[string]*models.OfflineComic, len(comics))
	for i := range comics {
		byID[comics[i].ID] = &comics[i]
	}
	var cachedClusters []DedupCluster
	if res := GetMaintainDedupResult(); res != nil {
		cachedClusters = res.Clusters
	}

	out := make([]IgnoreItemView, 0, len(list))
	for _, ig := range list {
		v := IgnoreItemView{IgnoredIdentifier: ig}
		switch ig.Type {
		case "title":
			seen := parseSeenComicIDs(ig.SeenComicIDs)
			members := make([]IgnoreMemberView, 0, len(seen))
			for id := range seen {
				if c := byID[id]; c != nil {
					members = append(members, newIgnoreMember(c))
				}
			}
			sort.Slice(members, func(i, j int) bool {
				return members[i].Comic.ID < members[j].Comic.ID
			})
			v.Members = members
			v.MatchedCount = len(members)
			v.GroupCount, v.GroupedCount = groupStats(members)
			v.NewCount = countNewFromCache(ig, cachedClusters)
		case "gid":
			members := make([]IgnoreMemberView, 0, 1)
			for i := range comics {
				if comics[i].GID != "" && comics[i].GID == ig.GID {
					members = append(members, newIgnoreMember(&comics[i]))
				}
			}
			v.Members, v.MatchedCount = members, len(members)
		}
		out = append(out, v)
	}
	return out, nil
}

// groupStats 按 clusterKey 统计「构成重复」的组数与本数（组内 ≥2 本）
func groupStats(members []IgnoreMemberView) (groupCount, groupedCount int) {
	byKey := map[string]int{}
	for i := range members {
		key := memberClusterKey(&members[i].Comic)
		if key == "" {
			continue
		}
		byKey[key]++
	}
	grouped := map[string]bool{}
	for k, n := range byKey {
		if n < 2 {
			continue
		}
		groupCount++
		groupedCount += n
		grouped[k] = true
	}
	for i := range members {
		if grouped[memberClusterKey(&members[i].Comic)] {
			members[i].Grouped = true
		}
	}
	return groupCount, groupedCount
}

// memberClusterKey 单本漫画在聚类口径下的主键（title_jpn 优先、title 兜底；含卷号与画师）
func memberClusterKey(c *models.OfflineComic) string {
	tags := UnmarshalTagSlice(c.OnlineTags)
	artist := extractNamespaceTag(tags, "artist")
	for _, src := range []string{c.TitleJpn, c.Title} {
		if strings.TrimSpace(src) == "" {
			continue
		}
		fp := fingerprintTitle(src, artist)
		if fp.Core == "" {
			continue
		}
		return clusterKey(fp)
	}
	return ""
}

// countNewFromCache 从维护结果缓存中找「与忽略集合有交集的簇」，返回其集合外新增成员数（保守口径）
func countNewFromCache(ig models.IgnoredIdentifier, clusters []DedupCluster) int {
	if len(clusters) == 0 {
		return 0
	}
	seen := parseSeenComicIDs(ig.SeenComicIDs)
	if len(seen) == 0 {
		return 0
	}
	entry := &TitleIgnoreEntry{
		ID:        ig.ID,
		TitleKey:  ig.TitleKey,
		Artist:    ig.Artist,
		CreatedAt: ig.CreatedAt.UnixMilli(),
		seen:      seen,
	}
	best := 0
	for i := range clusters {
		cl := &clusters[i]
		inter := 0
		for j := range cl.Members {
			if seen[cl.Members[j].Comic.ID] {
				inter++
				break
			}
		}
		if inter == 0 {
			continue
		}
		if _, n := entry.Evaluate(cl.Members); n > best {
			best = n
		}
	}
	return best
}

// AckIgnoreSnapshot 「确认新增」：把当前簇的成员并入 title 型忽略集合。
func AckIgnoreSnapshot(db *gorm.DB, id string, memberIDs []string) (*models.IgnoredIdentifier, error) {
	if db == nil {
		return nil, fmt.Errorf("非法参数：db 不能为空")
	}
	var rec models.IgnoredIdentifier
	if err := db.First(&rec, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrIgnoreNotFound
		}
		return nil, fmt.Errorf("读取忽略条目失败: %v", err)
	}
	if rec.Type != "title" {
		return &rec, nil // 仅 title 型有成员集合
	}
	if len(memberIDs) == 0 {
		return &rec, nil // 前端未提供成员：保持现状
	}
	blob, err := json.Marshal(unionComicIDs(rec.SeenComicIDs, memberIDs))
	if err != nil {
		return nil, fmt.Errorf("序列化成员集合失败: %v", err)
	}
	if err := db.Model(&models.IgnoredIdentifier{}).Where("id = ?", id).
		Updates(map[string]interface{}{
			"seen_comic_ids":   string(blob),
			"snapshot_version": ignoreSnapshotVersion,
		}).Error; err != nil {
		return nil, fmt.Errorf("更新成员集合失败: %v", err)
	}
	rec.SeenComicIDs = string(blob)
	rec.SnapshotVersion = ignoreSnapshotVersion
	return &rec, nil
}

// PurgeLegacyIgnores 清空旧语义忽略条目（Round51 破坏性更新，幂等）。
// 返回清理条数。
func PurgeLegacyIgnores(db *gorm.DB) int {
	if db == nil {
		return 0
	}
	res := db.Where("snapshot_version < ?", ignoreSnapshotVersion).
		Delete(&models.IgnoredIdentifier{})
	if res.Error != nil {
		return 0
	}
	return int(res.RowsAffected)
}

// RestoreIgnore 恢复（删除）忽略条目；id 不存在时返回 ErrIgnoreNotFound（容错）
func RestoreIgnore(db *gorm.DB, id string) error {
	if db == nil {
		return fmt.Errorf("非法参数：db 不能为空")
	}
	res := db.Delete(&models.IgnoredIdentifier{}, "id = ?", id)
	if res.Error != nil {
		return fmt.Errorf("恢复忽略条目失败: %v", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrIgnoreNotFound
	}
	return nil
}

// ── 成员集合工具 ──

func dedupeComicIDs(ids []string) []string {
	seen := make(map[string]bool, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// unionComicIDs 合并两个 JSON 数组形式的成员集合（去重 + 排序）
func unionComicIDs(raw string, ids []string) []string {
	merged := make([]string, 0, len(ids)+4)
	merged = append(merged, ids...)
	raw = strings.TrimSpace(raw)
	if raw != "" {
		var old []string
		if err := json.Unmarshal([]byte(raw), &old); err == nil {
			merged = append(merged, old...)
		}
	}
	return dedupeComicIDs(merged)
}

func parseSeenComicIDs(raw string) map[string]bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var ids []string
	if err := json.Unmarshal([]byte(raw), &ids); err != nil {
		return nil
	}
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		if id != "" {
			out[id] = true
		}
	}
	return out
}

// loadDedupCandidates 查重候选集：离线来源 + 「离线维护」开关过滤（与维护查重**本地口径**一致）
//
// Round50（G）：与维护查重同步改为 filterOfflineLocalCandidates（不排除老化/失效本）——
// 否则忽略成员视图与「忽略后新增」判定会与聚类输入不一致。
func loadDedupCandidates(db *gorm.DB) ([]models.OfflineComic, error) {
	if db == nil {
		return nil, fmt.Errorf("非法参数：db 不能为空")
	}
	// 表不存在（如仅迁移了忽略表的隔离测试库）→ 视为无候选，不报错
	if !db.Migrator().HasTable(&models.OfflineComic{}) {
		return nil, nil
	}
	var comics []models.OfflineComic
	if err := db.Where("source = ?", models.SourceOffline).Order("updated_at desc").Find(&comics).Error; err != nil {
		return nil, fmt.Errorf("读取离线漫画失败: %v", err)
	}
	return filterOfflineLocalCandidates(db, comics), nil
}

func newIgnoreMember(c *models.OfflineComic) IgnoreMemberView {
	tags := UnmarshalTagSlice(c.OnlineTags)
	pc := c.OriginalPageCount
	if pc <= 0 {
		pc = c.PageCount
	}
	return IgnoreMemberView{
		Comic:     *c,
		PageCount: pc,
		Lang:      extractNamespaceTag(tags, "language"),
		Artist:    extractNamespaceTag(tags, "artist"),
	}
}

// ── 查重期内存索引（每次维护查重开始时加载一次） ──

// TitleIgnoreEntry 一条 title 型忽略的运行时视图
type TitleIgnoreEntry struct {
	ID        string
	TitleKey  string
	Artist    string
	CreatedAt int64           // 忽略创建时间戳(ms)：「新增」判定的时间基线
	seen      map[string]bool // 成员集合（忽略时该簇的全部成员 id）
}

// Evaluate 评估一个簇与忽略集合的关系。
//
// covered=true：簇成员全部在集合内 → 应静默。
// covered=false：存在集合外成员 → 应列出；newCount 为其中「忽略之后新入库」的成员数。
// 保守口径：AddedAt 缺失或早于忽略创建时间 → 不计入新增。
func (e *TitleIgnoreEntry) Evaluate(members []ClusterMember) (covered bool, newCount int) {
	if e == nil || len(e.seen) == 0 {
		return false, 0
	}
	covered = true
	for i := range members {
		m := &members[i]
		if e.seen[m.Comic.ID] {
			continue
		}
		covered = false
		if m.addedAtMs > 0 && m.addedAtMs > e.CreatedAt {
			newCount++
		}
	}
	return covered, newCount
}

// IgnoreIndex 忽略索引：title 型按成员集合匹配，gid 型按 gid 匹配
type IgnoreIndex struct {
	titleEntries map[string]*TitleIgnoreEntry // key: 忽略条目 ID
	gids         map[string]bool              // key: gid
}

// LoadIgnoreIndex 加载全部忽略条目到内存索引
func LoadIgnoreIndex(db *gorm.DB) *IgnoreIndex {
	idx := &IgnoreIndex{
		titleEntries: map[string]*TitleIgnoreEntry{},
		gids:         map[string]bool{},
	}
	if db == nil {
		return idx
	}
	var list []models.IgnoredIdentifier
	if err := db.Find(&list).Error; err != nil {
		return idx
	}
	for _, ig := range list {
		switch ig.Type {
		case "title":
			idx.titleEntries[ig.ID] = &TitleIgnoreEntry{
				ID:        ig.ID,
				TitleKey:  ig.TitleKey,
				Artist:    ig.Artist,
				CreatedAt: ig.CreatedAt.UnixMilli(),
				seen:      parseSeenComicIDs(ig.SeenComicIDs),
			}
		case "gid":
			idx.gids[ig.GID] = true
		}
	}
	return idx
}

// MatchCluster 找与簇成员交集最大的忽略条目；无交集返回 nil。
func (idx *IgnoreIndex) MatchCluster(members []ClusterMember) *TitleIgnoreEntry {
	if idx == nil || len(members) == 0 {
		return nil
	}
	idSet := make(map[string]bool, len(members))
	for i := range members {
		idSet[members[i].Comic.ID] = true
	}
	var best *TitleIgnoreEntry
	bestInter := 0
	for _, e := range idx.titleEntries {
		if e == nil || len(e.seen) == 0 {
			continue
		}
		inter := 0
		for id := range e.seen {
			if idSet[id] {
				inter++
			}
		}
		if inter > bestInter {
			bestInter, best = inter, e
		}
	}
	return best
}

// IsGIDIgnored gid 型命中（父画廊 gid，规则 3 例外）
func (idx *IgnoreIndex) IsGIDIgnored(gid string) bool {
	return idx != nil && idx.gids[gid]
}
