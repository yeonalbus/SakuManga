package services

import (
	"testing"

	"SakuManga/internal/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// Round49：老书签字段回填（迁移）单测——验证 role/bulkDone 初值与 invalid 清理，
// 并确认二次执行不再覆盖（幂等）。
func TestMigrateScrapeBookmarkFields(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("打开内存数据库失败: %v", err)
	}
	if err := db.AutoMigrate(&models.ScrapeBookmark{}); err != nil {
		t.Fatalf("建表失败: %v", err)
	}

	rows := []models.ScrapeBookmark{
		{UserID: 1, Name: "artist:煌野一人 最旧", Anchor: `{"gid":"111","invalid":{"kind":"unreachable","at":1}}`},
		{UserID: 1, Name: "artist:煌野一人 最新", Anchor: `{"gid":"222"}`},
		{UserID: 1, Name: "已下载完", Anchor: `{"gid":"333"}`},
		{UserID: 1, Name: "默认标记", Anchor: `null`},
	}
	for i := range rows {
		if err := db.Create(&rows[i]).Error; err != nil {
			t.Fatalf("插入书签失败: %v", err)
		}
	}

	if n := MigrateScrapeBookmarkFields(db); n != len(rows) {
		t.Fatalf("迁移条数 = %d，期望 %d", n, len(rows))
	}

	type want struct {
		role     string
		bulkDone bool
	}
	wants := []want{
		{"oldest", false},
		{"newest", false},
		{"newest", true},
		{"interest", false},
	}

	var got []models.ScrapeBookmark
	if err := db.Order("id ASC").Find(&got).Error; err != nil {
		t.Fatalf("读取迁移结果失败: %v", err)
	}
	for i := range got {
		if got[i].Role != wants[i].role || got[i].BulkDone != wants[i].bulkDone {
			t.Errorf("第 %d 条：role=%s bulkDone=%v，期望 %s/%v",
				i+1, got[i].Role, got[i].BulkDone, wants[i].role, wants[i].bulkDone)
		}
		if !got[i].FieldsMigrated {
			t.Errorf("第 %d 条：FieldsMigrated 未置位", i+1)
		}
	}

	// invalid 标记应被清除（其余键保留）
	if got[0].Anchor != `{"gid":"111"}` {
		t.Errorf("anchor 清理结果 = %q，期望 %q", got[0].Anchor, `{"gid":"111"}`)
	}
	// 无 invalid 的锚点原样保留
	if got[1].Anchor != `{"gid":"222"}` {
		t.Errorf("不应改动的 anchor = %q", got[1].Anchor)
	}

	// Round49：旧 name 一律清空（信息已提取进 role / bulkDone），并置 NameReset
	for i := range got {
		if got[i].Name != "" {
			t.Errorf("第 %d 条：旧 name 未清空（%q）", i+1, got[i].Name)
		}
		if !got[i].NameReset {
			t.Errorf("第 %d 条：NameReset 未置位", i+1)
		}
	}

	// 幂等：二次迁移不处理任何记录
	if n := MigrateScrapeBookmarkFields(db); n != 0 {
		t.Errorf("二次迁移应返回 0，实际 %d", n)
	}
}
