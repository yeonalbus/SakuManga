package services

import (
	"testing"

	"SakuManga/internal/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

func newTagRepairDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
		NamingStrategy: schema.NamingStrategy{
			SingularTable: true, // 与生产库保持一致（offline_comics 表名）
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.OfflineComic{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestStripTagAliasSuffix(t *testing.T) {
	cases := []struct{ in, want string }{
		{"language:japanese | definition revised please read the wiki", "language:japanese"},
		{"character:cure answer | anna akechi", "character:cure answer"},
		{"parody:pokemon", "parody:pokemon"},
		{"  parody:touhou project  ", "  parody:touhou project  "},
		{"| only right", "| only right"},
		{"", ""},
	}
	for _, c := range cases {
		if got := stripTagAliasSuffix(c.in); got != c.want {
			t.Errorf("stripTagAliasSuffix(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestRepairTagAliasSuffix(t *testing.T) {
	db := newTagRepairDB(t)

	dirty := models.OfflineComic{
		ID:         "c1",
		Title:      "脏数据",
		LocalPath:  "Z:\\dirty",
		OnlineTags: `["language:japanese | definition revised please read the wiki","parody:pokemon | pocket monsters","female:ahegao"]`,
		Tags:       `["language:japanese | definition revised please read the wiki","female:ahegao"]`,
	}
	clean := models.OfflineComic{
		ID:         "c2",
		Title:      "干净数据",
		LocalPath:  "Z:\\clean",
		OnlineTags: `["artist:tiler","female:halo"]`,
		Tags:       `["artist:tiler"]`,
	}
	if err := db.Create(&dirty).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&clean).Error; err != nil {
		t.Fatal(err)
	}

	comics, tags, err := repairTagAliasSuffix(db)
	if err != nil {
		t.Fatal(err)
	}
	if comics != 1 {
		t.Fatalf("应只修复 1 个漫画，实际 %d", comics)
	}
	if tags != 3 {
		t.Fatalf("应修复 3 个 tag（online 2 + 旧 tags 1），实际 %d", tags)
	}

	var got models.OfflineComic
	if err := db.First(&got, "id = ?", "c1").Error; err != nil {
		t.Fatal(err)
	}
	wantOnline := `["language:japanese","parody:pokemon","female:ahegao"]`
	if got.OnlineTags != wantOnline {
		t.Fatalf("online_tags 清洗结果不符\n got: %s\nwant: %s", got.OnlineTags, wantOnline)
	}
	wantTags := `["language:japanese","female:ahegao"]`
	if got.Tags != wantTags {
		t.Fatalf("tags 清洗结果不符\n got: %s\nwant: %s", got.Tags, wantTags)
	}

	// 幂等：再跑一次不应再产生变更
	comics2, tags2, err := repairTagAliasSuffix(db)
	if err != nil {
		t.Fatal(err)
	}
	if comics2 != 0 || tags2 != 0 {
		t.Fatalf("二次执行应无变更，实际 comics=%d tags=%d", comics2, tags2)
	}
}
