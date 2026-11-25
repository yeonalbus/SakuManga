package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"SakuManga/internal/models"
	"SakuManga/internal/services"
)

// ─────────────────────────────────────────────────────────────
// Round54：恢复接口权限与响应契约
//   - 全部接口仅管理员可访问
//   - start：无候选 400、恢复中 409、正常 200
//   - cancel：空 body = 全部取消，返回 cancelled 计数
// ─────────────────────────────────────────────────────────────

func newRecoveryHandlerTestDB(t *testing.T) (*gorm.DB, *RecoveryHandler) {
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
	if err := db.AutoMigrate(&models.OfflineComic{}, &models.RecoveryCandidate{},
		&models.FallenComic{}, &models.DownloadSetting{}, &models.DownloadTask{}); err != nil {
		t.Fatalf("迁移模型失败: %v", err)
	}
	mgr := services.NewDownloadManager(db, nil)
	return db, NewRecoveryHandler(db, mgr)
}

func recoveryRouter(h *RecoveryHandler) *gin.Engine {
	r := gin.New()
	adminMW := authAsUser(&models.User{ID: 1, Role: services.RoleAdmin})
	memberMW := authAsUser(&models.User{ID: 2, Role: "member"})
	r.GET("/recovery/status", adminMW, h.GetStatus)
	r.GET("/recovery/status-member", memberMW, h.GetStatus) // 权限用例
	r.GET("/recovery/candidates", adminMW, h.ListCandidates)
	r.POST("/recovery/start", adminMW, h.StartRecovery)
	r.POST("/recovery/cancel", adminMW, h.CancelRecovery)
	r.POST("/recovery/clear-candidates", adminMW, h.ClearCandidates)
	r.POST("/recovery/reconcile-default", adminMW, h.ReconcileDefault)
	return r
}

func TestRecoveryStatusPermissions(t *testing.T) {
	_, h := newRecoveryHandlerTestDB(t)
	r := recoveryRouter(h)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/recovery/status-member", nil))
	if w.Code != http.StatusForbidden {
		t.Fatalf("非管理员应 403，得到 %d: %s", w.Code, w.Body.String())
	}
}

func TestRecoveryCandidatesAndStatus(t *testing.T) {
	db, h := newRecoveryHandlerTestDB(t)
	db.Create(&models.RecoveryCandidate{GID: "100", Token: "t", Title: "a"})
	r := recoveryRouter(h)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/recovery/status", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("管理员 status 应 200，得到 %d: %s", w.Code, w.Body.String())
	}
	var status services.RecoveryStatus
	if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil {
		t.Fatalf("解析状态失败: %v", err)
	}
	if status.Phase != "ready" || status.CandidateCount != 1 {
		t.Fatalf("状态不符: %+v", status)
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/recovery/candidates?page=1&size=10", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("候选列表应 200，得到 %d: %s", w.Code, w.Body.String())
	}
}

func TestRecoveryStartContract(t *testing.T) {
	db, h := newRecoveryHandlerTestDB(t)
	r := recoveryRouter(h)

	// 无候选 → 400
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/recovery/start",
		strings.NewReader(`{"scheme":"gallery"}`)))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("无候选启动应 400，得到 %d: %s", w.Code, w.Body.String())
	}

	// 有候选 → 200，创建任务
	db.Create(&models.RecoveryCandidate{GID: "100", Token: "t", Title: "a", ComicID: "c1"})
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/recovery/start",
		strings.NewReader(`{"scheme":"gallery"}`)))
	if w.Code != http.StatusOK {
		t.Fatalf("有候选启动应 200，得到 %d: %s", w.Code, w.Body.String())
	}
	var res services.RecoveryStartResult
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("解析启动结果失败: %v", err)
	}
	if res.Created != 1 {
		t.Fatalf("应创建 1 条任务，得到 %+v", res)
	}

	// 恢复中再次启动 → 409
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/recovery/start",
		strings.NewReader(`{"scheme":"gallery"}`)))
	if w.Code != http.StatusConflict {
		t.Fatalf("恢复中启动应 409，得到 %d: %s", w.Code, w.Body.String())
	}

	// 取消（空 body = 全部）
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/recovery/cancel", strings.NewReader("")))
	if w.Code != http.StatusOK {
		t.Fatalf("取消应 200，得到 %d: %s", w.Code, w.Body.String())
	}
	var cancelRes services.RecoveryCancelResult
	if err := json.Unmarshal(w.Body.Bytes(), &cancelRes); err != nil {
		t.Fatalf("解析取消结果失败: %v", err)
	}
	if cancelRes.Cancelled != 1 {
		t.Fatalf("应取消 1 条任务，得到 %+v", cancelRes)
	}
}

