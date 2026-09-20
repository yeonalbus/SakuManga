package services

import (
	"fmt"
	"regexp"
	"strings"

	"SakuManga/internal/models"
)

// ─────────────────────────────────────────────────────────────
// Round42 Tier 2：近似层（多字段加权打分 · 概率证据 · 分档输出）
//
// 定位：Tier 1（精确归一键 + 作者硬否决）只能覆盖「剥净版本标记后完全同键」的情形；
// 对「括注增删」（如 `(Elsword)` 有/无）、「罗马音 vs 日文混排」等近似情形无能为力。
// Tier 2 在同一作者桶内用多字段相似度打分，达阈值即产出疑似重复簇，并按分数分 high/medium 档。
//
// 风险控制（对应 plans/round42-* 的决策）：
//  1. **作者硬否决优先**（D10）：只在同一 artist 桶内比较；artist 缺失归入「未知作者桶」，
//     可与有 artist 的桶比较但不因此把两个不同作者的书并到一起（未知桶内比较时要求双方至少一方无 tag）。
//  2. **卷号隔离**：`volume` 不同直接跳过；且「去掉数字/罗马数字后主干相同、但数字不同」视为
//     同系列不同卷（如 `蟲鳥 14` vs `蟲鳥 15`、`魔法少女19.0` vs `21.0`）→ 跳过，避免误报。
//  3. **页数容忍**：差值 > dedupPageTolerance 直接跳过。
//  4. **阈值标定**：由 plans/round42-dedup-calibration-set.md 的 P/N 样本标定（见常量注释）。
// ─────────────────────────────────────────────────────────────

const (
	// dedupScoreHigh 高置信阈值（≥ 视为「很可能重复」，前端按 high 展示）
	dedupScoreHigh = 0.85
	// dedupScoreMedium 中置信阈值（≥ 且 < high 视为「疑似重复」，前端 medium 展示）；低于此值丢弃
	dedupScoreMedium = 0.70
	// dedupBodySimMin 正文相似度硬门槛（去括注后的核心名，两字段取最大）。
	// 标定依据：括注差异多为**元数据**（原作名/社团标注增删，如 `(Elsword)` 有无）→ 不该否决；
	// 而正文差异多为**内容差异**（续篇/加笔，如 `… 甘露寺蜜璃` vs `… 甘露寺蜜璃 妊娠中`、
	// `… saimin 3 …` vs `… saimin 4 …`）→ 必须否决。实测正样本正文相似度 1.00、上述误报样本 0.67~0.80。
	dedupBodySimMin = 0.95
)

// reAnyParen 任意圆/方括号/中文方头括号及其内容（此处 core 已剥净版本标记，残留括注基本是作品名/原作名）
var reAnyParen = regexp.MustCompile(`[\(\[【]([^\)\]】]*)[\)\]】]`)

// bodyOf 取「正文」= 去掉所有括注后的核心名（折叠空白）
func bodyOf(core string) string {
	return strings.Join(strings.Fields(reAnyParen.ReplaceAllString(core, " ")), " ")
}

// parenSet 取括注内的 token 集合（作品名/原作名等元数据）
func parenSet(core string) map[string]bool {
	out := map[string]bool{}
	for _, m := range reAnyParen.FindAllStringSubmatch(core, -1) {
		for _, t := range strings.Fields(m[1]) {
			out[t] = true
		}
	}
	return out
}

// bodySimOf 正文字段相似度：title_jpn 与 title 两两组合取最大
func bodySimOf(a, b *approxCandidate) float64 {
	best := 0.0
	for _, x := range []string{a.coreJp, a.coreTt} {
		if x == "" {
			continue
		}
		xs := tokenSet(bodyOf(x))
		if len(xs) == 0 {
			continue
		}
		for _, y := range []string{b.coreJp, b.coreTt} {
			if y == "" {
				continue
			}
			if v := jaccard(xs, tokenSet(bodyOf(y))); v > best {
				best = v
			}
		}
	}
	return best
}

