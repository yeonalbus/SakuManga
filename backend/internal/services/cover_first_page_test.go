package services

import (
	"os"
	"path/filepath"
	"testing"
)

// ─────────────────────────────────────────────────────────────
// Round53 阶段一：D11 封面 = 页表第一张未隐藏图（自然排序）
// ─────────────────────────────────────────────────────────────

func TestCoverEqualsFirstPageNaturalOrder(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "img10.jpg"), "x")
	mustWriteFile(t, filepath.Join(dir, "img2.jpg"), "x")

	got, err := GetCoverFromDir(dir, HiddenSet{Paths: map[string]bool{}})
	if err != nil {
		t.Fatalf("取封面失败: %v", err)
	}
	// 自然排序：img2 < img10（字典序会得到 img10）
	if filepath.Base(got) != "img2.jpg" {
		t.Fatalf("封面应为自然排序第一张 img2.jpg，实际 %s", filepath.Base(got))
	}
}

func TestCoverSkipsHiddenPage(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "01_ad.jpg"), "x")
	mustWriteFile(t, filepath.Join(dir, "02_1.jpg"), "x")

	hidden := HiddenSet{Paths: map[string]bool{"01_ad.jpg": true}}
	got, err := GetCoverFromDir(dir, hidden)
	if err != nil {
		t.Fatalf("取封面失败: %v", err)
	}
	if filepath.Base(got) != "02_1.jpg" {
		t.Fatalf("隐藏首图后封面应为 02_1.jpg，实际 %s", filepath.Base(got))
	}

	// 全部隐藏 → 无封面
	all := HiddenSet{Paths: map[string]bool{"01_ad.jpg": true, "02_1.jpg": true}}
	if _, err := GetCoverFromDir(dir, all); err == nil {
		t.Fatalf("全部隐藏时应返回错误")
	}
}

func TestCoverFromZipSkipsHiddenPage(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "x.zip")
	mustWriteZip(t, zipPath, "01_ad.jpg", "02_1.jpg")

	hidden := HiddenSet{Paths: map[string]bool{"01_ad.jpg": true}}
	data, contentType, err := GetCoverFromZip(zipPath, hidden)
	if err != nil {
		t.Fatalf("取 zip 封面失败: %v", err)
	}
	if len(data) != 1 || contentType != "image/jpeg" {
		t.Fatalf("应读到 02_1.jpg 内容（1 字节 jpeg），实际 len=%d type=%s", len(data), contentType)
	}
}

// TestInvalidateCoverCache 隐藏页保存后封面缓存必须失效（D11）。
func TestInvalidateCoverCache(t *testing.T) {
	comicID := "test-invalidate-cover"
	cachePath := coverCachePath(comicID)
	if err := os.MkdirAll(coverCacheDir, 0o755); err != nil {
		t.Fatalf("创建缓存目录失败: %v", err)
	}
	if err := os.WriteFile(cachePath, []byte("fake"), 0o644); err != nil {
		t.Fatalf("写缓存失败: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(coverCacheDir) })

	if err := InvalidateCoverCache(comicID); err != nil {
		t.Fatalf("失效失败: %v", err)
	}
	if _, err := os.Stat(cachePath); !os.IsNotExist(err) {
		t.Fatalf("缓存文件应已删除")
	}
	// 幂等：文件不存在时不报错
	if err := InvalidateCoverCache(comicID); err != nil {
		t.Fatalf("二次失效应幂等，实际: %v", err)
	}
}
