package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"SakuHentai/internal/middleware"
	"SakuHentai/internal/models"
)

// LibraryHandler 书架 / 历史 / 评分 / 阅读清单（均按用户隔离，后端 DB 持久化）
type LibraryHandler struct {
	db *gorm.DB
}

func NewLibraryHandler(db *gorm.DB) *LibraryHandler {
	return &LibraryHandler{db: db}
}

// ─────────────────────────────────────────────────────────────
// 辅助
// ─────────────────────────────────────────────────────────────

func parseComicIDs(raw string) []string {
	if raw == "" {
		return []string{}
	}
	var ids []string
	if err := json.Unmarshal([]byte(raw), &ids); err != nil {
		return []string{}
	}
	return ids
}

func joinComicIDs(ids []string) string {
	b, _ := json.Marshal(ids)
	return string(b)
}

// historyLimit 读取每用户历史记录上限（可配置，默认 200）
func (h *LibraryHandler) historyLimit() int {
	return getOrCreateServerSetting(h.db).HistoryLimit
}

// trimHistory 每用户每来源保留 historyLimit 条，淘汰最旧记录
func (h *LibraryHandler) trimHistory(userID uint, source models.ComicSource) {
	var ids []uint
	h.db.Model(&models.HistoryRecord{}).
		Where("user_id = ? AND source = ?", userID, source).
		Order("last_read_at desc").
		Pluck("id", &ids)
	limit := h.historyLimit()
	if limit <= 0 || len(ids) <= limit {
		return
	}
	keep := ids[:limit]
	h.db.Where("user_id = ? AND source = ? AND id NOT IN ?", userID, source, keep).
		Delete(&models.HistoryRecord{})
}

// ─────────────────────────────────────────────────────────────
// 书架（Bookshelf，按用户隔离）
// ─────────────────────────────────────────────────────────────

// GetBookshelves 当前用户的书架列表 GET /api/v1/bookshelves
func (h *LibraryHandler) GetBookshelves(c *gin.Context) {
	user := middleware.CurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	var shelves []models.Bookshelf
	if err := h.db.Where("user_id = ?", user.ID).Order("name asc").Find(&shelves).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取书架失败"})
		return
	}
	// comicIds 以 JSON 数组字符串存库，响应统一转为数组返回，
	// 避免前端拿到字符串后调用 .filter/.includes 抛错（comicIds.filter is not a function）
	resp := make([]gin.H, 0, len(shelves))
	for _, s := range shelves {
		ids := parseComicIDs(s.ComicIDs)
		resp = append(resp, gin.H{
			"id":       s.ID,
			"name":     s.Name,
			"count":    len(ids),
			"comicIds": ids,
		})
	}
	c.JSON(http.StatusOK, gin.H{"bookshelves": resp})
}

// CreateBookshelf 新建书架 POST /api/v1/bookshelves
func (h *LibraryHandler) CreateBookshelf(c *gin.Context) {
	user := middleware.CurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "书架名称不能为空"})
		return
	}

	shelf := models.Bookshelf{
		ID:       "shelf-" + strconv.FormatInt(time.Now().UnixMilli(), 10),
		UserID:   user.ID,
		Name:     req.Name,
		Count:    0,
		ComicIDs: "[]",
	}
	if err := h.db.Create(&shelf).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建书架失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "书架已创建", "data": shelf})
}

// UpdateBookshelf 重命名书架 PUT /api/v1/bookshelves/:id
func (h *LibraryHandler) UpdateBookshelf(c *gin.Context) {
	user := middleware.CurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	var shelf models.Bookshelf
	if err := h.db.Where("id = ? AND user_id = ?", c.Param("id"), user.ID).First(&shelf).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "书架不存在"})
		return
	}

	var req struct {
		Name *string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数解析失败"})
		return
	}
	if req.Name != nil && *req.Name != "" {
		shelf.Name = *req.Name
	}
	shelf.Count = len(parseComicIDs(shelf.ComicIDs))
	if err := h.db.Save(&shelf).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存书架失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "书架已更新", "data": shelf})
}

