package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"SakuManga/internal/middleware"
	"SakuManga/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// Round13：书架置顶 + 批量加入接口单测（内存 SQLite）
func newLibraryTestDB(t *testing.T) (*gorm.DB, *LibraryHandler) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("打开内存库失败: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("获取底层连接失败: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { sqlDB.Close() })
	if err := db.AutoMigrate(&models.Bookshelf{}, &models.OfflineComic{}); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	gin.SetMode(gin.TestMode)
	return db, NewLibraryHandler(db)
}

// 注入当前用户到 gin context（等价 AuthRequired 的效果，测试免 token）
func authAs(db *gorm.DB, userID uint) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(middleware.ContextUserKey, &models.User{ID: userID})
		c.Next()
	}
}

// 造一个属于 userID 的书架（模拟当前用户）
func seedShelf(t *testing.T, db *gorm.DB, userID uint, name string, comicIDs []string) models.Bookshelf {
	t.Helper()
	s := models.Bookshelf{
		ID:       "shelf-test-" + name,
		UserID:   userID,
		Name:     name,
		ComicIDs: joinComicIDs(comicIDs),
		Count:    len(comicIDs),
		SortOrder: 0,
	}
	if err := db.Create(&s).Error; err != nil {
		t.Fatalf("创建书架失败: %v", err)
	}
	return s
}

func TestSetBookshelfPinned(t *testing.T) {
	db, h := newLibraryTestDB(t)
	s := seedShelf(t, db, 1, "A", nil)

	r := gin.New()
	r.PUT("/bookshelves/:id/pin", authAs(db, 1), h.SetBookshelfPinned)

	// 1) 置顶
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/bookshelves/"+s.ID+"/pin", strings.NewReader(`{"pinned":true}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("置顶返回 %d: %s", w.Code, w.Body.String())
	}
	var stored models.Bookshelf
	if err := db.First(&stored, "id = ?", s.ID).Error; err != nil || !stored.Pinned {
		t.Fatalf("置顶后 Pinned 应为 true (err=%v)", err)
	}

	// 2) 取消置顶
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/bookshelves/"+s.ID+"/pin", strings.NewReader(`{"pinned":false}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if err := db.First(&stored, "id = ?", s.ID).Error; err != nil || stored.Pinned {
		t.Fatalf("取消置顶后 Pinned 应为 false (err=%v)", err)
	}

	// 3) 非本人书架 → 404
	other := seedShelf(t, db, 2, "B", nil)
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/bookshelves/"+other.ID+"/pin", strings.NewReader(`{"pinned":true}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("他人书架应返回 404，得到 %d", w.Code)
	}
}

func TestBatchAddComicsToBookshelf(t *testing.T) {
	db, h := newLibraryTestDB(t)
	s := seedShelf(t, db, 1, "A", []string{"c1", "c2"})

	r := gin.New()
	r.POST("/bookshelves/:id/comics/batch", authAs(db, 1), h.BatchAddComicsToBookshelf)

	doBatch := func(t *testing.T, body string) (int, int, int) {
		t.Helper()
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/bookshelves/"+s.ID+"/comics/batch", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("批量加入返回 %d: %s", w.Code, w.Body.String())
		}
		var resp struct {
			Added   int `json:"added"`
			Skipped int `json:"skipped"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("解析响应失败: %v", err)
		}
		return resp.Added, resp.Skipped, w.Code
	}

	// 1) 新增 2 个 + 重复 1 个
	added, skipped, _ := doBatch(t, `{"comicIds":["c3","c1","c4"]}`)
	if added != 2 || skipped != 1 {
		t.Fatalf("期望 added=2 skipped=1，得到 added=%d skipped=%d", added, skipped)
	}
	var stored models.Bookshelf
	if err := db.First(&stored, "id = ?", s.ID).Error; err != nil {
		t.Fatalf("读取书架失败: %v", err)
	}
	ids := parseComicIDs(stored.ComicIDs)
	if len(ids) != 4 {
		t.Fatalf("加入后应有 4 个 id，得到 %v", ids)
	}
	if stored.Count != 4 {
		t.Fatalf("Count 应为 4，得到 %d", stored.Count)
	}

	// 2) 全重复 → added=0 skipped=N
	added, skipped, _ = doBatch(t, `{"comicIds":["c1","c2","c3","c4"]}`)
	if added != 0 || skipped != 4 {
		t.Fatalf("全重复场景期望 added=0 skipped=4，得到 added=%d skipped=%d", added, skipped)
	}

	// 3) 空数组 → 400
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/bookshelves/"+s.ID+"/comics/batch", strings.NewReader(`{"comicIds":[]}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("空数组应返回 400，得到 %d", w.Code)
	}
}

