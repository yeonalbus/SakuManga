package services

import (
	"testing"

	"SakuManga/internal/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// ─────────────────────────────────────────────────────────────
// Round33：维护查重结果缓存的定向同步
// （忽略 / 恢复 / 删除后即时更新列表，无需整份重新扫描）
//
// 覆盖：
//   - title 型忽略 → 缓存中的疑似重复簇即时消失；恢复 → 即时回归
//   - comic 型忽略 → 簇成员收缩 / <2 成员时簇消失
//   - gid 型忽略 → 增量语义下「父画廊更新提示」即时移除（全量语义不豁免）
//   - 删除 → 已删项与其配对项一并移出，且定向同步后清除 stale 标记
// ─────────────────────────────────────────────────────────────

// newSyncTestDB 构造内存库并迁移三张相关表
func newSyncTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("打开内存库失败: %v", err)
	}
	if err := db.AutoMigrate(
		&models.OfflineComic{},
		&models.ExtraScanPath{},
		&models.IgnoredIdentifier{},
	); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	return db
}

// resetMaintainResultForTest 清理全局结果缓存，避免用例间互相污染
func resetMaintainResultForTest(t *testing.T) {
	t.Helper()
	offlineTaskMu.Lock()
	offlineMaintainRes = nil
	offlineUpdateRes = nil
	offlineTaskMu.Unlock()
	t.Cleanup(func() {
		offlineTaskMu.Lock()
		offlineMaintainRes = nil
		offlineUpdateRes = nil
		offlineTaskMu.Unlock()
	})
}

// seedClusterComics 写入 A 组疑似重复（同名同画师、gid 各不相同、页数差 ≤5）
func seedClusterComics(t *testing.T, db *gorm.DB) []models.OfflineComic {
	t.Helper()
	comics := []models.OfflineComic{
		mkComic("a1", "[サークル] サンプルタイトル", []string{"artist:てすと", "language:japanese"}, 30),
		mkComic("a2", "[サークル] サンプルタイトル (English)", []string{"artist:てすと", "language:english"}, 31),
		mkComic("a3", "[サークル] サンプルタイトル 汉化版", []string{"artist:てすと", "language:chinese"}, 32),
	}
	for i := range comics {
		comics[i].Source = models.SourceOffline
		comics[i].GID = "sync-gid-" + comics[i].ID
		comics[i].LocalPath = "/nonexistent/" + comics[i].ID // 目录不存在 → 内容签名规则自动跳过
		if err := db.Create(&comics[i]).Error; err != nil {
			t.Fatalf("创建漫画失败: %v", err)
		}
	}
	return comics
}

// TestSyncMaintainDedupClustersTitleIgnore 忽略即时生效 + 恢复即时回归
func TestSyncMaintainDedupClustersTitleIgnore(t *testing.T) {
	resetMaintainResultForTest(t)
	db := newSyncTestDB(t)
	seedClusterComics(t, db)

	res, err := MaintainDedupWithProgress(db, nil, nil, false)
	if err != nil {
		t.Fatalf("增量维护查重失败: %v", err)
	}
	StoreMaintainDedupResult(res, false)
	if len(GetMaintainDedupResult().Clusters) != 1 {
		t.Fatalf("基线应有 1 个疑似重复簇，得到 %d", len(GetMaintainDedupResult().Clusters))
	}

	// 忽略整组（title 型）→ 缓存即时同步，无需重新扫描
	ig, err := CreateIgnore(db, "title", "サンプルタイトル", "てすと", "", "", "")
	if err != nil {
		t.Fatalf("创建忽略失败: %v", err)
	}
	MarkLibraryChanged() // 模拟书库变更（下载/删除）后结果「未同步」的状态
	SyncMaintainDedupClusters(db)
	if got := GetMaintainDedupResult(); len(got.Clusters) != 0 {
		t.Errorf("忽略后缓存应立即无簇，得到 %d 簇", len(got.Clusters))
	}
	if got := GetMaintainDedupResult(); got.Stale {
		t.Error("定向同步后不应残留 stale 标记（否则前端仍提示重新扫描）")
	}
	if HasUnsyncedLibraryChanges() {
		t.Error("定向同步后不应再存在「未反映变更」信号（否则进页面仍提示结果过期）")
	}

	// 恢复忽略 → 簇立即回归
	if err := RestoreIgnore(db, ig.ID); err != nil {
		t.Fatalf("恢复忽略失败: %v", err)
	}
	SyncMaintainDedupClusters(db)
	got := GetMaintainDedupResult()
	if len(got.Clusters) != 1 {
		t.Fatalf("恢复后缓存应立即回到 1 簇，得到 %d", len(got.Clusters))
	}
	if len(got.Clusters[0].Members) != 3 {
		t.Errorf("恢复后簇成员应为 3，得到 %d", len(got.Clusters[0].Members))
	}
}

