package handlers

import (
	"SakuHentai/internal/database"
	"SakuHentai/internal/models"
	"SakuHentai/internal/services"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
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
	HiddenPagesList       []int               `json:"hiddenPagesList"`       // 隐藏的物理页索引（Round23 自定义删除页面）
	OriginalPageCount     int                 `json:"originalPageCount"`     // 原始物理页数（隐藏页后 pageCount 为有效页数）
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

		// 需求2：为列表填充翻译后的 TagItem(Tags) 与原始 tag 串(TagRaws)，前端据此做本地 tag 搜索/语言过滤。
		// 注意：OfflineComicResponse.Tags 遮蔽了内嵌的 models.OfflineComic.Tags(string)，必须显式赋值，
		// 否则 JSON 输出 tags=null，前端离线书库将永远无法按 tag 搜索。
		onlineTags := services.UnmarshalTagSlice(comic.OnlineTags)
		offlineAddTags := services.UnmarshalTagSlice(comic.OfflineAddTags)
		offlineRemoveTags := services.UnmarshalTagSlice(comic.OfflineRemoveTags)
		merged := services.MergeTags(onlineTags, offlineAddTags, offlineRemoveTags)
		if len(merged) == 0 && comic.OnlineTags == "" {
			merged = parseRawTags(comic.Tags)
		}
		translatedTags := services.GlobalTagEngine.TranslateTags(merged)

		resp = append(resp, OfflineComicResponse{
			OfflineComic:      comic,
			SourceLabel:       label,
			Tags:              translatedTags,
			TagRaws:           merged,
			HiddenPagesList:   services.ParseHiddenPages(comic.HiddenPages),
			OriginalPageCount: comic.OriginalPageCount,
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
		HiddenPagesList:       services.ParseHiddenPages(comic.HiddenPages),
		OriginalPageCount:     comic.OriginalPageCount,
	})
}

// GetComicCover 动态服务封面图
// Round14：优先走缩略图缓存（ZIP/CBZ 不再反复解压整包、散图不再直传完整原图），
// 带 Cache-Control/ETag，浏览器二次进入零请求 → 修复离线界面卡顿。
func GetComicCover(c *gin.Context) {
	id := c.Param("id")
	var comic models.OfflineComic
	if err := database.DB.First(&comic, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "找不到该漫画"})
		return
	}

	// Round14：缩略图缓存（ZIP/Dir 共用）
	data, cachePath, cached, err := services.GetCoverThumb(comic)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	// ETag = 缓存文件 modtime（命中后浏览器可直接 304）
	if cached {
		fi, _ := os.Stat(cachePath)
		if fi != nil {
			etag := fmt.Sprintf(`"%d"`, fi.ModTime().Unix())
			c.Header("Cache-Control", "public, max-age=86400")
			c.Header("ETag", etag)
			if match := c.GetHeader("If-None-Match"); match != "" && match == etag {
				c.Status(http.StatusNotModified)
				return
			}
		}
		c.File(cachePath)
		return
	}

	// 未命中缓存（无需缩放/解码失败）：原图直传，仍带长缓存头
	contentType := "image/jpeg"
	c.Header("Cache-Control", "public, max-age=86400")
	c.Data(http.StatusOK, contentType, data)
}

