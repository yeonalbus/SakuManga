package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"

	"SakuManga/internal/models"
)

// ─────────────────────────────────────────────────────────────
// Round53 阶段二：D15 来源标签
//
// 路径存在 → Name；ScanPathID 空 → 「下载」；路径已删/Name 空 → 「已移除来源」。
// 此前失效路径会 fallback 成「下载」，把额外路径漫画伪装成下载导入。
// ─────────────────────────────────────────────────────────────

func TestResolveSourceLabel(t *testing.T) {
	names := map[string]string{"p1": "JHentai"}
	cases := []struct{ name, id, want string }{
		{"下载导入", "", "下载"},
		{"路径有名称", "p1", "JHentai"},
		{"路径已删除", "ghost", "已移除来源"},
		{"路径存在但名称为空", "p2", "已移除来源"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := resolveSourceLabel(c.id, names); got != c.want {
				t.Errorf("resolveSourceLabel(%q) = %q，期望 %q", c.id, got, c.want)
			}
		})
	}
}

func TestComicDetailReturnsSourceLabel(t *testing.T) {
	db := newComicPagesTestDB(t)
	if err := db.AutoMigrate(&models.ExtraScanPath{}); err != nil {
		t.Fatalf("迁移路径表失败: %v", err)
	}
	if err := db.Create(&models.ExtraScanPath{ID: "p1", Path: "Z:/x", Name: "JHentai"}).Error; err != nil {
		t.Fatalf("创建路径失败: %v", err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "1.jpg"), []byte("x"), 0o644); err != nil {
		t.Fatalf("写图片失败: %v", err)
	}
	if err := db.Create(&models.OfflineComic{ID: "c1", Title: "t", LocalPath: dir, ScanPathID: "p1", PageCount: 1}).Error; err != nil {
		t.Fatalf("创建漫画失败: %v", err)
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/comics/:id", GetOfflineComicDetail)

	readLabel := func() string {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/comics/c1", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("详情返回 %d: %s", w.Code, w.Body.String())
		}
		var resp struct {
			SourceLabel string `json:"sourceLabel"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("解析详情失败: %v", err)
		}
		return resp.SourceLabel
	}

	if got := readLabel(); got != "JHentai" {
		t.Fatalf("详情来源标签应为 JHentai，实际 %q", got)
	}

	// 路径被删除 → 已移除来源（不再伪装成下载）
	if err := db.Where("id = ?", "p1").Delete(&models.ExtraScanPath{}).Error; err != nil {
		t.Fatalf("删除路径失败: %v", err)
	}
	if got := readLabel(); got != "已移除来源" {
		t.Fatalf("路径删除后详情来源标签应为「已移除来源」，实际 %q", got)
	}
}
