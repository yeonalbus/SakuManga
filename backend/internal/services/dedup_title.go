package services

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"SakuManga/internal/models"
)

// ─────────────────────────────────────────────────────────────
// Round26 O3：疑似重复（名称级）——清洗层 / 判定层 / 决策层
//
// 定位：纯本地、纯建议的弱证据规则，产出「疑似重复组」（DedupCluster）；
// 与规则 1/2/3/4（强证据）互不干扰；永远只出建议，不自动删除/忽略。
//
// 清洗层 fingerprintTitle：
//   - 画师一律取自 OnlineTags 的 artist:xxx（E 站标准化，比从标题猜可靠）；
//   - 标题开头方括号前缀（社团/画师/汉化组混排，无法可靠区分）整体剥离，
//     画师维度由 tag 兜底，不丢失判定信息；
//   - 卷号（第N巻/話、Vol.N、Part N、罗马数字、後編/上巻等）提取为独立特征；
//   - 版本后缀（(C94)、[Digital]、DL版、语言、reupload/remaster、汉化组名等）剥离；
//   - 罗马数字 → 阿拉伯、全角 → 半角、大小写/标点归一。
//
// 判定层 detectTitleClusters：
//   - 分组 key =（归一核心名 + 卷号 + 画师小写）→ 续集/分卷因卷号不同天然隔离，
//     不会误判为重复；多语言版/重传版卷号一致 → 正确聚组。
//   - 组内页数容差：两两页数差 ≤5 为「同」的强支持信号（高置信）；
//     存在 >5 的差异（重绘/合集 vs 单本/双页合并）→ 降为中置信，仍列出。
//
// 决策层：high / medium 两档输出，低置信静默；阈值常量 dedupPageTolerance。
// ─────────────────────────────────────────────────────────────

// dedupPageTolerance 页数容差：≤5 判为「同」的强支持信号（计划书 §4.2 双向不对称页数规则）
const dedupPageTolerance = 5

// DedupCluster 疑似重复组（名称级，弱证据，只建议）
type DedupCluster struct {
	ID         string          `json:"id"`
	TitleKey   string          `json:"titleKey"`         // 归一化核心名（忽略条目标记用）
	Artist     string          `json:"artist,omitempty"` // 共同画师（来自 artist:xxx）
	Confidence string          `json:"confidence"`       // high | medium
	Reason     string          `json:"reason"`           // 判定依据摘要
	Members    []ClusterMember `json:"members"`          // 成员（≥2）
	Ignored    bool            `json:"ignored"`          // 全量核对时命中忽略；增量直接跳过
}

// ClusterMember 疑似重复组单个成员
type ClusterMember struct {
	Comic     models.OfflineComic `json:"comic"`
	PageCount int                 `json:"pageCount"` // 物理页数（OriginalPageCount 优先，隐藏页不影响比对）
	Lang      string              `json:"lang,omitempty"`
}

// TitleFingerprint 标题指纹（清洗层输出）
type TitleFingerprint struct {
	Core   string // 归一化核心名（已剥离卷号/版本后缀）
	Volume string // 卷号特征（空=无；数字=卷/话；後編/上巻 等保留原文；合集=総集編/Compilation 等）
	Artist string // 画师（来自 OnlineTags artist:xxx，原样）
}

// ── 清洗层 ──

