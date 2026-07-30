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

// GetPageList 获取画廊内所有图片的相对路径/文件名列表
func GetPageList(localPath string) ([]string, error) {
	fi, err := os.Stat(localPath)
	if err != nil {
		return nil, err
	}

	var images []string

	// 1. 散图文件夹
	if fi.IsDir() {
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

// GetPageData 获取特定页码的图片二进制流
func GetPageData(localPath string, pageIndex int) ([]byte, string, error) {
	pages, err := GetPageList(localPath)
	if err != nil || pageIndex < 0 || pageIndex >= len(pages) {
		return nil, "", errors.New("页码超出范围")
	}

	targetFile := pages[pageIndex]
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