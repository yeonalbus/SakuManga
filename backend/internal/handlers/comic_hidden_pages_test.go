package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"SakuManga/internal/database"
	"SakuManga/internal/models"
)

// ─────────────────────────────────────────────────────────────
// Round53 阶段一：D9/D10 页接口契约
//   - /pages 返回物理页表 + 隐藏页相对路径；
//   - PUT /hidden-pages 接受相对路径，重算有效页数，失效封面缓存。
// ─────────────────────────────────────────────────────────────

func newComicPagesTestDB(t *testing.T) *gorm.DB {
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
	t.Cleanup(func() { _ = sqlDB.Close() })

	if err := db.AutoMigrate(&models.OfflineComic{}, &models.ComicBookmark{}, &models.ComicChapter{}); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	old := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = old })
	return db
}

func TestHiddenPagesUseRelativePathsAndInvalidateCover(t *testing.T) {
	db := newComicPagesTestDB(t)

	dir := t.TempDir()
	for _, name := range []string{"01_ad.jpg", "02_1.jpg", "03_2.jpg"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatalf("写文件失败: %v", err)
		}
	}
	comic := models.OfflineComic{ID: "c1", Title: "t", LocalPath: dir, PageCount: 3, OriginalPageCount: 3}
	if err := db.Create(&comic).Error; err != nil {
		t.Fatalf("创建漫画失败: %v", err)
	}

	// 预置封面缓存，验证 PUT 后失效
	cacheDir := filepath.Join("cover_cache")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatalf("创建缓存目录失败: %v", err)
	}
	cacheFile := filepath.Join(cacheDir, "c1.jpg")
	if err := os.WriteFile(cacheFile, []byte("fake"), 0o644); err != nil {
		t.Fatalf("写缓存失败: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(cacheDir) })

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/comics/:id/pages", GetComicPages)
	r.PUT("/comics/:id/hidden-pages", UpdateComicHiddenPages)

	// 1) /pages：3 页、无隐藏
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/comics/c1/pages", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("/pages 返回 %d: %s", w.Code, w.Body.String())
	}
	var pagesResp struct {
		Pages         []string `json:"pages"`
		Total         int      `json:"total"`
		OriginalTotal int      `json:"originalTotal"`
		HiddenPages   []string `json:"hiddenPages"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &pagesResp); err != nil {
		t.Fatalf("解析 /pages 失败: %v", err)
	}
	if len(pagesResp.Pages) != 3 || pagesResp.OriginalTotal != 3 || pagesResp.Total != 3 {
		t.Fatalf("/pages 应返回 3 物理页 / 3 有效页，实际 %+v", pagesResp)
	}
	if len(pagesResp.HiddenPages) != 0 {
		t.Fatalf("初始不应有隐藏页，实际 %v", pagesResp.HiddenPages)
	}

	// 2) 隐藏第一张（相对路径）
	w = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/comics/c1/hidden-pages", strings.NewReader(`{"hiddenPages":["01_ad.jpg"]}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT hidden-pages 返回 %d: %s", w.Code, w.Body.String())
	}
	var putResp struct {
		PageCount         int      `json:"pageCount"`
		OriginalPageCount int      `json:"originalPageCount"`
		HiddenPages       []string `json:"hiddenPages"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &putResp); err != nil {
		t.Fatalf("解析 PUT 响应失败: %v", err)
	}
	if putResp.PageCount != 2 || putResp.OriginalPageCount != 3 {
		t.Fatalf("应为有效 2 / 物理 3，实际 %+v", putResp)
	}
	if len(putResp.HiddenPages) != 1 || putResp.HiddenPages[0] != "01_ad.jpg" {
		t.Fatalf("隐藏页应以相对路径返回，实际 %v", putResp.HiddenPages)
	}

	// 3) 落库格式：相对路径 JSON
	var got models.OfflineComic
	if err := db.First(&got, "id = ?", "c1").Error; err != nil {
		t.Fatalf("读取漫画失败: %v", err)
	}
	if got.HiddenPages != `["01_ad.jpg"]` {
		t.Fatalf("隐藏页应存相对路径，实际 %q", got.HiddenPages)
	}
	if got.PageCount != 2 || got.OriginalPageCount != 3 {
		t.Fatalf("落库页数应为 2/3，实际 %d/%d", got.PageCount, got.OriginalPageCount)
	}

	// 4) 封面缓存已失效（D11）
	if _, err := os.Stat(cacheFile); !os.IsNotExist(err) {
		t.Fatalf("隐藏页保存后封面缓存应被删除")
	}

	// 5) /pages 反映隐藏结果
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/comics/c1/pages", nil))
	pagesResp = struct {
		Pages         []string `json:"pages"`
		Total         int      `json:"total"`
		OriginalTotal int      `json:"originalTotal"`
		HiddenPages   []string `json:"hiddenPages"`
	}{}
	if err := json.Unmarshal(w.Body.Bytes(), &pagesResp); err != nil {
		t.Fatalf("解析 /pages 失败: %v", err)
	}
	if pagesResp.Total != 2 || pagesResp.OriginalTotal != 3 {
		t.Fatalf("隐藏后应为有效 2 / 物理 3，实际 %+v", pagesResp)
	}
	if len(pagesResp.HiddenPages) != 1 || pagesResp.HiddenPages[0] != "01_ad.jpg" {
		t.Fatalf("隐藏页应为 01_ad.jpg，实际 %v", pagesResp.HiddenPages)
	}
}

// TestHiddenPagesIgnoresUnknownPaths 校验只接受页表中存在的相对路径。
func TestHiddenPagesIgnoresUnknownPaths(t *testing.T) {
	db := newComicPagesTestDB(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "1.jpg"), []byte("x"), 0o644); err != nil {
		t.Fatalf("写文件失败: %v", err)
	}
	if err := db.Create(&models.OfflineComic{ID: "c2", Title: "t", LocalPath: dir, PageCount: 1, OriginalPageCount: 1}).Error; err != nil {
		t.Fatalf("创建漫画失败: %v", err)
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.PUT("/comics/:id/hidden-pages", UpdateComicHiddenPages)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/comics/c2/hidden-pages", strings.NewReader(`{"hiddenPages":["not-exist.jpg"]}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT 返回 %d: %s", w.Code, w.Body.String())
	}

	var got models.OfflineComic
	if err := db.First(&got, "id = ?", "c2").Error; err != nil {
		t.Fatalf("读取漫画失败: %v", err)
	}
	if got.HiddenPages != "[]" {
		t.Fatalf("不存在的路径应被丢弃，实际 %q", got.HiddenPages)
	}
}
