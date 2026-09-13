package services

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"SakuManga/internal/models"
)

// Round39：封面缓存 TTL 回收——超期文件删除、近期文件保留、子目录不受影响
func TestCleanCoverCache(t *testing.T) {
	dir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	if err := os.MkdirAll(coverCacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// write 造缓存文件并指定 mtime（模拟「最后使用时间」）
	write := func(name string, mod time.Time) string {
		t.Helper()
		p := filepath.Join(coverCacheDir, name)
		if err := os.WriteFile(p, []byte("cover-bytes"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, mod, mod); err != nil {
			t.Fatal(err)
		}
		return p
	}

	now := time.Now()
	fresh := write("comic-fresh.jpg", now.Add(-time.Hour))
	staleThumb := write("comic-stale.jpg", now.Add(-8*24*time.Hour))
	staleProxy := write("proxy_0123456789abcdef.webp", now.Add(-30*24*time.Hour))
	subDir := filepath.Join(coverCacheDir, "sub")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}

	removed, freed, kept, err := CleanCoverCache(CoverCacheTTL)
	if err != nil {
		t.Fatalf("清理失败: %v", err)
	}
	if removed != 2 {
		t.Fatalf("应删除 2 个超期文件，得到 %d", removed)
	}
	if freed != 22 { // 两个文件各 11 字节
		t.Fatalf("释放字节数应为 22，得到 %d", freed)
	}
	if kept != 1 {
		t.Fatalf("应保留 1 个近期文件，得到 %d", kept)
	}
	if _, statErr := os.Stat(staleThumb); !os.IsNotExist(statErr) {
		t.Fatalf("8 天前的离线缩略图应被删除: %v", statErr)
	}
	if _, statErr := os.Stat(staleProxy); !os.IsNotExist(statErr) {
		t.Fatalf("30 天前的代理缓存应被删除: %v", statErr)
	}
	if _, statErr := os.Stat(fresh); statErr != nil {
		t.Fatalf("1 小时前的缓存不应被删: %v", statErr)
	}
	if fi, statErr := os.Stat(subDir); statErr != nil || !fi.IsDir() {
		t.Fatalf("子目录不应被清理: %v", statErr)
	}

	// 再跑一轮：已无可回收文件（幂等，不误删保留项）
	removed2, _, kept2, err := CleanCoverCache(CoverCacheTTL)
	if err != nil || removed2 != 0 || kept2 != 1 {
		t.Fatalf("第二轮应无事发生: err=%v removed=%d kept=%d", err, removed2, kept2)
	}
}

// Round39：缓存目录不存在时清理应静默通过（首次启动尚未生成任何封面）
func TestCleanCoverCacheMissingDir(t *testing.T) {
	dir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	removed, freed, kept, err := CleanCoverCache(CoverCacheTTL)
	if err != nil {
		t.Fatalf("目录不存在不应报错: %v", err)
	}
	if removed != 0 || freed != 0 || kept != 0 {
		t.Fatalf("目录不存在应无统计变化: removed=%d freed=%d kept=%d", removed, freed, kept)
	}
}

// Round39：命中打点——超期文件刷新 mtime；未过降频阈值的文件不改写；缺失路径不 panic
func TestTouchCoverCacheFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "thumb.jpg")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 1) mtime 距今 2 天（超过降频阈值）→ 刷新为 now
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(p, old, old); err != nil {
		t.Fatal(err)
	}
	TouchCoverCacheFile(p)
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(fi.ModTime()) > time.Minute {
		t.Fatalf("超期缓存的 mtime 应被刷新到当前时间，得到 %s", fi.ModTime())
	}

	// 2) 刚刷新过（未过降频阈值）→ 不重复写盘
	before := fi.ModTime()
	TouchCoverCacheFile(p)
	fi2, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if !fi2.ModTime().Equal(before) {
		t.Fatalf("未过降频阈值不应改写 mtime：%s → %s", before, fi2.ModTime())
	}

	// 3) 路径不存在 / 空路径 / 目录 → 静默返回
	TouchCoverCacheFile(filepath.Join(dir, "nope.jpg"))
	TouchCoverCacheFile("")
	TouchCoverCacheFile(dir)
}

// Round39：命中缓存应续期——否则「长期未访问但仍在用」的封面会被 TTL 误回收。
// 手法：源 mtime 拨到 30 小时前、缓存 mtime 拨到 25 小时前（仍晚于源 → 命中且已过
// 24 小时降频阈值），命中后缓存 mtime 应刷新到当前时间。
func TestCoverThumbHitRefreshesModTime(t *testing.T) {
	dir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	zipPath := filepath.Join(dir, "gc.cbz")
	buildTestZipWithImage(t, zipPath, 800, 600)
	comic := models.OfflineComic{ID: "comic-cover-gc", LocalPath: zipPath}

	if _, _, cached, err := GetCoverThumb(comic); err != nil || !cached {
		t.Fatalf("首次应生成缓存: err=%v cached=%v", err, cached)
	}
	cachePath := coverCachePath(comic.ID)

	srcOld := time.Now().Add(-30 * time.Hour)
	if err := os.Chtimes(zipPath, srcOld, srcOld); err != nil {
		t.Fatal(err)
	}
	cacheOld := time.Now().Add(-25 * time.Hour)
	if err := os.Chtimes(cachePath, cacheOld, cacheOld); err != nil {
		t.Fatal(err)
	}

	// 命中一次 → 续期
	if _, _, cached, err := GetCoverThumb(comic); err != nil || !cached {
		t.Fatalf("应命中缓存: err=%v cached=%v", err, cached)
	}
	fi, err := os.Stat(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	if fi.ModTime().Before(time.Now().Add(-time.Minute)) {
		t.Fatalf("命中后缓存 mtime 应刷新到当前时间，得到 %s", fi.ModTime())
	}
}
