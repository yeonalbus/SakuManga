package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"SakuManga/internal/database"
	"SakuManga/internal/models"
	"SakuManga/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// newHandlerTestDB 构造内存库并挂到全局 database.DB（handler 通过全局句柄取库）
func newHandlerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("打开内存数据库失败: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("获取底层连接失败: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&models.NetworkSetting{}, &models.ArchiveBinding{}); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}

	old := database.DB
	database.DB = db
	t.Cleanup(func() {
		database.DB = old
		_ = sqlDB.Close()
	})
	return db
}

// fakeClashServer 最小控制面假服务（鉴权 + 版本 + 组 + 延迟）
func fakeClashServer(t *testing.T, secret string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	auth := func(w http.ResponseWriter, r *http.Request) bool {
		if secret != "" && r.Header.Get("Authorization") != "Bearer "+secret {
			w.WriteHeader(http.StatusUnauthorized)
			return false
		}
		return true
	}
	mux.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"version": "v1.19.33", "meta": true})
	})
	mux.HandleFunc("/proxies/Ehentai", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"type": "Selector", "now": "HK 01", "all": []string{"HK 01", "JP 01"},
		})
	})
	mux.HandleFunc("/group/Ehentai/delay", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]int{"HK 01": 280, "JP 01": 350})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func postJSON(t *testing.T, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST(path, TestClashHandler)
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// TestClashHandlerUsesSavedSecretWhenOmitted 覆盖「保存后密钥输入框为空也能直接测试」的体验：
// 请求只带 controller、不带 secret 时，应自动沿用 DB 里已保存的密钥。
func TestClashHandlerUsesSavedSecretWhenOmitted(t *testing.T) {
	db := newHandlerTestDB(t)
	server := fakeClashServer(t, "s3cret")
	controller := strings.TrimPrefix(server.URL, "http://")

	if err := services.SaveNetworkSetting(db, &models.NetworkSetting{
		AdvancedProxyEnabled: true,
		ClashController:      controller,
		ClashSecret:          "s3cret",
		GroupEGallery:        "Ehentai",
		GroupEGalleryEx:      "Ehentai",
	}); err != nil {
		t.Fatalf("保存设置失败: %v", err)
	}

	// 不带 secret（模拟刷新页面后输入框为空）
	w := postJSON(t, "/network/clash/test", `{"controller":"`+controller+`","secret":""}`)
	if w.Code != http.StatusOK {
		t.Fatalf("应沿用已保存密钥并测试成功，实际 %d：%s", w.Code, w.Body.String())
	}
	var resp struct {
		OK     bool   `json:"ok"`
		Group  string `json:"group"`
		Usable int    `json:"usable"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	if !resp.OK || resp.Group != "Ehentai" || resp.Usable != 2 {
		t.Fatalf("测试结果异常: %+v", resp)
	}
}

// TestClashHandlerEmptySecretWhenControllerChanged 改了控制面地址又没填密钥时，
// 不应沿用旧密钥（那是别的地址的密钥），应返回鉴权失败。
func TestClashHandlerEmptySecretWhenControllerChanged(t *testing.T) {
	db := newHandlerTestDB(t)
	server := fakeClashServer(t, "s3cret")
	controller := strings.TrimPrefix(server.URL, "http://")

	if err := services.SaveNetworkSetting(db, &models.NetworkSetting{
		AdvancedProxyEnabled: true,
		ClashController:      "127.0.0.1:12345", // 与本次测试使用的地址不同
		ClashSecret:          "s3cret",
		GroupEGallery:        "Ehentai",
		GroupEGalleryEx:      "Ehentai",
	}); err != nil {
		t.Fatalf("保存设置失败: %v", err)
	}

	w := postJSON(t, "/network/clash/test", `{"controller":"`+controller+`","secret":""}`)
	if w.Code == http.StatusOK {
		t.Fatalf("地址变更且未填密钥时不应测试成功：%s", w.Body.String())
	}
}
