package services

import (
	"testing"

	"SakuHentai/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"

	"github.com/glebarez/sqlite"
)

// 端到端验证：EditComicTags 叉除后，详情响应组装逻辑能返回 offlineRemoveTagsList（Round23 置灰恢复的前置数据）
func TestRound23TagRemoveRestoreE2E(t *testing.T) {
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

	comic := models.OfflineComic{
		ID:        "test-id-1",
		Title:     "测试漫画",
		OnlineTags: `["parody:touhou project","female:ahegao"]`,
		Tags:      `["parody:touhou project","female:ahegao"]`,
		PageCount: 10,
	}
	if err := db.Create(&comic).Error; err != nil {
		t.Fatal(err)
	}

	svc := &TagMaintainService{db: db}

	// 1. 叉除 parody:touhou project
	if err := svc.EditComicTags("test-id-1", nil, []string{"parody:touhou project"}); err != nil {
		t.Fatal(err)
	}

	// 2. 重读 DB，验证落库
	var after models.OfflineComic
	if err := db.First(&after, "id = ?", "test-id-1").Error; err != nil {
		t.Fatal(err)
	}
	removeList := UnmarshalTagSlice(after.OfflineRemoveTags)
	if len(removeList) != 1 || removeList[0] != "parody:touhou project" {
		t.Fatalf("offline_remove_tags 落库不符: %v", removeList)
	}
	t.Logf("落库 offlineRemoveTags=%v", removeList)

	// 3. 模拟详情响应组装（与 GetOfflineComicDetail 一致）
	online := UnmarshalTagSlice(after.OnlineTags)
	add := UnmarshalTagSlice(after.OfflineAddTags)
	remove := UnmarshalTagSlice(after.OfflineRemoveTags)
	merged := MergeTags(online, add, remove)
	if contains(merged, "parody:touhou project") {
		t.Fatal("合并列表不应包含被叉除 tag（搜索过滤依据）")
	}
	if !contains(online, "parody:touhou project") {
		t.Fatal("onlineTagsList 应包含被叉除 tag（前端置灰展示依据）")
	}
	t.Logf("onlineTagsList=%v / offlineRemoveTagsList=%v / merged=%v", online, remove, merged)

	// 4. 恢复（addTags 命中 remove → 剔除）
	if err := svc.EditComicTags("test-id-1", []string{"parody:touhou project"}, nil); err != nil {
		t.Fatal(err)
	}
	var restored models.OfflineComic
	if err := db.First(&restored, "id = ?", "test-id-1").Error; err != nil {
		t.Fatal(err)
	}
	if len(UnmarshalTagSlice(restored.OfflineRemoveTags)) != 0 {
		t.Fatalf("恢复后 offline_remove_tags 应为空: %v", UnmarshalTagSlice(restored.OfflineRemoveTags))
	}
	if !contains(UnmarshalTagSlice(restored.OnlineTags), "parody:touhou project") {
		t.Fatal("onlineTags 应保持完整")
	}
	t.Log("恢复链路 OK")
}
