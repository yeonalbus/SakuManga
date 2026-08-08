package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"SakuHentai/internal/models"
	"SakuHentai/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// 问题1回归测试：保存 Tag 维护设置时必须接受 enableFSearchAutoCorrect 字段，
// 否则「在线搜索 Tag 语法自动修正」开关在界面上关闭/开启均无效。
// 覆盖 handler 层（saveTagSettingReq 需包含该字段并被 SaveSetting 处理）。
func TestSaveSettingHonorsFSearchAutoCorrect(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("打开内存库失败: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("获取底层连接失败: %v", err)
	}
	// :memory: 库需固定单连接，否则建表对其他连接不可见
	sqlDB.SetMaxOpenConns(1)
	defer sqlDB.Close()

	if err := db.AutoMigrate(&models.TagMaintainSetting{}); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}

	gin.SetMode(gin.TestMode)
	h := &TagMaintainHandler{db: db}
	r := gin.New()
	r.POST("/api/v1/offline/tags/setting", h.SaveSetting)

	doSave := func(t *testing.T, body string) *models.TagMaintainSetting {
		t.Helper()
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/offline/tags/setting", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("保存设置返回 %d: %s", w.Code, w.Body.String())
		}
		var saved models.TagMaintainSetting
		if err := json.Unmarshal(w.Body.Bytes(), &saved); err != nil {
			t.Fatalf("解析响应失败: %v", err)
		}
		return &saved
	}

	// 1) 初始默认开启（LoadTagMaintainSetting 会同步包级缓存）
	if loaded := services.LoadTagMaintainSetting(db); !loaded.EnableFSearchAutoCorrect {
		t.Fatal("初始默认应为开启")
	}
	if !services.FSearchAutoCorrectEnabled() {
		t.Fatal("初始缓存应为 true")
	}

	// 2) 仅提交 enableFSearchAutoCorrect=false（其余字段缺省，不得影响本字段）
	saved := doSave(t, `{"enableFSearchAutoCorrect": false}`)
	if saved.EnableFSearchAutoCorrect {
		t.Fatal("响应中 EnableFSearchAutoCorrect 应为 false")
	}
	if services.FSearchAutoCorrectEnabled() {
		t.Fatal("保存 false 后包级缓存应为 false（修复前会卡在 true）")
	}
	loaded := services.LoadTagMaintainSetting(db)
	if loaded.EnableFSearchAutoCorrect {
		t.Fatal("重新 Load 后字段应为 false")
	}
	if services.FSearchAutoCorrectEnabled() {
		t.Fatal("重新 Load 后缓存应仍为 false")
	}

	// 3) 再次开启
	saved = doSave(t, `{"enableFSearchAutoCorrect": true}`)
	if !saved.EnableFSearchAutoCorrect {
		t.Fatal("响应中 EnableFSearchAutoCorrect 应为 true")
	}
	if !services.FSearchAutoCorrectEnabled() {
		t.Fatal("保存 true 后包级缓存应为 true")
	}

	// 4) 缺少该字段的请求（兼容旧前端）不得改动当前值
	saved = doSave(t, `{"enableDailyRefresh": true}`)
	if !saved.EnableFSearchAutoCorrect {
		t.Fatal("缺字段时不应改动 EnableFSearchAutoCorrect（应保持 true）")
	}
}