// Round22：书架列表单书架 LexoRank 移动（PUT /bookshelves/:id/position）
func TestMoveBookshelfPosition(t *testing.T) {
	db, h := newLibraryTestDB(t)
	s := seedShelf(t, db, 1, "A", nil)

	r := gin.New()
	r.PUT("/bookshelves/:id/position", authAs(db, 1), h.MoveBookshelfPosition)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/bookshelves/"+s.ID+"/position", strings.NewReader(`{"sortKey":1500}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("移动返回 %d: %s", w.Code, w.Body.String())
	}
	var stored models.Bookshelf
	if err := db.First(&stored, "id = ?", s.ID).Error; err != nil || stored.SortKey != 1500 {
		t.Fatalf("SortKey 应为 1500 (err=%v, got=%v)", err, stored.SortKey)
	}

	// sortKey=0 也须合法（移到顶部时权值可为 0）
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/bookshelves/"+s.ID+"/position", strings.NewReader(`{"sortKey":0}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("sortKey=0 应合法，得到 %d", w.Code)
	}

	// 他人书架 → 404
	other := seedShelf(t, db, 2, "B", nil)
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/bookshelves/"+other.ID+"/position", strings.NewReader(`{"sortKey":500}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("他人书架应返回 404，得到 %d", w.Code)
	}
}

// Round22：书架内本子单键权值更新（PUT /bookshelves/:id/order {comicId, sortKey}）
func TestReorderBookshelfComicsSingleKey(t *testing.T) {
	db, h := newLibraryTestDB(t)
	s := seedShelf(t, db, 1, "A", []string{"c1", "c2", "c3"})
	// 预置权值 1000/2000/3000
	if err := db.Model(&s).Update("sort_keys", `{"c1":1000,"c2":2000,"c3":3000}`).Error; err != nil {
		t.Fatalf("预置权值失败: %v", err)
	}

	r := gin.New()
	r.PUT("/bookshelves/:id/order", authAs(db, 1), h.ReorderBookshelfComics)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/bookshelves/"+s.ID+"/order", strings.NewReader(`{"comicId":"c2","sortKey":1500}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("单键更新返回 %d: %s", w.Code, w.Body.String())
	}
	var stored models.Bookshelf
	if err := db.First(&stored, "id = ?", s.ID).Error; err != nil {
		t.Fatalf("读取书架失败: %v", err)
	}
	sk := parseSortKeys(stored.SortKeys)
	if sk["c2"] != 1500 {
		t.Fatalf("c2 权值应为 1500，得到 %v", sk["c2"])
	}
	// comicIds 数组不应被改动
	if ids := parseComicIDs(stored.ComicIDs); len(ids) != 3 {
		t.Fatalf("单键更新不应改变 comicIds: %v", ids)
	}
	// 展示顺序：c1(1000) < c2(1500) < c3(3000)
	ordered := sortedComicIDs(parseComicIDs(stored.ComicIDs), sk)
	if ordered[0] != "c1" || ordered[1] != "c2" || ordered[2] != "c3" {
		t.Fatalf("排序结果异常: %v", ordered)
	}
}

// Round22：书架内本子全量重排（PUT /bookshelves/:id/order {comicIds}）重建权值 1000*i
func TestReorderBookshelfComicsFull(t *testing.T) {
	db, h := newLibraryTestDB(t)
	s := seedShelf(t, db, 1, "A", []string{"c1", "c2"})

	r := gin.New()
	r.PUT("/bookshelves/:id/order", authAs(db, 1), h.ReorderBookshelfComics)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/bookshelves/"+s.ID+"/order", strings.NewReader(`{"comicIds":["c2","c1"]}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("全量重排返回 %d: %s", w.Code, w.Body.String())
	}
	var stored models.Bookshelf
	if err := db.First(&stored, "id = ?", s.ID).Error; err != nil {
		t.Fatalf("读取书架失败: %v", err)
	}
	sk := parseSortKeys(stored.SortKeys)
	if sk["c2"] != 1000 || sk["c1"] != 2000 {
		t.Fatalf("全量重排应重建权值 1000*i: %v", sk)
	}
	ordered := sortedComicIDs(parseComicIDs(stored.ComicIDs), sk)
	if ordered[0] != "c2" || ordered[1] != "c1" {
		t.Fatalf("排序结果异常: %v", ordered)
	}
}

