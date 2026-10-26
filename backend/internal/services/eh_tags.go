package services

import (
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// parseGalleryTags 解析 E-H 画廊详情页 #taglist 为规范 tag 列表（namespace:key）。
//
// E-H 页面结构（2026-10 实网验证）：
//
//	<tr>
//	  <td class="tc">artist:</td>
//	  <td>
//	    <div id="td_artist:gohan_pakupaku_maru" class="gt">
//	      <a id="ta_artist:gohan_pakupaku_maru"
//	         href="https://e-hentai.org/tag/artist:gohan+pakupaku+maru"
//	         onclick="return toggle_tagmenu(…,'artist:gohan pakupaku maru',this)">gohan pakupaku maru | gpm</a>
//	    </div>
//	  </td>
//	</tr>
//
// 关键：<a> 的显示文本可能带 " | 别名/提示"（如 "japanese | definition revised please read the wiki"、
// "cure answer | anna akechi"），但规范 tag 身份在 id / href / onclick 里（即 "|" 左侧）。
// E-H 自身的 tag 页/卡片/搜索也都只用左侧，入库必须同口径，否则：
//   - 词典按 namespace:key 精确查表必 miss → 显示原文而非翻译；
//   - f_search 拼出 `ns:"key | alias$"` → 0 结果（Round46 修复）。
//
// 解析优先级：id → href → onclick → 显示文本（按 "|" 截断）→ 行内 div 文本（最后兜底）。
func parseGalleryTags(doc *goquery.Document) []string {
	tags := make([]string, 0, 32)
	seen := make(map[string]bool, 32)

	appendTag := func(tag string) {
		if tag == "" || seen[tag] {
			return
		}
		seen[tag] = true
		tags = append(tags, tag)
	}

	doc.Find("#taglist tr").Each(func(_ int, tr *goquery.Selection) {
		ns := strings.TrimSuffix(strings.TrimSpace(tr.Find("td.tc").Text()), ":")
		if ns == "" {
			ns = "other"
		}

		anchors := tr.Find("a[id^='ta_'], a[href*='/tag/'], a[onclick*='toggle_tagmenu']")
		if anchors.Length() == 0 {
			// 兼容旧页面 / 结构变化：退回原口径（div.gt / div.gtl 的文本），但仍裁掉 "|" 右侧。
			tr.Find("div.gt, div.gtl").Each(func(_ int, node *goquery.Selection) {
				appendTag(normalizeTagKey(ns + ":" + node.Text()))
			})
			return
		}

		anchors.Each(func(_ int, a *goquery.Selection) {
			if tag := tagFromAnchorID(a); tag != "" {
				appendTag(tag)
				return
			}
			if tag := tagFromHref(a); tag != "" {
				appendTag(tag)
				return
			}
			if tag := tagFromOnclick(a); tag != "" {
				appendTag(tag)
				return
			}
			appendTag(normalizeTagKey(ns + ":" + a.Text()))
		})
	})
	return tags
}

// tagFromAnchorID 从 <a id="ta_ns:key"> 提取规范 tag（id 本身不含 "|" 后缀，最可靠）。
func tagFromAnchorID(a *goquery.Selection) string {
	id, ok := a.Attr("id")
	if !ok {
		return ""
	}
	return normalizeTagKey(strings.TrimPrefix(strings.TrimSpace(id), "ta_"))
}

// tagFromHref 从 <a href="…/tag/ns:key"> 兜底提取（URL 中空格以 "+" 表示）。
func tagFromHref(a *goquery.Selection) string {
	href, ok := a.Attr("href")
	if !ok {
		return ""
	}
	const marker = "/tag/"
	i := strings.Index(href, marker)
	if i < 0 {
		return ""
	}
	seg := href[i+len(marker):]
	if q := strings.IndexAny(seg, "?#"); q >= 0 {
		seg = seg[:q]
	}
	seg = strings.TrimSuffix(seg, "/")
	return normalizeTagKey(strings.ReplaceAll(seg, "+", " "))
}

// tagFromOnclick 从 onclick="return toggle_tagmenu(tagID,'ns:key',this)" 兜底提取。
func tagFromOnclick(a *goquery.Selection) string {
	onclick, ok := a.Attr("onclick")
	if !ok {
		return ""
	}
	const marker = "toggle_tagmenu("
	i := strings.Index(onclick, marker)
	if i < 0 {
		return ""
	}
	rest := onclick[i+len(marker):]
	start := strings.Index(rest, "'")
	if start < 0 {
		return ""
	}
	end := strings.Index(rest[start+1:], "'")
	if end < 0 {
		return ""
	}
	return normalizeTagKey(rest[start+1 : start+1+end])
}

// normalizeTagKey 归一为规范 "namespace:key"：
//   - 截掉第一个 "|" 及其右侧（E-H 显示文本里的别名 / wiki 提示）；
//   - 下划线还原为空格（E-H 的 id 与显示文本以 "_" 表示空格）；
//   - namespace 小写、去首尾空白；非法（缺 namespace 或 key）返回空串。
func normalizeTagKey(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if i := strings.Index(raw, "|"); i >= 0 {
		raw = strings.TrimSpace(raw[:i])
	}
	raw = strings.ReplaceAll(raw, "_", " ")
	i := strings.Index(raw, ":")
	if i <= 0 || i == len(raw)-1 {
		return ""
	}
	ns := strings.ToLower(strings.TrimSpace(raw[:i]))
	key := strings.TrimSpace(raw[i+1:])
	if ns == "" || key == "" {
		return ""
	}
	return ns + ":" + key
}