var (
	// 开头连续括号/方括号前缀（[circle] / [汉化组] / (C94) / (Artbook) 等），循环剥离
	reBracketPrefix = regexp.MustCompile(`^\s*(?:\([^)]*\)|\[[^\]]*\])\s*`)
	// 日文卷号：第N巻/第N話/第N編/第N回（N 可为阿拉伯或一二三…）
	reVolumeJp = regexp.MustCompile(`第\s*([0-9]+|[一二三四五六七八九十两])\s*[巻卷話话編编回夜章]`)
	// 英文卷号：vol.2 / Vol 2 / V2 / Part 2 / Ep.3 / Chapter 4 / No.5
	reVolumeEn = regexp.MustCompile(`(?i)\b(?:vol\.?|v\.?|no\.?|part|ep\.?|episode|ch\.?|chapter)\s*\.?\s*([0-9]+|[ivxlcdm]+)\b`)
	// 罗马数字独立 token（判定/转阿拉伯用）
	reRomanToken = regexp.MustCompile(`(?i)\b([ivxlcdm]+)\b`)
	// 合集/总集特征
	reOmnibus = regexp.MustCompile(`(?i)総集編|总集编|合集|compilation|complete\s*works|omnibus|anthology|まとめ`)
	// 前/後編 与 上/下巻 特征（卷结构）
	reVolPair = regexp.MustCompile(`[前後上下]編|[前後上下]篇|[前後上下]巻|[前後上下]卷`)
	// 版本后缀 token（逐个剥离，循环到稳定）：展会届数 / 数字版 / DL版 / 语言 / 重传重制 / 汉化组名
	// 安全原则（Round26 审查修复）：英文 token（english/digital/reupload 等）必须「括号包裹」才剥离，
	// 防误伤作品名内嵌单词（如 "Digital Devil Story" 不应被剥成 "Devil Story"）；中文后缀（汉化版/DL版 等）保留无括号剥离。
	reVersionTokens = []*regexp.Regexp{
		// ── Round42：先整块剥离「括号内含量标记」的 token。**顺序关键**：必须排在无括号中文正则之前，
		//    否则 `[空気系☆漢化]` 会先被 "汉化" 剥成 `[空気系☆ ]`、`[廉价汉化组]` 被剥成 `[廉价 组]`，
		//    整块规则之后再也匹配不上（用户报告样本正是被这类残留卡住）。──
		reEventToken,        // 展会/期号：(C94) [C99] (COMIC1☆6) (例大祭7) (コミティア140) (FF46) 等
		reParenVersionToken, // 圆/方括号内含版本/语言/汉化关键词的整块：[無修正] [中国翻訳] [空気系☆漢化] [廉价汉化组] (オリジナル) 等

		regexp.MustCompile(`(?i)\(c\s*\d{2,3}\)`),                                                               // (C94) / (C101)
		regexp.MustCompile(`(?i)\(\[?digital\]?\)|\[digital\]`),                                                 // [Digital] / (Digital)
		regexp.MustCompile(`(?i)dl\s*版`),                                                                        // DL版 / DL 版（无括号）
		regexp.MustCompile(`(?i)\((?:english|chinese|japanese|korean|french|german|spanish|russian|italian)\)`), // 英文语言后缀（须括号包裹）
		regexp.MustCompile(`\(?(?:中文版|汉化版|漢化版|简中版|繁中版|中文|汉化|漢化|简中|繁中)\)?`),                                      // 中文/汉化后缀（长词优先，括号可选——中文词作版本标记概率高）
		regexp.MustCompile(`(?i)\((?:reupload|re-upload|remaster|renewal|re-edition|new\s*edition|reprint|re-release|復刻版|新装版|完全版|修正版|無修正|无修正)\)`), // 版本/重制标记（须括号包裹）
		regexp.MustCompile(`(?i)\bv2\b|(?i)\bver\.?\s*2\b`),                 // v2 / ver.2（无括号）
		regexp.MustCompile(`[\[\]()]?[^\[\]()\s]*汉化组[^\[\]()\s]*[\[\]()]?`), // 汉化组名（如 [萌你妹汉化组]）

		// 无括号中文/汉化后缀补充（放最后：仅在整块规则未命中时才逐词剥离）
		reCnSuffixNoParen,
	}

	// Round42：剥离后残留的空括号必须清理（如 `[DL版]` 被剥成 `[ ]`），
	// 否则同一作品会因差一个 "[ ]" 而无法同键（用户报告样本即卡在这一层）。
	reEmptyParen = regexp.MustCompile(`[\(\[]\s*[\)\]]`)
)

// reEventToken 展会/期号标记（圆括号或方括号），剥净后同一作品的不同首发届数可归一
var reEventToken = regexp.MustCompile(
	`(?i)[\(\[]\s*(?:c\s*\d{2,3}|comic1[☆\s]*\d+|例大祭\s*\d*|コミティア\s*\d*|コミック[^\s\)\]]*\d*|ff\s*\d+|sunshine\s*creation\s*\d*|red\s*box\s*\d*|comic\s*castle\s*\d*|コミックキャッスル\s*\d*)\s*[\)\]]`,
)

// reParenVersionToken 圆/方括号内**含版本/语言/汉化/润色关键词**的整块 token（黑名单式）。
// 只有括注内含下列关键词才剥离；作品名括注（如 `(Fate/Grand Order)`、`(ブルーアーカイブ)`）保持不动。
// 注意：不收英文 `original`（易误伤作品名），只收日文 `オリジナル`。
var reParenVersionToken = regexp.MustCompile(
	`(?i)[\(\[]\s*[^\)\]]*(?:dl\s*版|digital|decensored|uncensored|無修正|无修正|中国翻訳|中国語|中文翻译|翻譯|翻译|translated|chinese|english|japanese|korean|french|german|spanish|russian|italian|汉化|漢化|润色|潤色|机翻|機翻|reupload|re-upload|remaster|renewal|re-edition|reprint|re-release|復刻版|新装版|完全版|修正版|オリジナル|v2)[^\)\]]*[\)\]]`,
)