// Round22：批量移出书架（DELETE /bookshelves/:id/comics/batch）同步清理权值
func TestBatchRemoveComicsFromBookshelf(t *testing.T) {
	db, h := newLibraryTestDB(t)
	s := seedShelf(t, db, 1, "A", []string{"c1", "c2", "c3"})
	if err := db.Model(&s).Update("sort_keys", `{"c1":1000,"c2":2000,"c3":3000}`).Error; err != nil {
		t.Fatalf("预置权值失败: %v", err)
	}

	r := gin.New()
	r.DELETE("/bookshelves/:id/comics/batch", authAs(db, 1), h.BatchRemoveComicsFromBookshelf)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/bookshelves/"+s.ID+"/comics/batch", strings.NewReader(`{"comicIds":["c1","c3"]}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("批量移出返回 %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Removed int `json:"removed"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil || resp.Removed != 2 {
		t.Fatalf("应移除 2 本 (err=%v, removed=%d)", err, resp.Removed)
	}
	var stored models.Bookshelf
	if err := db.First(&stored, "id = ?", s.ID).Error; err != nil {
		t.Fatalf("读取书架失败: %v", err)
	}
	ids := parseComicIDs(stored.ComicIDs)
	if len(ids) != 1 || ids[0] != "c2" {
		t.Fatalf("剩余应为 [c2]: %v", ids)
	}
	sk := parseSortKeys(stored.SortKeys)
	if _, ok := sk["c1"]; ok {
		t.Fatalf("c1 权值应被清理: %v", sk)
	}
	if sk["c2"] != 2000 {
		t.Fatalf("c2 权值应保留 2000: %v", sk)
	}

	// 空数组 → 400
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/bookshelves/"+s.ID+"/comics/batch", strings.NewReader(`{"comicIds":[]}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("空数组应返回 400，得到 %d", w.Code)
	}
}