// DeleteBookshelf 删除书架 DELETE /api/v1/bookshelves/:id
func (h *LibraryHandler) DeleteBookshelf(c *gin.Context) {
	user := middleware.CurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	if err := h.db.Where("id = ? AND user_id = ?", c.Param("id"), user.ID).Delete(&models.Bookshelf{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除书架失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "书架已删除"})
}

// AddComicToBookshelf 将漫画加入书架 POST /api/v1/bookshelves/:id/comics
func (h *LibraryHandler) AddComicToBookshelf(c *gin.Context) {
	user := middleware.CurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	var req struct {
		ComicID string `json:"comicId" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.ComicID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 comicId"})
		return
	}

	var shelf models.Bookshelf
	if err := h.db.Where("id = ? AND user_id = ?", c.Param("id"), user.ID).First(&shelf).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "书架不存在"})
		return
	}

	ids := parseComicIDs(shelf.ComicIDs)
	found := false
	for _, cid := range ids {
		if cid == req.ComicID {
			found = true
			break
		}
	}
	if !found {
		ids = append(ids, req.ComicID)
	}
	shelf.ComicIDs = joinComicIDs(ids)
	shelf.Count = len(ids)
	if err := h.db.Save(&shelf).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存书架失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "已加入书架", "data": shelf})
}

// RemoveComicFromBookshelf 将漫画移出书架 DELETE /api/v1/bookshelves/:id/comics?comicId=..
func (h *LibraryHandler) RemoveComicFromBookshelf(c *gin.Context) {
	user := middleware.CurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	comicID := c.Query("comicId")
	if comicID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 comicId 参数"})
		return
	}

	var shelf models.Bookshelf
	if err := h.db.Where("id = ? AND user_id = ?", c.Param("id"), user.ID).First(&shelf).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "书架不存在"})
		return
	}

	var newIDs []string
	for _, cid := range parseComicIDs(shelf.ComicIDs) {
		if cid != comicID {
			newIDs = append(newIDs, cid)
		}
	}
	shelf.ComicIDs = joinComicIDs(newIDs)
	shelf.Count = len(newIDs)
	if err := h.db.Save(&shelf).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存书架失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "已移出书架", "data": shelf})
}

// ─────────────────────────────────────────────────────────────
// 历史（HistoryRecord，按用户隔离 + 上限淘汰）
// ─────────────────────────────────────────────────────────────

// GetHistory 当前用户的历史记录 GET /api/v1/history?source=online|offline&limit=..
func (h *LibraryHandler) GetHistory(c *gin.Context) {
	user := middleware.CurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	limit := h.historyLimit()
	if l, err := strconv.Atoi(c.DefaultQuery("limit", strconv.Itoa(limit))); err == nil && l > 0 && l <= 1000 {
		limit = l
	}

	q := h.db.Where("user_id = ?", user.ID)
	source := c.Query("source")
	if source == "online" || source == "offline" {
		q = q.Where("source = ?", source)
	}

	// Round3-任务1：可选 comicId 单条查询（阅读器进入时按账号精确读取进度）
	if comicID := c.Query("comicId"); comicID != "" {
		var rec models.HistoryRecord
		err := q.Where("comic_id = ?", comicID).First(&rec).Error
		if err == nil {
			c.JSON(http.StatusOK, gin.H{"items": []models.HistoryRecord{rec}, "total": 1})
		} else {
			c.JSON(http.StatusOK, gin.H{"items": []models.HistoryRecord{}, "total": 0})
		}
		return
	}

	var records []models.HistoryRecord
	if err := q.Order("last_read_at desc").Limit(limit).Find(&records).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取历史失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": records, "total": len(records)})
}

// AddHistory 写入/更新一条历史记录 POST /api/v1/history
func (h *LibraryHandler) AddHistory(c *gin.Context) {
	user := middleware.CurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	var req struct {
		ComicID          string `json:"comicId" binding:"required"`
		Source           string `json:"source" binding:"required"`
		ComicTitle       string `json:"comicTitle"`
		CoverURL         string `json:"coverUrl"`
		Token            string `json:"token"`
		LastChapterTitle string `json:"lastChapterTitle"`
		LastPageIndex    int    `json:"lastPageIndex"`
		TotalPageCount   int    `json:"totalPageCount"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.ComicID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 comicId / source 参数"})
		return
	}
	if req.Source != "online" && req.Source != "offline" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source 必须为 online 或 offline"})
		return
	}

	var rec models.HistoryRecord
	if err := h.db.Where("user_id = ? AND comic_id = ? AND source = ?", user.ID, req.ComicID, req.Source).First(&rec).Error; err != nil {
		rec = models.HistoryRecord{
			UserID:     user.ID,
			ComicID:    req.ComicID,
			Source:     models.ComicSource(req.Source),
			ComicTitle: req.ComicTitle,
			CoverURL:   req.CoverURL,
			Token:      req.Token,
		}
	}
	rec.ComicTitle = req.ComicTitle
	rec.CoverURL = req.CoverURL
	rec.LastChapterTitle = req.LastChapterTitle
	if req.Token != "" {
		rec.Token = req.Token
	}
	// 进度保护：仅当入参进度 > 0 时才覆盖已有进度，避免卡片点击(lastPageIndex=0)清零上次阅读位置。
	if req.LastPageIndex > 0 {
		rec.LastPageIndex = req.LastPageIndex
	}
	if req.TotalPageCount > 0 {
		rec.TotalPageCount = req.TotalPageCount
	}
	rec.LastReadAt = time.Now()
	if err := h.db.Save(&rec).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存历史失败"})
		return
	}

	// 上限淘汰（每用户每来源）
	h.trimHistory(user.ID, models.ComicSource(req.Source))

	c.JSON(http.StatusOK, gin.H{"message": "历史已记录", "data": rec})
}

