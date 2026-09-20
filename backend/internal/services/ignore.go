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
// 忽略标记（Round26 O2 建立，Round44 升级为「成员快照 + 新增感知」）
//
// 作用域：仅疑似重复判定（O3 名称级簇，Tier1 + Tier2）+ 规则 3 父子画廊；
// 规则 1/2/4（同 GID/hash/内容签名）与更新检测不读取本表。
//
// Round44 语义变更（珱垣拍板）：
//   - 旧：命中 title 型忽略 → 簇直接静默（增量）/ 仍列出并标 Ignored（全量核对）。
//     结果：已忽略的组永远占位，"待处理数"永远清不到 0；同作品新入库的重复也不会被提示。
//   - 新：命中 title 型忽略 → 比对「忽略当时的成员快照」：
//     簇成员全在快照内 → 静默跳过；
//     出现快照外的成员（忽略之后新入库）→ 照常列出，标记 Ignored=true + IgnoredNewCount。
//     用户点「确认新增」刷新快照后继续静默。
//   - 由此不再需要"全量核对永久列出已忽略项"这条兜底，主列表可以真正清零。
// ─────────────────────────────────────────────────────────────

// ErrIgnoreNotFound 忽略条目不存在（恢复时容错）
var ErrIgnoreNotFound = errors.New("忽略条目不存在")