// parenSimOf 括注相似度（辅助加分项，不用作否决）
func parenSimOf(a, b *approxCandidate) float64 {
	best := 0.0
	for _, x := range []string{a.coreJp, a.coreTt} {
		if x == "" {
			continue
		}
		xs := parenSet(x)
		for _, y := range []string{b.coreJp, b.coreTt} {
			if y == "" {
				continue
			}
			if v := jaccard(xs, parenSet(y)); v > best {
				best = v
			}
		}
	}
	return best
}

// approxCandidate Tier 2 候选（多字段特征快照）
type approxCandidate struct {
	comic    *models.OfflineComic
	coreJp   string // title_jpn 归一核心名（可能为空）
	coreTt   string // title 归一核心名
	vol      string // 卷号
	volJp    string
	volTt    string
	artist   string
	pages    int
	size     int64
	tags     map[string]bool
	noArtist bool
}

// tokenSet 空白分词集合（归一核心名已是小写 + 折叠空白）
func tokenSet(s string) map[string]bool {
	out := map[string]bool{}
	for _, t := range strings.Fields(s) {
		out[t] = true
	}
	return out
}

func jaccard(a, b map[string]bool) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	inter := 0
	for k := range a {
		if b[k] {
			inter++
		}
	}
	return float64(inter) / float64(len(a)+len(b)-inter)
}

// ratio 比例（较小值/较大值）；任一方为 0 时返回 0（视为无信息）
func ratio64(a, b int64) float64 {
	if a <= 0 || b <= 0 {
		return 0
	}
	if a > b {
		a, b = b, a
	}
	return float64(a) / float64(b)
}

// digitsStripped 去掉数字与罗马数字 token（用于识别「同系列不同卷」）
func digitsStripped(s string) string {
	var b strings.Builder
	for _, t := range strings.Fields(s) {
		if isNumericToken(t) {
			continue
		}
		b.WriteString(t)
		b.WriteString(" ")
	}
	return strings.TrimSpace(b.String())
}

// isNumericToken 纯数字、带 # 前缀的数字（归一化产物）或罗马数字 token
func isNumericToken(t string) bool {
	if t == "" {
		return false
	}
	allDigit := true
	for _, r := range t {
		if r < '0' || r > '9' {
			allDigit = false
			break
		}
	}
	if allDigit {
		return true
	}
	// 罗马数字（i/v/x/l/c/d/m 组成且长度 ≤6，避免误判作品名内嵌字母）
	if len(t) <= 6 {
		ok := true
		for _, r := range t {
			switch r {
			case 'i', 'v', 'x', 'l', 'c', 'd', 'm':
			default:
				ok = false
			}
		}
		if ok {
			return true
		}
	}
	return false
}

// scorePair 多字段加权总分 + 正文相似度。
// 权重经标定集校准：正文（去括注）为主判据，括注/标签/页数/体积为辅助；
// 括注仅**加分**不扣分——括注差异属元数据（原作名/社团标注增删），不构成否决。
func scorePair(a, b *approxCandidate) (score, bodySim float64) {
	bodySim = bodySimOf(a, b)
	parenSim := parenSimOf(a, b)
	tagSim := jaccard(a.tags, b.tags)
	pageRatio := ratio64(int64(a.pages), int64(b.pages))
	sizeRatio := ratio64(a.size, b.size)
	score = 0.60*bodySim + 0.10*parenSim + 0.10*tagSim + 0.15*pageRatio + 0.05*sizeRatio
	return score, bodySim
}

