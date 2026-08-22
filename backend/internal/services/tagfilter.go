package services

// Round20-Bug3：E-Hentai f_search 语法解析器（Go 版，与前端 src/utils/tagFilter.ts 语义一致）。
//
// 背景：在线搜索使用 E-Hentai 标准语法（多词 tag 带引号 + $ 锚定，如 `female:"magical girl$"`），
// 而离线本地匹配此前按「裸 namespace:key 子串/精确」处理，两种格式互不通用。
// 本解析器把任意 f_search token 归一为规范形（ns:key，小写、_→空格、去引号/锚点），
// 本地匹配按 E 站语义执行：有 $ → 精确相等；无 $ → 前缀匹配（决策 D5=A）。

import "strings"

// FSearchTag 单个 f_search tag 的解析结果
type FSearchTag struct {
	Namespace string // 命名空间（小写）；裸词为空
	Key       string // tag key（小写，下划线已归一为空格）
	Anchored  bool   // 是否带 $ 锚定（$=精确匹配，无 $=前缀匹配）
	IsTag     bool   // 是否为 tag 形（含命名空间）
}

// ParseFSearchTag 解析单个 token（容忍负号前缀、双引号、$ 锚点、大小写、下划线）。
// 示例：
//
//	`female:"magical girl$"` → {female, magical girl, anchored, tag}
//	`female:yuri$`           → {female, yuri, anchored, tag}
//	`female:yuri`            → {female, yuri, !anchored, tag}
//	`parody:genshin impact`  → {parody, genshin impact, !anchored, tag}
//	`3d`（裸词）             → {IsTag:false}
func ParseFSearchTag(raw string) FSearchTag {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "-")
	s = strings.TrimSpace(s)
	idx := strings.Index(s, ":")
	if idx <= 0 {
		return FSearchTag{}
	}
	ns := strings.ToLower(strings.TrimSpace(s[:idx]))
	keyPart := strings.ReplaceAll(s[idx+1:], `"`, "")
	anchored := strings.HasSuffix(keyPart, "$")
	key := strings.TrimSuffix(keyPart, "$")
	key = strings.ToLower(strings.TrimSpace(key))
	key = strings.ReplaceAll(key, "_", " ")
	if ns == "" || key == "" {
		return FSearchTag{Namespace: ns, Key: key, Anchored: anchored, IsTag: ns != "" && key != ""}
	}
	return FSearchTag{Namespace: ns, Key: key, Anchored: anchored, IsTag: true}
}

// Canonical 规范形 `ns:key`（本地 tagRaws 的匹配基准）
func (t FSearchTag) Canonical() string {
	return t.Namespace + ":" + t.Key
}

// MatchTag 按 E 站语义匹配一个已归一化的本地 tag（ns:key，小写、_→空格）：
// 有 $ → 精确相等；无 $ → 前缀匹配（与 E-Hentai 搜索语义一致）
func (t FSearchTag) MatchTag(normalizedTag string) bool {
	if !t.IsTag {
		return false
	}
	if t.Anchored {
		return normalizedTag == t.Canonical()
	}
	return strings.HasPrefix(normalizedTag, t.Canonical())
}

// EscapeLike 转义 SQL LIKE 通配符（_ % \），避免用户输入的通配符污染匹配语义
func EscapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

// TagJSONMatchPatterns 生成匹配 tags JSON 数组元素（形如 ["ns:key", ...]）的 LIKE 模式。
// 覆盖空格与下划线两种存储形态（E-Hentai 显示/URL 两种形态均可能落库）。
// 返回的模式需配合 ESCAPE '\' 使用。
func (t FSearchTag) TagJSONMatchPatterns() []string {
	if !t.IsTag {
		return nil
	}
	space := t.Canonical()
	underscore := t.Namespace + ":" + strings.ReplaceAll(t.Key, " ", "_")
	patterns := []string{`%"` + EscapeLike(space) + `%`}
	if underscore != space {
		patterns = append(patterns, `%"`+EscapeLike(underscore)+`%`)
	}
	return patterns
}
