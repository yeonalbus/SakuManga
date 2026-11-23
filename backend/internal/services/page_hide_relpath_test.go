package services

import (
	"reflect"
	"testing"
)

// ─────────────────────────────────────────────────────────────
// Round53 阶段一：D10 隐藏页改相对路径
//
// 反例：旧实现存物理索引，页面头部插入广告页后 → 已隐藏的广告页重新冒出来、
// 正常漫画页被误隐藏。相对路径在增删页后仍指向同一张图。
// ─────────────────────────────────────────────────────────────

func TestHiddenSetParseLegacyNumericIndex(t *testing.T) {
	pages := []string{"a.jpg", "b.jpg", "c.jpg"}

	// 旧格式：数字索引 → 映射为相对路径，越界丢弃
	h := ParseHiddenSet("[1,2,9]", pages)
	if len(h.Paths) != 2 || !h.Paths["b.jpg"] || !h.Paths["c.jpg"] {
		t.Fatalf("旧数字索引应映射为 b.jpg/c.jpg，实际: %v", h.PathsList())
	}

	// 新格式：字符串数组
	h2 := ParseHiddenSet(`["a.jpg","sub/d.jpg"]`, pages)
	if len(h2.Paths) != 2 || !h2.Paths["a.jpg"] || !h2.Paths["sub/d.jpg"] {
		t.Fatalf("字符串数组应原样保留，实际: %v", h2.PathsList())
	}

	// 空/非法
	for _, raw := range []string{"", "null", "not-json"} {
		if got := ParseHiddenSet(raw, pages); len(got.Paths) != 0 {
			t.Errorf("raw=%q 应解析为空集，实际: %v", raw, got.PathsList())
		}
	}
}

// TestHiddenSetByRelativePathSurvivesPageInsertion 反例：头部插入新广告页后隐藏仍指向原图。
func TestHiddenSetByRelativePathSurvivesPageInsertion(t *testing.T) {
	oldPages := []string{"01_ad.jpg", "02_1.jpg", "03_2.jpg"}
	h := ParseHiddenSet(`["01_ad.jpg"]`, oldPages)
	if !h.Paths["01_ad.jpg"] {
		t.Fatalf("应隐藏 01_ad.jpg")
	}

	// 更新后前面插入了一张新的招募页
	newPages := []string{"00_recruit.jpg", "01_ad.jpg", "02_1.jpg", "03_2.jpg"}
	if got := EffectivePageCount(newPages, h); got != 3 {
		t.Fatalf("有效页数应为 3（4 页隐藏 1 页），实际 %d", got)
	}
	visible := FilterHiddenPages(newPages, h)
	want := []string{"00_recruit.jpg", "02_1.jpg", "03_2.jpg"}
	if !reflect.DeepEqual(visible, want) {
		t.Fatalf("可见页应为 %v，实际 %v", want, visible)
	}
}

func TestNormalizeHiddenSetDropsMissing(t *testing.T) {
	h := HiddenSet{Paths: map[string]bool{"keep.jpg": true, "gone.jpg": true}}
	got := NormalizeHiddenSet(h, []string{"keep.jpg", "new.jpg"})
	if len(got.Paths) != 1 || !got.Paths["keep.jpg"] {
		t.Fatalf("应只保留页表中存在的路径，实际: %v", got.PathsList())
	}
}

func TestHiddenSetMarshalRoundTrip(t *testing.T) {
	h := HiddenSet{Paths: map[string]bool{"b.jpg": true, "a.jpg": true}}
	raw := h.Marshal()
	if raw != `["a.jpg","b.jpg"]` {
		t.Fatalf("序列化应排序稳定，实际: %s", raw)
	}
	if got := ParseHiddenSet(raw, []string{"a.jpg", "b.jpg"}); len(got.Paths) != 2 {
		t.Fatalf("往返解析失败: %s → %v", raw, got.PathsList())
	}
	// 空集
	if got := (HiddenSet{}).Marshal(); got != "[]" {
		t.Fatalf("空集应序列化为 []，实际: %s", got)
	}
}
