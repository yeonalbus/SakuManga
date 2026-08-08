package services

import (
	"archive/zip"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
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

	publishTime := ""
	if detail != nil {
		publishTime = detail.UpdatedAt // 在线 Posted 日期（问题1/2 发布时间来源）
	}

	jsonMeta := EHMetadataJSON{
		GID:         gid,
		Token:       token,
		ParentGID:   parentGID,
		Title:       title,
		TitleJpn:    titleJpn,
		Category:    category,
		Uploader:    uploader,
		Rating:      rating,
		FileCount:   pageCount,
		Filesize:    filesize,
		Expunged:    false,
		Tags:        tags,
		PublishTime: publishTime,
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
	// 发布时间，兼容两种序列化 key：
	// E-H 官方 metadata 用 publish_time；JHentai 序列化（用户贴的样例）用 publishTime
	PublishTime    string `json:"publish_time,omitempty"`
	PublishTimeAlt string `json:"publishTime,omitempty"`
}

// Struct 用于存储解析汇总结果
type ParsedMetadata struct {
	Title       string
	TitleJpn    string // 日文原名（问题2，来自 metadata title_jpn / ComicInfo AlternateSeries）
	Category    string
	Tags        []string
	GID         string // E 站画廊 GID（metadata/ametadata 内）
	Token       string
	ParentGID   string
	FileCount   int
	FileSize    int64
	PublishTime string // 发布时间字符串（问题1）
}

// applyJSONMeta 将 EH/JH metadata JSON 合并进解析结果
func (r *ParsedMetadata) applyJSONMeta(jsonMeta EHMetadataJSON) {
	if jsonMeta.Title != "" {
		r.Title = jsonMeta.Title
	}
	if jsonMeta.TitleJpn != "" {
		r.TitleJpn = jsonMeta.TitleJpn
	}
	if jsonMeta.Category != "" {
		r.Category = jsonMeta.Category
	}
	if len(jsonMeta.Tags) > 0 {
		r.Tags = append(r.Tags, jsonMeta.Tags...)
	}
	if r.GID == "" {
		r.GID = jsonMeta.GID
	}
	if r.Token == "" {
		r.Token = jsonMeta.Token
	}
	if r.ParentGID == "" {
		r.ParentGID = jsonMeta.ParentGID
	}
	if jsonMeta.FileCount > 0 {
		r.FileCount = jsonMeta.FileCount
	}
	if jsonMeta.Filesize > 0 {
		r.FileSize = jsonMeta.Filesize
	}
	if r.PublishTime == "" {
		r.PublishTime = jsonMeta.PublishTime
	}
	if r.PublishTime == "" {
		r.PublishTime = jsonMeta.PublishTimeAlt
	}
}

// parseEHJSONMetadata 从 metadata / ametadata 字节流中稳健提取字段（读盘路径）。
//
// 兼容三种实际格式：
//  1. 本程序写入的平铺格式：{"gid":1509130,"token":"...","tags":["a","b"],...}
//  2. JHentai 归档 ametadata（平铺，tags 为逗号分隔字符串、gid 为数字）：
//     {"gid":4092682,"token":"...","title":"...","pageCount":21,"size":192413696,"publishTime":"...","tags":"a,b,..."}
//  3. JHentai 画廊 metadata（gallery 包裹层 + images 字符串）：
//     {"gallery":{"gid":1509130,"token":"...","title":"...","tags":"a,b,...","oldVersionGalleryUrl":"..."},"images":"[...]"}
//
// 背景（问题3 根因）：此前用强类型 json.Unmarshal 直接反序列化，而实际文件里
//   - tags 是「逗号分隔字符串」而结构体是 []string → 类型不匹配 → 整体解析失败；
//   - gid 是「数字」而结构体是 string → 同样整体失败；
//   - 画廊版还多一层 gallery 包裹。
// 任一不匹配都会让 gid/token/title/tags 全部丢失，导致「同 GID 查重 / 更新检测」永远为 0。
// 因此这里改用 map[string]json.RawMessage + 逐字段类型兼容提取。
func parseEHJSONMetadata(data []byte) EHMetadataJSON {
	meta := EHMetadataJSON{Tags: []string{}}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		return meta
	}
	// 画廊 metadata 存在 gallery 包裹层 → 改从 gallery 内部提取（归档 ametadata 平铺，直接用 root）
	src := root
	if raw, ok := root["gallery"]; ok {
		var g map[string]json.RawMessage
		if err := json.Unmarshal(raw, &g); err == nil {
			src = g
		}
	}

	// 字符串字段：兼容「gid/token 为数字」「title 为 sanitizedTitle」等别名与类型混用
	getStr := func(keys ...string) string {
		for _, k := range keys {
			raw, ok := src[k]
			if !ok {
				continue
			}
			var s string
			if err := json.Unmarshal(raw, &s); err == nil {
				return strings.TrimSpace(s)
			}
			var n json.Number
			if err := json.Unmarshal(raw, &n); err == nil {
				return n.String()
			}
		}
		return ""
	}
	// 整数字段：兼容 pageCount/filecount、filesize/size 别名，以及字符串数字
	getInt := func(keys ...string) int {
		for _, k := range keys {
			raw, ok := src[k]
			if !ok {
				continue
			}
			var n int
			if err := json.Unmarshal(raw, &n); err == nil {
				return n
			}
			if v, err := strconv.Atoi(getStr(k)); err == nil {
				return v
			}
		}
		return 0
	}
	// 标签字段：兼容 JSON 数组与逗号分隔字符串两种形态
	getTags := func() []string {
		raw, ok := src["tags"]
		if !ok {
			return nil
		}
		var arr []string
		if err := json.Unmarshal(raw, &arr); err == nil {
			return arr
		}
		var s string
		if err := json.Unmarshal(raw, &s); err == nil {
			parts := strings.Split(s, ",")
			out := make([]string, 0, len(parts))
			for _, p := range parts {
				if t := strings.TrimSpace(p); t != "" {
					out = append(out, t)
				}
			}
			return out
		}
		return nil
	}

	meta.GID = getStr("gid")
	meta.Token = getStr("token")
	meta.ParentGID = getStr("parent_gid", "parentGID")
	meta.Title = getStr("title", "sanitizedTitle")
	meta.TitleJpn = getStr("title_jpn", "titleJpn")
	meta.Category = getStr("category")
	meta.Uploader = getStr("uploader")
	meta.Tags = getTags()
	meta.FileCount = getInt("pageCount", "filecount")
	meta.Filesize = int64(getInt("filesize", "size"))
	meta.PublishTime = getStr("publish_time")
	meta.PublishTimeAlt = getStr("publishTime")
	// 画廊下载：父画廊关系编码在 oldVersionGalleryUrl 中（https://.../g/<parent_gid>/<token>/）
	if meta.ParentGID == "" {
		if u := getStr("oldVersionGalleryUrl"); u != "" {
			meta.ParentGID = extractGIDFromURL(u)
		}
	}
	return meta
}

