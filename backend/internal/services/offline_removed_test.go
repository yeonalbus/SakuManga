package services

import (
	"errors"
	"testing"

	"SakuHentai/internal/models"

	"gorm.io/gorm"
)

// ─────────────────────────────────────────────────────────────
// 需求 3(2)：画廊被删除/移除（RemovedStatus）持久化与过滤
//
// 覆盖：
//   - filterOfflineUpdateEnabled 对 RemovedStatus=true 的漫画过滤
//     （该函数被 checkUpdatesWithProgress / ageCheckWithProgress /
//     maintainDedupWithProgress 三处复用，一处过滤覆盖全部后续联网扫描）
//   - ClearOfflineUpdateByComicID 清除更新标记但保留 RemovedStatus
//   - 空 comicID / 记录不存在等边界
// ─────────────────────────────────────────────────────────────

func TestFilterOfflineUpdateEnabledSkipsRemoved(t *testing.T) {
	mgr := newTestDownloadManager(t)
	if err := mgr.db.AutoMigrate(&models.OfflineComic{}, &models.ExtraScanPath{}); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}

	// 额外路径 disabled：其中的漫画不参与（保持原有语义）
	disabledPath := &models.ExtraScanPath{
		ID:                  "path-disabled",
		Path:                "D:/disabled",
		EnableOfflineUpdate: false,
	}
	// 额外路径 enabled：其中的漫画参与
	enabledPath := &models.ExtraScanPath{
		ID:                  "path-enabled",
		Path:                "D:/enabled",
		EnableOfflineUpdate: true,
	}
	for _, p := range []*models.ExtraScanPath{disabledPath, enabledPath} {
		if err := mgr.db.Create(p).Error; err != nil {
			t.Fatalf("创建路径失败: %v", err)
		}
	}

	comics := []models.OfflineComic{
		// 普通下载导入漫画（ScanPathID 空）：始终参与
		{ID: "removed-normal", GID: "100", Title: "normal", LocalPath: "/x/100"},
		// 已老化：应被过滤（原有语义）
		{ID: "removed-aged", GID: "200", Title: "aged", LocalPath: "/x/200", AgedStatus: true},
		// RemovedStatus=true：应被过滤（需求 3(2)）
		{ID: "removed-gone", GID: "300", Title: "gone", LocalPath: "/x/300", RemovedStatus: true, RemovedAt: 1234567890000},
		// RemovedStatus + 额外路径 disabled：应被过滤（两者任一命中即排除）
		{ID: "removed-disabled", GID: "400", Title: "disabled", LocalPath: "/x/400", ScanPathID: "path-disabled", RemovedStatus: true},
		// RemovedStatus=false + 额外路径 enabled：应保留
		{ID: "removed-enabled", GID: "500", Title: "enabled", LocalPath: "/x/500", ScanPathID: "path-enabled"},
	}

	got := filterOfflineUpdateEnabled(mgr.db, comics)

	if len(got) != 2 {
		t.Fatalf("应保留 2 条，得到 %d 条: %+v", len(got), got)
	}
	seen := make(map[string]bool, len(got))
	for _, c := range got {
		seen[c.ID] = true
	}
	for _, id := range []string{"removed-normal", "removed-enabled"} {
		if !seen[id] {
			t.Errorf("期望保留 %s，实际被过滤", id)
		}
	}
	for _, id := range []string{"removed-aged", "removed-gone", "removed-disabled"} {
		if seen[id] {
			t.Errorf("%s 应被过滤（aged/removed/路径关闭），实际被保留", id)
		}
	}
}

// TestFilterOfflineUpdateEnabledAllRemoved 验证全部漫画均为 RemovedStatus 时返回空切片。
func TestFilterOfflineUpdateEnabledAllRemoved(t *testing.T) {
	mgr := newTestDownloadManager(t)
	if err := mgr.db.AutoMigrate(&models.OfflineComic{}, &models.ExtraScanPath{}); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	comics := []models.OfflineComic{
		{ID: "r1", GID: "100", LocalPath: "/x/100", RemovedStatus: true},
		{ID: "r2", GID: "200", LocalPath: "/x/200", RemovedStatus: true},
	}
	got := filterOfflineUpdateEnabled(mgr.db, comics)
	if len(got) != 0 {
		t.Fatalf("应返回空切片，得到 %d 条", len(got))
	}
}

