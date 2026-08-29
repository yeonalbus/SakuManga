package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"SakuManga/internal/models"

	"github.com/gin-gonic/gin"
)

// Round20-Bug1：离线历史按 gid 合并去重

func TestDedupeHistoryByGid(t *testing.T) {
	base := time.Now()
	records := []models.HistoryRecord{
		{ID: 1, ComicID: "md5-new", GID: "12345", LastReadAt: base},                // 最新
		{ID: 2, ComicID: "md5-old", GID: "12345", LastReadAt: base.Add(-time.Hour)}, // 同 gid 旧行
		{ID: 3, ComicID: "md5-only", LastReadAt: base.Add(-2 * time.Hour)},          // 无 gid 回退 comic_id
		{ID: 4, ComicID: "md5-dup", LastReadAt: base.Add(-3 * time.Hour)},           // 无 gid 重复（不应出现但防御）
	}
	got := dedupeHistoryByGid(records)
	if len(got) != 3 {
		t.Fatalf("应去重为 3 条，得到 %d: %+v", len(got), got)
	}
	// 同 gid 只保留最新（ID=1）
	if got[0].ID != 1 {
		t.Errorf("同 gid 应保留最新记录（ID=1），得到 ID=%d", got[0].ID)
	}
}

// TestAddHistoryGidMerge 验证 AddHistory 写入同 gid 新 id 时删除旧 id 行（同用户离线来源）
func TestAddHistoryGidMerge(t *testing.T) {
	db, h := newLibraryTestDB(t)
	if err := db.AutoMigrate(&models.HistoryRecord{}); err != nil {
		t.Fatalf("迁移 HistoryRecord 失败: %v", err)
	}
	// 预先存在旧 id 的历史行（同 gid 12345）
	old := models.HistoryRecord{
		UserID: 1, ComicID: "md5-old", GID: "12345", Source: models.SourceOffline,
		ComicTitle: "Old", LastReadAt: time.Now(),
	}
	if err := db.Create(&old).Error; err != nil {
		t.Fatalf("预置旧历史失败: %v", err)
	}

	post := func(body string) *httptest.ResponseRecorder {
		r := gin.New()
		r.POST("/history", authAs(db, 1), h.AddHistory)
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/history", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		return w
	}

	// 写入同 gid 的新 id（带 gid 字段）
	w := post(`{"comicId":"md5-new","source":"offline","gid":"12345","comicTitle":"New","coverUrl":"/c.jpg"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("AddHistory 返回 %d: %s", w.Code, w.Body.String())
	}

	var rows []models.HistoryRecord
	db.Where("user_id = 1 AND source = ?", models.SourceOffline).Find(&rows)
	if len(rows) != 1 {
		t.Fatalf("同 gid 合并后应只剩 1 条，得到 %d: %+v", len(rows), rows)
	}
	if rows[0].ComicID != "md5-new" || rows[0].GID != "12345" {
		t.Errorf("应保留新 id 行且回填 gid，得到 %+v", rows[0])
	}

	// 无 gid 的写入（旧客户端）不触发 gid 合并，仅按 comic_id upsert
	db.Create(&models.HistoryRecord{UserID: 1, ComicID: "other", Source: models.SourceOffline, LastReadAt: time.Now()})
	w2 := post(`{"comicId":"other","source":"offline","comicTitle":"Other"}`)
	if w2.Code != http.StatusOK {
		t.Fatalf("AddHistory(无 gid) 返回 %d: %s", w2.Code, w2.Body.String())
	}
	var rows2 []models.HistoryRecord
	db.Where("user_id = 1 AND source = ?", models.SourceOffline).Find(&rows2)
	if len(rows2) != 2 {
		t.Fatalf("无 gid 写入不应合并，应 2 条，得到 %d", len(rows2))
	}
}

// TestGetHistoryDedupesByGid 验证 GetHistory 离线列表按 gid 去重
func TestGetHistoryDedupesByGid(t *testing.T) {
	db, h := newLibraryTestDB(t)
	if err := db.AutoMigrate(&models.HistoryRecord{}); err != nil {
		t.Fatalf("迁移 HistoryRecord 失败: %v", err)
	}
	base := time.Now()
	db.Create(&models.HistoryRecord{UserID: 1, ComicID: "md5-new", GID: "12345", Source: models.SourceOffline, LastReadAt: base})
	db.Create(&models.HistoryRecord{UserID: 1, ComicID: "md5-old", GID: "12345", Source: models.SourceOffline, LastReadAt: base.Add(-time.Hour)})
	db.Create(&models.HistoryRecord{UserID: 1, ComicID: "single", Source: models.SourceOffline, LastReadAt: base.Add(-2 * time.Hour)})

	r := gin.New()
	r.GET("/history", authAs(db, 1), h.GetHistory)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/history?source=offline&limit=50", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GetHistory 返回 %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Items []models.HistoryRecord `json:"items"`
		Total int                    `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	if resp.Total != 2 || len(resp.Items) != 2 {
		t.Fatalf("应去重为 2 条，得到 total=%d len=%d", resp.Total, len(resp.Items))
	}
	if resp.Items[0].ComicID != "md5-new" {
		t.Errorf("应保留最新（md5-new），得到 %s", resp.Items[0].ComicID)
	}
}
