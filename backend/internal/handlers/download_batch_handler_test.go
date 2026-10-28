package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"SakuManga/internal/middleware"
	"SakuManga/internal/models"
	"SakuManga/internal/services"
)

// Round47：下载统计 / 批量操作 handler 的权限与响应契约。

func newDownloadHandlerTestDB(t *testing.T) (*gorm.DB, *DownloadHandler) {
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
	if err := db.AutoMigrate(&models.DownloadSetting{}, &models.DownloadTask{}); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	mgr := services.NewDownloadManager(db, nil)
	return db, NewDownloadHandler(db, nil, mgr)
}

func authAsUser(user *models.User) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(middleware.ContextUserKey, user)
		c.Next()
	}
}

func seedDownloadTask(t *testing.T, db *gorm.DB, id string, userID uint, status models.DownloadTaskStatus) {
	t.Helper()
	task := &models.DownloadTask{
		ID:     id,
		UserID: userID,
		GID:    id,
		Token:  "t",
		Title:  "task-" + id,
		Status: status,
		Mode:   models.DownloadModeGallery,
	}
	if err := db.Create(task).Error; err != nil {
		t.Fatalf("创建任务失败: %v", err)
	}
}

func TestDownloadStatsHandler(t *testing.T) {
	db, h := newDownloadHandlerTestDB(t)
	seedDownloadTask(t, db, "u1-q", 1, models.DownloadQueued)
	seedDownloadTask(t, db, "u1-p", 1, models.DownloadPaused)
	seedDownloadTask(t, db, "u2-q", 2, models.DownloadQueued)

	r := gin.New()
	r.GET("/downloads/stats", authAsUser(&models.User{ID: 1, Role: services.RoleAdmin}), h.GetDownloadStats)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/downloads/stats", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("管理员 stats 返回 %d: %s", w.Code, w.Body.String())
	}
	var stats services.DownloadStats
	if err := json.Unmarshal(w.Body.Bytes(), &stats); err != nil {
		t.Fatalf("解析 stats 失败: %v", err)
	}
	if stats.Total != 3 || stats.Active != 3 {
		t.Fatalf("管理员 stats 应为 total=3 active=3，得到 %+v", stats)
	}

	// 普通成员只看自己的任务
	r2 := gin.New()
	r2.GET("/downloads/stats", authAsUser(&models.User{ID: 1, Role: services.RoleMember, AllowDownload: true}), h.GetDownloadStats)
	w = httptest.NewRecorder()
	r2.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/downloads/stats", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("成员 stats 返回 %d: %s", w.Code, w.Body.String())
	}
	if err := json.Unmarshal(w.Body.Bytes(), &stats); err != nil {
		t.Fatalf("解析 stats 失败: %v", err)
	}
	if stats.Total != 2 {
		t.Fatalf("成员 stats 应只统计自己的 2 个任务，得到 %+v", stats)
	}

	// 无下载许可 → 403
	r3 := gin.New()
	r3.GET("/downloads/stats", authAsUser(&models.User{ID: 1, Role: services.RoleMember}), h.GetDownloadStats)
	w = httptest.NewRecorder()
	r3.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/downloads/stats", nil))
	if w.Code != http.StatusForbidden {
		t.Fatalf("无许可 stats 应 403，得到 %d", w.Code)
	}
}

func TestBatchActionHandlerScopesAndValidation(t *testing.T) {
	db, h := newDownloadHandlerTestDB(t)
	seedDownloadTask(t, db, "u1-q1", 1, models.DownloadQueued)
	seedDownloadTask(t, db, "u1-q2", 1, models.DownloadQueued)
	seedDownloadTask(t, db, "u2-q1", 2, models.DownloadQueued)

	r := gin.New()
	r.POST(
		"/downloads/batch-action",
		authAsUser(&models.User{ID: 1, Role: services.RoleMember, AllowDownload: true}),
		h.BatchAction,
	)

	post := func(body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/downloads/batch-action", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		return w
	}

	w := post(`{"action":"pause","status":"queued","mode":""}`)
	if w.Code != http.StatusOK {
		t.Fatalf("批量暂停返回 %d: %s", w.Code, w.Body.String())
	}
	var result services.BatchActionResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("解析批量结果失败: %v", err)
	}
	if result.Affected != 2 || result.Failed != 0 {
		t.Fatalf("成员批量暂停应只影响自己的 2 个任务，得到 %+v", result)
	}
	var other models.DownloadTask
	if err := db.First(&other, "id = ?", "u2-q1").Error; err != nil || other.Status != models.DownloadQueued {
		t.Fatalf("其他用户任务不应被修改（err=%v status=%s）", err, other.Status)
	}

	// 参数非法 → 400
	w = post(`{"action":"pause","status":"error"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("非法状态/动作应 400，得到 %d: %s", w.Code, w.Body.String())
	}

	// 无下载许可 → 403
	r2 := gin.New()
	r2.POST(
		"/downloads/batch-action",
		authAsUser(&models.User{ID: 1, Role: services.RoleMember}),
		h.BatchAction,
	)
	w = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/downloads/batch-action", strings.NewReader(`{"action":"pause","status":"queued"}`))
	req.Header.Set("Content-Type", "application/json")
	r2.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("无许可批量操作应 403，得到 %d", w.Code)
	}
}
