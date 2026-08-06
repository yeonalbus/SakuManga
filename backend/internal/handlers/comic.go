package handlers

import (
	"SakuHentai/internal/database"
	"SakuHentai/internal/models"
	"SakuHentai/internal/services"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

type OfflineComicResponse struct {
	models.OfflineComic
	SourceLabel           string              `json:"sourceLabel,omitempty"` // 来源标签（问题3：额外路径 Name；空=下载导入）
	Tags                  []*services.TagItem `json:"tags"`                  // 展示用（合并后的翻译结果）
	TagRaws               []string            `json:"tagRaws"`               // 与 Tags 一一对应的原始 tag 字符串（删除时精确匹配）
	TagSources            []string            `json:"tagSources"`            // 与 Tags 一一对应的来源：online | local
	OnlineTagsList        []string            `json:"onlineTagsList"`        // 原始三态（前端区分官方/本地展示）
	OfflineAddTagsList    []string            `json:"offlineAddTagsList"`    // 本地新增 tag
	OfflineRemoveTagsList []string            `json:"offlineRemoveTagsList"` // 本地删除的 online tag
}

func parseRawTags(tagsStr string) []string {
	tagsStr = strings.TrimSpace(tagsStr)
	if tagsStr == "" {
		return []string{}
	}
	var tags []string
	if strings.HasPrefix(tagsStr, "[") {
		_ = json.Unmarshal([]byte(tagsStr), &tags)
		return tags
	}
	parts := strings.Split(tagsStr, ",")
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			tags = append(tags, t)
		}
	}
	return tags
}

// GetOfflineComics 获取离线漫画列表
func GetOfflineComics(c *gin.Context) {
	// 排序参数（问题1）：白名单映射，防止 SQL 注入
	sortBy := c.DefaultQuery("sortBy", "updatedAt")
	sortOrder := c.DefaultQuery("sortOrder", "desc")
	colMap := map[string]string{
		"updatedAt":      "updated_at",
		"addedAt":        "added_at",
		"publishedAt":    "published_at",
		"fileModifiedAt": "file_modified_at",
		"title":          "title",
		"rating":         "rating",
		"readCount":      "read_count",
	}
	col, ok := colMap[sortBy]
	if !ok {
		col = "updated_at"
	}
	dir := "DESC"
	if strings.ToLower(sortOrder) == "asc" {
		dir = "ASC"
	}
	q := database.DB
	// NULL 时间排最后（旧数据无 added_at/published_at/file_modified_at）
	if col == "added_at" || col == "published_at" || col == "file_modified_at" {
		q = q.Order(col + " IS NULL")
	}
	q = q.Order(col + " " + dir)

	var comics []models.OfflineComic
	q.Find(&comics)

	// 额外路径 ID→Name 映射（问题3：来源标签）
	pathNames := map[string]string{}
	var paths []models.ExtraScanPath
	database.DB.Select("id", "name").Find(&paths)
	for _, p := range paths {
		if p.Name != "" {
			pathNames[p.ID] = p.Name
		}
	}

	resp := make([]OfflineComicResponse, 0, len(comics))
	for _, comic := range comics {
		label := "下载"
		if comic.ScanPathID != "" {
			if name, ok := pathNames[comic.ScanPathID]; ok {
				label = name
			}
		}
		resp = append(resp, OfflineComicResponse{
			OfflineComic: comic,
			SourceLabel:  label,
		})
	}
	c.JSON(http.StatusOK, resp)
}