// reCnSuffixNoParen 无括号的中文/汉化类后缀（中文词作版本标记概率高，故允许无括号剥离）
var reCnSuffixNoParen = regexp.MustCompile(
	`(?i)中国翻訳|中国翻译|中国語|机翻|機翻|ai\s*润色|个人润色|個人潤色|個人翻譯|个人翻译`,
)

// fingerprintTitle 标题 + 画师 → 指纹（纯函数，可测）
func fingerprintTitle(title, artist string) TitleFingerprint {
	t := normalizeTitleForDedup(title)

	// 1) 剥离开头连续括号前缀（循环：可能有多层）
	for changed := true; changed; {
		changed = false
		for {
			loc := reBracketPrefix.FindStringIndex(t)
			if loc == nil {
				break
			}
			t = t[loc[1]:]
			changed = true
		}
	}

	// 2) 提取卷号特征（优先级：合集 > 日文 第N巻 > 英文 vol.N/part N > 前後編/上下巻）
	volume := ""
	if reOmnibus.MatchString(t) {
		volume = "合集"
		t = reOmnibus.ReplaceAllString(t, " ")
	} else if m := reVolumeJp.FindStringSubmatch(t); m != nil {
		volume = cjkOrArabicToNum(m[1])
		t = strings.Replace(t, m[0], " ", 1)
	} else if m := reVolumeEn.FindStringSubmatch(t); m != nil {
		volume = arabicOrRomanToNum(m[1])
		t = strings.Replace(t, m[0], " ", 1)
	} else if m := reVolPair.FindString(t); m != "" {
		volume = m
		t = strings.Replace(t, m, " ", 1)
	}

	// 3) 剥离版本后缀 token（循环到稳定，上限防死循环）
	//    Round42：每轮剥离后清理残留空括号（[DL版] → "[ ]" → 清空），避免同作品因空括号而不同键
	for pass := 0; pass < 10; pass++ {
		before := t
		for _, re := range reVersionTokens {
			t = re.ReplaceAllString(t, " ")
		}
		t = reEmptyParen.ReplaceAllString(t, " ")
		if strings.TrimSpace(t) == strings.TrimSpace(before) {
			break
		}
	}

	// 4) 残余罗马数字独立 token 转阿拉伯（防 Vol.III 未被英文卷号正则命中时残留差异）
	//    仅在剥离后仍是独立 token 时转换（避免误伤作品名内嵌罗马数字）。
	t = reRomanToken.ReplaceAllStringFunc(t, func(s string) string {
		if n, ok := romanToArabic(strings.ToLower(s)); ok {
			return fmt.Sprintf(" %d ", n)
		}
		return s
	})

	// 5) 归一核心名：小写、保留字母数字与 CJK、折叠空白
	core := strings.Join(strings.Fields(t), " ")
	core = strings.TrimSpace(core)
	return TitleFingerprint{Core: core, Volume: volume, Artist: strings.TrimSpace(artist)}
}

// normalizeTitleForDedup 清洗前归一：全角→半角（ASCII 区）、小写、折叠空白
func normalizeTitleForDedup(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= '！' && r <= '～': // 全角 ASCII 区（U+FF01..U+FF5E）→ 半角
			b.WriteRune(r - '！' + '!')
		case r == '　': // 全角空格
			b.WriteRune(' ')
		default:
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// cjkOrArabicToNum 一二三…十…或阿拉伯数字 → 阿拉伯字符串（无法解析则原样返回）
func cjkOrArabicToNum(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if s[0] >= '0' && s[0] <= '9' {
		return s
	}
	digits := map[rune]int{'一': 1, '二': 2, '三': 3, '四': 4, '五': 5, '六': 6, '七': 7, '八': 8, '九': 9, '两': 2}
	total := 0
	cur := 0
	for _, r := range s {
		switch r {
		case '十':
			if cur == 0 {
				cur = 1
			}
			total += cur * 10
			cur = 0
		default:
			d, ok := digits[r]
			if !ok {
				return s // 无法解析，原样返回
			}
			cur = d
		}
	}
	total += cur
	return fmt.Sprintf("%d", total)
}

// arabicOrRomanToNum 阿拉伯或罗马数字 → 阿拉伯字符串
func arabicOrRomanToNum(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if s[0] >= '0' && s[0] <= '9' {
		return s
	}
	if n, ok := romanToArabic(strings.ToLower(s)); ok {
		return fmt.Sprintf("%d", n)
	}
	return s
}

// romanToArabic 罗马数字 → 阿拉伯数字（ii/iv/xiii 等，小写输入）
func romanToArabic(s string) (int, bool) {
	vals := map[byte]int{'i': 1, 'v': 5, 'x': 10, 'l': 50, 'c': 100, 'd': 500, 'm': 1000}
	total := 0
	prev := 0
	for i := len(s) - 1; i >= 0; i-- {
		v, ok := vals[s[i]]
		if !ok {
			return 0, false
		}
		if v < prev {
			total -= v
		} else {
			total += v
		}
		prev = v
	}
	if total <= 0 || total > 3999 {
		return 0, false
	}
	return total, true
}

// extractNamespaceTag 从 tag 数组提取指定 namespace 的取值（artist/language 等）
func extractNamespaceTag(tags []string, ns string) string {
	prefix := ns + ":"
	for _, t := range tags {
		if strings.HasPrefix(t, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(t, prefix))
		}
	}
	return ""
}

