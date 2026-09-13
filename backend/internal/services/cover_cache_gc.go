package services

import (
	"log"
	"os"
	"path/filepath"
	"time"
)

// ─────────────────────────────────────────────────────────────
// Round39：封面缓存自动回收（TTL 7 天，按「最后使用时间」淘汰）
//
// 背景：cover_cache/ 同时容纳两类缓存——离线封面缩略图（<comicID>.jpg）与
// 在线封面代理缓存（proxy_<hash>.<ext>），此前只增不减，浏览过的封面永久占盘。
//
// 关键取舍：淘汰依据用「最后使用时间」而非「创建时间」。
// 缓存命中路径（GetCoverThumb / ProxyCover）原本不更新文件时间戳，若直接删
// 「创建超过 7 天」的文件，热门封面同样到期被删——而离线封面重建要打开整个
// ZIP 解压首图（Round14 正是为治此卡顿才引入缩略图缓存），代价极高。
// 故命中时调用 TouchCoverCacheFile 把 mtime 推到 now（模拟 atime），
// 只回收「超过 TTL 未被访问」的文件，语义等价于 LRU 淘汰。
//
// 注：命中判定仍是「缓存 mtime ≥ 源文件 mtime」。touch 只把缓存时间戳推新，
// 故「源文件被替换为时间戳更早的文件」依旧不触发重建——与本次改动前一致。
// ─────────────────────────────────────────────────────────────

const (
	// CoverCacheTTL 封面缓存保留期：超过该时长未被访问即回收
	CoverCacheTTL = 7 * 24 * time.Hour
	// coverCacheTouchInterval touch 降频阈值：mtime 距今不足该时长则不重复写盘
	// （避免每个封面请求都触发一次 utimes 系统调用）
	coverCacheTouchInterval = 24 * time.Hour
	// coverCacheGCInterval 定时清理周期
	coverCacheGCInterval = 6 * time.Hour
	// coverCacheGCInitialDelay 首轮清理延迟：避开启动瞬间首页数十张封面并发生成抢 IO
	coverCacheGCInitialDelay = 2 * time.Minute
)

// TouchCoverCacheFile 刷新缓存文件的「最后使用时间」（缓存命中时调用）。
// 降频：仅当 mtime 距今超过 coverCacheTouchInterval 才写入；
// 文件缺失等错误一律忽略——打点失败不该影响封面响应主流程。
func TouchCoverCacheFile(path string) {
	if path == "" {
		return
	}
	fi, err := os.Stat(path)
	if err != nil || !fi.Mode().IsRegular() {
		return
	}
	now := time.Now()
	if now.Sub(fi.ModTime()) < coverCacheTouchInterval {
		return
	}
	_ = os.Chtimes(path, now, now)
}

// CleanCoverCache 删除 cover_cache 目录下最后使用时间早于 now-ttl 的缓存文件。
// 返回（删除文件数, 释放字节数, 保留文件数）；目录不存在视为无事发生。
// 单个文件删除失败（如 Windows 下正被 c.File 读取）只跳过，不中断整轮清理。
func CleanCoverCache(ttl time.Duration) (removed int, freed int64, kept int, err error) {
	entries, err := os.ReadDir(coverCacheDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, 0, 0, nil
		}
		return 0, 0, 0, err
	}
	deadline := time.Now().Add(-ttl)
	for _, e := range entries {
		// 只回收普通文件：子目录、符号链接等一律不动
		if e.IsDir() {
			continue
		}
		fi, statErr := e.Info()
		if statErr != nil || !fi.Mode().IsRegular() {
			continue
		}
		if !fi.ModTime().Before(deadline) {
			kept++
			continue
		}
		if rmErr := os.Remove(filepath.Join(coverCacheDir, e.Name())); rmErr != nil {
			kept++
			continue
		}
		removed++
		freed += fi.Size()
	}
	return removed, freed, kept, nil
}

// StartCoverCacheGC 启动封面缓存定时回收（goroutine 内运行，永不返回）：
// 启动后延迟 coverCacheGCInitialDelay 首跑，之后每 coverCacheGCInterval 一轮。
func StartCoverCacheGC() {
	go func() {
		time.Sleep(coverCacheGCInitialDelay)
		for {
			runCoverCacheGC()
			time.Sleep(coverCacheGCInterval)
		}
	}()
}

// runCoverCacheGC 执行一轮清理；无删除时不打日志（避免长期空转刷日志）
func runCoverCacheGC() {
	removed, freed, kept, err := CleanCoverCache(CoverCacheTTL)
	if err != nil {
		log.Printf("[covergc] 封面缓存清理失败: %v", err)
		return
	}
	if removed == 0 {
		return
	}
	log.Printf("[covergc] 封面缓存回收：删除 %d 个（释放 %.1f MB），保留 %d 个",
		removed, float64(freed)/(1024*1024), kept)
}