// extractGIDTokenFromURL 从画廊 URL（https://exhentai.org/g/<gid>/<token>/）提取 gid 与 token。
func extractGIDTokenFromURL(raw string) (string, string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", ""
	}
	segs := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i, s := range segs {
		if s == "g" && i+2 < len(segs) {
			return segs[i+1], segs[i+2]
		}
	}
	return "", ""
}

func extractGIDFromURL(raw string) string {
	gid, _ := extractGIDTokenFromURL(raw)
	return gid
}

// applyXMLMeta 将 ComicInfo.xml 合并进解析结果
func (r *ParsedMetadata) applyXMLMeta(xmlMeta ComicInfoXML) {
	if r.Title == "" && xmlMeta.Title != "" {
		r.Title = xmlMeta.Title
	}
	if r.TitleJpn == "" && xmlMeta.AlternateSeries != "" {
		r.TitleJpn = xmlMeta.AlternateSeries
	}
	if r.Category == "" && xmlMeta.Genre != "" {
		r.Category = xmlMeta.Genre
	}
	if xmlMeta.Tags != "" {
		xmlTags := strings.Split(xmlMeta.Tags, ",")
		for _, t := range xmlTags {
			trimmed := strings.TrimSpace(t)
			if trimmed != "" {
				r.Tags = append(r.Tags, trimmed)
			}
		}
	}
	// 兜底：从 <Web>https://exhentai.org/g/<gid>/<token>/ 提取 gid/token——
	// 兼容只有 ComicInfo.xml（第三方下载器/EHViewer）而 JSON 元数据缺失的文件夹。
	if r.GID == "" || r.Token == "" {
		if gid, token := extractGIDTokenFromURL(xmlMeta.Web); gid != "" {
			if r.GID == "" {
				r.GID = gid
			}
			if r.Token == "" {
				r.Token = token
			}
		}
	}
	if r.FileCount == 0 && xmlMeta.PageCount > 0 {
		r.FileCount = xmlMeta.PageCount
	}
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

		// A. 匹配 JSON 元数据 (metadata / ametadata / info.json / .ehdata 等)
		if name == "metadata" || name == "ametadata" || name == "info.json" || name == ".ehdata" || strings.HasSuffix(name, ".json") {
			if data, err := os.ReadFile(fullPath); err == nil {
				// 稳健解析：兼容 JHentai 画廊(gallery 包裹层)/归档(平铺)格式与 tags 字符串/数组混用
				result.applyJSONMeta(parseEHJSONMetadata(data))
			}
		}

		// B. 匹配 ComicInfo.xml
		if name == "comicinfo.xml" {
			if data, err := os.ReadFile(fullPath); err == nil {
				var xmlMeta ComicInfoXML
				if err := xml.Unmarshal(data, &xmlMeta); err == nil {
					result.applyXMLMeta(xmlMeta)
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

		if name == "metadata" || name == "ametadata" || name == "info.json" || name == ".ehdata" || name == "comicinfo.xml" {
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
				result.applyJSONMeta(parseEHJSONMetadata(data))
			} else {
				// XML 解析
				var xmlMeta ComicInfoXML
				if err := xml.Unmarshal(data, &xmlMeta); err == nil {
					result.applyXMLMeta(xmlMeta)
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