// TestFallenListAndDismissContract 阵亡名单接口契约：列表 / x toggle / 权限 / 参数校验
func TestFallenListAndDismissContract(t *testing.T) {
	db, h := newRecoveryHandlerTestDB(t)
	db.Create(&models.FallenComic{DedupeKey: "gid:100", GID: "100", Title: "a"})

	r := gin.New()
	adminMW := authAsUser(&models.User{ID: 1, Role: services.RoleAdmin})
	memberMW := authAsUser(&models.User{ID: 2, Role: "member"})
	r.GET("/fallen", adminMW, h.GetFallen)
	r.GET("/fallen-member", memberMW, h.GetFallen)
	r.POST("/fallen/dismiss", adminMW, h.DismissFallen)

	// 权限
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/fallen-member", nil))
	if w.Code != http.StatusForbidden {
		t.Fatalf("非管理员应 403，得到 %d", w.Code)
	}

	// 列表
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/fallen", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("列表应 200，得到 %d: %s", w.Code, w.Body.String())
	}

	// 缺 id → 400
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/fallen/dismiss", strings.NewReader(`{"dismissed":true}`)))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("缺 id 应 400，得到 %d", w.Code)
	}

	// 正常 toggle
	var f models.FallenComic
	db.First(&f, "dedupe_key = ?", "gid:100")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/fallen/dismiss",
		strings.NewReader(`{"id":`+strconv.FormatUint(uint64(f.ID), 10)+`,"dismissed":true}`)))
	if w.Code != http.StatusOK {
		t.Fatalf("dismiss 应 200，得到 %d: %s", w.Code, w.Body.String())
	}
	db.First(&f, "dedupe_key = ?", "gid:100")
	if !f.Dismissed {
		t.Fatalf("dismissed 应置 true")
	}
}

func TestClearCandidatesAndReconcileDefaultEndpoints(t *testing.T) {
	db, h := newRecoveryHandlerTestDB(t)
	db.AutoMigrate(&models.Bookshelf{}, &models.HistoryRecord{}, &models.ReadingList{},
		&models.ComicRating{}, &models.ComicBookmark{}, &models.ComicChapter{})
	r := recoveryRouter(h)

	// 恢复中调用 clear-candidates → 409
	db.Create(&models.RecoveryCandidate{GID: "700", Token: "t", Title: "c700", ComicID: "c700"})
	db.Create(&models.DownloadTask{ID: "t700", GID: "700", Group: services.RecoveryGroup, Status: models.DownloadPaused})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/recovery/clear-candidates", nil))
	if w.Code != http.StatusConflict {
		t.Fatalf("恢复中清空候选应 409，得到 %d: %s", w.Code, w.Body.String())
	}

	// 取消任务后清空候选 → 200
	db.Where("id = ?", "t700").Delete(&models.DownloadTask{})
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/recovery/clear-candidates", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("清空候选应 200，得到 %d: %s", w.Code, w.Body.String())
	}

	// 默认目录对账接口 → 200
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/recovery/reconcile-default", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("默认目录对账应 200，得到 %d: %s", w.Code, w.Body.String())
	}
}

func TestFileMissingFilteredInBookshelfAndHistory(t *testing.T) {
	db, h := newLibraryTestDB(t)
	db.AutoMigrate(&models.HistoryRecord{}, &models.ServerSetting{})
	db.Create(&models.OfflineComic{ID: "alive", Title: "alive", LocalPath: "p1", CoverURL: "/c/alive"})
	db.Create(&models.OfflineComic{ID: "hidden", Title: "hidden", LocalPath: "p2", CoverURL: "/c/hidden", FileMissing: true})
	seedShelf(t, db, 1, "S1", []string{"hidden", "alive"})
	db.Create(&models.HistoryRecord{UserID: 1, ComicID: "alive", Source: models.SourceOffline, ComicTitle: "alive"})
	db.Create(&models.HistoryRecord{UserID: 1, ComicID: "hidden", Source: models.SourceOffline, ComicTitle: "hidden"})

	r := gin.New()
	r.GET("/bookshelves", authAs(db, 1), h.GetBookshelves)
	r.GET("/history", authAs(db, 1), h.GetHistory)

	// 书架 count=1、unread=1、coverUrl 回退到 alive，且 comicIds 保留冻结的 hidden
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/bookshelves", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("书架查询失败: %d", w.Code)
	}
	var shelfResp struct {
		Bookshelves []struct {
			Count       int      `json:"count"`
			UnreadCount int      `json:"unreadCount"`
			CoverURL    string   `json:"coverUrl"`
			ComicIDs    []string `json:"comicIds"`
		} `json:"bookshelves"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &shelfResp)
	s := shelfResp.Bookshelves[0]
	if s.Count != 1 || s.UnreadCount != 1 || s.CoverURL != "/c/alive" || len(s.ComicIDs) != 2 {
		t.Fatalf("书架应隐藏 file_missing=true 但保留冻结 ID，实际 %+v", s)
	}

	// 离线历史应排除 file_missing=true
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/history?source=offline", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("历史查询失败: %d", w.Code)
	}
	var histResp struct {
		Items []models.HistoryRecord `json:"items"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &histResp)
	if len(histResp.Items) != 1 || histResp.Items[0].ComicID != "alive" {
		t.Fatalf("离线历史应过滤 file_missing=true，实际 %+v", histResp.Items)
	}
}
