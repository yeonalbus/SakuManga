package services

import (
	"archive/zip"
	"encoding/json"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// 1. ComicInfo.xml 结构体映射
type ComicInfoXML struct {
	XMLName   xml.Name `xml:"ComicInfo"`
	Title     string   `xml:"Title"`
	Series    string   `xml:"Series"`
	Summary   string   `xml:"Summary"`
	Writer    string   `xml:"Writer"`
	Penciller string   `xml:"Penciller"`
	Tags      string   `xml:"Tags"`  // 逗号分隔的标签
	Genre     string   `xml:"Genre"` // 分类
}

// 2. EH/JH 下载附带的 JSON 元数据结构体 (metadata / ametadata)
type EHMetadataJSON struct {
	Title    string      `json:"title"`
	TitleJpn string      `json:"title_jpn"`
	Category string      `json:"category"`
	Uploader string      `json:"uploader"`
	Rating   interface{} `json:"rating"` // 可能是 string 或 float64
	Tags     []string    `json:"tags"`
}

// Struct 用于存储解析汇总结果
type ParsedMetadata struct {
	Title    string
	Category string
	Tags     []string
}

// 从散图文件夹中读取元数据
func ParseDirMetadata(dirPath string) *ParsedMetadata {
	result := &ParsedMetadata{
		Tags: []string{},
	}

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return result
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := strings.ToLower(entry.Name())
		fullPath := filepath.Join(dirPath, entry.Name())

		// A. 匹配 JSON 元数据 (metadata / ametadata / info.json 等)
		if name == "metadata" || name == "ametadata" || name == "info.json" || strings.HasSuffix(name, ".json") {
			if data, err := os.ReadFile(fullPath); err == nil {
				var jsonMeta EHMetadataJSON
				if err := json.Unmarshal(data, &jsonMeta); err == nil {
					if jsonMeta.Title != "" {
						result.Title = jsonMeta.Title
					}
					if jsonMeta.Category != "" {
						result.Category = jsonMeta.Category
					}
					if len(jsonMeta.Tags) > 0 {
						result.Tags = append(result.Tags, jsonMeta.Tags...)
					}
				}
			}
		}

		// B. 匹配 ComicInfo.xml
		if name == "comicinfo.xml" {
			if data, err := os.ReadFile(fullPath); err == nil {
				var xmlMeta ComicInfoXML
				if err := xml.Unmarshal(data, &xmlMeta); err == nil {
					if result.Title == "" && xmlMeta.Title != "" {
						result.Title = xmlMeta.Title
					}
					if result.Category == "" && xmlMeta.Genre != "" {
						result.Category = xmlMeta.Genre
					}
					if xmlMeta.Tags != "" {
						xmlTags := strings.Split(xmlMeta.Tags, ",")
						for _, t := range xmlTags {
							trimmed := strings.TrimSpace(t)
							if trimmed != "" {
								result.Tags = append(result.Tags, trimmed)
							}
						}
					}
				}
			}
		}
	}

	result.Tags = removeDuplicateTags(result.Tags)
	return result
}

// 从 ZIP / CBZ 压缩包内读取元数据
func ParseZipMetadata(zipPath string) *ParsedMetadata {
	result := &ParsedMetadata{
		Tags: []string{},
	}

	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return result
	}
	defer r.Close()

	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name := strings.ToLower(filepath.Base(f.Name))

		if name == "metadata" || name == "ametadata" || name == "info.json" || name == "comicinfo.xml" {
			rc, err := f.Open()
			if err != nil {
				continue
			}
			data, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				continue
			}

			// JSON 解析
			if name != "comicinfo.xml" {
				var jsonMeta EHMetadataJSON
				if err := json.Unmarshal(data, &jsonMeta); err == nil {
					if jsonMeta.Title != "" {
						result.Title = jsonMeta.Title
					}
					if jsonMeta.Category != "" {
						result.Category = jsonMeta.Category
					}
					if len(jsonMeta.Tags) > 0 {
						result.Tags = append(result.Tags, jsonMeta.Tags...)
					}
				}
			} else {
				// XML 解析
				var xmlMeta ComicInfoXML
				if err := xml.Unmarshal(data, &xmlMeta); err == nil {
					if result.Title == "" && xmlMeta.Title != "" {
						result.Title = xmlMeta.Title
					}
					if result.Category == "" && xmlMeta.Genre != "" {
						result.Category = xmlMeta.Genre
					}
					if xmlMeta.Tags != "" {
						xmlTags := strings.Split(xmlMeta.Tags, ",")
						for _, t := range xmlTags {
							trimmed := strings.TrimSpace(t)
							if trimmed != "" {
								result.Tags = append(result.Tags, trimmed)
							}
						}
					}
				}
			}
		}
	}

	result.Tags = removeDuplicateTags(result.Tags)
	return result
}

// 标签去重辅助函数
func removeDuplicateTags(tags []string) []string {
	keys := make(map[string]bool)
	list := []string{}
	for _, entry := range tags {
		if _, value := keys[entry]; !value {
			keys[entry] = true
			list = append(list, entry)
		}
	}
	return list
}