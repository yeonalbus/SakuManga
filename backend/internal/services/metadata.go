package services

import (
	"archive/zip"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"SakuHentai/internal/models"
)

// 1. ComicInfo.xml 结构体映射
// 字段对齐 JHentai/ComicInfo/comic_info.dart 的 EHGalleryComicInfo 输出
// （Title/Series/AlternateSeries/Writer/Penciller/Genre/Tags/Web/PageCount/
//   LanguageISO/BlackAndWhite/Manga/Characters/Locations/AgeRating/CommunityRating）。
// 可选字段使用 omitempty：仅在非空时输出，与 JHentai 的 isEmptyOrNull 判断一致。
type ComicInfoXML struct {
	XMLName         xml.Name `xml:"ComicInfo"`
	Title           string   `xml:"Title"`
	Series          string   `xml:"Series"`
	AlternateSeries string   `xml:"AlternateSeries,omitempty"`
	Writer          string   `xml:"Writer,omitempty"`    // 作者（artist 标签逗号连接）
	Penciller       string   `xml:"Penciller,omitempty"` // 画师（同 artist 标签）
	Genre           string   `xml:"Genre"`
	Tags            string   `xml:"Tags"` // 全部标签 namespace:key 逗号连接
	Web             string   `xml:"Web,omitempty"`
	PageCount       int      `xml:"PageCount"`
	LanguageISO     string   `xml:"LanguageISO,omitempty"`
	BlackAndWhite   string   `xml:"BlackAndWhite,omitempty"` // Yes / No（存在 full color 标签时为 No）
	Manga           string   `xml:"Manga,omitempty"`         // Yes（分类=Manga）/ No
	Characters      string   `xml:"Characters,omitempty"`
	Locations       string   `xml:"Locations,omitempty"`
	AgeRating       string   `xml:"AgeRating,omitempty"` // Non-H=Kids to Adults，否则 Adults Only 18+
	CommunityRating string   `xml:"CommunityRating,omitempty"`

	// 以下为标准 ComicInfo 可选字段（仅解析用，默认不输出）
	Summary         string `xml:"Summary,omitempty"`
	Notes           string `xml:"Notes,omitempty"`
	Number          string `xml:"Number,omitempty"`
	Inker           string `xml:"Inker,omitempty"`
	Format          string `xml:"Format,omitempty"`
	Teams           string `xml:"Teams,omitempty"`
	ScanInformation string `xml:"ScanInformation,omitempty"`
}

