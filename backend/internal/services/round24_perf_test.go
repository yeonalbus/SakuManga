package services

import (
	"archive/zip"
	"bytes"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"

	"SakuManga/internal/models"
)

// Round24-P0-3：页列表内存缓存——两次读取结果一致且不报错（zip 场景）
func TestGetPageListCacheZip(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "test.cbz")
	if err := writeTestZip(zipPath, []string{"03.jpg", "01.png", "02.jpg"}); err != nil {
		t.Fatal(err)
	}

	p1, err := GetPageList(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(p1) != 3 {
		t.Fatalf("页数应为 3，得到 %d", len(p1))
	}
	// 自然排序：01.png, 02.jpg, 03.jpg
	if p1[0] != "01.png" || p1[1] != "02.jpg" || p1[2] != "03.jpg" {
		t.Fatalf("排序不符: %v", p1)
	}

	// 第二次应命中缓存，结果一致
	p2, err := GetPageList(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(p2) != len(p1) || p2[0] != p1[0] || p2[2] != p1[2] {
		t.Fatalf("缓存结果不一致: %v vs %v", p1, p2)
	}
}

// Round24-P0-3：目录场景页列表缓存
func TestGetPageListCacheDir(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"b.jpg", "a.png", "c.gif"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	p1, err := GetPageList(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(p1) != 3 || p1[0] != "a.png" {
		t.Fatalf("目录页列表不符: %v", p1)
	}
	p2, err := GetPageList(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(p2) != len(p1) {
		t.Fatalf("缓存结果不一致: %v vs %v", p1, p2)
	}
}

// Round24-P0-1/P0-2：封面缩略图——首次生成写缓存，二次命中短路（cached=true）
func TestGetCoverThumbCacheShortCircuit(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "test.cbz")

	// 生成 600x400 JPEG（宽度 > CoverThumbWidth=480，会触发缩放写缓存）
	img := image.NewRGBA(image.Rect(0, 0, 600, 400))
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80}); err != nil {
		t.Fatal(err)
	}
	if err := writeTestZip(zipPath, nil); err != nil {
		t.Fatal(err)
	}
	// 往 zip 里追加一张图（writeTestZip 已写 01.jpg 占位，这里用独立 helper 重建）
	_ = os.Remove(zipPath)
	zf, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(zf)
	w, _ := zw.Create("01.jpg")
	if _, err := w.Write(buf.Bytes()); err != nil {
		t.Fatal(err)
	}
	_ = zw.Close()
	_ = zf.Close()

	comic := models.OfflineComic{ID: "cover-test-" + t.Name(), LocalPath: zipPath}
	cachePath := coverCachePath(comic.ID)
	defer os.Remove(cachePath)

	// 第一次：未命中 → 生成缓存
	_, _, cached, err := GetCoverThumb(comic)
	if err != nil {
		t.Fatal(err)
	}
	if !cached {
		t.Fatal("首次生成应返回缓存路径")
	}
	if _, err := os.Stat(cachePath); err != nil {
		t.Fatalf("缓存文件应已生成: %v", err)
	}

	// 第二次：命中短路
	_, cachePath2, cached2, err := GetCoverThumb(comic)
	if err != nil {
		t.Fatal(err)
	}
	if !cached2 || cachePath2 != cachePath {
		t.Fatalf("二次应命中缓存: cached=%v path=%v", cached2, cachePath2)
	}
}

// writeTestZip 创建含若干图片条目的 zip
func writeTestZip(path string, names []string) error {
	if len(names) == 0 {
		names = []string{"01.jpg"}
	}
	zf, err := os.Create(path)
	if err != nil {
		return err
	}
	defer zf.Close()
	zw := zip.NewWriter(zf)
	for _, n := range names {
		w, err := zw.Create(n)
		if err != nil {
			return err
		}
		if _, err := w.Write([]byte("fake image data")); err != nil {
			return err
		}
	}
	return zw.Close()
}
