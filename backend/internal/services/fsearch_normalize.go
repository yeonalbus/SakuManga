package services

import "strings"

// ─────────────────────────────────────────────────────────────
// E-Hentai f_search 语法规范化（简版幂等转义）
//
// E-Hentai 的 tag 搜索标准语法：
//   - 单词 tag：  namespace:key$          （如 female:futanari$）
//   - 多词 tag：  namespace:"key1 key2$"  （引号包裹 + $ 在引号内，如 parody:"blue archive$"）
//
// 裸词/自由输入若直接透传，如 group:da hootch 会被 E 站拆成
// group:da + hootch 两个独立 token，导致无结果。本函数在 f_search
// 拼接前把「单个 namespace: 的自由输入」修正为标准语法。
//
// 简版边界（避免过度修正）：
//   - 幂等：输入已含引号 " 或 $ 时视为用户手写标准语法，直接跳过；
//   - 仅处理「恰好一个 namespace: token」的输入，多 tag 查询（含多个
//     namespace）与纯裸词输入原样透传；
//   - 吞并后的 key 词数 ≤ 2 才整体转义（覆盖绝大多数多词 tag，如
//     blue archive / da hootch）；3 词及以上视为混合独立词输入，原样
//     透传，避免 character:reimu hakurei large penis 被错误合并。
// ─────────────────────────────────────────────────────────────

// NormalizeFSearch 将自由输入修正为 E-Hentai f_search 标准语法（简版幂等）。
func NormalizeFSearch(keyword string) string {
	kw := strings.TrimSpace(keyword)
	if kw == "" {
		return kw
	}

	// 幂等：已含引号或 $，视为用户已手写标准语法，不二次处理
	if strings.ContainsAny(kw, "\"$") {
		return kw
	}

	tokens := strings.Fields(kw)
	if len(tokens) == 0 {
		return kw
	}

	// 第一个 token 必须为 namespace:key 形式
	first := tokens[0]
	colonIdx := strings.IndexByte(first, ':')
	if colonIdx <= 0 || !isNamespaceName(first[:colonIdx]) {
		return kw
	}
	ns := first[:colonIdx]

	// 多 tag 查询（出现第二个 namespace token）不处理，原样透传
	for _, t := range tokens[1:] {
		if isNamespaceToken(t) {
			return kw
		}
	}

	// 吞并：冒号后的全部内容（含后续裸词 token）视为该 namespace 的 key
	keyStart := first[colonIdx+1:]
	full := strings.TrimSpace(keyStart + " " + strings.Join(tokens[1:], " "))
	words := strings.Fields(full)
	if len(words) == 0 {
		return kw
	}

	// 词数 ≥ 3：视为混合独立词输入，避免过度合并
	if len(words) > 2 {
		return kw
	}

	if len(words) == 1 {
		return ns + ":" + words[0] + "$"
	}
	return ns + ":\"" + words[0] + " " + words[1] + "$\""
}

// isNamespaceToken 判断 token 是否为 namespace:xxx 形式
func isNamespaceToken(s string) bool {
	idx := strings.IndexByte(s, ':')
	if idx <= 0 {
		return false
	}
	return isNamespaceName(s[:idx])
}

// isNamespaceName 判断是否为合法 namespace 名（E 站命名空间全为小写字母）
func isNamespaceName(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z') {
			return false
		}
	}
	return true
}
