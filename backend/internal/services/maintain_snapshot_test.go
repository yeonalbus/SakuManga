package services

import (
	"testing"

	"SakuManga/internal/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// ─────────────────────────────────────────────────────────────
// Round42 D6：维护查重结果快照（持久化）单测
// ─────────────────────────────────────────────────────────────

func newSnapshotTestDB(t *testing.T) *gorm.DB {
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
	if err := db.AutoMigrate(&models.MaintainDedupSnapshot{}); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	return db
}

func TestMaintainSnapshotRoundTrip(t *testing.T) {
	db := newSnapshotTestDB(t)
	res := &DedupResult{
		Items: []DedupItem{
			{Comic: models.OfflineComic{ID: "c1", Title: "样本"}, Reason: "同 GID", Keep: true, Rule: "gid"},
		},
		Clusters: []DedupCluster{
			{ID: "cluster-0", TitleKey: "key", Artist: "abc", Confidence: "high", Reason: "r"},
		},
		FinishedAt: 111,
		Stale:      true,
	}
	if err := SaveMaintainSnapshot(db, res, true); err != nil {
		t.Fatalf("保存快照失败: %v", err)
	}
	got, ok, err := LoadMaintainSnapshot(db)
	if err != nil || !ok {
		t.Fatalf("读取快照失败: ok=%v err=%v", ok, err)
	}
	if len(got.Items) != 1 || got.Items[0].Comic.ID != "c1" {
		t.Errorf("items 往返不一致: %+v", got.Items)
	}
	if len(got.Clusters) != 1 || got.Clusters[0].TitleKey != "key" {
		t.Errorf("clusters 往返不一致: %+v", got.Clusters)
	}
	if got.FinishedAt != 111 || !got.Stale {
		t.Errorf("finishedAt/stale 往返不一致: %d/%v", got.FinishedAt, got.Stale)
	}
	// forceFull 是未导出字段（不参与 JSON）→ 必须由快照列还原
	if !got.forceFull {
		t.Error("forceFull 应从快照列还原为 true")
	}
}

func TestMaintainSnapshotOverwriteSingleRow(t *testing.T) {
	db := newSnapshotTestDB(t)
	for i := 0; i < 3; i++ {
		if err := SaveMaintainSnapshot(db, &DedupResult{Items: []DedupItem{}, FinishedAt: int64(i)}, false); err != nil {
			t.Fatalf("第 %d 次保存失败: %v", i, err)
		}
	}
	var count int64
	if err := db.Model(&models.MaintainDedupSnapshot{}).Count(&count).Error; err != nil {
		t.Fatalf("统计失败: %v", err)
	}
	if count != 1 {
		t.Errorf("应为单行覆盖写，实际 %d 行", count)
	}
	got, ok, _ := LoadMaintainSnapshot(db)
	if !ok || got.FinishedAt != 2 {
		t.Errorf("应读到最后一次写入，得到 %+v", got)
	}
	// 传 nil → 删除快照
	if err := SaveMaintainSnapshot(db, nil, false); err != nil {
		t.Fatalf("删除快照失败: %v", err)
	}
	if _, ok, _ := LoadMaintainSnapshot(db); ok {
		t.Error("删除后不应再读到快照")
	}
}

func TestEnsureMaintainResultLoadedBackfill(t *testing.T) {
	db := newSnapshotTestDB(t)
	// 清空内存缓存（模拟进程重启）
	offlineTaskMu.Lock()
	offlineMaintainRes = nil
	offlineTaskMu.Unlock()

	res := &DedupResult{
		Items:      []DedupItem{},
		Clusters:   []DedupCluster{{ID: "cluster-0", TitleKey: "k", Confidence: "high"}},
		FinishedAt: 999,
	}
	if err := SaveMaintainSnapshot(db, res, false); err != nil {
		t.Fatalf("保存快照失败: %v", err)
	}

	if !EnsureMaintainResultLoaded(db) {
		t.Fatal("内存为空时应从快照回填")
	}
	got := GetMaintainDedupResult()
	if got == nil || len(got.Clusters) != 1 || got.FinishedAt != 999 {
		t.Errorf("回填结果不符: %+v", got)
	}
	// 再次调用不应重复回填（内存已有结果）
	if EnsureMaintainResultLoaded(db) {
		t.Error("内存已有结果时不应再回填")
	}
	// 清理，避免影响其它用例
	offlineTaskMu.Lock()
	offlineMaintainRes = nil
	offlineTaskMu.Unlock()
}

func TestEnsureMaintainResultLoadedNoSnapshot(t *testing.T) {
	db := newSnapshotTestDB(t)
	offlineTaskMu.Lock()
	offlineMaintainRes = nil
	offlineTaskMu.Unlock()
	if EnsureMaintainResultLoaded(db) {
		t.Error("无快照时不应回填")
	}
	if GetMaintainDedupResult() != nil {
		t.Error("无快照时结果应仍为 nil")
	}
}

func TestPublishLocalDedupSnapshot(t *testing.T) {
	// 本地判重先行：产出疑似重复簇并入缓存（不依赖联网）
	comics := []models.OfflineComic{
		mkComicFull("p1", "[夢ねこ屋 (むーにゃん)] 極東絢爛賭博島ドリームアイランド3 [中国翻訳] [DL版]", "",
			[]string{"artist:muunyan"}, 112),
		mkComicFull("p2", "[夢ねこ屋 (むーにゃん)] 極東絢爛賭博島ドリームアイランド3 [中国翻訳]", "",
			[]string{"artist:muunyan"}, 112),
	}
	var phases []string
	sink := func(done, total int, title, phase string) { phases = append(phases, phase) }
	n := PublishLocalDedupSnapshot(comics, nil, false, sink)
	if n != 1 {
		t.Errorf("应产出 1 组疑似重复，得到 %d", n)
	}
	got := GetMaintainDedupResult()
	if got == nil || len(got.Clusters) != 1 {
		t.Fatalf("应已发布结果到缓存，得到 %+v", got)
	}
	if len(got.Items) != 0 {
		t.Errorf("本地阶段不产出规则项（items 应为空），得到 %d 项", len(got.Items))
	}
	if len(phases) != 2 {
		t.Errorf("应上报两阶段进度（开始/完成），得到 %v", phases)
	}
	// 清理
	offlineTaskMu.Lock()
	offlineMaintainRes = nil
	offlineTaskMu.Unlock()
}