// Round22：GetBookshelves 按 sort_key 排序 + 返回权值表 + 展示顺序按权值
func TestGetBookshelvesSortKeyAndWeights(t *testing.T) {
	db, h := newLibraryTestDB(t)
	a := seedShelf(t, db, 1, "A", []string{"c1", "c2"})
	b := seedShelf(t, db, 1, "B", []string{"c3"})
	// A 排到 B 之后（A.sort_key=3000, B.sort_key=1000）
	if err := db.Model(&a).Update("sort_key", 3000).Error; err != nil {
		t.Fatalf("更新 A sort_key 失败: %v", err)
	}
	if err := db.Model(&b).Update("sort_key", 1000).Error; err != nil {
		t.Fatalf("更新 B sort_key 失败: %v", err)
	}
	// B 内权值 c3=1000；A 内 c1=2000 c2=1000（展示顺序应为 c2,c1）
	if err := db.Model(&a).Update("sort_keys", `{"c1":2000,"c2":1000}`).Error; err != nil {
		t.Fatalf("更新 A sort_keys 失败: %v", err)
	}

	r := gin.New()
	r.GET("/bookshelves", authAs(db, 1), h.GetBookshelves)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/bookshelves", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("查询返回 %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Bookshelves []struct {
			ID       string             `json:"id"`
			ComicIDs []string           `json:"comicIds"`
			SortKey  float64            `json:"sortKey"`
			SortKeys map[string]float64 `json:"sortKeys"`
		} `json:"bookshelves"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	if len(resp.Bookshelves) != 2 {
		t.Fatalf("应有 2 个书架: %+v", resp.Bookshelves)
	}
	if resp.Bookshelves[0].ID != b.ID || resp.Bookshelves[1].ID != a.ID {
		t.Fatalf("应按 sort_key 升序 [B,A]: %v", resp.Bookshelves[0].ID+","+resp.Bookshelves[1].ID)
	}
	// A 的展示顺序应按权值：c2(1000) 在 c1(2000) 前
	aShelf := resp.Bookshelves[1]
	if len(aShelf.ComicIDs) != 2 || aShelf.ComicIDs[0] != "c2" || aShelf.ComicIDs[1] != "c1" {
		t.Fatalf("A 展示顺序应为 [c2,c1]: %v", aShelf.ComicIDs)
	}
	if aShelf.SortKey != 3000 || aShelf.SortKeys["c1"] != 2000 {
		t.Fatalf("响应应携带权值: sortKey=%v sortKeys=%v", aShelf.SortKey, aShelf.SortKeys)
	}
}

// Round22：sortedComicIDs 混合态（有权值在前，无权值按数组顺序排后）
func TestSortedComicIDsMixed(t *testing.T) {
	ids := []string{"a", "b", "c", "d"}
	sk := map[string]float64{"b": 1000, "d": 3000}
	got := sortedComicIDs(ids, sk)
	want := []string{"b", "d", "a", "c"} // 有权值 b,d 按权值升序；无权值 a,c 按数组顺序
	if len(got) != len(want) {
		t.Fatalf("长度不符: %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("期望 %v 得到 %v", want, got)
		}
	}
}

// Round38：GetBookshelves 返回书架墙所需的未读数与封面
// 覆盖：未读按 read_count<=0 计、失效引用（本子已删除）不计入未读也不作封面、
// 封面取展示顺序中第一个仍存在且带封面的本子、跨用户隔离。
func TestGetBookshelvesUnreadAndCover(t *testing.T) {
	db, h := newLibraryTestDB(t)
	// c1 已读且封面为空（应被跳过）、c2 未读有封面、c3 未读无封面
	comics := []models.OfflineComic{
		{ID: "c1", Title: "已读", LocalPath: "p1", ReadCount: 3},
		{ID: "c2", Title: "未读有封面", LocalPath: "p2", CoverURL: "/api/v1/comics/c2/cover"},
		{ID: "c3", Title: "未读无封面", LocalPath: "p3"},
	}
	if err := db.Create(&comics).Error; err != nil {
		t.Fatalf("插入本子失败: %v", err)
	}

	a := seedShelf(t, db, 1, "A", []string{"c1", "c2", "c3", "ghost"})
	// 展示顺序 c1,c2,c3（c1 封面为空 → 封面应回退到 c2）
	if err := db.Model(&a).Update("sort_keys", `{"c1":1000,"c2":2000,"c3":3000}`).Error; err != nil {
		t.Fatalf("更新 A sort_keys 失败: %v", err)
	}
	seedShelf(t, db, 1, "B", []string{"missing"}) // 架内本子全失效
	seedShelf(t, db, 2, "他人", []string{"c2"})    // 他人书架不应出现

	r := gin.New()
	r.GET("/bookshelves", authAs(db, 1), h.GetBookshelves)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/bookshelves", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("查询返回 %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Bookshelves []struct {
			Name        string   `json:"name"`
			Count       int      `json:"count"`
			UnreadCount int      `json:"unreadCount"`
			CoverURL    string   `json:"coverUrl"`
			ComicIDs    []string `json:"comicIds"`
		} `json:"bookshelves"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	if len(resp.Bookshelves) != 2 {
		t.Fatalf("应只有本人 2 个书架: %+v", resp.Bookshelves)
	}
	byName := map[string]int{}
	for i, s := range resp.Bookshelves {
		byName[s.Name] = i
	}

	aShelf := resp.Bookshelves[byName["A"]]
	if aShelf.Count != 4 {
		t.Fatalf("count 应为架内 id 数 4: %d", aShelf.Count)
	}
	// c2、c3 未读；c1 已读；ghost 失效不计
	if aShelf.UnreadCount != 2 {
		t.Fatalf("A 未读数应为 2（ghost 不计、c1 已读）: %d", aShelf.UnreadCount)
	}
	// c1 封面为空 → 回退到展示顺序中第一个带封面的 c2
	if aShelf.CoverURL != "/api/v1/comics/c2/cover" {
		t.Fatalf("A 封面应回退到 c2: %q", aShelf.CoverURL)
	}

	bShelf := resp.Bookshelves[byName["B"]]
	if bShelf.UnreadCount != 0 || bShelf.CoverURL != "" {
		t.Fatalf("架内本子全失效时未读=0 且无封面: unread=%d cover=%q", bShelf.UnreadCount, bShelf.CoverURL)
	}
}

// Round38：loadBookshelfStats 空书架集合不触发查询、返回空统计
func TestLoadBookshelfStatsEmpty(t *testing.T) {
	db, _ := newLibraryTestDB(t)
	stats := loadBookshelfStats(db, map[string][]string{"s1": {}}, map[string]string{})
	if len(stats) != 1 {
		t.Fatalf("空书架也应有一条统计: %+v", stats)
	}
	if st := stats["s1"]; st.unread != 0 || st.coverURL != "" {
		t.Fatalf("空书架统计应为零值: %+v", st)
	}
}

