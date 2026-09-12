package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"SakuManga/internal/middleware"
	"SakuManga/internal/models"
	"SakuManga/internal/services"
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

// parseSortKeys 解析书架内本子权值表（Round22 LexoRank）：JSON map string→float64
func parseSortKeys(raw string) map[string]float64 {
	if raw == "" {
		return map[string]float64{}
	}
	var m map[string]float64
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return map[string]float64{}
	}
	if m == nil {
		return map[string]float64{}
	}
	return m
}

// joinSortKeys 序列化权值表为 JSON 字符串
func joinSortKeys(m map[string]float64) string {
	if len(m) == 0 {
		return "{}"
	}
	b, _ := json.Marshal(m)
	return string(b)
}

// sortedComicIDs 按 Round22 权值顺序返回书架内本子 ID：
// 有权值的按 sortKeys 升序在前，无权值（旧数据/新增）按 comicIds 数组顺序排后。
func sortedComicIDs(comicIDs []string, sortKeys map[string]float64) []string {
	type weightedID struct {
		id string
		w  float64
	}
	keyed := make([]weightedID, 0, len(comicIDs))
	var unkeyed []string
	for _, id := range comicIDs {
		if w, ok := sortKeys[id]; ok {
			keyed = append(keyed, weightedID{id, w})
		} else {
			unkeyed = append(unkeyed, id)
		}
	}
	sort.SliceStable(keyed, func(i, j int) bool { return keyed[i].w < keyed[j].w })
	out := make([]string, 0, len(comicIDs))
	for _, k := range keyed {
		out = append(out, k.id)
	}
	out = append(out, unkeyed...)
	return out
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
	// Round10：书架列表按自定义排序 sort_order 输出；Round22：优先按 LexoRank 权值 sort_key 输出
	// （sort_key 由单书架移动/全量重排维护；旧数据 sort_key=0 回退 sort_order / name）
	if err := h.db.Where("user_id = ?", user.ID).Order("sort_key asc, sort_order asc, name asc").Find(&shelves).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取书架失败"})
		return
	}
	// comicIds 以 JSON 数组字符串存库，响应统一转为数组返回，
	// 避免前端拿到字符串后调用 .filter/.includes 抛错（comicIds.filter is not a function）
	resp := make([]gin.H, 0, len(shelves))
	for _, s := range shelves {
		ids := parseComicIDs(s.ComicIDs)
		sk := parseSortKeys(s.SortKeys)
		resp = append(resp, gin.H{
			"id":       s.ID,
			"name":     s.Name,
			"count":    len(ids),
			"comicIds": sortedComicIDs(ids, sk), // Round22：按权值排序后的展示顺序
			"pinned":   s.Pinned,
			"sortKey":  s.SortKey,
			"sortKeys": sk,
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

	// Round10：新书架排在现有书架之后（sort_order = 当前最大值 + 1）
	// Round22：同时赋予 LexoRank 权值 sort_key = 当前最大权值 + 1000（旧数据 sort_key=0 时按 sort_order 兜底）
	var maxOrder int
	h.db.Model(&models.Bookshelf{}).
		Where("user_id = ?", user.ID).
		Select("COALESCE(MAX(sort_order), 0)").
		Scan(&maxOrder)
	var maxKey float64
	h.db.Model(&models.Bookshelf{}).
		Where("user_id = ?", user.ID).
		Select("COALESCE(MAX(sort_key), 0)").
		Scan(&maxKey)

	shelf := models.Bookshelf{
		ID:        "shelf-" + strconv.FormatInt(time.Now().UnixMilli(), 10),
		UserID:    user.ID,
		Name:      req.Name,
		Count:     0,
		ComicIDs:  "[]",
		SortOrder: maxOrder + 1,
		SortKey:   maxKey + 1000,
		SortKeys:  "{}",
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

// SetBookshelfPinned 置顶/取消置顶书架 PUT /api/v1/bookshelves/:id/pin
// （Round13：侧栏常驻高频书架，最多 5 个置顶由前端控制，后端只存标记）
func (h *LibraryHandler) SetBookshelfPinned(c *gin.Context) {
	user := middleware.CurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	var req struct {
		Pinned bool `json:"pinned"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数解析失败"})
		return
	}

	var shelf models.Bookshelf
	if err := h.db.Where("id = ? AND user_id = ?", c.Param("id"), user.ID).First(&shelf).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "书架不存在"})
		return
	}

	shelf.Pinned = req.Pinned
	if err := h.db.Model(&shelf).Update("pinned", req.Pinned).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存置顶状态失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "置顶状态已更新", "data": shelf})
}

// BatchAddComicsToBookshelf 批量将漫画加入书架 POST /api/v1/bookshelves/:id/comics/batch
// （Round13：离线多选快捷加入书架；已在书架中的自动跳过，返回 added/skipped）
func (h *LibraryHandler) BatchAddComicsToBookshelf(c *gin.Context) {
	user := middleware.CurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	var req struct {
		ComicIDs []string `json:"comicIds"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数解析失败"})
		return
	}
	if len(req.ComicIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 comicIds"})
		return
	}

	var shelf models.Bookshelf
	if err := h.db.Where("id = ? AND user_id = ?", c.Param("id"), user.ID).First(&shelf).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "书架不存在"})
		return
	}

	existing := parseComicIDs(shelf.ComicIDs)
	inSet := make(map[string]bool, len(existing))
	for _, id := range existing {
		inSet[id] = true
	}
	added := 0
	skipped := 0
	for _, id := range req.ComicIDs {
		if id == "" || inSet[id] {
			skipped++
			continue
		}
		existing = append(existing, id)
		inSet[id] = true
		added++
	}
	shelf.ComicIDs = joinComicIDs(existing)
	shelf.Count = len(existing)
	if err := h.db.Save(&shelf).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存书架失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "批量加入完成", "added": added, "skipped": skipped, "data": shelf})
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
	// Round22：同步清理该本子的权值
	sk := parseSortKeys(shelf.SortKeys)
	delete(sk, comicID)
	shelf.ComicIDs = joinComicIDs(newIDs)
	shelf.Count = len(newIDs)
	shelf.SortKeys = joinSortKeys(sk)
	if err := h.db.Save(&shelf).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存书架失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "已移出书架", "data": shelf})
}