// ClearHistory 清空当前用户的历史 DELETE /api/v1/history?source=online|offline
func (h *LibraryHandler) ClearHistory(c *gin.Context) {
	user := middleware.CurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	q := h.db.Where("user_id = ?", user.ID)
	if source := c.Query("source"); source == "online" || source == "offline" {
		q = q.Where("source = ?", source)
	}
	if err := q.Delete(&models.HistoryRecord{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "清空历史失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "历史已清空"})
}

// DeleteHistory 删除单条历史 DELETE /api/v1/history/:id
func (h *LibraryHandler) DeleteHistory(c *gin.Context) {
	user := middleware.CurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	if err := h.db.Where("id = ? AND user_id = ?", c.Param("id"), user.ID).Delete(&models.HistoryRecord{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除历史失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "历史已删除"})
}

// AdminGetHistory 管理员查看成员历史（可按 userId / source 过滤）GET /api/v1/admin/history
func (h *LibraryHandler) AdminGetHistory(c *gin.Context) {
	q := h.db.Model(&models.HistoryRecord{})
	if userIdStr := c.Query("userId"); userIdStr != "" {
		if uid, err := strconv.ParseUint(userIdStr, 10, 32); err == nil {
			q = q.Where("user_id = ?", uint(uid))
		}
	}
	if source := c.Query("source"); source == "online" || source == "offline" {
		q = q.Where("source = ?", source)
	}
	limit := 200
	if l, err := strconv.Atoi(c.DefaultQuery("limit", "200")); err == nil && l > 0 && l <= 1000 {
		limit = l
	}

	var records []models.HistoryRecord
	if err := q.Order("last_read_at desc").Limit(limit).Find(&records).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取历史失败"})
		return
	}

	// 附带用户名映射（前端按成员展示）
	userMap := map[uint]string{}
	seen := map[uint]bool{}
	var userIDs []uint
	for _, r := range records {
		if !seen[r.UserID] {
			seen[r.UserID] = true
			userIDs = append(userIDs, r.UserID)
		}
	}
	if len(userIDs) > 0 {
		var users []models.User
		h.db.Where("id IN ?", userIDs).Find(&users)
		for _, u := range users {
			userMap[u.ID] = u.Username
		}
	}
	c.JSON(http.StatusOK, gin.H{"items": records, "total": len(records), "users": userMap})
}

// ─────────────────────────────────────────────────────────────
// 评分（ComicRating，按用户隔离）
// ─────────────────────────────────────────────────────────────

// GetRatings 当前用户全部评分（返回 comicId → score 映射）GET /api/v1/ratings
func (h *LibraryHandler) GetRatings(c *gin.Context) {
	user := middleware.CurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	var ratings []models.ComicRating
	if err := h.db.Where("user_id = ?", user.ID).Find(&ratings).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取评分失败"})
		return
	}
	m := map[string]int{}
	for _, r := range ratings {
		m[r.ComicID] = r.Score
	}
	c.JSON(http.StatusOK, gin.H{"ratings": m})
}

