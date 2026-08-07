package services

import (
	"testing"

	"SakuHentai/internal/models"
)

// ─────────────────────────────────────────────────────────────
// 需求 2：下载完成消除更新标记（ClearOfflineUpdateByGID）
//
// 覆盖两种场景：
//   - 漫画自身 gid = 下载 gid（用户手动下载标记需更新的漫画本体）
//   - 漫画 new_gid = 下载 gid（用户下载检测到的新版本，父画廊标记一并消除）
// 以及：无更新标记 / gid 不匹配的漫画不受影响。
// ─────────────────────────────────────────────────────────────

func TestClearOfflineUpdateByGID(t *testing.T) {
	mgr := newTestDownloadManager(t)
	if err := mgr.db.AutoMigrate(&models.OfflineComic{}); err != nil {
		t.Fatalf("迁移 OfflineComic 失败: %v", err)
	}

	// 场景 A：漫画本体 gid=100 被标记需要更新（新版本 gid=101）
	self := &models.OfflineComic{
		ID:          "offline-clear-self",
		GID:         "100",
		Title:       "self",
		LocalPath:   "/x/100",
		NeedsUpdate: true,
		NewGID:      "101",
		NewToken:    "t1",
		UpdateNote:  "检测到更新版本：最新版 gid=101",
	}
	// 场景 B：父画廊 gid=200，检测到新版 gid=101（new_gid 匹配下载 gid）
	parent := &models.OfflineComic{
		ID:          "offline-clear-parent",
		GID:         "200",
		Title:       "parent",
		LocalPath:   "/x/200",
		NeedsUpdate: true,
		NewGID:      "101",
		NewToken:    "t1",
		UpdateNote:  "检测到更新版本：最新版 gid=101",
	}
	// 场景 C：无更新标记（needs_update=false），不应被触碰
	clean := &models.OfflineComic{
		ID:        "offline-clear-clean",
		GID:       "300",
		Title:     "clean",
		LocalPath: "/x/300",
	}
	// 场景 D：gid 与新 gid 均不匹配下载 gid，不应被触碰
	other := &models.OfflineComic{
		ID:          "offline-clear-other",
		GID:         "400",
		Title:       "other",
		LocalPath:   "/x/400",
		NeedsUpdate: true,
		NewGID:      "999",
		NewToken:    "t9",
		UpdateNote:  "检测到更新版本：最新版 gid=999",
	}

	for _, c := range []*models.OfflineComic{self, parent, clean, other} {
		if err := mgr.db.Create(c).Error; err != nil {
			t.Fatalf("创建漫画 %q 失败: %v", c.Title, err)
		}
	}

	// 阶段 1：下载完成 gid=100（漫画本体）→ 仅清除 gid=100 的标记（场景 A）
	n1, err := ClearOfflineUpdateByGID(mgr.db, "100")
	if err != nil {
		t.Fatalf("ClearOfflineUpdateByGID(100) 失败: %v", err)
	}
	if n1 != 1 {
		t.Fatalf("阶段1 应清除 1 条，得到 %d", n1)
	}

	var selfGot models.OfflineComic
	if err := mgr.db.First(&selfGot, "id = ?", "offline-clear-self").Error; err != nil {
		t.Fatalf("读取 self 失败: %v", err)
	}
	if selfGot.NeedsUpdate || selfGot.NewGID != "" || selfGot.NewToken != "" || selfGot.UpdateNote != "" {
		t.Errorf("self 更新标记应被全部清除，得到 needsUpdate=%v newGID=%q note=%q",
			selfGot.NeedsUpdate, selfGot.NewGID, selfGot.UpdateNote)
	}

	// parent 阶段1 不应被清除（gid=200 ≠ 100）
	var parentGot models.OfflineComic
	if err := mgr.db.First(&parentGot, "id = ?", "offline-clear-parent").Error; err != nil {
		t.Fatalf("读取 parent 失败: %v", err)
	}
	if !parentGot.NeedsUpdate {
		t.Errorf("parent 不应被阶段1 清除，needsUpdate=%v", parentGot.NeedsUpdate)
	}

	// 阶段 2：下载完成 gid=101（检测到的新版本）→ 清除 new_gid=101 的父画廊标记（场景 B）
	n2, err := ClearOfflineUpdateByGID(mgr.db, "101")
	if err != nil {
		t.Fatalf("ClearOfflineUpdateByGID(101) 失败: %v", err)
	}
	if n2 != 1 {
		t.Fatalf("阶段2 应清除 1 条（parent），得到 %d", n2)
	}
	if err := mgr.db.First(&parentGot, "id = ?", "offline-clear-parent").Error; err != nil {
		t.Fatalf("读取 parent 失败: %v", err)
	}
	if parentGot.NeedsUpdate || parentGot.NewGID != "" || parentGot.NewToken != "" || parentGot.UpdateNote != "" {
		t.Errorf("parent 更新标记应被清除，得到 needsUpdate=%v newGID=%q note=%q",
			parentGot.NeedsUpdate, parentGot.NewGID, parentGot.UpdateNote)
	}

	// clean / other 始终不受影响
	for _, id := range []string{"offline-clear-clean", "offline-clear-other"} {
		var got models.OfflineComic
		if err := mgr.db.First(&got, "id = ?", id).Error; err != nil {
			t.Fatalf("读取 %s 失败: %v", id, err)
		}
		if id == "offline-clear-clean" && got.NeedsUpdate {
			t.Errorf("clean（无标记）不应被清除")
		}
		if id == "offline-clear-other" && (!got.NeedsUpdate || got.NewGID != "999") {
			t.Errorf("other（gid 不匹配）不应被清除，得到 needsUpdate=%v newGID=%q", got.NeedsUpdate, got.NewGID)
		}
	}
}

// TestClearOfflineUpdateByGIDEmptyGID 验证空 gid 直接返回，不产生副作用。
func TestClearOfflineUpdateByGIDEmptyGID(t *testing.T) {
	mgr := newTestDownloadManager(t)
	if err := mgr.db.AutoMigrate(&models.OfflineComic{}); err != nil {
		t.Fatalf("迁移 OfflineComic 失败: %v", err)
	}
	n, err := ClearOfflineUpdateByGID(mgr.db, "")
	if err != nil {
		t.Fatalf("空 gid 不应报错: %v", err)
	}
	if n != 0 {
		t.Fatalf("空 gid 应清除 0 条，得到 %d", n)
	}
}
