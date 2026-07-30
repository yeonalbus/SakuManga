package services

import (
	"crypto/md5"
	"encoding/hex"
	"SakuHentai/internal/database"
	"SakuHentai/internal/models"
	"os"
	"encoding/json"
	"path/filepath"
	"strings"
	"time"
)

func generateID(path string) string {
	hash := md5.Sum([]byte(path))
	return hex.EncodeToString(hash[:])
}

func ScanAndSaveDirectory(rootPath string, includeSubfolders bool) (int, error) {
	if _, err := os.Stat(rootPath); os.IsNotExist(err) {
		return 0, err
	}

	comicCount := 0

	saveComic := func(localPath string, isDir bool) {
		title := filepath.Base(localPath)
		if !isDir {
			title = strings.TrimSuffix(title, filepath.Ext(title))
		}
	
		// 🎯 1. 尝试解析本地/压缩包内部的 metadata、ametadata 以及 ComicInfo.xml
		var meta *ParsedMetadata
		if isDir {
			meta = ParseDirMetadata(localPath)
		} else {
			meta = ParseZipMetadata(localPath)
		}
	
		// 优先使用元数据中的标题与分类
		if meta.Title != "" {
			title = meta.Title
		}
		category := "Doujinshi"
		if meta.Category != "" {
			category = meta.Category
		}
	
		// 默认兜底标签
		tags := meta.Tags
		if len(tags) == 0 {
			tags = []string{"本地扫描"}
		}
		tagsJSON, _ := json.Marshal(tags)
	
		pageCount := 0
		if isDir {
			entries, _ := os.ReadDir(localPath)
			for _, entry := range entries {
				if !entry.IsDir() && IsImage(entry.Name()) {
					pageCount++
				}
			}
		}
	
		comicID := generateID(localPath)
		coverURL := "http://localhost:8081/api/v1/comics/" + comicID + "/cover"
	
		comic := models.OfflineComic{
			ID:           comicID,
			Title:        title,
			CoverURL:     coverURL,
			Source:       models.SourceOffline,
			Category:     category,             // 写入解析出的分类
			Tags:         string(tagsJSON),     // 写入解析出的多元标签数组 JSON
			PageCount:    pageCount,
			UpdatedAt:    time.Now(),
			IsDownloaded: true,
			LocalPath:    localPath,
		}
	
		database.DB.Save(&comic)
		comicCount++
	}

	if includeSubfolders {
		err := filepath.WalkDir(rootPath, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}

			if d.IsDir() {
				entries, err := os.ReadDir(path)
				if err != nil {
					return nil
				}

				hasImage := false
				for _, entry := range entries {
					if !entry.IsDir() && IsImage(entry.Name()) {
						hasImage = true
						break
					}
				}

				if hasImage {
					saveComic(path, true)
					if path != rootPath {
						return filepath.SkipDir
					}
				}
				return nil
			}

			if !d.IsDir() && IsArchive(d.Name()) {
				saveComic(path, false)
			}

			return nil
		})
		return comicCount, err
	} else {
		entries, err := os.ReadDir(rootPath)
		if err != nil {
			return 0, err
		}

		for _, entry := range entries {
			fullPath := filepath.Join(rootPath, entry.Name())
			if entry.IsDir() {
				subEntries, err := os.ReadDir(fullPath)
				if err != nil {
					continue
				}
				for _, sub := range subEntries {
					if !sub.IsDir() && IsImage(sub.Name()) {
						saveComic(fullPath, true)
						break
					}
				}
			} else if IsArchive(entry.Name()) {
				saveComic(fullPath, false)
			}
		}
		return comicCount, nil
	}
}