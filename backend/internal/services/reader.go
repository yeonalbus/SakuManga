package services

import (
	"archive/zip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// 自然排序拆块：数字块按数值比较，非数字块按字符串比较。
// 注意：`.` 单独成块（不可并入 \D+），否则 "img.jpg" 会被贪婪匹配为
// 整块 ["img.jpg"]，与 "img0001.jpg" 的 ["img","0001",".jpg"] 比较时，
// 短前缀 "img" < "img.jpg" 会把 img.jpg 错误排到 img0029.jpg 之后（Round25-Bug1）。
var digitChunkRegexp = regexp.MustCompile(`\d+|[^\d.]+|\.`)

// 自然文件名排序：将字符串拆分为“数字”与“非数字”块分别比较
func sortFilenames(files []string) {
	sort.Slice(files, func(i, j int) bool {
		strI := strings.ToLower(files[i])
		strJ := strings.ToLower(files[j])

		chunksI := digitChunkRegexp.FindAllString(strI, -1)
		chunksJ := digitChunkRegexp.FindAllString(strJ, -1)

		minLen := len(chunksI)
		if len(chunksJ) < minLen {
			minLen = len(chunksJ)
		}

		for k := 0; k < minLen; k++ {
			if chunksI[k] != chunksJ[k] {
				numI, errI := strconv.Atoi(chunksI[k])
				numJ, errJ := strconv.Atoi(chunksJ[k])

				// 两边都是纯数字块时，按数值大小比较 (10 > 2)
				if errI == nil && errJ == nil {
					return numI < numJ
				}
				// 否则按字符串字典序比较
				return chunksI[k] < chunksJ[k]
			}
		}

		return len(chunksI) < len(chunksJ)
	})
}

// ─────────────────────────────────────────────────────────────
// Round24-P0-3：页列表内存缓存
//
// 根因：GetPageData/GetVisiblePageData 每次翻页都调用 GetPageList，
// 对 zip 每次 zip.OpenReader（读中央目录）+ 遍历全部条目，N150 磁盘 IO 慢 → 翻页卡顿。
// 方案：内存缓存页列表。
//   - 归档：以文件 modtime 精确失效（zip 内容变化 → mtime 变化）；
//   - 散图目录：父目录 mtime 快检 + 30s TTL 兜底（子目录内图片增删不改变父目录 mtime，
//     用 TTL 保证新增图片最迟 30s 内可见；翻页高频场景 100% 命中缓存）。
// ─────────────────────────────────────────────────────────────

var pageListCacheMu sync.Mutex
var pageListCache = map[string]pageListEntry{}
const pageListCacheMax = 512 // 超限整体清空（简单 LRU 替代，够用）
const dirPageListTTL = 30 * time.Second

type pageListEntry struct {
	modTime   time.Time // 归档=文件 mtime；目录=父目录 mtime（快检）
	fetchedAt time.Time // 目录 TTL 基准
	isDir     bool
	pages     []string
}

// GetPageList 获取画廊内所有图片的相对路径/文件名列表（带内存缓存，Round24）
func GetPageList(localPath string) ([]string, error) {
	fi, err := os.Stat(localPath)
	if err != nil {
		return nil, err
	}
	isDir := fi.IsDir()
	mod := fi.ModTime()
	now := time.Now()

	// 命中检查
	pageListCacheMu.Lock()
	if e, ok := pageListCache[localPath]; ok {
		stale := false
		if e.isDir {
			if now.Sub(e.fetchedAt) > dirPageListTTL {
				stale = true
			} else if !e.modTime.Equal(mod) {
				stale = true
			}
		} else if !e.modTime.Equal(mod) {
			stale = true
		}
		if !stale {
			pages := e.pages
			pageListCacheMu.Unlock()
			return pages, nil
		}
	}
	pageListCacheMu.Unlock()

	// 未命中 → 扫描
	pages, err := scanPageList(localPath, isDir)
	if err != nil {
		return nil, err
	}

	// 写缓存
	pageListCacheMu.Lock()
	if len(pageListCache) >= pageListCacheMax {
		pageListCache = map[string]pageListEntry{}
	}
	pageListCache[localPath] = pageListEntry{
		modTime:   mod,
		fetchedAt: now,
		isDir:     isDir,
		pages:     pages,
	}
	pageListCacheMu.Unlock()
	return pages, nil
}

// ErrUnsupportedArchive 识别但不可读的归档格式（D4：当前仅支持 zip/cbz）
var ErrUnsupportedArchive = errors.New("不支持的归档格式（当前仅支持 zip/cbz）")

// hasHiddenSegment 路径中任一段以 "." 开头（D1 全局忽略规则；兼容 / 与 \ 分隔）
func hasHiddenSegment(name string) bool {
	for _, seg := range strings.FieldsFunc(name, func(r rune) bool { return r == '/' || r == '\\' }) {
		if isHiddenEntry(seg) {
			return true
		}
	}
	return false
}

// listImagesRecursive 返回目录内全部图片的相对路径：
// 递归任意深度子目录、跳过 "." 开头的隐藏目录/文件、统一为 "/" 分隔、自然排序。
// 该函数是「页表」的唯一实现（D3），扫描入库的页数与阅读器页表共用它。
func listImagesRecursive(dir string) ([]string, error) {
	var images []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if path == dir {
			return nil
		}
		if isHiddenEntry(d.Name()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.IsDir() && IsImage(d.Name()) {
			rel, rerr := filepath.Rel(dir, path)
			if rerr != nil {
				rel = path
			}
			images = append(images, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sortFilenames(images)
	return images, nil
}

// listImagesInZip 返回 zip/cbz 内全部图片条目名（跳过隐藏段、自然排序）。
func listImagesInZip(zipPath string) ([]string, error) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	images := make([]string, 0, len(r.File))
	for _, f := range r.File {
		if f.FileInfo().IsDir() || hasHiddenSegment(f.Name) {
			continue
		}
		if IsImage(f.Name) {
			images = append(images, filepath.ToSlash(f.Name))
		}
	}
	sortFilenames(images)
	return images, nil
}

// scanPageList 原 GetPageList 扫描逻辑（不缓存）
func scanPageList(localPath string, isDir bool) ([]string, error) {
	if isDir {
		// 1. 散图文件夹：递归页表（D3）
		return listImagesRecursive(localPath)
	}
	// 2. 归档：仅 zip/cbz 可读（D4）
	if IsUnsupportedArchive(localPath) {
		return nil, ErrUnsupportedArchive
	}
	if !IsSupportedArchive(localPath) {
		return nil, errors.New("不支持的格式")
	}
	return listImagesInZip(localPath)
}

// GetPageData 获取特定页码的图片二进制流（物理页索引）
func GetPageData(localPath string, pageIndex int) ([]byte, string, error) {
	pages, err := GetPageList(localPath)
	if err != nil || pageIndex < 0 || pageIndex >= len(pages) {
		return nil, "", errors.New("页码超出范围")
	}
	return getPageDataByName(localPath, pages[pageIndex])
}

// getPageDataByName 按物理文件名读取图片二进制流（文件夹直读 / ZIP 定位读取）
func getPageDataByName(localPath string, targetFile string) ([]byte, string, error) {
	fi, _ := os.Stat(localPath)

	// 1. 散图文件夹直接读取文件
	if fi.IsDir() {
		fullPath := filepath.Join(localPath, filepath.FromSlash(targetFile))
		data, err := os.ReadFile(fullPath)
		if err != nil {
			return nil, "", err
		}
		return data, getContentType(targetFile), nil
	}

	// 2. 归档：仅 zip/cbz 可读（D4）
	if IsUnsupportedArchive(localPath) {
		return nil, "", ErrUnsupportedArchive
	}
	if IsSupportedArchive(localPath) {
		r, err := zip.OpenReader(localPath)
		if err != nil {
			return nil, "", err
		}
		defer r.Close()

		for _, f := range r.File {
			if f.Name == targetFile {
				rc, err := f.Open()
				if err != nil {
					return nil, "", err
				}
				defer rc.Close()
				data, err := io.ReadAll(rc)
				if err != nil {
					return nil, "", err
				}
				return data, getContentType(targetFile), nil
			}
		}
	}

	return nil, "", errors.New("读取页面失败")
}

func getContentType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	case ".avif":
		return "image/avif"
	default:
		return "image/jpeg"
	}
}