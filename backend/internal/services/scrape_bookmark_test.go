package services

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

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

// Round33：锚点更新（迁移 / 失效标记写回）——扩展字段必须原样保留
func TestUpdateScrapeBookmarkAnchor(t *testing.T) {
	db := newScrapeBookmarkTestDB(t)
	user := uint(1)
	created, _ := CreateScrapeBookmark(db, user, validInput())

	// 带扩展字段的锚点（invalid / migratedFrom）→ 原样存储，不被 struct 往返丢弃
	rich := json.RawMessage(`{"gid":"999","token":"tt","title":"新锚点","postedAt":"2026-09-12 01:47","invalid":{"kind":"removed","at":1730000000},"migratedFrom":{"gid":"123","title":"旧锚点"},"listIndex":7}`)
	if err := UpdateScrapeBookmarkAnchor(db, user, created.ID, rich); err != nil {
		t.Fatalf("锚点更新失败: %v", err)
	}
	items, _ := ListScrapeBookmarks(db, user)
	got := string(items[0].Anchor)
	for _, must := range []string{"999", "invalid", "migratedFrom", "listIndex"} {
		if !strings.Contains(got, must) {
			t.Fatalf("扩展字段 %s 丢失: %s", must, got)
		}
	}

	// 非法锚点：缺 gid / 非对象 → 拒绝
	if err := UpdateScrapeBookmarkAnchor(db, user, created.ID, json.RawMessage(`{"token":"x"}`)); !errors.Is(err, ErrBookmarkInvalidAnchor) {
		t.Fatalf("缺 gid 应拒绝: %v", err)
	}
	if err := UpdateScrapeBookmarkAnchor(db, user, created.ID, json.RawMessage(`"str"`)); !errors.Is(err, ErrBookmarkInvalidAnchor) {
		t.Fatalf("非对象应拒绝: %v", err)
	}
	// null 合法（清空锚点：退化为纯位置书签）
	if err := UpdateScrapeBookmarkAnchor(db, user, created.ID, json.RawMessage(`null`)); err != nil {
		t.Fatalf("null 锚点应允许: %v", err)
	}

	// 越权：他人书签不可更新
	other, _ := CreateScrapeBookmark(db, uint(2), validInput())
	if err := UpdateScrapeBookmarkAnchor(db, user, other.ID, rich); !errors.Is(err, ErrBookmarkNotFound) {
		t.Fatalf("越权更新应报不存在: %v", err)
	}
}

// Round33：锚点解析（探测前置）——null / 空 / 缺 gid 一律视为无锚点
func TestParseAnchorJSON(t *testing.T) {
	cases := []struct {
		raw    string
		wantOK bool
	}{
		{`null`, false},
		{``, false},
		{`{}`, false},
		{`{"gid":""}`, false},
		{`not-json`, false},
		{`{"gid":"123","token":"abc"}`, true},
	}
	for _, c := range cases {
		a, _ := parseAnchorJSON(c.raw)
		if (a != nil) != c.wantOK {
			t.Fatalf("parseAnchorJSON(%q) 期望有效=%v 得到 %+v", c.raw, c.wantOK, a)
		}
	}
	a, err := parseAnchorJSON(`{"gid":"123","token":"abc","postedAt":"2026-09-12 01:47"}`)
	if err != nil || a == nil || a.Token != "abc" || a.PostedAt != "2026-09-12 01:47" {
		t.Fatalf("锚点字段解析异常: %+v %v", a, err)
	}
}

// ─────────────────────────────────────────────────────────────
// Round37：侧栏拖动排序（LexoRank 单点移动）
// ─────────────────────────────────────────────────────────────

// orderOf 取当前用户书签的展示顺序（服务层已按 sort_key ASC, id ASC 排序）
func orderOf(t *testing.T, db *gorm.DB, userID uint) []uint {
	t.Helper()
	items, err := ListScrapeBookmarks(db, userID)
	if err != nil {
		t.Fatalf("读取书签失败: %v", err)
	}
	ids := make([]uint, 0, len(items))
	for _, it := range items {
		ids = append(ids, it.ID)
	}
	return ids
}

// 新建书签「追加到末尾」：权值 1000*(n+1)，顺序为创建顺序
func TestScrapeBookmarkCreateAppendsToEnd(t *testing.T) {
	db := newScrapeBookmarkTestDB(t)
	user := uint(1)

	a, _ := CreateScrapeBookmark(db, user, validInput())
	b, _ := CreateScrapeBookmark(db, user, validInput())
	c, _ := CreateScrapeBookmark(db, user, validInput())

	if a.SortKey != 1000 || b.SortKey != 2000 || c.SortKey != 3000 {
		t.Fatalf("追加权值应为 1000/2000/3000，得到 %v/%v/%v", a.SortKey, b.SortKey, c.SortKey)
	}
	if got := orderOf(t, db, user); !reflect.DeepEqual(got, []uint{a.ID, b.ID, c.ID}) {
		t.Fatalf("创建顺序应为 %v，得到 %v", []uint{a.ID, b.ID, c.ID}, got)
	}
}

