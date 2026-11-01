package services

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"SakuManga/internal/models"
)

// ─────────────────────────────────────────────────────────────
// Round52：离线画廊「页缩略图」缓存
//
// 背景：维护页展开对比要一次显示一本前若干页的缩略图（点开看原图），
// 直接用 /raw-page 原图会重演 Round14 封面卡顿问题（每张几 MB + zip 解压解码）。
//
// 方案：首次请求按有效页索引取页图 → 等比缩放到宽 PageThumbWidth → JPEG 写
// <dataDir>/page_cache/<comicId>/<index>.jpg；命中直接回缓存文件（带 ETag/强缓存）。
// 复用封面的思路：per(comic,page) 单飞去重 + 全局解码信号量限流 + 源 mtime 变更检测。
// 页索引与 /comics/:id/page/:index 一致（自动跳过用户隐藏页）。
// ─────────────────────────────────────────────────────────────

// PageThumbWidth 页缩略图目标宽度（维护页缩略图墙 64px 展示，240px 足够清晰且体积小）
const PageThumbWidth = 240

// pageCacheDir 页缩略图缓存目录（与 manga.db / cover_cache 同级）
const pageCacheDir = "page_cache"

var pageCacheMu sync.Mutex

// pageGenSem 页缩略图解码并发信号量（同时最多 2 个，避免打满 CPU）
var pageGenSem = make(chan struct{}, 2)

// pageGenMu 保护单飞表
var pageGenMu sync.Mutex
var pageGenerating = map[string]chan struct{}{} // "<comicID>/<index>" → 完成通知

func pageCachePath(comicID string, index int) string {
	return filepath.Join(pageCacheDir, comicID, fmt.Sprintf("%d.jpg", index))
}

func ensurePageCacheDir(comicID string) error {
	pageCacheMu.Lock()
	defer pageCacheMu.Unlock()
	return os.MkdirAll(filepath.Join(pageCacheDir, comicID), 0o755)
}

func beginPageGen(key string) (chan struct{}, bool) {
	pageGenMu.Lock()
	defer pageGenMu.Unlock()
	if ch, ok := pageGenerating[key]; ok {
		return ch, true
	}
	ch := make(chan struct{})
	pageGenerating[key] = ch
	return ch, false
}

func endPageGen(key string) {
	pageGenMu.Lock()
	if ch, ok := pageGenerating[key]; ok {
		delete(pageGenerating, key)
		close(ch)
	}
	pageGenMu.Unlock()
}

// GetPageThumb 获取某离线漫画某个有效页的缩略图。
//   - 缓存命中且源未变更 → 返回 (nil, cachePath, true)，零 IO（除 stat）；
//   - 未命中 → 读页图 → 缩放到 240px → 写缓存；
//   - 无法解码/无需缩放（页图本身很窄）→ 返回原图字节（调用方直传）。
func GetPageThumb(comic models.OfflineComic, index int) (data []byte, cachePath string, cached bool, err error) {
	if index < 0 {
		return nil, "", false, fmt.Errorf("无效页码: %d", index)
	}
	cachePath = pageCachePath(comic.ID, index)

	srcMod, err := pageSourceModTime(comic)
	if err != nil {
		return nil, "", false, err
	}
	// 缓存命中短路
	if fi, statErr := os.Stat(cachePath); statErr == nil {
		if srcMod.IsZero() || !fi.ModTime().Before(srcMod) {
			return nil, cachePath, true, nil
		}
	}

	key := fmt.Sprintf("%s/%d", comic.ID, index)
	waitCh, waiting := beginPageGen(key)
	if waiting {
		<-waitCh
		if _, statErr := os.Stat(cachePath); statErr == nil {
			return nil, cachePath, true, nil
		}
		// 对方生成失败 → 自行生成
	}
	defer endPageGen(key)

	pageGenSem <- struct{}{}
	defer func() { <-pageGenSem }()

	hidden := ParseHiddenPages(comic.HiddenPages)
	srcData, _, err := GetVisiblePageData(comic.LocalPath, index, hidden)
	if err != nil {
		return nil, "", false, err
	}
	thumb, ok := decodeAndResizeTo(srcData, PageThumbWidth)
	if !ok {
		// 解码失败或原图比目标宽还小：直接回源图
		return srcData, "", false, nil
	}
	if err := ensurePageCacheDir(comic.ID); err != nil {
		return nil, "", false, err
	}
	if err := os.WriteFile(cachePath, thumb, 0o644); err != nil {
		return nil, "", false, err
	}
	return nil, cachePath, true, nil
}

// pageSourceModTime 页缩略图的缓存失效基准：归档取包 mtime，文件夹取目录 mtime。
// 与封面一致（目录内同名覆盖写不改 mtime 的盲区同样存在，属可接受代价）。
func pageSourceModTime(comic models.OfflineComic) (time.Time, error) {
	fi, err := os.Stat(comic.LocalPath)
	if err != nil {
		return time.Time{}, err
	}
	return fi.ModTime(), nil
}

// PageCacheDir 返回页缩略图缓存目录（供清理逻辑复用）
func PageCacheDir() string { return pageCacheDir }

// CleanPageCacheForMissingComics 清理已不存在的漫画的页缓存目录。
// 返回删除的目录数。供 GC 任务调用（可选）。
func CleanPageCacheForMissingComics(existingIDs map[string]bool) int {
	entries, err := os.ReadDir(pageCacheDir)
	if err != nil {
		return 0
	}
	removed := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if existingIDs[e.Name()] {
			continue
		}
		if err := os.RemoveAll(filepath.Join(pageCacheDir, e.Name())); err != nil {
			log.Printf("[page_cache] 清理 %s 失败: %v", e.Name(), err)
			continue
		}
		removed++
	}
	return removed
}
