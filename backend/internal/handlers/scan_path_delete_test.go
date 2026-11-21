package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"SakuManga/internal/database"
	"SakuManga/internal/models"
	"SakuManga/internal/services"
)

// ─────────────────────────────────────────────────────────────
// Round53：DELETE /scan-paths/:id 必须连带清理该路径下的离线漫画记录
// （修复「删除扫描路径后漫画仍残留在程序中」）
// ─────────────────────────────────────────────────────────────

func newScanPathTestDB(t *testing.T) *gorm.DB {
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

	if err := db.AutoMigrate(
		&models.ExtraScanPath{},
		&models.OfflineComic{},
		&models.HistoryRecord{},
		&models.Bookshelf{},
		&models.ReadingList{},
	); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}

	// handler 通过全局 database.DB 取库；测试结束后恢复。
	oldDB := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = oldDB })

	// 屏蔽 XP 全局服务：避免触发后台全量重建 goroutine 访问已关闭的测试库。
	oldXp := services.GlobalXpCloud
	services.GlobalXpCloud = nil
	t.Cleanup(func() { services.GlobalXpCloud = oldXp })

	return db
}

func scanPathRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/scan-paths/:id/impact", GetScanPathImpact)
	r.DELETE("/scan-paths/:id", DeleteScanPath)
	return r
}

func TestDeleteScanPathCascadesComics(t *testing.T) {
	db := newScanPathTestDB(t)

	if err := db.Create(&models.ExtraScanPath{ID: "path-a", Path: "Z:/a", Name: "A"}).Error; err != nil {
		t.Fatalf("创建路径失败: %v", err)
	}
	if err := db.Create(&models.ExtraScanPath{ID: "path-b", Path: "Z:/b", Name: "B"}).Error; err != nil {
		t.Fatalf("创建路径失败: %v", err)
	}
	comics := []models.OfflineComic{
		{ID: "a1", GID: "100", Title: "a1", LocalPath: "Z:/a/1", ScanPathID: "path-a"},
		{ID: "b1", GID: "200", Title: "b1", LocalPath: "Z:/b/1", ScanPathID: "path-b"},
		{ID: "dl", GID: "300", Title: "dl", LocalPath: "Z:/dl"},
	}
	for i := range comics {
		if err := db.Create(&comics[i]).Error; err != nil {
			t.Fatalf("创建漫画 %s 失败: %v", comics[i].ID, err)
		}
	}

	r := scanPathRouter()

	// 1. impact 计数
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/scan-paths/path-a/impact", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("impact 返回 %d: %s", w.Code, w.Body.String())
	}
	var impact struct {
		ComicCount int64 `json:"comicCount"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &impact); err != nil {
		t.Fatalf("解析 impact 失败: %v", err)
	}
	if impact.ComicCount != 1 {
		t.Fatalf("path-a 应统计 1 本，得到 %d", impact.ComicCount)
	}

	// 2. 删除路径 → 连带清理漫画
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/scan-paths/path-a", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("删除返回 %d: %s", w.Code, w.Body.String())
	}
	var delResp struct {
		Message       string `json:"message"`
		RemovedComics int    `json:"removedComics"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &delResp); err != nil {
		t.Fatalf("解析删除响应失败: %v", err)
	}
	if delResp.RemovedComics != 1 {
		t.Errorf("应清理 1 本，得到 %d（响应：%s）", delResp.RemovedComics, w.Body.String())
	}

	// 3. 断言：a1 与 path-a 消失；b1/dl/path-b 保留
	var count int64
	db.Model(&models.OfflineComic{}).Where("id = ?", "a1").Count(&count)
	if count != 0 {
		t.Errorf("a1 应被清理，实际仍存在")
	}
	for _, id := range []string{"b1", "dl"} {
		count = 0
		db.Model(&models.OfflineComic{}).Where("id = ?", id).Count(&count)
		if count != 1 {
			t.Errorf("%s 不应被清理", id)
		}
	}
	db.Model(&models.ExtraScanPath{}).Where("id = ?", "path-a").Count(&count)
	if count != 0 {
		t.Errorf("path-a 应被删除")
	}
	db.Model(&models.ExtraScanPath{}).Where("id = ?", "path-b").Count(&count)
	if count != 1 {
		t.Errorf("path-b 不应被删除")
	}

	// 4. 幂等：重复删除返回 200 / removedComics 0
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/scan-paths/path-a", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("重复删除应幂等返回 200，得到 %d: %s", w.Code, w.Body.String())
	}
	delResp = struct {
		Message       string `json:"message"`
		RemovedComics int    `json:"removedComics"`
	}{}
	if err := json.Unmarshal(w.Body.Bytes(), &delResp); err != nil {
		t.Fatalf("解析幂等响应失败: %v", err)
	}
	if delResp.RemovedComics != 0 {
		t.Errorf("重复删除不应再清理漫画，得到 %d", delResp.RemovedComics)
	}
}

// TestDeleteScanPathGhostIdKeepsComics 校验「路径记录已不存在」时不得按该 id 批量删库：
// 否则伪造任意 id 即可清空库中带该 scan_path_id 的记录（后门）。
func TestDeleteScanPathGhostIdKeepsComics(t *testing.T) {
	db := newScanPathTestDB(t)

	// 历史残留（路径记录已不在路径表中）
	if err := db.Create(&models.OfflineComic{
		ID: "orphan", Title: "orphan", LocalPath: "Z:/orphan", ScanPathID: "ghost-path",
	}).Error; err != nil {
		t.Fatalf("创建漫画失败: %v", err)
	}

	r := scanPathRouter()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/scan-paths/ghost-path", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("幂等删除应返回 200，得到 %d: %s", w.Code, w.Body.String())
	}

	var count int64
	db.Model(&models.OfflineComic{}).Where("id = ?", "orphan").Count(&count)
	if count != 1 {
		t.Errorf("路径不存在时不得删除漫画记录（防任意 id 批量删除后门）")
	}
}