// CreateIgnore 新增忽略条目（幂等：同 type+key 已存在则返回现有条目）
//
// Round44：title 型条目在创建时即记录当前匹配到的本地漫画 id 集合（成员快照），
// 作为后续「新增感知」的比较基线。
func CreateIgnore(db *gorm.DB, typ, titleKey, artist, gid, comicID, note string) (*models.IgnoredIdentifier, error) {
	if db == nil {
		return nil, fmt.Errorf("非法参数：db 不能为空")
	}
	typ = strings.TrimSpace(typ)
	switch typ {
	case "title":
		titleKey = strings.TrimSpace(titleKey)
		if titleKey == "" {
			return nil, fmt.Errorf("非法参数：title 类型必须提供核心名")
		}
	case "gid":
		gid = strings.TrimSpace(gid)
		if gid == "" {
			return nil, fmt.Errorf("非法参数：gid 类型必须提供画廊 gid")
		}
	case "comic":
		comicID = strings.TrimSpace(comicID)
		if comicID == "" {
			return nil, fmt.Errorf("非法参数：comic 类型必须提供本地漫画 id")
		}
	default:
		return nil, fmt.Errorf("非法参数：忽略类型仅支持 title / gid / comic")
	}

	// 幂等去重：同类型同 key 不重复插入
	var existing models.IgnoredIdentifier
	q := db.Where("type = ?", typ)
	switch typ {
	case "title":
		q = q.Where("title_key = ? AND artist = ?", titleKey, artist)
	case "gid":
		q = q.Where("g_id = ?", gid)
	case "comic":
		q = q.Where("comic_id = ?", comicID)
	}
	if err := q.First(&existing).Error; err == nil {
		return &existing, nil
	}

	rec := &models.IgnoredIdentifier{
		// 主键唯一性：UnixNano 在 Windows 上受系统时钟 tick 精度限制（约 15.6ms），
		// 同一 tick 内连续创建不同条目会生成相同 ID 触发 UNIQUE 冲突（既有 flaky 测试根因）。
		// 追加随机后缀保证同 tick 内多次创建也唯一（Go 1.20+ math/rand 自动随机种子）。
		ID:        fmt.Sprintf("ignore-%d-%d", time.Now().UnixNano(), rand.Int63()),
		Type:      typ,
		TitleKey:  titleKey,
		Artist:    artist,
		GID:       gid,
		ComicID:   comicID,
		Note:      strings.TrimSpace(note),
		CreatedAt: time.Now(),
	}
	// Round44：title 型记录成员快照（当前匹配到的本子集合）
	if typ == "title" {
		ids := matchTitleMemberIDs(db, titleKey, artist)
		if b, err := json.Marshal(ids); err == nil {
			rec.SeenComicIDs = string(b)
		}
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

// ── Round44：忽略清单成员视图（宽松口径） ──

// IgnoreMemberView 忽略清单里的一条成员（宽松口径：指纹匹配到的全部本子）
type IgnoreMemberView struct {
	Comic     models.OfflineComic `json:"comic"`
	PageCount int                 `json:"pageCount"` // 物理页数（OriginalPageCount 优先）
	Lang      string              `json:"lang,omitempty"`
	Artist    string              `json:"artist,omitempty"`
	Grouped   bool                `json:"grouped"` // 与同条目下其他成员构成同组（真的会判为重复）
	IsNew     bool                `json:"isNew"`   // 忽略之后新入库（新增感知）
}

// IgnoreItemView 忽略清单条目视图
type IgnoreItemView struct {
	models.IgnoredIdentifier
	ComicTitle   string             `json:"comicTitle,omitempty"` // comic 型：被忽略漫画的标题（后端补查）
	Members      []IgnoreMemberView `json:"members,omitempty"`    // 宽松口径成员列表
	MatchedCount int                `json:"matchedCount"`         // 覆盖本数
	GroupCount   int                `json:"groupCount"`           // 其中构成重复的组数
	GroupedCount int                `json:"groupedCount"`         // 构成重复的本数
	NewCount     int                `json:"newCount"`             // 忽略后新增本数
}

// ListIgnoresWithMembers 忽略清单（成员卡片数据源）。
//
// 口径说明（珱垣已确认的"宽松口径 + 标注"）：
//   - title 型：列出「核心名 + 画师」指纹匹配到的**全部**本地本子；其中按 clusterKey（含卷号）
//     能与同条目其他成员成组的标 Grouped=true（这些本之间才真的会判为疑似重复），
//     其余只是同作品/同指纹但不会聚成一组；
//   - gid 型：列出本地 g_id 相同的本子；
//   - comic 型：查该 id（记录可能已被删除 → 成员为空，前端提示已失效）。
//
// 性能：一次全库指纹扫描建索引，再按条目查表（数千本量级约百毫秒）。
func ListIgnoresWithMembers(db *gorm.DB) ([]IgnoreItemView, error) {
	list, err := ListIgnores(db)
	if err != nil {
		return nil, err
	}
	comics, cerr := loadDedupCandidates(db)
	if cerr != nil {
		comics = nil // 读库失败时降级为纯清单（不带成员），不阻断清单展示
	}

	// 一次全库扫描：title 指纹键 → 命中
	idx := buildIgnoreMatchIndex(comics)

	out := make([]IgnoreItemView, 0, len(list))
	for _, ig := range list {
		v := IgnoreItemView{IgnoredIdentifier: ig}
		switch ig.Type {
		case "title":
			hits := idx.lookup(ig.TitleKey, ig.Artist)
			v.Members, v.MatchedCount, v.GroupCount, v.GroupedCount, v.NewCount = buildTitleMembers(hits, ig.SeenComicIDs, ig.CreatedAt.UnixMilli())
		case "gid":
			members := make([]IgnoreMemberView, 0, 1)
			for i := range comics {
				if comics[i].GID != "" && comics[i].GID == ig.GID {
					members = append(members, newIgnoreMember(&comics[i]))
				}
			}
			v.Members, v.MatchedCount = members, len(members)
		case "comic":
			for i := range comics {
				if comics[i].ID == ig.ComicID {
					v.ComicTitle = comics[i].Title
					v.Members = []IgnoreMemberView{newIgnoreMember(&comics[i])}
					v.MatchedCount = 1
					break
				}
			}
			if v.ComicTitle == "" {
				// 记录可能已被删除：单独查一次标题，便于前端提示「已失效」
				var c models.OfflineComic
				if err := db.Select("title").First(&c, "id = ?", ig.ComicID).Error; err == nil {
					v.ComicTitle = c.Title
				}
			}
		}
		out = append(out, v)
	}
	return out, nil
}

// AckIgnoreSnapshot 「确认新增」：把 title 型条目的成员快照刷新为当前匹配集合。
func AckIgnoreSnapshot(db *gorm.DB, id string) (*models.IgnoredIdentifier, error) {
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
		return &rec, nil // 仅 title 型有快照
	}
	ids := matchTitleMemberIDs(db, rec.TitleKey, rec.Artist)
	blob, err := json.Marshal(ids)
	if err != nil {
		return nil, fmt.Errorf("序列化成员快照失败: %v", err)
	}
	if err := db.Model(&models.IgnoredIdentifier{}).Where("id = ?", id).
		Update("seen_comic_ids", string(blob)).Error; err != nil {
		return nil, fmt.Errorf("更新成员快照失败: %v", err)
	}
	rec.SeenComicIDs = string(blob)
	return &rec, nil
}

// BackfillIgnoreSnapshots 回填存量 title 型条目的成员快照（幂等）。
//
// 背景：Round44 之前写入的忽略条目没有快照字段。若把"空快照"直接当作"无已知成员"，
// 所有存量忽略项都会在升级后集体"复活"（簇被列出并标新增）。因此启动时按当前匹配集合
// 回填一次，使升级前后行为连续；此后新入库的本子才会触发新增提示。
// 返回回填条数。
func BackfillIgnoreSnapshots(db *gorm.DB) int {
	if db == nil {
		return 0
	}
	var list []models.IgnoredIdentifier
	if err := db.Where("type = ?", "title").Find(&list).Error; err != nil {
		return 0
	}
	filled := 0
	for _, ig := range list {
		if strings.TrimSpace(ig.SeenComicIDs) != "" {
			continue
		}
		ids := matchTitleMemberIDs(db, ig.TitleKey, ig.Artist)
		blob, err := json.Marshal(ids)
		if err != nil {
			continue
		}
		if err := db.Model(&models.IgnoredIdentifier{}).Where("id = ?", ig.ID).
			Update("seen_comic_ids", string(blob)).Error; err == nil {
			filled++
		}
	}
	return filled
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

// ── 指纹匹配（忽略键 ↔ 本地漫画） ──

// loadDedupCandidates 查重候选集：离线来源 + 「离线维护」开关过滤（与维护查重同口径）
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
	return filterOfflineUpdateEnabled(db, comics), nil
}

// comicIgnoreKeys 单本漫画在忽略表口径下的匹配键（title_jpn 优先、title 兜底；artist 必须一致）
func comicIgnoreKeys(c *models.OfflineComic) []string {
	tags := UnmarshalTagSlice(c.OnlineTags)
	artist := extractNamespaceTag(tags, "artist")
	keys := make([]string, 0, 2)
	for _, src := range []string{c.TitleJpn, c.Title} {
		if strings.TrimSpace(src) == "" {
			continue
		}
		fp := fingerprintTitle(src, artist)
		if fp.Core == "" {
			continue
		}
		keys = append(keys, titleIgnoreKey(fp.Core, artist))
	}
	return keys
}

// ignoreLookupKeys 一条 title 型忽略的查询键：精确键 + Round42 D8 归一键（存量旧 key 兼容）
func ignoreLookupKeys(titleKey, artist string) []string {
	keys := []string{titleIgnoreKey(titleKey, artist)}
	if fp := fingerprintTitle(titleKey, artist); fp.Core != "" {
		if k := titleIgnoreKey(fp.Core, artist); k != keys[0] {
			keys = append(keys, k)
		}
	}
	return keys
}

// matchTitleMemberIDs 一条 title 型忽略当前匹配到的本地漫画 id（成员快照的数据源）
func matchTitleMemberIDs(db *gorm.DB, titleKey, artist string) []string {
	comics, err := loadDedupCandidates(db)
	if err != nil {
		return []string{}
	}
	want := map[string]bool{}
	for _, k := range ignoreLookupKeys(titleKey, artist) {
		want[k] = true
	}
	out := make([]string, 0, 4)
	for i := range comics {
		for _, k := range comicIgnoreKeys(&comics[i]) {
			if want[k] {
				out = append(out, comics[i].ID)
				break
			}
		}
	}
	sort.Strings(out)
	return out
}

// ignoreMatchHit 全库索引里的一条命中
type ignoreMatchHit struct {
	comic *models.OfflineComic
	clusterKey string // clusterKey(fp)：核心名 + 卷号 + 画师（同组判定）
}

// ignoreMatchIndex title 指纹键 → 命中列表（一次全库扫描，供清单批量查询）
type ignoreMatchIndex map[string][]ignoreMatchHit

func buildIgnoreMatchIndex(comics []models.OfflineComic) ignoreMatchIndex {
	idx := make(ignoreMatchIndex, len(comics))
	for i := range comics {
		c := &comics[i]
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
			idx[titleIgnoreKey(fp.Core, artist)] = append(idx[titleIgnoreKey(fp.Core, artist)],
				ignoreMatchHit{comic: c, clusterKey: clusterKey(fp)})
		}
	}
	return idx
}