// ── 判定层 + 决策层 ──

// clusterCandidate 分组过程中的候选成员
type clusterCandidate struct {
	comic *models.OfflineComic
	fp    TitleFingerprint // 主指纹（title_jpn 优先，title 兜底）
}

// clusterKey 分组键：归一核心名 + 卷号 + 画师（小写）。
// 卷号进 key → 续集/分卷天然隔离；画师进 key → **作者硬否决**（Round42 决策 D10：
// artist tag 双方都非空且不同时不可能同键，从源头避免「不同作者同名」误报）。
func clusterKey(fp TitleFingerprint) string {
	return strings.ToLower(fp.Core) + "\x00" + fp.Volume + "\x00" + strings.ToLower(fp.Artist)
}

// coreVolKeyOf 从 clusterKey 截出「核心名 + 卷号」段（去掉 artist），
// 供 D10 的「作者未知」兜底判定使用。
func coreVolKeyOf(key string) string {
	if i := strings.LastIndex(key, "\x00"); i >= 0 {
		return key[:i]
	}
	return key
}

// detectTitleClusters 名称级疑似重复检测（O3 规则 5，纯本地）
//
// comics 应为「未被确定性规则标记删除」的漫画集合（调用方过滤 removeSet）。
// forceFull=false 时命中忽略（核心名+画师）的簇直接跳过；
// forceFull=true 时全部输出，命中忽略的簇带 Ignored=true（前端折叠展示）。
//
// Round42 改造（D9 标定集驱动）：
//   - **双字段池**：`title_jpn`（E 站规范标题）与 `title` 各自生成指纹，任一字段同键即成组。
//     此前只用 `title`（metadata 抓取常为罗马音），导致卡片显示的日文标题搜不到 / 聚不了组。
//   - **并查集合并**：一本可能持有 2 个键（两个字段），用并查集按「相同键」连通，
//     等价于多键精确匹配，且保持传递性正确（键是精确匹配，非相似度）。
func detectTitleClusters(comics []models.OfflineComic, ignoreIdx *IgnoreIndex, forceFull bool) []DedupCluster {
	cands := make([]*clusterCandidate, 0, len(comics))
	keySets := make([][]string, 0, len(comics))
	artistKnown := make([]bool, 0, len(comics))
	for i := range comics {
		c := &comics[i]
		// Round26-2：成员级忽略——被忽略的漫画（type=comic）不参与名称聚类
		if ignoreIdx != nil && ignoreIdx.IsComicIgnored(c.ID) {
			continue
		}
		tags := UnmarshalTagSlice(c.OnlineTags)
		artist := extractNamespaceTag(tags, "artist")

		// 双字段池：title_jpn 优先（E 站规范标题），title 兜底；两者都可作为分组依据
		var main TitleFingerprint
		keys := make([]string, 0, 2)
		for _, src := range []string{c.TitleJpn, c.Title} {
			if strings.TrimSpace(src) == "" {
				continue
			}
			fp := fingerprintTitle(src, artist)
			if fp.Core == "" {
				continue
			}
			if main.Core == "" {
				main = fp
			}
			keys = append(keys, clusterKey(fp))
		}
		if len(keys) == 0 {
			continue
		}
		cands = append(cands, &clusterCandidate{comic: c, fp: main})
		keySets = append(keySets, keys)
		artistKnown = append(artistKnown, strings.TrimSpace(main.Artist) != "")
	}

	// 并查集：同一键的书连通为同一组
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
	firstSeen := map[string]int{}
	for i, keys := range keySets {
		for _, k := range keys {
			if j, ok := firstSeen[k]; ok {
				union(i, j)
			} else {
				firstSeen[k] = i
			}
		}
	}

	// 轮 2：**作者未知兜底**（D10 语义：artist 缺失不构成否决）。
	// 同一「核心名 + 卷号」下若**存在 artist 缺失的书**，则该组整体并为一簇——
	// 空 artist 只代表「作者未知」，不能据此否决；供人工优先确认（buildCluster 会降档并标注）。
	// 若该组内所有书 artist 都非空，则保持轮 1 的硬否决结果（不同作者同名不合并）。
	byCoreVol := map[string][]int{}
	for i, keys := range keySets {
		for _, k := range keys {
			cv := coreVolKeyOf(k)
			byCoreVol[cv] = append(byCoreVol[cv], i)
		}
	}
	for _, members := range byCoreVol {
		if len(members) < 2 {
			continue
		}
		hasUnknown := false
		for _, i := range members {
			if !artistKnown[i] {
				hasUnknown = true
				break
			}
		}
		if !hasUnknown {
			continue
		}
		for _, i := range members[1:] {
			union(members[0], i)
		}
	}

	// 按根聚合（保持输入顺序，便于结果稳定）
	groups := map[int][]*clusterCandidate{}
	roots := make([]int, 0, len(cands))
	for i := range cands {
		r := find(i)
		if _, ok := groups[r]; !ok {
			roots = append(roots, r)
		}
		groups[r] = append(groups[r], cands[i])
	}

	var clusters []DedupCluster
	for _, r := range roots {
		g := groups[r]
		if len(g) < 2 {
			continue
		}
		cl := buildCluster(g)
		if cl == nil {
			continue
		}
		if ignoreIdx != nil && ignoreIdx.IsTitleIgnored(cl.TitleKey, cl.Artist) {
			if !forceFull {
				continue // 增量：忽略的簇直接跳过
			}
			cl.Ignored = true // 全量：仍列出，带标记
		}
		clusters = append(clusters, *cl)
	}

	// 排序：成员多者优先（疑似度更高的组排前面）
	sort.Slice(clusters, func(i, j int) bool {
		return len(clusters[i].Members) > len(clusters[j].Members)
	})
	for i := range clusters {
		clusters[i].ID = fmt.Sprintf("cluster-%d", i)
	}
	return clusters
}

