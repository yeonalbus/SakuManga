package services

import (
	"encoding/json"
	"errors"
	"testing"

	"SakuManga/internal/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// Round28：搜刮书签服务测试（后端化，按用户隔离 + 宽松归一化）

func newScrapeBookmarkTestDB(t *testing.T) *gorm.DB {
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
	if err := db.AutoMigrate(&models.ScrapeBookmark{}); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	return db
}

func validInput() ScrapeBookmarkInput {
	return ScrapeBookmarkInput{
		Name:    "测试书签",
		Type:    "search",
		Keyword: "touhou",
		Config:  json.RawMessage(`{"keyword":"touhou","activeCategories":["Doujinshi"]}`),
		Anchor:  json.RawMessage(`{"gid":"123","token":"abc","title":"画廊"}`),
	}
}

func TestScrapeBookmarkCRUDAndIsolation(t *testing.T) {
	db := newScrapeBookmarkTestDB(t)
	userA := uint(1)
	userB := uint(2)

	// 创建
	created, err := CreateScrapeBookmark(db, userA, validInput())
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	if created.ID == 0 || created.Name != "测试书签" {
		t.Fatalf("创建结果异常: %+v", created)
	}

	// 列表：本人可见
	items, err := ListScrapeBookmarks(db, userA)
	if err != nil || len(items) != 1 {
		t.Fatalf("A 列表应为 1 条: %v %d", err, len(items))
	}
	// 用户隔离：B 看不到 A 的书签
	itemsB, err := ListScrapeBookmarks(db, userB)
	if err != nil || len(itemsB) != 0 {
		t.Fatalf("B 列表应为空（用户隔离）: %v %d", err, len(itemsB))
	}

	// 重命名
	if err := RenameScrapeBookmark(db, userA, created.ID, "新名字"); err != nil {
		t.Fatalf("重命名失败: %v", err)
	}
	items, _ = ListScrapeBookmarks(db, userA)
	if items[0].Name != "新名字" {
		t.Fatalf("重命名未生效: %s", items[0].Name)
	}
	// 越权重命名：B 操作 A 的书签 → 不存在
	if err := RenameScrapeBookmark(db, userB, created.ID, "hack"); !errors.Is(err, ErrBookmarkNotFound) {
		t.Fatalf("越权重命名应报不存在: %v", err)
	}

	// 删除
	if err := DeleteScrapeBookmark(db, userA, created.ID); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	items, _ = ListScrapeBookmarks(db, userA)
	if len(items) != 0 {
		t.Fatalf("删除后应为空: %d", len(items))
	}
	// 越权删除：B 删 A 的（已删）→ 不存在
	if err := DeleteScrapeBookmark(db, userB, created.ID); !errors.Is(err, ErrBookmarkNotFound) {
		t.Fatalf("越权删除应报不存在: %v", err)
	}
}

func TestScrapeBookmarkNormalizeLenient(t *testing.T) {
	db := newScrapeBookmarkTestDB(t)
	user := uint(1)

	// 宽松归一化（BUG2 教训：不因坏字段拒收/丢数据）
	created, err := CreateScrapeBookmark(db, user, ScrapeBookmarkInput{
		Name:    "   ",          // Round29：空名合法（留空 → 前端展示位置+发布时间）
		Type:    "unknown-type", // 非法类型 → home
		Keyword: "  kw  ",
		Config:  json.RawMessage(`"corrupt-string"`), // 非对象 → {}
		Anchor:  json.RawMessage(`"not-an-object"`),  // 非法 → null
	})
	if err != nil {
		t.Fatalf("宽松创建失败: %v", err)
	}
	if created.Name != "" {
		t.Fatalf("空名应保持空串（前端展示位置+时间）: %q", created.Name)
	}
	if created.Type != "home" {
		t.Fatalf("非法类型应兜底 home: %q", created.Type)
	}
	if string(created.Config) != "{}" {
		t.Fatalf("坏 config 应兜底空对象: %s", created.Config)
	}
	if string(created.Anchor) != "null" {
		t.Fatalf("坏 anchor 应兜底 null: %s", created.Anchor)
	}

	// anchor=null 与合法 anchor 正常透传
	noAnchor, err := CreateScrapeBookmark(db, user, ScrapeBookmarkInput{
		Name:   "x",
		Type:   "home",
		Config: json.RawMessage(`{"keyword":""}`),
		Anchor: json.RawMessage(`null`),
	})
	if err != nil || string(noAnchor.Anchor) != "null" {
		t.Fatalf("null anchor 透传失败: %v %s", err, noAnchor.Anchor)
	}
}

func TestScrapeBookmarkRenameEmptyIgnored(t *testing.T) {
	db := newScrapeBookmarkTestDB(t)
	user := uint(1)
	created, _ := CreateScrapeBookmark(db, user, validInput())
	// 空名重命名：忽略（保持原名，不报错）
	if err := RenameScrapeBookmark(db, user, created.ID, "   "); err != nil {
		t.Fatalf("空名重命名应忽略不报错: %v", err)
	}
	items, _ := ListScrapeBookmarks(db, user)
	if items[0].Name != "测试书签" {
		t.Fatalf("空名重命名不应改名称: %s", items[0].Name)
	}
}
