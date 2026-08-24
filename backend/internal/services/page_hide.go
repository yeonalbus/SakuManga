package services

import (
	"encoding/json"
	"sort"
	"strings"
)

// ─────────────────────────────────────────────────────────────
// 页面自定义隐藏工具（Round23：剔除汉化组/平台广告页）
//
// 隐藏页为「软删除」：不物理删除文件，仅记录被隐藏的物理页索引（0-based 原文件索引）。
//   - HiddenPages       存储 JSON 数组（如 [3,20,45]）
//   - PageCount         展示/阅读用「有效页数」= 物理页数 − 有效隐藏数
//   - OriginalPageCount 原始（物理）页数，供更新检测/维护查重比对（避免隐藏后误判画廊被扩充）
// ─────────────────────────────────────────────────────────────

// ParseHiddenPages 解析隐藏页 JSON 数组为排序去重后的物理页索引；空/非法返回 nil
func ParseHiddenPages(raw string) []int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var pages []int
	if err := json.Unmarshal([]byte(raw), &pages); err != nil {
		return nil
	}
	return NormalizeHiddenPages(pages, -1)
}

// NormalizeHiddenPages 归一化隐藏页索引：去重、排序、裁剪越界（limit<0 时不裁剪）
func NormalizeHiddenPages(pages []int, limit int) []int {
	set := map[int]bool{}
	out := []int{}
	for _, p := range pages {
		if p < 0 {
			continue
		}
		if limit >= 0 && p >= limit {
			continue // 越界（物理页已减少）的索引直接裁剪
		}
		if set[p] {
			continue
		}
		set[p] = true
		out = append(out, p)
	}
	sort.Ints(out)
	return out
}

// MarshalHiddenPages JSON 序列化隐藏页数组（空则存 "[]"）
func MarshalHiddenPages(pages []int) string {
	if pages == nil {
		pages = []int{}
	}
	data, err := json.Marshal(pages)
	if err != nil {
		return "[]"
	}
	return string(data)
}

// EffectivePageCount 有效页数 = 物理页数 − 有效隐藏数（越界索引忽略）
func EffectivePageCount(physicalCount int, hidden []int) int {
	if physicalCount <= 0 {
		return 0
	}
	cnt := 0
	for _, p := range hidden {
		if p < physicalCount {
			cnt++
		}
	}
	if cnt > physicalCount {
		cnt = physicalCount
	}
	return physicalCount - cnt
}

// CountPages 统计画廊物理页数（文件夹图片数 / 压缩包内图片数）
func CountPages(localPath string) (int, error) {
	pages, err := GetPageList(localPath)
	if err != nil {
		return 0, err
	}
	return len(pages), nil
}

// FilterHiddenPages 从物理页列表中剔除隐藏页，返回有效页列表（隐藏索引越界自动忽略）
func FilterHiddenPages(pages []string, hidden []int) []string {
	if len(hidden) == 0 || len(pages) == 0 {
		return pages
	}
	set := map[int]bool{}
	for _, p := range hidden {
		if p >= 0 && p < len(pages) {
			set[p] = true
		}
	}
	out := make([]string, 0, len(pages)-len(set))
	for i, p := range pages {
		if !set[i] {
			out = append(out, p)
		}
	}
	return out
}
