package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"SakuHentai/internal/middleware"
	"SakuHentai/internal/models"

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
	if err := db.AutoMigrate(&models.Bookshelf{}); err != nil {
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