// 单点移动：只改被移动项的权值，顺序随之变化；越权移动报不存在
func TestScrapeBookmarkMovePosition(t *testing.T) {
	db := newScrapeBookmarkTestDB(t)
	user := uint(1)

	a, _ := CreateScrapeBookmark(db, user, validInput())
	b, _ := CreateScrapeBookmark(db, user, validInput())
	c, _ := CreateScrapeBookmark(db, user, validInput())

	// 末尾 c 移到最前：权值取 a 之前（模拟前端 between(undefined, 1000)）
	if err := MoveScrapeBookmarkPosition(db, user, c.ID, 999); err != nil {
		t.Fatalf("单点移动失败: %v", err)
	}
	if got := orderOf(t, db, user); !reflect.DeepEqual(got, []uint{c.ID, a.ID, b.ID}) {
		t.Fatalf("移动后顺序应为 [c a b]，得到 %v", got)
	}
	// 其余两项权值不受影响（单点移动语义）
	items, _ := ListScrapeBookmarks(db, user)
	for _, it := range items {
		if it.ID == a.ID && it.SortKey != 1000 {
			t.Fatalf("a 的权值不应被改动，得到 %v", it.SortKey)
		}
		if it.ID == b.ID && it.SortKey != 2000 {
			t.Fatalf("b 的权值不应被改动，得到 %v", it.SortKey)
		}
	}

	// 越权：他人书签不可移动
	other, _ := CreateScrapeBookmark(db, uint(2), validInput())
	if err := MoveScrapeBookmarkPosition(db, user, other.ID, 1); !errors.Is(err, ErrBookmarkNotFound) {
		t.Fatalf("越权移动应报不存在: %v", err)
	}
}

// 升级前老数据（sort_key=0）按 id 兜底排序，新建书签仍落在末尾
func TestScrapeBookmarkLegacyZeroKeyFallback(t *testing.T) {
	db := newScrapeBookmarkTestDB(t)
	user := uint(1)

	legacy := make([]models.ScrapeBookmark, 0, 3)
	for i := 0; i < 3; i++ {
		legacy = append(legacy, models.ScrapeBookmark{
			UserID:    user,
			Name:      "老书签",
			Type:      "home",
			Config:    "{}",
			Anchor:    "null",
			SortKey:   0, // 升级前未赋权
			CreatedAt: time.Now(),
		})
	}
	if err := db.Create(&legacy).Error; err != nil {
		t.Fatalf("写入老数据失败: %v", err)
	}

	fresh, err := CreateScrapeBookmark(db, user, validInput())
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	if fresh.SortKey != 1000 {
		t.Fatalf("老数据全 0 时新书签权值应为 1000，得到 %v", fresh.SortKey)
	}

	want := []uint{legacy[0].ID, legacy[1].ID, legacy[2].ID, fresh.ID}
	if got := orderOf(t, db, user); !reflect.DeepEqual(got, want) {
		t.Fatalf("升级后顺序应保持老数据在前、新书签在末尾（%v），得到 %v", want, got)
	}
}

// 全量重置：ids 顺序即新顺序，权值 1000*(i+1)
func TestScrapeBookmarkReorderWeights(t *testing.T) {
	db := newScrapeBookmarkTestDB(t)
	user := uint(1)

	a, _ := CreateScrapeBookmark(db, user, validInput())
	b, _ := CreateScrapeBookmark(db, user, validInput())
	c, _ := CreateScrapeBookmark(db, user, validInput())

	if err := ReorderScrapeBookmarks(db, user, []uint{c.ID, a.ID, b.ID}); err != nil {
		t.Fatalf("全量重置失败: %v", err)
	}
	if got := orderOf(t, db, user); !reflect.DeepEqual(got, []uint{c.ID, a.ID, b.ID}) {
		t.Fatalf("重置后顺序应为 [c a b]，得到 %v", got)
	}
	items, _ := ListScrapeBookmarks(db, user)
	wantKeys := map[uint]float64{c.ID: 1000, a.ID: 2000, b.ID: 3000}
	for _, it := range items {
		if it.SortKey != wantKeys[it.ID] {
			t.Fatalf("书签 %d 权值应为 %v，得到 %v", it.ID, wantKeys[it.ID], it.SortKey)
		}
	}

	// 越权 id 静默跳过：他人书签权值不受影响
	other, _ := CreateScrapeBookmark(db, uint(2), validInput())
	if err := ReorderScrapeBookmarks(db, user, []uint{other.ID}); err != nil {
		t.Fatalf("含越权 id 的整理不应报错: %v", err)
	}
	otherItems, _ := ListScrapeBookmarks(db, uint(2))
	if len(otherItems) != 1 || otherItems[0].SortKey != other.SortKey {
		t.Fatalf("他人书签权值被串改: %+v", otherItems)
	}
}