// newApproxCandidate 由离线漫画构造 Tier 2 候选（忽略成员或核心名为空者返回 nil）
func newApproxCandidate(c *models.OfflineComic, ignoreIdx *IgnoreIndex) *approxCandidate {
	if ignoreIdx != nil && ignoreIdx.IsComicIgnored(c.ID) {
		return nil
	}
	tags := UnmarshalTagSlice(c.OnlineTags)
	artist := extractNamespaceTag(tags, "artist")

	fpJp := fingerprintTitle(c.TitleJpn, artist)
	fpTt := fingerprintTitle(c.Title, artist)
	coreJp, coreTt := fpJp.Core, fpTt.Core
	if coreJp == "" && coreTt == "" {
		return nil
	}
	tagSet := map[string]bool{}
	for _, t := range tags {
		if s := strings.ToLower(strings.TrimSpace(t)); s != "" {
			tagSet[s] = true
		}
	}
	pages := c.OriginalPageCount
	if pages <= 0 {
		pages = c.PageCount
	}
	vol := fpJp.Volume
	if vol == "" {
		vol = fpTt.Volume
	}
	return &approxCandidate{
		comic:    c,
		coreJp:   coreJp,
		coreTt:   coreTt,
		vol:      vol,
		volJp:    fpJp.Volume,
		volTt:    fpTt.Volume,
		artist:   strings.ToLower(strings.TrimSpace(artist)),
		pages:    pages,
		size:     c.FileSize,
		tags:     tagSet,
		noArtist: strings.TrimSpace(artist) == "",
	}
}

// approxComparable 判断一对候选是否允许比较（作者硬否决 + 卷号隔离 + 页数容忍）
func approxComparable(a, b *approxCandidate) bool {
	// 作者硬否决（D10）：双方 artist 都非空且不同 → 不比
	if a.artist != "" && b.artist != "" && a.artist != b.artist {
		return false
	}
	// 卷号隔离：任一方识别出卷号且两者不同 → 不比
	if a.vol != "" || b.vol != "" {
		if a.vol != b.vol {
			return false
		}
	}
	// 「同系列不同卷」防护：主干（去数字/罗马数字）相同但正文不同 → 视为不同卷
	if a.coreJp != "" && b.coreJp != "" && a.coreJp != b.coreJp &&
		digitsStripped(a.coreJp) == digitsStripped(b.coreJp) {
		return false
	}
	if a.coreTt != "" && b.coreTt != "" && a.coreTt != b.coreTt &&
		digitsStripped(a.coreTt) == digitsStripped(b.coreTt) {
		return false
	}
	// 页数容忍
	if a.pages > 0 && b.pages > 0 {
		d := a.pages - b.pages
		if d < 0 {
			d = -d
		}
		if d > dedupPageTolerance {
			return false
		}
	}
	return true
}