// lookup 按忽略条目取成员（精确键 + 归一键，按 comic id 去重）
func (idx ignoreMatchIndex) lookup(titleKey, artist string) []ignoreMatchHit {
	if len(idx) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]ignoreMatchHit, 0, 4)
	for _, k := range ignoreLookupKeys(titleKey, artist) {
		for _, h := range idx[k] {
			if seen[h.comic.ID] {
				continue
			}
			seen[h.comic.ID] = true
			out = append(out, h)
		}
	}
	return out
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

// buildTitleMembers 组装 title 型条目的成员视图 + 统计（含同组标注与新增感知）
// createdAt 为该忽略条目的创建时间戳(ms)：新增＝不在快照内 **且** 忽略之后才入库（见 CountNewMembers 注释）。
func buildTitleMembers(hits []ignoreMatchHit, seenRaw string, createdAt int64) (members []IgnoreMemberView, matched, groupCount, groupedCount, newCount int) {
	seen := parseSeenComicIDs(seenRaw)
	// 同组判定：按 clusterKey 分组，组内 ≥2 本即构成重复
	byKey := map[string][]int{}
	for i, h := range hits {
		byKey[h.clusterKey] = append(byKey[h.clusterKey], i)
	}
	grouped := make([]bool, len(hits))
	for _, ids := range byKey {
		if len(ids) < 2 {
			continue
		}
		groupCount++
		groupedCount += len(ids)
		for _, i := range ids {
			grouped[i] = true
		}
	}
	members = make([]IgnoreMemberView, 0, len(hits))
	for i, h := range hits {
		m := newIgnoreMember(h.comic)
		m.Grouped = grouped[i]
		// 新增感知：快照存在、该本不在快照内、且是忽略之后才入库的（见 CountNewMembers 注释）
		if len(seen) > 0 && !seen[h.comic.ID] && isAddedAfter(h.comic.AddedAt, createdAt) {
			m.IsNew = true
			newCount++
		}
		members = append(members, m)
	}
	return members, len(members), groupCount, groupedCount, newCount
}