// GetComicRating 单本评分 GET /api/v1/ratings/:comicId
func (h *LibraryHandler) GetComicRating(c *gin.Context) {
	user := middleware.CurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	var r models.ComicRating
	if err := h.db.Where("user_id = ? AND comic_id = ?", user.ID, c.Param("comicId")).First(&r).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"score": 0})
		return
	}
	c.JSON(http.StatusOK, gin.H{"score": r.Score})
}

// SetComicRating 设置/更新评分（score<=0 视为删除）PUT /api/v1/ratings/:comicId
func (h *LibraryHandler) SetComicRating(c *gin.Context) {
	user := middleware.CurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	var req struct {
		Score int `json:"score" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 score 参数"})
		return
	}

	comicID := c.Param("comicId")
	if req.Score <= 0 {
		h.db.Where("user_id = ? AND comic_id = ?", user.ID, comicID).Delete(&models.ComicRating{})
		c.JSON(http.StatusOK, gin.H{"message": "评分已清除"})
		return
	}
	if req.Score > 10 {
		req.Score = 10
	}

	var r models.ComicRating
	if err := h.db.Where("user_id = ? AND comic_id = ?", user.ID, comicID).First(&r).Error; err != nil {
		r = models.ComicRating{UserID: user.ID, ComicID: comicID}
	}
	r.Score = req.Score
	r.UpdatedAt = time.Now()
	if err := h.db.Save(&r).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存评分失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "评分已保存", "score": r.Score})
}

// DeleteComicRating 删除单本评分 DELETE /api/v1/ratings/:comicId
func (h *LibraryHandler) DeleteComicRating(c *gin.Context) {
	user := middleware.CurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	if err := h.db.Where("user_id = ? AND comic_id = ?", user.ID, c.Param("comicId")).Delete(&models.ComicRating{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除评分失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "评分已删除"})
}

// ─────────────────────────────────────────────────────────────
// 阅读清单（ReadingList，每用户每来源一个队列）
// ─────────────────────────────────────────────────────────────

// GetReadingList 读取指定来源的阅读清单 GET /api/v1/reading-list?source=online|offline
func (h *LibraryHandler) GetReadingList(c *gin.Context) {
	user := middleware.CurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	source := c.Query("source")
	if source != "online" && source != "offline" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source 必须为 online 或 offline"})
		return
	}

	var rl models.ReadingList
	if err := h.db.Where("user_id = ? AND source = ?", user.ID, source).First(&rl).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"source": source, "items": []interface{}{}})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"source":    rl.Source,
		"items":     json.RawMessage(rl.Items), // 内嵌原始 JSON 数组
		"updatedAt": rl.UpdatedAt,
	})
}

// SaveReadingList 整体保存指定来源的阅读清单 PUT /api/v1/reading-list
func (h *LibraryHandler) SaveReadingList(c *gin.Context) {
	user := middleware.CurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	var req struct {
		Source string          `json:"source" binding:"required"`
		Items  json.RawMessage `json:"items"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数解析失败"})
		return
	}
	if req.Source != "online" && req.Source != "offline" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source 必须为 online 或 offline"})
		return
	}
	itemsStr := string(req.Items)
	if itemsStr == "" {
		itemsStr = "[]"
	}

	var rl models.ReadingList
	if err := h.db.Where("user_id = ? AND source = ?", user.ID, req.Source).First(&rl).Error; err != nil {
		rl = models.ReadingList{UserID: user.ID, Source: req.Source}
	}
	rl.Items = itemsStr
	rl.UpdatedAt = time.Now()
	if err := h.db.Save(&rl).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存阅读清单失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "阅读清单已保存"})
}