// BatchRemoveComicsFromBookshelf 批量将漫画移出书架 DELETE /api/v1/bookshelves/:id/comics/batch
// （Round22：书架内多选快捷移除；同步清理权值表，与批量加入 /comics/batch 对称）
func (h *LibraryHandler) BatchRemoveComicsFromBookshelf(c *gin.Context) {
	user := middleware.CurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	var req struct {
		ComicIDs []string `json:"comicIds"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数解析失败"})
		return
	}
	if len(req.ComicIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 comicIds"})
		return
	}

	var shelf models.Bookshelf
	if err := h.db.Where("id = ? AND user_id = ?", c.Param("id"), user.ID).First(&shelf).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "书架不存在"})
		return
	}

	removedSet := make(map[string]bool, len(req.ComicIDs))
	for _, id := range req.ComicIDs {
		if id != "" {
			removedSet[id] = true
		}
	}
	var newIDs []string
	removed := 0
	for _, cid := range parseComicIDs(shelf.ComicIDs) {
		if removedSet[cid] {
			removed++
			continue
		}
		newIDs = append(newIDs, cid)
	}
	sk := parseSortKeys(shelf.SortKeys)
	for id := range removedSet {
		delete(sk, id)
	}
	shelf.ComicIDs = joinComicIDs(newIDs)
	shelf.Count = len(newIDs)
	shelf.SortKeys = joinSortKeys(sk)
	if err := h.db.Save(&shelf).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存书架失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "批量移出完成", "removed": removed, "data": shelf})
}

// ReorderBookshelfComics 重排书架内项目 PUT /api/v1/bookshelves/:id/order
// （Round10 全量模式：{comicIds:[...]} 整包重写数组并重建权值表 1000*i；
//  Round22 单键模式：{comicId, sortKey} 只更新一本的 LexoRank 权值——拖动/移动到的主路径）
func (h *LibraryHandler) ReorderBookshelfComics(c *gin.Context) {
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

	raw, _ := c.GetRawData()
	if len(raw) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数解析失败"})
		return
	}
	// 权值模式：{sortKeys: {id: weight}}（批量防抖合并）或 {comicId, sortKey}（单键）
	var single struct {
		ComicID string             `json:"comicId"`
		SortKey float64            `json:"sortKey"`
		SortKeys map[string]float64 `json:"sortKeys"`
	}
	if err := json.Unmarshal(raw, &single); err == nil {
		sk := parseSortKeys(shelf.SortKeys)
		changed := false
		if single.ComicID != "" {
			sk[single.ComicID] = single.SortKey
			changed = true
		}
		if len(single.SortKeys) > 0 {
			for id, w := range single.SortKeys {
				sk[id] = w
			}
			changed = true
		}
		if changed {
			shelf.SortKeys = joinSortKeys(sk)
			if err := h.db.Model(&shelf).Update("sort_keys", shelf.SortKeys).Error; err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "保存书架排序失败"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"message": "书架排序已更新", "data": shelf})
			return
		}
	}

	// 全量模式：{comicIds:[...]}（Round10 向后兼容；精度用尽全量重置时前端同样走此路径）
	var req struct {
		ComicIDs []string `json:"comicIds"`
	}
	if err := json.Unmarshal(raw, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数解析失败"})
		return
	}
	if req.ComicIDs == nil {
		req.ComicIDs = []string{}
	}
	shelf.ComicIDs = joinComicIDs(req.ComicIDs)
	shelf.Count = len(req.ComicIDs)
	// Round22：全量重排后权值表重建为 1000*i，与数组顺序一致
	sk := make(map[string]float64, len(req.ComicIDs))
	for i, id := range req.ComicIDs {
		sk[id] = float64((i + 1) * 1000)
	}
	shelf.SortKeys = joinSortKeys(sk)
	if err := h.db.Save(&shelf).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存书架排序失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "书架排序已更新", "data": shelf})
}

// MoveBookshelfPosition 单书架移动 PUT /api/v1/bookshelves/:id/position
// （Round22：书架列表 LexoRank 单点移动，body {sortKey} 只更新该项权值，取代全量 reorder）
func (h *LibraryHandler) MoveBookshelfPosition(c *gin.Context) {
	user := middleware.CurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	var req struct {
		SortKey float64 `json:"sortKey"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数解析失败"})
		return
	}

	var shelf models.Bookshelf
	if err := h.db.Where("id = ? AND user_id = ?", c.Param("id"), user.ID).First(&shelf).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "书架不存在"})
		return
	}

	if err := h.db.Model(&shelf).Update("sort_key", req.SortKey).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存书架顺序失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "书架顺序已更新", "data": shelf})
}

