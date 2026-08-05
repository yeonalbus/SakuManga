package services

import (
	"SakuHentai/internal/database"
	"SakuHentai/internal/models"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"log"
	"os"
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
		var fileSize int64
		if isDir {
			entries, _ := os.ReadDir(localPath)
			for _, entry := range entries {
				if !entry.IsDir() && IsImage(entry.Name()) {
					pageCount++
				}
				if !entry.IsDir() {
					if fi, err := entry.Info(); err == nil {
						fileSize += fi.Size()
					}
				}
			}
		} else {
			if fi, err := os.Stat(localPath); err == nil {
				fileSize = fi.Size()
			}
		}
	
		comicID := generateID(localPath)
		coverURL := "/api/v1/comics/" + comicID + "/cover"
	
		// 🎯 2. gid 查重：同一 gid 已有记录时，优先保留「文件夹」形态（画廊下载），跳过「压缩包」形态（归档）
		if meta.GID != "" {
			var existing models.OfflineComic
			err := database.DB.Where("g_id = ? AND id != ?", meta.GID, comicID).First(&existing).Error
			if err == nil {
				existingIsDir := existing.SourceMode == "gallery" ||
					!strings.HasSuffix(strings.ToLower(existing.LocalPath), ".zip")
				if existingIsDir && !isDir {
					// 已存在文件夹形态（画廊下载），跳过压缩包形态（归档）
					log.Printf("%s [scan] 跳过同 gid=%s 的压缩包（已存在文件夹形态 %q）: %q",
						dlLogTag, meta.GID, existing.LocalPath, localPath)
					return
				}
				if !existingIsDir && isDir {
					// 新的是文件夹形态，删除旧的压缩包记录，保留文件夹
					log.Printf("%s [scan] gid=%s 由压缩包 %q 升级为文件夹 %q，移除旧压缩包记录",
						dlLogTag, meta.GID, existing.LocalPath, localPath)
					database.DB.Delete(&existing)
				}
			}
		}
	
		sourceMode := "gallery"
		if !isDir {
			sourceMode = "archive"
		}
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
			FileSize:     fileSize,
			GID:          meta.GID,
			Token:        meta.Token,
			ParentGID:    meta.ParentGID,
			SourceMode:   sourceMode,
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

		// 根目录自身直接平铺图片（归档解压落地后 extractDir 直接含图片，非子目录形态）
		rootHasImage := false
		for _, entry := range entries {
			if !entry.IsDir() && IsImage(entry.Name()) {
				rootHasImage = true
				break
			}
		}
		if rootHasImage {
			saveComic(rootPath, true)
			return 1, nil
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