// TestClearOfflineUpdateByComicID 验证清除更新标记时保留 RemovedStatus。
func TestClearOfflineUpdateByComicID(t *testing.T) {
	mgr := newTestDownloadManager(t)
	if err := mgr.db.AutoMigrate(&models.OfflineComic{}); err != nil {
		t.Fatalf("迁移 OfflineComic 失败: %v", err)
	}

	// 场景 A：被标记删除的漫画同时带更新标记（此前 needs_update=true 残留）→ 清除更新标记，保留 RemovedStatus
	removed := &models.OfflineComic{
		ID:            "offline-removed-a",
		GID:           "100",
		Title:         "removed",
		LocalPath:     "/x/100",
		NeedsUpdate:   true,
		NewGID:        "101",
		NewToken:      "t1",
		UpdateNote:    "检测到更新版本：最新版 gid=101",
		RemovedStatus: true,
		RemovedAt:     1234567890000,
	}
	// 场景 B：无更新标记的漫画 → 返回 false，不触碰
	clean := &models.OfflineComic{
		ID:        "offline-removed-b",
		GID:       "200",
		Title:     "clean",
		LocalPath: "/x/200",
	}
	for _, c := range []*models.OfflineComic{removed, clean} {
		if err := mgr.db.Create(c).Error; err != nil {
			t.Fatalf("创建漫画 %q 失败: %v", c.Title, err)
		}
	}

	// 场景 A：清除成功，返回 true
	cleared, err := ClearOfflineUpdateByComicID(mgr.db, "offline-removed-a")
	if err != nil {
		t.Fatalf("ClearOfflineUpdateByComicID(A) 失败: %v", err)
	}
	if !cleared {
		t.Fatalf("场景 A 应返回 cleared=true")
	}
	var gotA models.OfflineComic
	if err := mgr.db.First(&gotA, "id = ?", "offline-removed-a").Error; err != nil {
		t.Fatalf("读取 A 失败: %v", err)
	}
	if gotA.NeedsUpdate || gotA.NewGID != "" || gotA.NewToken != "" || gotA.UpdateNote != "" {
		t.Errorf("更新标记应被全部清除，得到 needsUpdate=%v newGID=%q note=%q",
			gotA.NeedsUpdate, gotA.NewGID, gotA.UpdateNote)
	}
	if !gotA.RemovedStatus {
		t.Errorf("RemovedStatus 应保留（避免后续联网扫描），实际被清除")
	}
	if gotA.RemovedAt != removed.RemovedAt {
		t.Errorf("RemovedAt 应保留 %d，得到 %d", removed.RemovedAt, gotA.RemovedAt)
	}

	// 场景 B：无更新标记 → 返回 false
	clearedB, err := ClearOfflineUpdateByComicID(mgr.db, "offline-removed-b")
	if err != nil {
		t.Fatalf("ClearOfflineUpdateByComicID(B) 失败: %v", err)
	}
	if clearedB {
		t.Fatalf("场景 B 无更新标记，应返回 cleared=false")
	}
}

// TestClearOfflineUpdateByComicIDEdge 验证边界：空 comicID / 记录不存在 / nil db。
func TestClearOfflineUpdateByComicIDEdge(t *testing.T) {
	mgr := newTestDownloadManager(t)
	if err := mgr.db.AutoMigrate(&models.OfflineComic{}); err != nil {
		t.Fatalf("迁移 OfflineComic 失败: %v", err)
	}

	// 空 comicID：直接返回 false, nil
	cleared, err := ClearOfflineUpdateByComicID(mgr.db, "")
	if err != nil {
		t.Fatalf("空 comicID 不应报错: %v", err)
	}
	if cleared {
		t.Fatalf("空 comicID 应返回 cleared=false")
	}

	// 记录不存在：返回 gorm.ErrRecordNotFound（handler 据此回 404）
	if _, err := ClearOfflineUpdateByComicID(mgr.db, "no-such-id"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("记录不存在应返回 ErrRecordNotFound，得到 %v", err)
	}

	// nil db：返回 false, nil
	cleared, err = ClearOfflineUpdateByComicID(nil, "whatever")
	if err != nil {
		t.Fatalf("nil db 不应报错: %v", err)
	}
	if cleared {
		t.Fatalf("nil db 应返回 cleared=false")
	}
}