// buildFullComicInfo 依据画廊详情 + 任务字段构建完整元数据（参考 JHentai EHGalleryComicInfo 映射）。
// detail 可为 nil：此时回退到任务字段的最小集（标题 + 分类 Doujinshi）。
// galleryURL 为画廊详情页 URL（Web 字段），可为空（为空则省略 Web）。
func buildFullComicInfo(task *models.DownloadTask, detail *GalleryDetailResult, galleryURL string) (ComicInfoXML, EHMetadataJSON) {
	title := ""
	titleJpn := ""
	category := ""
	uploader := ""
	tags := []string{}
	parentGID := ""
	gid := ""
	token := ""
	pageCount := 0
	filesize := int64(0)
	rating := 0.0

	if task != nil {
		title = task.Title
		gid = task.GID
		token = task.Token
		pageCount = task.TotalFiles
		filesize = task.DoneBytes
	}

	if detail != nil {
		// 英文标题 (#gn = SubTitle)，日文标题 (#gj = Title，仅当与英文不同)
		if detail.SubTitle != "" {
			title = detail.SubTitle
		} else if detail.Title != "" {
			title = detail.Title
		}
		if detail.Title != "" && detail.Title != title {
			titleJpn = detail.Title
		}
		if detail.Category != "" {
			category = detail.Category
		}
		uploader = detail.Uploader
		tags = detail.Tags
		parentGID = detail.ParentGID
		if detail.PageCount > 0 {
			pageCount = detail.PageCount
		}
		rating = detail.Rating
		if detail.ID != "" {
			gid = detail.ID
		}
		if detail.Token != "" {
			token = detail.Token
		}
	}
	if title == "" {
		title = "untitled"
	}
	if category == "" {
		category = "Doujinshi"
	}

	// 从 tags（"namespace:key"）拆出 artist / character / location / full color
	artistTags := []string{}
	characterTags := []string{}
	locationTags := []string{}
	fullColor := false
	for _, t := range tags {
		ns, key := t, ""
		if i := strings.Index(t, ":"); i >= 0 {
			ns, key = t[:i], t[i+1:]
		} else {
			key = t
		}
		switch ns {
		case "artist":
			artistTags = append(artistTags, key)
		case "character":
			characterTags = append(characterTags, key)
		case "location":
			locationTags = append(locationTags, key)
		}
		if strings.EqualFold(key, "full color") {
			fullColor = true
		}
	}

	blackAndWhite := "Yes"
	if fullColor {
		blackAndWhite = "No"
	}
	manga := "No"
	if category == "Manga" {
		manga = "Yes"
	}
	ageRating := "Adults Only 18+"
	if category == "Non-H" {
		ageRating = "Kids to Adults"
	}

	xmlMeta := ComicInfoXML{
		Title:           title,
		Series:          title,
		AlternateSeries: titleJpn,
		Writer:          strings.Join(artistTags, ","),
		Penciller:       strings.Join(artistTags, ","),
		Genre:           category,
		Tags:            strings.Join(tags, ","),
		Web:             galleryURL,
		PageCount:       pageCount,
		BlackAndWhite:   blackAndWhite,
		Manga:           manga,
		Characters:      strings.Join(characterTags, ","),
		Locations:       strings.Join(locationTags, ","),
		AgeRating:       ageRating,
		CommunityRating: fmt.Sprintf("%.1f", rating),
	}

	jsonMeta := EHMetadataJSON{
		GID:       gid,
		Token:     token,
		ParentGID: parentGID,
		Title:     title,
		TitleJpn:  titleJpn,
		Category:  category,
		Uploader:  uploader,
		Rating:    rating,
		FileCount: pageCount,
		Filesize:  filesize,
		Expunged:  false,
		Tags:      tags,
	}

	return xmlMeta, jsonMeta
}

// 2. EH/JH 下载附带的 JSON 元数据结构体 (metadata / ametadata)
type EHMetadataJSON struct {
	GID       string      `json:"gid"`
	Token     string      `json:"token"`
	ParentGID string      `json:"parent_gid"`
	Title     string      `json:"title"`
	TitleJpn  string      `json:"title_jpn"`
	Category  string      `json:"category"`
	Uploader  string      `json:"uploader"`
	Rating    interface{} `json:"rating"` // 可能是 string 或 float64
	FileCount int         `json:"filecount"`
	Filesize  int64       `json:"filesize"`
	Expunged  bool        `json:"expunged"`
	Tags      []string    `json:"tags"`
}

// Struct 用于存储解析汇总结果
type ParsedMetadata struct {
	Title     string
	Category  string
	Tags      []string
	GID       string // E 站画廊 GID（metadata/ametadata 内）
	Token     string
	ParentGID string
	FileCount int
	FileSize  int64
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
					if result.GID == "" {
						result.GID = jsonMeta.GID
					}
					if result.Token == "" {
						result.Token = jsonMeta.Token
					}
					if result.ParentGID == "" {
						result.ParentGID = jsonMeta.ParentGID
					}
					if jsonMeta.FileCount > 0 {
						result.FileCount = jsonMeta.FileCount
					}
					if jsonMeta.Filesize > 0 {
						result.FileSize = jsonMeta.Filesize
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
					if result.GID == "" {
						result.GID = jsonMeta.GID
					}
					if result.Token == "" {
						result.Token = jsonMeta.Token
					}
					if result.ParentGID == "" {
						result.ParentGID = jsonMeta.ParentGID
					}
					if jsonMeta.FileCount > 0 {
						result.FileCount = jsonMeta.FileCount
					}
					if jsonMeta.Filesize > 0 {
						result.FileSize = jsonMeta.Filesize
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