// UpdateOfflineComic 修改离线漫画标题/本地备注 PUT /api/v1/comics/:id
// Round11-Opt3：title 传空字符串 = 恢复原标题（OriginalTitle，首次入库标题）；
// remark 为本地备注；两者均为指针，未传字段不修改。
func UpdateOfflineComic(c *gin.Context) {
	id := c.Param("id")
	var comic models.OfflineComic
	if err := database.DB.First(&comic, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "未找到该漫画记录"})
		return
	}

	var req struct {
		Title  *string `json:"title"`
		Remark *string `json:"remark"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数解析失败"})
		return
	}
	if req.Title == nil && req.Remark == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "没有可更新的字段"})
		return
	}

	if req.Title != nil {
		if *req.Title == "" {
			// 清空输入 → 恢复原标题（保留首次入库标题，便于反复尝试）
			if comic.OriginalTitle != "" {
				comic.Title = comic.OriginalTitle
			}
		} else {
			comic.Title = *req.Title
		}
	}
	if req.Remark != nil {
		comic.Remark = *req.Remark
	}
	if err := database.DB.Save(&comic).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "已更新", "data": gin.H{"title": comic.Title, "remark": comic.Remark, "originalTitle": comic.OriginalTitle}})
}

// GetComicPages 获取指定漫画的有效页列表（Round23：剔除用户自定义隐藏页）。
// 返回 total=有效页数、pages=有效页文件名列表、originalTotal=物理页数、hiddenPages=隐藏索引，
// 供阅读器/预览按有效序号读取；管理模式用 hiddenPages 还原隐藏态。
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

	hidden := services.ParseHiddenPages(comic.HiddenPages)
	visible := services.FilterHiddenPages(pages, hidden)

	c.JSON(http.StatusOK, gin.H{
		"total":          len(visible),
		"pages":          visible,
		"originalTotal":  len(pages),
		"hiddenPages":    hidden,
		"originalCount":  comic.OriginalPageCount,
	})
}

// GetComicPageImage 响应单页图片数据（pageIndex 为剔除隐藏页后的有效序号，Round23 自动映射物理页）
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

	hidden := services.ParseHiddenPages(comic.HiddenPages)
	data, contentType, err := services.GetVisiblePageData(comic.LocalPath, pageIdx, hidden)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	// 🎯 开启强缓存：浏览器命中本地缓存后零延迟加载
	c.Header("Cache-Control", "public, max-age=86400")
	c.Data(http.StatusOK, contentType, data)
}

// GetComicRawPageImage 按物理页索引直读图片（Round23 管理模式预览用，不应用隐藏页映射）。
// 与 GetComicPageImage（有效索引，自动跳过隐藏页）区分：管理模式需展示含隐藏页的全量页面。
func GetComicRawPageImage(c *gin.Context) {
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

// UpdateComicHiddenPages 自定义删除页面：更新隐藏页列表 PUT /api/v1/comics/:id/hidden-pages
// body: { hiddenPages: [物理页索引(0-based)...] }（传空数组 = 全部恢复）。
// 软删除（不物理删文件）：同步重算 pageCount（有效页数）并记录 originalPageCount（原物理页数，
// 供更新检测/维护查重比对，避免隐藏页后误判「画廊被扩充」）。
func UpdateComicHiddenPages(c *gin.Context) {
	id := c.Param("id")
	var comic models.OfflineComic
	if err := database.DB.First(&comic, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "找不到该漫画"})
		return
	}

	var req struct {
		HiddenPages []int `json:"hiddenPages"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数解析失败"})
		return
	}

	physicalCount, err := services.CountPages(comic.LocalPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取画廊失败: " + err.Error()})
		return
	}

	// 裁剪越界索引（物理页可能因文件增删而变化）并排序去重
	hidden := services.NormalizeHiddenPages(req.HiddenPages, physicalCount)
	// 原页数：首次设置时记录物理页数；此后物理页数变大时同步抬升（保持「原页数 ≥ 有效页数」）
	orig := comic.OriginalPageCount
	if orig <= 0 || physicalCount > orig {
		orig = physicalCount
	}

	comic.HiddenPages = services.MarshalHiddenPages(hidden)
	comic.OriginalPageCount = orig
	comic.PageCount = services.EffectivePageCount(physicalCount, hidden)
	comic.UpdatedAt = time.Now()
	if err := database.DB.Save(&comic).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":           "已更新隐藏页",
		"pageCount":         comic.PageCount,
		"originalPageCount": comic.OriginalPageCount,
		"hiddenPages":       hidden,
	})
}

// DeleteOfflineComic 删除本地画廊。
// 查询参数 deleteFile=true 时同时物理删除本地文件；默认仅删除记录。
// 删除时自动清理书架与历史记录中的引用。
// 幽灵文件容错：记录不存在（可能已被其他设备删除）→ 视为删除成功，返回 alreadyDeleted=true。
func DeleteOfflineComic(c *gin.Context) {
	id := c.Param("id")
	deleteFile := c.Query("deleteFile") == "true"

	if err := services.DeleteOfflineComic(database.DB, id, deleteFile); err != nil {
		if errors.Is(err, services.ErrComicNotFound) {
			c.JSON(http.StatusOK, gin.H{"message": "删除成功", "alreadyDeleted": true})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
}

// RecordComicClick 记录离线漫画阅读次数自增（排行榜持久化）。
// 前端阅读按钮进入阅读器后 fire-and-forget 上报；DB 原子自增，多设备并发安全。
func RecordComicClick(c *gin.Context) {
	id := c.Param("id")
	res := database.DB.Model(&models.OfflineComic{}).
		Where("id = ?", id).
		UpdateColumn("read_count", gorm.Expr("read_count + 1"))
	if res.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": res.Error.Error()})
		return
	}
	if res.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "未找到该漫画记录"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "readCount": res.RowsAffected})
}