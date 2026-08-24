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

var digitChunkRegexp = regexp.MustCompile(`\d+|\D+`)

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

// scanPageList 原 GetPageList 扫描逻辑（不缓存）
func scanPageList(localPath string, isDir bool) ([]string, error) {
	var images []string

	// 1. 散图文件夹
	if isDir {
		err := filepath.WalkDir(localPath, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if !d.IsDir() && IsImage(d.Name()) {
				// 记录相对路径
				rel, _ := filepath.Rel(localPath, path)
				images = append(images, rel)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	} else if IsArchive(localPath) {
		// 2. ZIP / CBZ 压缩包
		r, err := zip.OpenReader(localPath)
		if err != nil {
			return nil, err
		}
		defer r.Close()

		for _, f := range r.File {
			if !f.FileInfo().IsDir() && IsImage(f.Name) {
				images = append(images, f.Name)
			}
		}
	}

	sortFilenames(images)
	return images, nil
}

// GetPageData 获取特定页码的图片二进制流（物理页索引）
func GetPageData(localPath string, pageIndex int) ([]byte, string, error) {
	pages, err := GetPageList(localPath)
	if err != nil || pageIndex < 0 || pageIndex >= len(pages) {
		return nil, "", errors.New("页码超出范围")
	}
	return getPageDataByName(localPath, pages[pageIndex])
}

// GetVisiblePageData 获取「有效页索引」对应的图片二进制流（自动跳过隐藏页，Round23）。
// pageIndex 为剔除隐藏页后的有效序号（0-based）：第 N 张可见图 = 物理页列表中第 N 个未隐藏项。
func GetVisiblePageData(localPath string, pageIndex int, hidden []int) ([]byte, string, error) {
	pages, err := GetPageList(localPath)
	if err != nil {
		return nil, "", err
	}
	visible := FilterHiddenPages(pages, hidden)
	if pageIndex < 0 || pageIndex >= len(visible) {
		return nil, "", errors.New("页码超出范围")
	}
	return getPageDataByName(localPath, visible[pageIndex])
}

// getPageDataByName 按物理文件名读取图片二进制流（文件夹直读 / ZIP 定位读取）
func getPageDataByName(localPath string, targetFile string) ([]byte, string, error) {
	fi, _ := os.Stat(localPath)

	// 1. 散图文件夹直接读取文件
	if fi.IsDir() {
		fullPath := filepath.Join(localPath, targetFile)
		data, err := os.ReadFile(fullPath)
		if err != nil {
			return nil, "", err
		}
		return data, getContentType(targetFile), nil
	}

	// 2. ZIP 压缩包定位读取
	if IsArchive(localPath) {
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