// isAddedAfter 本子是否在给定时间戳(ms)之后入库；AddedAt 为零值时保守返回 false（不算新增）
func isAddedAfter(addedAt time.Time, ts int64) bool {
	if addedAt.IsZero() || ts <= 0 {
		return false
	}
	return addedAt.UnixMilli() > ts
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

// ── 查重期内存索引（每次维护查重开始时加载一次） ──

// TitleIgnoreEntry 一条 title 型忽略的运行时视图（含成员快照）
type TitleIgnoreEntry struct {
	ID        string
	TitleKey  string
	Artist    string
	CreatedAt int64           // 忽略创建时间戳(ms)：新增感知的时间基线
	seen      map[string]bool // 成员快照（忽略当时匹配到的 comic id）
}

// CountNewMembers 统计簇内「忽略之后才入库、且不在快照内」的成员数。
//
// 为什么必须叠加「入库时间晚于忽略时间」这一条（Round44 实测修正）：
// 忽略键是「核心名 + 组级 artist」，而簇内成员的 artist 可能各自不同
// （如「作者未知兜底」合并出的组：组级 artist 取首成员，其余成员 artist 为空或不同）——
// 此时那些成员不会被该忽略键匹配到，快照里自然没有它们的 id。
// 若只按「不在快照内」判定，这类组在忽略后会立刻被误报为「新增」，形成假阳性提醒。
// 因此新增的准确语义是：**忽略之后新入库的本子**（快照缺失 + 入库时间晚于忽略创建时间）。
func (e *TitleIgnoreEntry) CountNewMembers(members []ClusterMember) int {
	if e == nil {
		return 0
	}
	// 快照为空（存量条目尚未回填 / 测试库无候选集）→ 不参与新增感知，保守按"无新增"处理，
	// 避免把整批已忽略的组重新炸出来。
	if len(e.seen) == 0 {
		return 0
	}
	n := 0
	for i := range members {
		m := members[i]
		if !m.Comic.AddedAt.IsZero() && m.Comic.AddedAt.UnixMilli() <= e.CreatedAt {
			continue // 忽略之前就在库里的本子 → 不算新增
		}
		if e.seen[m.Comic.ID] {
			continue // 已在快照内（忽略时已知悉）
		}
		n++
	}
	return n
}

// IgnoreIndex 忽略索引：title 型按「核心名+画师」匹配，gid 型按 gid 匹配，comic 型按漫画 id 匹配
type IgnoreIndex struct {
	titleEntries     map[string]*TitleIgnoreEntry // key: titleIgnoreKey（原样）
	titleEntriesNorm map[string]*TitleIgnoreEntry // Round42 D8：存量旧 key 经新清洗归一后的键
	gids             map[string]bool              // key: gid
	comicIDs         map[string]bool              // key: comicID（Round26-2：成员级忽略，聚类时剔除）
}

// LoadIgnoreIndex 加载全部忽略条目到内存索引
func LoadIgnoreIndex(db *gorm.DB) *IgnoreIndex {
	idx := &IgnoreIndex{
		titleEntries:     map[string]*TitleIgnoreEntry{},
		titleEntriesNorm: map[string]*TitleIgnoreEntry{},
		gids:             map[string]bool{},
		comicIDs:         map[string]bool{},
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
			entry := &TitleIgnoreEntry{
				ID:        ig.ID,
				TitleKey:  ig.TitleKey,
				Artist:    ig.Artist,
				CreatedAt: ig.CreatedAt.UnixMilli(),
				seen:      parseSeenComicIDs(ig.SeenComicIDs),
			}
			idx.titleEntries[titleIgnoreKey(ig.TitleKey, ig.Artist)] = entry
			idx.titleEntriesNorm[ignoreTitleKeyOf(ig.TitleKey, ig.Artist)] = entry
		case "gid":
			idx.gids[ig.GID] = true
		case "comic":
			idx.comicIDs[ig.ComicID] = true
		}
	}
	return idx
}