// buildCluster 组内判定 → 簇（nil 表示不构成疑似重复）
func buildCluster(g []*clusterCandidate) *DedupCluster {
	fp := g[0].fp
	maxDiff := 0
	members := make([]ClusterMember, 0, len(g))
	for _, cand := range g {
		tags := UnmarshalTagSlice(cand.comic.OnlineTags)
		lang := extractNamespaceTag(tags, "language")
		pc := cand.comic.OriginalPageCount
		if pc <= 0 {
			pc = cand.comic.PageCount
		}
		members = append(members, ClusterMember{Comic: *cand.comic, PageCount: pc, Lang: lang})
	}
	for i := 0; i < len(members); i++ {
		for j := i + 1; j < len(members); j++ {
			d := members[i].PageCount - members[j].PageCount
			if d < 0 {
				d = -d
			}
			if d > maxDiff {
				maxDiff = d
			}
		}
	}

	confidence := "high"
	reason := fmt.Sprintf("画师相同（artist:%s）+ 核心名相同 + 页数差 ≤%d，疑似多语言/重传/不同版本", fp.Artist, dedupPageTolerance)
	if maxDiff > dedupPageTolerance {
		confidence = "medium"
		reason = fmt.Sprintf("核心名 + 画师相同，但页数差 %d 页（>%d），可能为重绘/合集/双页合并版，请人工确认", maxDiff, dedupPageTolerance)
	}
	// Round42 决策 D10（语义已与用户对齐）：作者标识只用 `artist` tag 做硬否决（体现在分组键 + 轮 2 兜底）；
	// 组内存在「作者未知」成员（无 artist tag）时不否决，但置信度降一档并明确标注，供人工优先复核。
	// 实测全库 278 本无 artist tag，其中 218 本可用开头中括号（circle 名）辅助判断。
	hasUnknownAuthor := strings.TrimSpace(fp.Artist) == ""
	if !hasUnknownAuthor {
		for _, cand := range g {
			if strings.TrimSpace(cand.fp.Artist) == "" {
				hasUnknownAuthor = true
				break
			}
		}
	}
	if hasUnknownAuthor {
		reason += "；**含作者未知条目**（无 artist tag），请优先人工确认"
		if confidence == "high" {
			confidence = "medium"
		}
	}

	return &DedupCluster{
		TitleKey:   fp.Core,
		Artist:     fp.Artist,
		Confidence: confidence,
		Reason:     reason,
		Members:    members,
	}
}