// TestSyncMaintainDedupClustersComicIgnore 成员级忽略：簇收缩 → 消失
func TestSyncMaintainDedupClustersComicIgnore(t *testing.T) {
	resetMaintainResultForTest(t)
	db := newSyncTestDB(t)
	seedClusterComics(t, db)

	res, err := MaintainDedupWithProgress(db, nil, nil, false)
	if err != nil {
		t.Fatalf("增量维护查重失败: %v", err)
	}
	StoreMaintainDedupResult(res, false)

	// 忽略 1 个成员 → 簇收缩为 2
	if _, err := CreateIgnore(db, "comic", "", "", "", "a2", ""); err != nil {
		t.Fatalf("创建成员级忽略失败: %v", err)
	}
	SyncMaintainDedupClusters(db)
	got := GetMaintainDedupResult()
	if len(got.Clusters) != 1 || len(got.Clusters[0].Members) != 2 {
		t.Fatalf("忽略 1 个成员后应为 2 成员 1 簇，得到 %d 簇", len(got.Clusters))
	}
	for _, m := range got.Clusters[0].Members {
		if m.Comic.ID == "a2" {
			t.Error("被忽略成员 a2 不应出现在簇中")
		}
	}

	// 再忽略 1 个成员 → 剩余 1 成员 → 簇消失
	if _, err := CreateIgnore(db, "comic", "", "", "", "a1", ""); err != nil {
		t.Fatalf("创建成员级忽略失败: %v", err)
	}
	SyncMaintainDedupClusters(db)
	if got := GetMaintainDedupResult(); len(got.Clusters) != 0 {
		t.Errorf("组内剩余 1 成员不应成簇，得到 %d 簇", len(got.Clusters))
	}
}

// TestSyncMaintainDedupClustersGIDIgnore 规则 3 提示：增量语义即时移除、全量语义保留
func TestSyncMaintainDedupClustersGIDIgnore(t *testing.T) {
	resetMaintainResultForTest(t)
	db := newSyncTestDB(t)

	old := mkComic("p1", "[P] 旧版作品", []string{"artist:p"}, 20)
	old.Source = models.SourceOffline
	old.GID = "777"
	newer := mkComic("p2", "[P] 新版作品", []string{"artist:p"}, 20)
	newer.Source = models.SourceOffline
	newer.GID = "888"

	build := func() *DedupResult {
		return &DedupResult{Items: []DedupItem{
			{Comic: old, Reason: "旧版被新版取代，建议删除", Keep: false, Rule: "parent", PairComic: &newer},
			{Comic: newer, Reason: "建议保留", Keep: true, PairComic: &old},
		}}
	}

	// 忽略父画廊 gid（gid 型）
	if _, err := CreateIgnore(db, "gid", "", "", "777", "", ""); err != nil {
		t.Fatalf("创建 gid 型忽略失败: %v", err)
	}

	// 增量语义：命中忽略的「旧版可删除」提示即时移除
	StoreMaintainDedupResult(build(), false)
	SyncMaintainDedupClusters(db)
	got := GetMaintainDedupResult()
	if len(got.Items) != 1 {
		t.Fatalf("增量语义下应移除被忽略的父画廊提示（剩 1 项），得到 %d 项", len(got.Items))
	}
	if !got.Items[0].Keep {
		t.Error("剩余项应为「建议保留」项")
	}

	// 全量语义：忽略不豁免，提示仍列出
	StoreMaintainDedupResult(build(), true)
	SyncMaintainDedupClusters(db)
	if got := GetMaintainDedupResult(); len(got.Items) != 2 {
		t.Errorf("全量核对时不豁免 gid 忽略，应保留 2 项，得到 %d 项", len(got.Items))
	}
}

