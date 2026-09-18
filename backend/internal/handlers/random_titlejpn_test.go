package handlers

import (
	"reflect"
	"testing"

	"SakuManga/internal/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// Round41-Bug（离线搜索结果条数不全）：离线抽卡关键词须能命中日文原名（title_jpn）。
// 背景：metadata 入库时 title 常为罗马音、title_jpn 才是日文原名，而前端卡片主标题展示的是
// titleJpn（ItemCard：displayTitle = titleJpn || title）——只匹配 title 会让「按看得见的标题」
// 抽不到卡。同一根因的前端修复见 src/views/offline/OfflineHome.vue 的 comicTitleTexts。
func newRandomTitleJpnTestDB(t *testing.T) *gorm.DB {
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
	if err := db.AutoMigrate(&models.OfflineComic{}); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	return db
}

func seedOfflineComic(t *testing.T, db *gorm.DB, id, title, titleJpn, tags string) {
	t.Helper()
	c := models.OfflineComic{
		ID:        id,
		Title:     title,
		TitleJpn:  titleJpn,
		Tags:      tags,
		LocalPath: `Z:\test\` + id,
	}
	if err := db.Create(&c).Error; err != nil {
		t.Fatalf("插入离线漫画失败: %v", err)
	}
}

func TestBuildOfflineQueryMatchesTitleJpn(t *testing.T) {
	db := newRandomTitleJpnTestDB(t)
	// a：title 罗马音 + title_jpn 日文（此前抽不到）
	seedOfflineComic(t, db, "a",
		"[Yumenekoya (Muunyan)] Kyokutou Kenran Tobakutou Dream Island 2",
		"[夢ねこ屋 (むーにゃん)] 極東絢爛賭博島ドリームアイランド2 スカサハ&ナイチンゲール編",
		`["artist:muunyan"]`)
	// b：title 本身就是日文（此前能抽到）
	seedOfflineComic(t, db, "b",
		"[夢ねこ屋 (むーにゃん)] 極東絢爛賭博島ドリームアイランドー宮本武蔵編",
		"[夢ねこ屋 (むーにゃん)] 極東絢爛賭博島ドリームアイランドー宮本武蔵編", "")
	// c：完全无关（负向测试中应保留）
	seedOfflineComic(t, db, "c", "[Other] Unrelated Work", "", "")
	// d：title_jpn 为 SQL NULL 的老数据行（负向关键词不得因 NULL 误排整行）
	seedOfflineComic(t, db, "d", "[Legacy] No Jpn Title", "placeholder", "")
	if err := db.Exec("UPDATE offline_comics SET title_jpn = NULL WHERE id = 'd'").Error; err != nil {
		t.Fatalf("置 NULL 失败: %v", err)
	}

	idsOf := func(f offlineFilter) []string {
		t.Helper()
		var got []models.OfflineComic
		if err := buildOfflineQuery(db, f).Order("id").Find(&got).Error; err != nil {
			t.Fatalf("查询失败: %v", err)
		}
		ids := make([]string, 0, len(got))
		for _, c := range got {
			ids = append(ids, c.ID)
		}
		return ids
	}

	cases := []struct {
		name string
		f    offlineFilter
		want []string
	}{
		{
			name: "单关键词命中 title_jpn（罗马音 title 也能搜到）",
			f:    offlineFilter{keyword: "極東絢爛賭博島ドリームアイランド"},
			want: []string{"a", "b"},
		},
		{
			name: "词条关键词按 AND 语义匹配 title_jpn",
			f:    offlineFilter{keywords: []string{"ドリームアイランド", "宮本武蔵編"}},
			want: []string{"b"},
		},
		{
			name: "负向关键词含 title_jpn，且 NULL 行不误排",
			f:    offlineFilter{excludeKeywords: []string{"ドリームアイランド"}},
			want: []string{"c", "d"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := idsOf(tc.f); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("命中 %v，期望 %v", got, tc.want)
			}
		})
	}
}
