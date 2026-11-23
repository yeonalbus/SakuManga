package services

import (
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"
)

// ─────────────────────────────────────────────────────────────
// 页面自定义隐藏工具（Round23 起；Round53/D10 改为相对路径存储）
//
// 隐藏页为「软删除」：不物理删除文件，仅记录被隐藏的页。
//   - HiddenPages       存 JSON：相对路径数组（如 ["001_ad.jpg","sub/cover.jpg"]）
//   - PageCount         展示/阅读用「有效页数」= 物理页数 − 有效隐藏数
//   - OriginalPageCount 原始（物理）页数，供更新检测/维护查重比对
//
// 为什么用相对路径而不是物理索引：更新检测/增补替换后页面会在前面插入
// 招募页/替换封面，物理索引整体偏移 → 已隐藏的广告页重新冒出来、正常页被误隐藏。
// 相对路径在页面增删后仍指向同一张图（页名变化时由 NormalizeHiddenSet 丢弃）。
// ─────────────────────────────────────────────────────────────

// HiddenSet 隐藏页集合（以页表相对路径为键，统一 "/" 分隔）
type HiddenSet struct {
	Paths map[string]bool
}

// ParseHiddenSet 解析隐藏页 JSON，兼容两种历史格式：
//   - 字符串数组（当前）：["a.jpg","sub/b.jpg"]
//   - 数字数组（旧物理索引）：[1,5] → 按传入 pages 映射为相对路径，越界索引丢弃
func ParseHiddenSet(raw string, pages []string) HiddenSet {
	set := HiddenSet{Paths: map[string]bool{}}
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return set
	}
	var items []json.RawMessage
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return set
	}
	for _, item := range items {
		var s string
		if err := json.Unmarshal(item, &s); err == nil {
			p := strings.TrimSpace(filepath.ToSlash(s))
			if p != "" {
				set.Paths[p] = true
			}
			continue
		}
		var n int
		if err := json.Unmarshal(item, &n); err == nil {
			if n >= 0 && n < len(pages) {
				set.Paths[pages[n]] = true
			}
		}
	}
	return set
}

// Marshal 序列化为相对路径数组（排序稳定；空则 "[]"）
func (h HiddenSet) Marshal() string {
	paths := h.PathsList()
	if len(paths) == 0 {
		return "[]"
	}
	data, err := json.Marshal(paths)
	if err != nil {
		return "[]"
	}
	return string(data)
}

// PathsList 返回排序后的相对路径列表（接口返回 / 测试断言用）
func (h HiddenSet) PathsList() []string {
	if len(h.Paths) == 0 {
		return []string{}
	}
	paths := make([]string, 0, len(h.Paths))
	for p := range h.Paths {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths
}

// NormalizeHiddenSet 丢弃页表中已不存在的路径（页面改名/减少后自愈）
func NormalizeHiddenSet(h HiddenSet, pages []string) HiddenSet {
	out := HiddenSet{Paths: map[string]bool{}}
	if len(h.Paths) == 0 || len(pages) == 0 {
		return out
	}
	valid := make(map[string]bool, len(pages))
	for _, p := range pages {
		valid[p] = true
	}
	for p := range h.Paths {
		if valid[p] {
			out.Paths[p] = true
		}
	}
	return out
}

// EffectivePageCount 有效页数 = 页表中未被隐藏的页数
func EffectivePageCount(pages []string, h HiddenSet) int {
	cnt := 0
	for _, p := range pages {
		if !h.Paths[p] {
			cnt++
		}
	}
	return cnt
}

// CountPages 统计画廊物理页数（文件夹递归图片数 / zip 内图片数）
func CountPages(localPath string) (int, error) {
	pages, err := GetPageList(localPath)
	if err != nil {
		return 0, err
	}
	return len(pages), nil
}

// FilterHiddenPages 从页表中剔除隐藏页，保持原有顺序
func FilterHiddenPages(pages []string, h HiddenSet) []string {
	if len(h.Paths) == 0 || len(pages) == 0 {
		return pages
	}
	out := make([]string, 0, len(pages))
	for _, p := range pages {
		if !h.Paths[p] {
			out = append(out, p)
		}
	}
	return out
}