// TestInvalidateMaintainDedupResultPrunesPair 删除：已删项与其配对项一并移出
func TestInvalidateMaintainDedupResultPrunesPair(t *testing.T) {
	resetMaintainResultForTest(t)
	db := newSyncTestDB(t)

	dup := mkComic("d1", "[D] 同一作品 压缩包", []string{"artist:d"}, 20)
	dup.Source = models.SourceOffline
	keep := mkComic("d2", "[D] 同一作品 文件夹", []string{"artist:d"}, 20)
	keep.Source = models.SourceOffline
	other := mkComic("d3", "[D] 无关作品", []string{"artist:d"}, 20)
	other.Source = models.SourceOffline

	StoreMaintainDedupResult(&DedupResult{Items: []DedupItem{
		{Comic: dup, Reason: "同 GID 重复", Keep: false, Rule: "gid", PairComic: &keep},
		{Comic: keep, Reason: "建议保留", Keep: true, PairComic: &dup},
		{Comic: other, Reason: "同 GID 重复", Keep: false, Rule: "gid"},
	}}, false)

	InvalidateMaintainDedupResult([]string{"d1"})
	got := GetMaintainDedupResult()
	if !got.Stale {
		t.Error("删除后应标记 stale（供调用方随后定向同步清除）")
	}
	for _, it := range got.Items {
		if it.Comic.ID == "d1" {
			t.Error("已删除项 d1 不应残留在结果中")
		}
		if it.Comic.ID == "d2" {
			t.Error("配对项已被删除 → d2 应一并移出（避免孤儿「建议保留」）")
		}
	}
	if len(got.Items) != 1 || got.Items[0].Comic.ID != "d3" {
		t.Errorf("应仅保留无关项 d3，得到 %d 项", len(got.Items))
	}

	// 定向同步后 stale 清除，前端不再提示重新扫描
	SyncMaintainDedupClusters(db)
	if got := GetMaintainDedupResult(); got.Stale {
		t.Error("定向同步后应清除 stale 标记")
	}
}

// removeMemberForTest 复现 handler 的删除链路：删记录 → 缓存剪除 → 定向重算簇
func removeMemberForTest(t *testing.T, db *gorm.DB, id string) {
	t.Helper()
	if err := RemoveDedupComic(db, id, false); err != nil {
		t.Fatalf("删除漫画 %s 失败: %v", id, err)
	}
	InvalidateMaintainDedupResult([]string{id})
	SyncMaintainDedupClusters(db)
}

// TestSyncMaintainDedupClustersAfterMemberRemoval 删除簇内成员 → 簇收缩 / 剩 1 本即消失（Round43）
//
// 需求语义：在疑似重复区删掉某组内的画廊后——
//   - 该组仍有 ≥2 本 → 视为未处理完毕，继续显示（成员数收缩）
//   - 该组只剩 1 本 → 视为处理完毕，不再显示该类
func TestSyncMaintainDedupClustersAfterMemberRemoval(t *testing.T) {
	resetMaintainResultForTest(t)
	db := newSyncTestDB(t)
	seedClusterComics(t, db)

	res, err := MaintainDedupWithProgress(db, nil, nil, false)
	if err != nil {
		t.Fatalf("增量维护查重失败: %v", err)
	}
	StoreMaintainDedupResult(res, false)
	if got := GetMaintainDedupResult(); len(got.Clusters) != 1 || len(got.Clusters[0].Members) != 3 {
		t.Fatalf("基线应为 1 簇 3 成员，得到 %d 簇", len(got.Clusters))
	}

	// 删掉 1 本（保留本地文件）→ 组内仍 2 本，继续显示
	removeMemberForTest(t, db, "a1")
	got := GetMaintainDedupResult()
	if len(got.Clusters) != 1 || len(got.Clusters[0].Members) != 2 {
		t.Fatalf("删 1 本后应为 2 成员 1 簇，得到 %d 簇", len(got.Clusters))
	}
	for _, m := range got.Clusters[0].Members {
		if m.Comic.ID == "a1" {
			t.Error("已删除的 a1 不应残留在簇成员中")
		}
	}

	// 再删 1 本 → 组内只剩 1 本，不成组 → 该类不再显示
	removeMemberForTest(t, db, "a2")
	got = GetMaintainDedupResult()
	if len(got.Clusters) != 0 {
		t.Errorf("组内仅剩 1 本应视为处理完毕（不再显示该类），得到 %d 簇", len(got.Clusters))
	}
	if got.Stale {
		t.Error("定向同步后应清除 stale 标记（前端不提示重新扫描）")
	}
}