// Round38-R5：手动指定封面优先，指定项失效/非本架时回退自动封面
func TestGetBookshelvesManualCover(t *testing.T) {
	db, h := newLibraryTestDB(t)
	comics := []models.OfflineComic{
		{ID: "c1", Title: "第一本", LocalPath: "p1", CoverURL: "/cover/c1"},
		{ID: "c2", Title: "第二本", LocalPath: "p2", CoverURL: "/cover/c2"},
		{ID: "c3", Title: "非本架", LocalPath: "p3", CoverURL: "/cover/c3"},
	}
	if err := db.Create(&comics).Error; err != nil {
		t.Fatalf("插入本子失败: %v", err)
	}
	shelf := seedShelf(t, db, 1, "A", []string{"c1", "c2"})
	if err := db.Model(&shelf).Update("sort_keys", `{"c1":1000,"c2":2000}`).Error; err != nil {
		t.Fatalf("更新权值失败: %v", err)
	}

	r := gin.New()
	r.GET("/bookshelves", authAs(db, 1), h.GetBookshelves)
	fetchCover := func() string {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/bookshelves", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("查询返回 %d: %s", w.Code, w.Body.String())
		}
		var resp struct {
			Bookshelves []struct {
				CoverURL     string `json:"coverUrl"`
				CoverComicID string `json:"coverComicId"`
			} `json:"bookshelves"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("解析响应失败: %v", err)
		}
		if len(resp.Bookshelves) != 1 {
			t.Fatalf("应只有 1 个书架: %+v", resp.Bookshelves)
		}
		return resp.Bookshelves[0].CoverURL
	}

	// 未指定 → 自动取展示顺序第一本
	if got := fetchCover(); got != "/cover/c1" {
		t.Fatalf("未指定时应取第一本封面，得到 %q", got)
	}
	// 指定第二本 → 覆盖自动结果
	if err := db.Model(&shelf).Update("cover_comic_id", "c2").Error; err != nil {
		t.Fatalf("设置手指定封面失败: %v", err)
	}
	if got := fetchCover(); got != "/cover/c2" {
		t.Fatalf("指定 c2 后封面应为 /cover/c2，得到 %q", got)
	}
	// 指定非本架本子 → 回退自动（不信任脏数据）
	if err := db.Model(&shelf).Update("cover_comic_id", "c3").Error; err != nil {
		t.Fatalf("设置非法封面失败: %v", err)
	}
	if got := fetchCover(); got != "/cover/c1" {
		t.Fatalf("指定非本架本子应回退自动封面，得到 %q", got)
	}
	// 指定已失效 id → 回退自动
	if err := db.Model(&shelf).Update("cover_comic_id", "ghost").Error; err != nil {
		t.Fatalf("设置失效封面失败: %v", err)
	}
	if got := fetchCover(); got != "/cover/c1" {
		t.Fatalf("指定失效 id 应回退自动封面，得到 %q", got)
	}
}

// Round38-R5：PUT /bookshelves/:id/cover 设置/清除封面 + 归属校验 + 跨用户隔离
func TestSetBookshelfCover(t *testing.T) {
	db, h := newLibraryTestDB(t)
	comics := []models.OfflineComic{
		{ID: "c1", Title: "第一本", LocalPath: "p1", CoverURL: "/cover/c1"},
		{ID: "c2", Title: "第二本", LocalPath: "p2", CoverURL: "/cover/c2"},
	}
	if err := db.Create(&comics).Error; err != nil {
		t.Fatalf("插入本子失败: %v", err)
	}
	shelf := seedShelf(t, db, 1, "A", []string{"c1"})

	r := gin.New()
	r.PUT("/bookshelves/:id/cover", authAs(db, 1), h.SetBookshelfCover)

	put := func(id, body string) int {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/bookshelves/"+id+"/cover", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		return w.Code
	}

	// 非本架本子 → 400
	if code := put(shelf.ID, `{"comicId":"c2"}`); code != http.StatusBadRequest {
		t.Fatalf("非本架本子应返回 400，得到 %d", code)
	}
	// 架内本子 → 200 且落库
	if code := put(shelf.ID, `{"comicId":"c1"}`); code != http.StatusOK {
		t.Fatalf("架内本子应返回 200，得到 %d", code)
	}
	var stored models.Bookshelf
	db.First(&stored, "id = ?", shelf.ID)
	if stored.CoverComicID != "c1" {
		t.Fatalf("封面应落库为 c1: %q", stored.CoverComicID)
	}
	// 空串 → 恢复自动（清空）
	if code := put(shelf.ID, `{"comicId":""}`); code != http.StatusOK {
		t.Fatalf("清空封面应返回 200，得到 %d", code)
	}
	db.First(&stored, "id = ?", shelf.ID)
	if stored.CoverComicID != "" {
		t.Fatalf("清空后封面应为空串: %q", stored.CoverComicID)
	}
	// 他人书架 → 404
	other := seedShelf(t, db, 2, "B", []string{"c1"})
	if code := put(other.ID, `{"comicId":"c1"}`); code != http.StatusNotFound {
		t.Fatalf("他人书架应返回 404，得到 %d", code)
	}
}