// detectApproxClusters Tier 2 主入口：产出「Tier 1 未覆盖」的近似重复簇。
//
// exclude 为已被 Tier 1 聚入簇的漫画 ID（不重复参与，避免同一条目出现在两个簇里）。
func detectApproxClusters(comics []models.OfflineComic, exclude map[string]bool, ignoreIdx *IgnoreIndex, forceFull bool) []DedupCluster {
	cands := make([]*approxCandidate, 0, len(comics))
	for i := range comics {
		c := &comics[i]
		if exclude[c.ID] {
			continue
		}
		// 忽略标记（title 型）在簇生成后统一过滤，这里只做成员级排除
		if ac := newApproxCandidate(c, ignoreIdx); ac != nil {
			cands = append(cands, ac)
		}
	}
	if len(cands) < 2 {
		return nil
	}

	// 作者分桶：同桶内两两比较；无 artist 者进「未知作者桶」，且需与其它桶单独比较
	buckets := map[string][]int{}
	for i, c := range cands {
		buckets[c.artist] = append(buckets[c.artist], i)
	}

	parent := make([]int, len(cands))
	for i := range parent {
		parent[i] = i
	}
	find := func(x int) int {
		for parent[x] != x {
			parent[x] = parent[parent[x]]
			x = parent[x]
		}
		return x
	}
	union := func(a, b int) {
		ra, rb := find(a), find(b)
		if ra != rb {
			parent[rb] = ra
		}
	}

	// pairScore 记录簇内最低分（用于分档）
	bestPair := map[[2]int]float64{}
	consider := func(i, j int) {
		a, b := cands[i], cands[j]
		if !approxComparable(a, b) {
			return
		}
		score, bodySim := scorePair(a, b)
		// 硬门槛：正文（去括注）相似度不足 → 不做近似判定（防续篇/不同卷误报）
		if bodySim < dedupBodySimMin || score < dedupScoreMedium {
			return
		}
		union(i, j)
		key := [2]int{i, j}
		if i > j {
			key = [2]int{j, i}
		}
		bestPair[key] = score
	}

	// 同桶内两两（含未知作者桶内部）
	for _, idxs := range buckets {
		for a := 0; a < len(idxs); a++ {
			for b := a + 1; b < len(idxs); b++ {
				consider(idxs[a], idxs[b])
			}
		}
	}
	// 未知作者桶 ↔ 其它桶（跨桶：仅当一方无 artist tag 时允许，符合 D10 语义）
	if unknown, ok := buckets[""]; ok {
		for artist, idxs := range buckets {
			if artist == "" {
				continue
			}
			for _, i := range unknown {
				for _, j := range idxs {
					consider(i, j)
				}
			}
		}
	}

	// 聚合 + 分档
	groups := map[int][]int{}
	roots := make([]int, 0, len(cands))
	for i := range cands {
		r := find(i)
		if _, ok := groups[r]; !ok {
			roots = append(roots, r)
		}
		groups[r] = append(groups[r], i)
	}

	var clusters []DedupCluster
	for _, r := range roots {
		members := groups[r]
		if len(members) < 2 {
			continue
		}
		// 簇内最小相似分 → 置信度
		minScore := 1.0
		hasUnknown := false
		for a := 0; a < len(members); a++ {
			if cands[members[a]].noArtist {
				hasUnknown = true
			}
			for b := a + 1; b < len(members); b++ {
				key := [2]int{members[a], members[b]}
				if members[a] > members[b] {
					key = [2]int{members[b], members[a]}
				}
				s, ok := bestPair[key]
				if !ok {
					// 通过中间节点连通（并查集传递）→ 该对无直接相似证据，按最低档处理
					s = dedupScoreMedium
				}
				if s < minScore {
					minScore = s
				}
			}
		}
		confidence := "medium"
		if minScore >= dedupScoreHigh {
			confidence = "high"
		}
		reason := fmt.Sprintf("标题/标签多重相似（最低配对分 %.2f，阈值 %.2f），疑似同作品不同版本，请人工确认",
			minScore, dedupScoreHigh)
		if hasUnknown {
			reason += "；**含作者未知条目**（无 artist tag），请优先人工确认"
			if confidence == "high" {
				confidence = "medium"
			}
		}

		membersOut := make([]ClusterMember, 0, len(members))
		maxDiff := 0
		for _, idx := range members {
			c := cands[idx]
			tags := UnmarshalTagSlice(c.comic.OnlineTags)
			membersOut = append(membersOut, ClusterMember{
				Comic:     *c.comic,
				PageCount: c.pages,
				Lang:      extractNamespaceTag(tags, "language"),
				Artist:    extractNamespaceTag(tags, "artist"), // Round43：成员各自的画师
			})
		}
		for i := 0; i < len(membersOut); i++ {
			for j := i + 1; j < len(membersOut); j++ {
				d := membersOut[i].PageCount - membersOut[j].PageCount
				if d < 0 {
					d = -d
				}
				if d > maxDiff {
					maxDiff = d
				}
			}
		}
		if maxDiff > dedupPageTolerance {
			reason += fmt.Sprintf("；页数差 %d 页（>%d），可能为重绘/合集", maxDiff, dedupPageTolerance)
		}

		// 簇的 TitleKey/Artist 取首个成员（与 Tier 1 语义一致：用于忽略表匹配）
		first := cands[members[0]]
		titleKey := first.coreJp
		if titleKey == "" {
			titleKey = first.coreTt
		}
		cl := DedupCluster{
			TitleKey:   titleKey,
			Artist:     first.artist,
			Confidence: confidence,
			Reason:     reason,
			Members:    membersOut,
		}
		if ignoreIdx != nil && ignoreIdx.IsTitleIgnored(cl.TitleKey, cl.Artist) {
			if !forceFull {
				continue
			}
			cl.Ignored = true
		}
		clusters = append(clusters, cl)
	}
	return clusters
}
