package handlers

import (
	"SakuHentai/internal/database"
	"SakuHentai/internal/models"
	"SakuHentai/internal/services"
	"encoding/json"
	"net/http"
	"strings"
	"os"
	"fmt"
	"github.com/gin-gonic/gin"
)

type OfflineComicResponse struct {
	models.OfflineComic
	Tags []*services.TagItem `json:"tags"`
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
	var comics []models.OfflineComic
	database.DB.Order("updated_at desc").Find(&comics)
	c.JSON(http.StatusOK, comics)
}

// GetOfflineComicDetail 获取单个离线画廊详情
func GetOfflineComicDetail(c *gin.Context) {
	id := c.Param("id")
	var comic models.OfflineComic
	if err := database.DB.First(&comic, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "找不到该画廊"})
		return
	}

	rawTags := parseRawTags(comic.Tags)
	translatedTags := services.GlobalTagEngine.TranslateTags(rawTags)

	c.JSON(http.StatusOK, OfflineComicResponse{
		OfflineComic: comic,
		Tags:         translatedTags,
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