func titleIgnoreKey(titleKey, artist string) string {
	return strings.ToLower(strings.TrimSpace(titleKey)) + "\x00" + strings.ToLower(strings.TrimSpace(artist))
}

// ignoreTitleKeyOf 归一化忽略键：把 title_key 过一遍现有清洗（Round42 新算法）后再组键。
// 用于「存量旧 key ↔ 新算法产出的 key」对齐；清洗后为空（极端输入）时退回原值，避免误匹配。
func ignoreTitleKeyOf(titleKey, artist string) string {
	core := fingerprintTitle(titleKey, artist).Core
	if core == "" {
		core = strings.TrimSpace(titleKey)
	}
	return titleIgnoreKey(core, artist)
}

// MatchTitle 命中 title 型忽略时返回条目（先精确键，再归一键兜底）；未命中返回 nil
func (idx *IgnoreIndex) MatchTitle(titleKey, artist string) *TitleIgnoreEntry {
	if idx == nil {
		return nil
	}
	if e, ok := idx.titleEntries[titleIgnoreKey(titleKey, artist)]; ok {
		return e
	}
	return idx.titleEntriesNorm[ignoreTitleKeyOf(titleKey, artist)]
}

// IsTitleIgnored title 型命中（不论是否新增成员）。
// Round44 起，聚类侧应改用 MatchTitle + CountNewMembers 以支持「新增感知」；
// 本方法保留给"只关心是否命中"的调用方（如在线复核计数）。
func (idx *IgnoreIndex) IsTitleIgnored(titleKey, artist string) bool {
	return idx.MatchTitle(titleKey, artist) != nil
}

// IsGIDIgnored gid 型命中（父画廊 gid，规则 3 例外）
func (idx *IgnoreIndex) IsGIDIgnored(gid string) bool {
	return idx != nil && idx.gids[gid]
}

// IsComicIgnored comic 型命中（成员级忽略：该漫画不参与名称级聚类）
func (idx *IgnoreIndex) IsComicIgnored(comicID string) bool {
	return idx != nil && idx.comicIDs[comicID]
}