// GetOfflineComicDetail 获取单个离线画廊详情
func GetOfflineComicDetail(c *gin.Context) {
	id := c.Param("id")
	var comic models.OfflineComic
	if err := database.DB.First(&comic, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "找不到该画廊"})
		return
	}

	// 双轨三态
	onlineTags := services.UnmarshalTagSlice(comic.OnlineTags)
	offlineAddTags := services.UnmarshalTagSlice(comic.OfflineAddTags)
	offlineRemoveTags := services.UnmarshalTagSlice(comic.OfflineRemoveTags)

	// 展示合并：(online ∪ offlineAdd) − offlineRemove
	merged := services.MergeTags(onlineTags, offlineAddTags, offlineRemoveTags)
	// 兼容旧数据：三态全为空（且未迁移）时回退到旧 Tags 字段
	if len(merged) == 0 && comic.OnlineTags == "" {
		merged = parseRawTags(comic.Tags)
	}

	// 计算每个展示 tag 的来源：本地新增标记为 local，其余视为官方 online
	onlineSet := map[string]bool{}
	for _, t := range onlineTags {
		onlineSet[t] = true
	}
	addSet := map[string]bool{}
	for _, t := range offlineAddTags {
		addSet[t] = true
	}
	sources := make([]string, 0, len(merged))
	for _, t := range merged {
		if addSet[t] && !onlineSet[t] {
			sources = append(sources, "local")
		} else {
			sources = append(sources, "online")
		}
	}

	translatedTags := services.GlobalTagEngine.TranslateTags(merged)

	c.JSON(http.StatusOK, OfflineComicResponse{
		OfflineComic:          comic,
		Tags:                  translatedTags,
		TagRaws:               merged,
		TagSources:            sources,
		OnlineTagsList:        onlineTags,
		OfflineAddTagsList:    offlineAddTags,
		OfflineRemoveTagsList: offlineRemoveTags,
	})
}

// GetComicCover 动态服务封面图
func GetComicCover(c *gin.Context) {
	id := c.Param("id")
	var comic models.OfflineComic
	if err := database.DB.First(&comic, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "找不到该漫画"})
		return
	}

	fi, err := os.Stat(comic.LocalPath)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "文件路径不存在"})
		return
	}

	// 1. 如果是散图文件夹
	if fi.IsDir() {
		imgPath, err := services.GetCoverFromDir(comic.LocalPath)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.File(imgPath) // 直接流式输出本地文件
		return
	}

	// 2. 如果是 ZIP/CBZ 压缩包
	if services.IsArchive(comic.LocalPath) {
		imgBytes, contentType, err := services.GetCoverFromZip(comic.LocalPath)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.Data(http.StatusOK, contentType, imgBytes)
		return
	}

	c.JSON(http.StatusBadRequest, gin.H{"error": "不支持的格式"})
}

// GetComicDetail 获取单本漫画详情
func GetComicDetail(c *gin.Context) {
	id := c.Param("id")
	var comic models.OfflineComic
	if err := database.DB.First(&comic, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "未找到该漫画记录"})
		return
	}
	c.JSON(http.StatusOK, comic)
}

// GetComicPages 获取指定漫画的所有页数信息
func GetComicPages(c *gin.Context) {
	id := c.Param("id")
	var comic models.OfflineComic
	if err := database.DB.First(&comic, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "找不到该漫画"})
		return
	}

	pages, err := services.GetPageList(comic.LocalPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取画廊失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"total": len(pages),
		"pages": pages,
	})
}

// GetComicPageImage 响应具体的单页图片数据
func GetComicPageImage(c *gin.Context) {
	id := c.Param("id")
	pageIdxStr := c.Param("index")

	var pageIdx int
	if _, err := fmt.Sscanf(pageIdxStr, "%d", &pageIdx); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的页码"})
		return
	}

	var comic models.OfflineComic
	if err := database.DB.First(&comic, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "找不到该漫画"})
		return
	}

	data, contentType, err := services.GetPageData(comic.LocalPath, pageIdx)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	// 🎯 开启强缓存：浏览器命中本地缓存后零延迟加载
	c.Header("Cache-Control", "public, max-age=86400")
	c.Data(http.StatusOK, contentType, data)
}

// DeleteOfflineComic 删除本地画廊。
// 查询参数 deleteFile=true 时同时物理删除本地文件；默认仅删除记录。
// 删除时自动清理书架与历史记录中的引用。
func DeleteOfflineComic(c *gin.Context) {
	id := c.Param("id")
	deleteFile := c.Query("deleteFile") == "true"

	var comic models.OfflineComic
	if err := database.DB.First(&comic, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "找不到该漫画"})
		return
	}

	if err := services.DeleteOfflineComic(database.DB, id, deleteFile); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
}