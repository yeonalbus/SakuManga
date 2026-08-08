package services

import (
	"testing"

	"SakuHentai/internal/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// 在线搜索 f_search 自动修正开关回归测试：
// SaveTagMaintainSetting / LoadTagMaintainSetting 必须同步 fSearchAutoCorrect 包级原子缓存，
// 确保 FetchGalleryList 在运行期读取到的是最新全局开关（而非旧值）。
func TestFSearchAutoCorrectSwitchRoundTrip(t *testing.T) {
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

	// 1) 保存开启 → 缓存应为 true
	on := &models.TagMaintainSetting{EnableFSearchAutoCorrect: true}
	if _, err := SaveTagMaintainSetting(db, on); err != nil {
		t.Fatalf("保存开启失败: %v", err)
	}
	if !FSearchAutoCorrectEnabled() {
		t.Fatal("保存开启后缓存应为 true")
	}

	// 2) 关闭 → 保存后缓存应为 false，重新 Load 后字段与缓存均保持 false
	off := &models.TagMaintainSetting{EnableFSearchAutoCorrect: false}
	if _, err := SaveTagMaintainSetting(db, off); err != nil {
		t.Fatalf("保存关闭失败: %v", err)
	}
	if FSearchAutoCorrectEnabled() {
		t.Fatal("保存关闭后缓存应为 false")
	}
	loaded := LoadTagMaintainSetting(db)
	if loaded.EnableFSearchAutoCorrect {
		t.Fatal("重新 Load 后字段应为 false")
	}
	if FSearchAutoCorrectEnabled() {
		t.Fatal("重新 Load 后缓存应仍为 false")
	}

	// 3) 恢复开启（幂等回到默认态）
	on.ID = 1
	if _, err := SaveTagMaintainSetting(db, on); err != nil {
		t.Fatalf("恢复开启失败: %v", err)
	}
	if !FSearchAutoCorrectEnabled() {
		t.Fatal("恢复开启后缓存应为 true")
	}
}
