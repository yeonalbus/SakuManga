package services

import (
	"archive/zip"
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
	"time"

	"SakuHentai/internal/models"
)

// buildTestZipWithImage 构造含一张 w×h JPEG 的 ZIP/CBZ 测试包
func buildTestZipWithImage(t *testing.T, zipPath string, w, h int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatalf("构造测试图失败: %v", err)
	}
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	w2, err := zw.Create("001.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w2.Write(buf.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
}

// Round14：封面缩略图缓存——生成/命中/源变更重建/小图回退
func TestCoverThumbCache(t *testing.T) {
	dir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	// 大图（800x600）→ 触发缩放生成缓存
	zipPath := filepath.Join(dir, "test.cbz")
	buildTestZipWithImage(t, zipPath, 800, 600)
	comic := models.OfflineComic{ID: "comic-cover-test", LocalPath: zipPath}

	// 1) 首次调用：生成缓存
	data, cachePath, cached, err := GetCoverThumb(comic)
	if err != nil {
		t.Fatalf("首次调用失败: %v", err)
	}
	if !cached || data != nil {
		t.Fatalf("大图应生成缓存（cached=true data=nil），得到 cached=%v dataLen=%d", cached, len(data))
	}
	if _, statErr := os.Stat(cachePath); statErr != nil {
		t.Fatalf("缓存文件应存在: %v", statErr)
	}

	// 2) 第二次调用：命中缓存
	_, cachePath2, cached2, err := GetCoverThumb(comic)
	if err != nil || !cached2 || cachePath2 != cachePath {
		t.Fatalf("第二次应命中缓存: err=%v cached=%v", err, cached2)
	}

	// 3) 源变更（ZIP modtime 晚于缓存）→ 重建
	past := time.Now().Add(-2 * time.Hour)
	_ = os.Chtimes(cachePath, past, past)
	_, _, cached3, err := GetCoverThumb(comic)
	if err != nil {
		t.Fatalf("源变更后重建失败: %v", err)
	}
	if !cached3 {
		t.Fatalf("源变更后应重建并返回缓存，得到 cached=%v", cached3)
	}
}

// 小图（200x150 ≤ 480）→ 无需缩放，回退原图直传（cached=false）
func TestCoverThumbSmallImageFallback(t *testing.T) {
	dir := t.TempDir()
	oldWd, _ := os.Getwd()
	_ = os.Chdir(dir)
	defer func() { _ = os.Chdir(oldWd) }()

	zipPath := filepath.Join(dir, "small.cbz")
	buildTestZipWithImage(t, zipPath, 200, 150)
	comic := models.OfflineComic{ID: "comic-cover-small", LocalPath: zipPath}

	data, _, cached, err := GetCoverThumb(comic)
	if err != nil {
		t.Fatalf("GetCoverThumb 失败: %v", err)
	}
	if cached || len(data) == 0 {
		t.Fatalf("小图应回退原图直传（cached=false data 非空），得到 cached=%v dataLen=%d", cached, len(data))
	}
}
