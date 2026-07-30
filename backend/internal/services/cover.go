package services

import (
	"archive/zip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func IsImage(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp", ".gif", ".bmp", ".avif":
		return true
	default:
		return false
	}
}

func IsArchive(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".zip", ".cbz", ".rar", ".7z":
		return true
	default:
		return false
	}
}

// GetCoverFromDir 递归查找文件夹（或深层子文件夹）中的第一张图片
func GetCoverFromDir(dirPath string) (string, error) {
	var firstImg string

	err := filepath.WalkDir(dirPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() && IsImage(d.Name()) {
			firstImg = path
			return filepath.SkipAll // 找到第一张图立即终止递归
		}
		return nil
	})

	if firstImg != "" {
		return firstImg, nil
	}
	if err != nil {
		return "", err
	}
	return "", errors.New("未找到图片")
}

// GetCoverFromZip 从 ZIP/CBZ 压缩包中流式读取第一张图片数据
func GetCoverFromZip(zipPath string) ([]byte, string, error) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, "", err
	}
	defer r.Close()

	for _, f := range r.File {
		if !f.FileInfo().IsDir() && IsImage(f.Name) {
			rc, err := f.Open()
			if err != nil {
				return nil, "", err
			}
			defer rc.Close()

			data, err := io.ReadAll(rc)
			if err != nil {
				return nil, "", err
			}

			ext := strings.ToLower(filepath.Ext(f.Name))
			contentType := "image/jpeg"
			switch ext {
			case ".png":
				contentType = "image/png"
			case ".webp":
				contentType = "image/webp"
			case ".gif":
				contentType = "image/gif"
			}
			return data, contentType, nil
		}
	}
	return nil, "", errors.New("压缩包内未找到图片")
}