// ReorderBookshelves 批量重排书架列表顺序 POST /api/v1/bookshelves/reorder
// （Round10：侧栏书架自定义排序，ids 数组下标即 sort_order；
//  Round22：全量重置时同步重建 sort_key=1000*i，与新权值体系一致）
func (h *LibraryHandler) ReorderBookshelves(c *gin.Context) {
	user := middleware.CurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	var req struct {
		IDs []string `json:"ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数解析失败"})
		return
	}
	for i, id := range req.IDs {
		h.db.Model(&models.Bookshelf{}).
			Where("id = ? AND user_id = ?", id, user.ID).
			Updates(map[string]interface{}{"sort_order": i, "sort_key": float64((i + 1) * 1000)})
	}
	c.JSON(http.StatusOK, gin.H{"message": "书架顺序已更新"})
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
	// Round20-Bug1：离线历史按 gid 合并去重（同 gid 不同 comic_id 视为同一本子，保留最新一条）。
	// 为避免重复条目挤占 limit 名额，离线先多取（2 倍，上限 1000）再按 gid 去重后截断。
	fetchLimit := limit
	if source == "offline" {
		fetchLimit = limit * 2
		if fetchLimit > 1000 {
			fetchLimit = 1000
		}
	}
	if err := q.Order("last_read_at desc").Limit(fetchLimit).Find(&records).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取历史失败"})
		return
	}
	if source == "offline" {
		records = dedupeHistoryByGid(records)
		if len(records) > limit {
			records = records[:limit]
		}
	}
	c.JSON(http.StatusOK, gin.H{"items": records, "total": len(records)})
}

// dedupeHistoryByGid 按 gid（缺省回退 comic_id）去重历史记录，保留首次出现（最新）的一条。
// 入参 records 须已按 last_read_at 降序。
func dedupeHistoryByGid(records []models.HistoryRecord) []models.HistoryRecord {
	seen := map[string]bool{}
	out := make([]models.HistoryRecord, 0, len(records))
	for _, r := range records {
		key := r.GID
		if key == "" {
			key = r.ComicID
		}
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, r)
	}
	return out
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
		Gid              string `json:"gid"`
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

	// Round20-Bug1：离线历史同 gid 合并——同 gid 不同 comic_id（更新替换/重扫产生的新旧 id）
	// 视为同一本子，写入前删除该用户同 gid 的其他行，保证每本每用户仅一条历史。
	if req.Source == "offline" && req.Gid != "" {
		if err := h.db.Where("user_id = ? AND source = ? AND g_id = ? AND comic_id != ?",
			user.ID, req.Source, req.Gid, req.ComicID).
			Delete(&models.HistoryRecord{}).Error; err != nil {
			log.Printf("[LIBRARY-WARN] 合并同 gid 历史失败: %v", err)
		}
	}

	var rec models.HistoryRecord
	if err := h.db.Where("user_id = ? AND comic_id = ? AND source = ?", user.ID, req.ComicID, req.Source).First(&rec).Error; err != nil {
		rec = models.HistoryRecord{
			UserID:     user.ID,
			ComicID:    req.ComicID,
			GID:        req.Gid,
			Source:     models.ComicSource(req.Source),
			ComicTitle: req.ComicTitle,
			CoverURL:   req.CoverURL,
			Token:      req.Token,
		}
	}
	// Round11-Bug3：标题/封面空值不覆盖已有记录。
	// 阅读器进度写回（syncHistory）可能只带 comicId 而不带标题/封面
	// （在线模式 fallback 为空串），无条件覆盖会把正常历史污染成 gid 乱码/无封面。
	if req.ComicTitle != "" {
		rec.ComicTitle = req.ComicTitle
	}
	if req.CoverURL != "" {
		rec.CoverURL = req.CoverURL
	}
	if req.LastChapterTitle != "" {
		rec.LastChapterTitle = req.LastChapterTitle
	}
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
	// Round20-Bug1：已有记录缺 gid 时回填（旧数据/迁移场景，供后续按 gid 合并去重）
	if rec.GID == "" && req.Gid != "" {
		rec.GID = req.Gid
	}
	rec.LastReadAt = time.Now()
	if err := h.db.Save(&rec).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存历史失败"})
		return
	}

	// 上限淘汰（每用户每来源）
	h.trimHistory(user.ID, models.ComicSource(req.Source))

	// Round32：离线历史的近期性是阅读侧信号之一 → 增量重算该用户对该本的 XP 贡献
	if req.Source == "offline" {
		services.XpRecomposeComicForUser(user.ID, req.ComicID)
	}

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
		if err := h.db.Where("user_id = ? AND comic_id = ?", user.ID, comicID).Delete(&models.ComicRating{}).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "评分清除失败"})
			return
		}
		// Round32：评分是阅读侧信号之一 → 增量重算
		services.XpRecomposeComicForUser(user.ID, comicID)
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

	// Round32：评分是阅读侧信号之一 → 增量重算
	services.XpRecomposeComicForUser(user.ID, comicID)

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

	// Round32：评分是阅读侧信号之一 → 增量重算
	services.XpRecomposeComicForUser(user.ID, c.Param("comicId"))

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
