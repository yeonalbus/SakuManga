package services

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"SakuManga/internal/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// ─────────────────────────────────────────────────────────────
// Round54：全量扫描失效对账单测
//
// 覆盖：
//   - 失效判定：文件存在/不存在/根路径整个缺失
//   - 分类：可恢复 → 候选；无 g_id/token → 阵亡 no-ehentai；removed_status → 阵亡 removed
//   - 自愈：文件重新存在 → 清除候选 / 未 dismissed 阵亡；dismissed 保留
//   - upsert：重复对账幂等、dismissed 状态跨对账保留
//   - 边界：pathExistsConservative 空路径/非 IsNotExist 错误
//   - 对账不修改 offline_comics
// ─────────────────────────────────────────────────────────────

// newRecoveryTestDB 构造带内存 sqlite 的对账测试库
func newRecoveryTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("打开内存库失败: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("获取底层连接失败: %v", err)
	}
	// 内存库固定单连接：glebarez/sqlite 的 :memory: 每连接独立
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&models.OfflineComic{}, &models.ExtraScanPath{},
		&models.RecoveryCandidate{}, &models.FallenComic{},
		&models.DownloadSetting{}, &models.DownloadTask{}); err != nil {
		t.Fatalf("迁移模型失败: %v", err)
	}
	return db
}

// TestReconcileMissingAndHealthy 正常场景：存在文件不产生候选，失效记录进候选，不修改 offline_comics
func TestReconcileMissingAndHealthy(t *testing.T) {
	db := newRecoveryTestDB(t)
	root := t.TempDir()
	healthyDir := filepath.Join(root, "healthy")
	if err := os.MkdirAll(healthyDir, 0o755); err != nil {
		t.Fatalf("创建测试目录失败: %v", err)
	}

	comics := []models.OfflineComic{
		{ID: "c1", Title: "healthy", LocalPath: healthyDir, GID: "100", Token: "t1", ScanPathID: "p1"},
		{ID: "c2", Title: "missing", LocalPath: filepath.Join(root, "missing"), GID: "200", Token: "t2", ScanPathID: "p1"},
	}
	if err := db.Create(&comics).Error; err != nil {
		t.Fatalf("写入漫画记录失败: %v", err)
	}

	res, err := ReconcileScanPathAgainstDisk(db, "p1", root, nil)
	if err != nil {
		t.Fatalf("对账失败: %v", err)
	}
	if res.Total != 2 || res.Missing != 1 || res.Candidates != 1 || res.Fallen != 0 || res.PathMissing {
		t.Fatalf("对账统计异常: %+v", res)
	}

	// 候选表只有 gid=200；gid=100 不应出现
	var cand models.RecoveryCandidate
	if err := db.First(&cand, "g_id = ?", "200").Error; err != nil {
		t.Fatalf("缺失记录应进候选: %v", err)
	}
	if cand.Token != "t2" || cand.ComicID != "c2" || cand.ScanPathID != "p1" {
		t.Fatalf("候选快照字段异常: %+v", cand)
	}
	var candCount int64
	db.Model(&models.RecoveryCandidate{}).Count(&candCount)
	if candCount != 1 {
		t.Fatalf("候选数量应为 1，实际 %d", candCount)
	}

	// 对账只写候选/阵亡，绝不修改 offline_comics
	var alive int64
	db.Model(&models.OfflineComic{}).Count(&alive)
	if alive != 2 {
		t.Fatalf("offline_comics 记录不应被修改/删除，实际 %d 条", alive)
	}
}

// TestReconcileClassifyFallen 失效记录分类：removed_status → removed；无 g_id/token → no-ehentai
func TestReconcileClassifyFallen(t *testing.T) {
	db := newRecoveryTestDB(t)
	root := t.TempDir()

	comics := []models.OfflineComic{
		{ID: "r1", GID: "300", Token: "t3", Title: "removed", LocalPath: filepath.Join(root, "gone1"), ScanPathID: "p1", RemovedStatus: true},
		{ID: "r2", Title: "noehentai", LocalPath: filepath.Join(root, "gone2"), ScanPathID: "p1"},
	}
	if err := db.Create(&comics).Error; err != nil {
		t.Fatalf("写入漫画记录失败: %v", err)
	}

	res, err := ReconcileScanPathAgainstDisk(db, "p1", root, nil)
	if err != nil {
		t.Fatalf("对账失败: %v", err)
	}
	if res.Missing != 2 || res.Candidates != 0 || res.Fallen != 2 {
		t.Fatalf("对账统计异常: %+v", res)
	}

	var removed models.FallenComic
	if err := db.First(&removed, "dedupe_key = ?", "gid:300").Error; err != nil {
		t.Fatalf("removed_status 应写阵亡 removed: %v", err)
	}
	if removed.Reason != models.FallenReasonRemoved {
		t.Fatalf("阵亡原因应为 %q，实际 %q", models.FallenReasonRemoved, removed.Reason)
	}

	var noeh models.FallenComic
	if err := db.First(&noeh, "dedupe_key = ?", "cid:r2").Error; err != nil {
		t.Fatalf("无 g_id/token 应写阵亡 no-ehentai: %v", err)
	}
	if noeh.Reason != models.FallenReasonNoEhentai {
		t.Fatalf("阵亡原因应为 %q，实际 %q", models.FallenReasonNoEhentai, noeh.Reason)
	}
}

// TestReconcilePathMissing 根路径整个不存在：PathMissing 标记 + 整条路径记录全部判失效
func TestReconcilePathMissing(t *testing.T) {
	db := newRecoveryTestDB(t)
	deletedRoot := filepath.Join(t.TempDir(), "deleted-root") // 故意不创建

	comics := []models.OfflineComic{
		{ID: "m1", GID: "400", Token: "t4", Title: "a", LocalPath: filepath.Join(deletedRoot, "a"), ScanPathID: "p1"},
		{ID: "m2", GID: "401", Token: "t5", Title: "b", LocalPath: filepath.Join(deletedRoot, "b"), ScanPathID: "p1"},
	}
	if err := db.Create(&comics).Error; err != nil {
		t.Fatalf("写入漫画记录失败: %v", err)
	}

	res, err := ReconcileScanPathAgainstDisk(db, "p1", deletedRoot, nil)
	if err != nil {
		t.Fatalf("对账失败: %v", err)
	}
	if !res.PathMissing {
		t.Fatalf("根路径不存在时应标记 PathMissing")
	}
	if res.Missing != 2 || res.Candidates != 2 {
		t.Fatalf("整条路径记录应全部判失效并进候选，实际 %+v", res)
	}
}

// TestReconcileHeal 自愈：文件重新存在 → 清除候选与未 dismissed 阵亡；dismissed 阵亡保留
func TestReconcileHeal(t *testing.T) {
	db := newRecoveryTestDB(t)
	root := t.TempDir()
	// local_path 有唯一索引，三条记录需各自独立目录
	existing1 := filepath.Join(root, "e1")
	existing2 := filepath.Join(root, "e2")
	existing3 := filepath.Join(root, "e3")
	for _, d := range []string{existing1, existing2, existing3} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("创建测试目录失败: %v", err)
		}
	}

	// 记录 1：文件存在，但候选表残留（gid=400）
	db.Create(&models.OfflineComic{ID: "h1", GID: "400", Token: "t", Title: "healed", LocalPath: existing1, ScanPathID: "p1"})
	db.Create(&models.RecoveryCandidate{GID: "400", Token: "t"})
	// 记录 2：文件存在，阵亡表残留（gid=500 未 dismissed）→ 清除
	db.Create(&models.OfflineComic{ID: "h2", GID: "500", Token: "t", Title: "healed2", LocalPath: existing2, ScanPathID: "p1"})
	db.Create(&models.FallenComic{DedupeKey: "gid:500", GID: "500", Title: "x"})
	// 记录 3：文件存在，阵亡表残留但用户已 dismissed → 保留
	db.Create(&models.OfflineComic{ID: "h3", GID: "600", Token: "t", Title: "healed3", LocalPath: existing3, ScanPathID: "p1"})
	db.Create(&models.FallenComic{DedupeKey: "gid:600", GID: "600", Title: "y", Dismissed: true})

	res, err := ReconcileScanPathAgainstDisk(db, "p1", root, nil)
	if err != nil {
		t.Fatalf("对账失败: %v", err)
	}
	if res.Healed != 2 { // 候选 1 + 未 dismissed 阵亡 1
		t.Fatalf("自愈数量应为 2，实际 %d", res.Healed)
	}

	var candCount int64
	db.Model(&models.RecoveryCandidate{}).Count(&candCount)
	if candCount != 0 {
		t.Fatalf("候选应被清除，实际剩 %d 条", candCount)
	}
	var gone models.FallenComic
	if err := db.First(&gone, "dedupe_key = ?", "gid:500").Error; err == nil {
		t.Fatalf("未 dismissed 的阵亡应被清除")
	}
	var kept models.FallenComic
	if err := db.First(&kept, "dedupe_key = ?", "gid:600").Error; err != nil {
		t.Fatalf("dismissed 阵亡应保留: %v", err)
	}
	if !kept.Dismissed {
		t.Fatalf("dismissed 状态应保留")
	}
}

// TestReconcileUpsertPreservesDismissed 重复对账幂等 + dismissed 状态跨对账保留
func TestReconcileUpsertPreservesDismissed(t *testing.T) {
	db := newRecoveryTestDB(t)
	root := t.TempDir()

	db.Create(&models.OfflineComic{
		ID: "x1", GID: "700", Token: "t", Title: "gone",
		LocalPath: filepath.Join(root, "gone"), ScanPathID: "p1", RemovedStatus: true,
	})
	db.Create(&models.FallenComic{
		DedupeKey: "gid:700", GID: "700", Title: "gone",
		Reason: models.FallenReasonRemoved, Dismissed: true,
	})

	// 连续两次对账：应 upsert 同一行，保留 dismissed
	for i := 0; i < 2; i++ {
		if _, err := ReconcileScanPathAgainstDisk(db, "p1", root, nil); err != nil {
			t.Fatalf("第 %d 次对账失败: %v", i+1, err)
		}
	}

	var count int64
	db.Model(&models.FallenComic{}).Count(&count)
	if count != 1 {
		t.Fatalf("阵亡条目应按 dedupe_key 幂等，实际 %d 条", count)
	}
	var f models.FallenComic
	if err := db.First(&f, "dedupe_key = ?", "gid:700").Error; err != nil {
		t.Fatalf("阵亡条目应存在: %v", err)
	}
	if !f.Dismissed {
		t.Fatalf("dismissed 状态应跨对账保留")
	}
}

// TestPathExistsConservative 存在性判定口径：空路径/不存在 → false；存在/非 IsNotExist 错误 → true
func TestPathExistsConservative(t *testing.T) {
	if pathExistsConservative("") {
		t.Fatalf("空路径应判定为不存在")
	}
	if !pathExistsConservative(t.TempDir()) {
		t.Fatalf("存在的目录应判定为存在")
	}
	if pathExistsConservative(filepath.Join(t.TempDir(), "no-such-file")) {
		t.Fatalf("不存在的路径应判定为不存在")
	}
}

// ─────────────────────────────────────────────────────────────
// 阶段2：恢复会话（状态推导 / 锁 / 启动 / 取消 / 候选转阵亡 / 移除路径边界）
// ─────────────────────────────────────────────────────────────

// TestGetRecoveryStatusPhase 状态推导：idle → ready → recovering
func TestGetRecoveryStatusPhase(t *testing.T) {
	db := newRecoveryTestDB(t)

	st, err := GetRecoveryStatus(db)
	if err != nil {
		t.Fatalf("状态查询失败: %v", err)
	}
	if st.Phase != "idle" {
		t.Fatalf("无候选应 idle，得到 %q", st.Phase)
	}

	if err := db.Create(&models.RecoveryCandidate{GID: "100", Token: "t1", Title: "a"}).Error; err != nil {
		t.Fatalf("创建候选失败: %v", err)
	}
	st, _ = GetRecoveryStatus(db)
	if st.Phase != "ready" || st.CandidateCount != 1 {
		t.Fatalf("有候选应 ready，得到 %+v", st)
	}

	if err := db.Create(&models.DownloadTask{
		ID: "r1", UserID: 1, GID: "100", Token: "t1",
		Mode: models.DownloadModeArchive, Status: models.DownloadQueued, Group: RecoveryGroup,
	}).Error; err != nil {
		t.Fatalf("创建任务失败: %v", err)
	}
	st, _ = GetRecoveryStatus(db)
	if st.Phase != "recovering" || st.ActiveTasks != 1 {
		t.Fatalf("有未终结任务应 recovering，得到 %+v", st)
	}
}

// TestRecoveryLockedPath 恢复中锁按路径：只锁候选来源路径
func TestRecoveryLockedPath(t *testing.T) {
	db := newRecoveryTestDB(t)
	db.Create(&models.RecoveryCandidate{GID: "100", Token: "t", Title: "a", ScanPathID: "p1"})
	db.Create(&models.DownloadTask{
		ID: "r1", GID: "100", Token: "t",
		Mode: models.DownloadModeArchive, Status: models.DownloadDownloading, Group: RecoveryGroup,
	})

	locked, err := IsRecoveryLockedPath(db, "p1")
	if err != nil || !locked {
		t.Fatalf("恢复中应锁定候选来源路径 p1，得到 locked=%v err=%v", locked, err)
	}
	if locked, _ := IsRecoveryLockedPath(db, "p2"); locked {
		t.Fatalf("无恢复任务的路径 p2 不应锁定")
	}
}

// TestStartRecovery 启动恢复：无候选 / 无效方案 / 正常批量创建 / 恢复中拒绝 / 取消后幂等重入
func TestStartRecovery(t *testing.T) {
	db := newRecoveryTestDB(t)
	mgr := NewDownloadManager(db, nil)

	if _, err := mgr.StartRecovery(RecoveryStartParams{Scheme: RecoverySchemeGallery, UserID: 1}); !errors.Is(err, ErrRecoveryNoCandidates) {
		t.Fatalf("无候选应返回 ErrRecoveryNoCandidates，得到 %v", err)
	}

	db.Create(&models.RecoveryCandidate{GID: "100", Token: "t1", Title: "a", ComicID: "c1"})
	db.Create(&models.RecoveryCandidate{GID: "200", Token: "t2", Title: "b", ComicID: "c2"})

	if _, err := mgr.StartRecovery(RecoveryStartParams{Scheme: "bogus", UserID: 1}); !errors.Is(err, ErrRecoverySchemeInvalid) {
		t.Fatalf("无效方案应返回 ErrRecoverySchemeInvalid，得到 %v", err)
	}

	res, err := mgr.StartRecovery(RecoveryStartParams{Scheme: RecoverySchemeArchiveOriginal, UserID: 1})
	if err != nil {
		t.Fatalf("启动恢复失败: %v", err)
	}
	if res.Created != 2 || res.Skipped != 0 {
		t.Fatalf("应创建 2 条任务，得到 %+v", res)
	}

	var tasks []models.DownloadTask
	if err := db.Where("\"group\" = ?", RecoveryGroup).Find(&tasks).Error; err != nil {
		t.Fatalf("查询任务失败: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("任务数量应为 2，实际 %d", len(tasks))
	}
	for _, task := range tasks {
		if !task.ForceDeleteOriginal || task.Mode != models.DownloadModeArchive || task.ArchiveType != models.ArchiveTypeOriginal {
			t.Fatalf("恢复任务字段异常: %+v", task)
		}
		if task.UpdateForComicID == "" || task.UserID != 1 {
			t.Fatalf("恢复任务关联/发起人异常: %+v", task)
		}
	}

	// 恢复中重复启动 → ErrRecoveryInProgress
	if _, err := mgr.StartRecovery(RecoveryStartParams{Scheme: RecoverySchemeGallery, UserID: 1}); !errors.Is(err, ErrRecoveryInProgress) {
		t.Fatalf("恢复中重复启动应返回 ErrRecoveryInProgress，得到 %v", err)
	}

	// 取消全部后再启动 → 候选保留，可幂等重新入队
	if _, err := mgr.CancelRecovery(RecoveryCancelParams{}); err != nil {
		t.Fatalf("取消失败: %v", err)
	}
	res2, err := mgr.StartRecovery(RecoveryStartParams{Scheme: RecoverySchemeGallery, UserID: 1})
	if err != nil || res2.Created != 2 {
		t.Fatalf("取消后应可重新入队 2 条，得到 %+v err=%v", res2, err)
	}
}

// TestCancelRecovery 取消恢复：单条 / 全部，候选保留
func TestCancelRecovery(t *testing.T) {
	db := newRecoveryTestDB(t)
	mgr := NewDownloadManager(db, nil)
	db.Create(&models.RecoveryCandidate{GID: "100", Token: "t", Title: "a"})
	db.Create(&models.RecoveryCandidate{GID: "200", Token: "t", Title: "b"})
	db.Create(&models.DownloadTask{
		ID: "r1", GID: "100", Token: "t",
		Mode: models.DownloadModeGallery, Status: models.DownloadQueued, Group: RecoveryGroup,
	})
	db.Create(&models.DownloadTask{
		ID: "r2", GID: "200", Token: "t",
		Mode: models.DownloadModeGallery, Status: models.DownloadQueued, Group: RecoveryGroup,
	})

	res, err := mgr.CancelRecovery(RecoveryCancelParams{GIDs: []string{"100"}})
	if err != nil || res.Cancelled != 1 {
		t.Fatalf("单条取消应成功 1 条，得到 %+v err=%v", res, err)
	}
	var t1 models.DownloadTask
	db.First(&t1, "id = ?", "r1")
	if t1.Status != models.DownloadCancelled {
		t.Fatalf("r1 应为 cancelled，得到 %s", t1.Status)
	}

	res, err = mgr.CancelRecovery(RecoveryCancelParams{})
	if err != nil || res.Cancelled != 1 {
		t.Fatalf("全部取消应取消剩余 1 条，得到 %+v err=%v", res, err)
	}

	var candCount int64
	db.Model(&models.RecoveryCandidate{}).Count(&candCount)
	if candCount != 2 {
		t.Fatalf("取消后候选应完整保留，实际 %d 条", candCount)
	}
}

// TestConvertCandidatesToFallen 候选转阵亡：path-removed、清空候选、已有 dismissed 保留
func TestConvertCandidatesToFallen(t *testing.T) {
	db := newRecoveryTestDB(t)
	db.Create(&models.RecoveryCandidate{GID: "100", Token: "t", Title: "a", ComicID: "c1", ScanPathID: "p1"})
	db.Create(&models.RecoveryCandidate{GID: "200", Token: "t", Title: "b", ComicID: "c2", ScanPathID: "p1"})
	db.Create(&models.FallenComic{DedupeKey: "gid:100", GID: "100", Title: "a", Dismissed: true})

	converted, err := convertCandidatesToFallen(db, "p1", "Z:/Comics")
	if err != nil || converted != 2 {
		t.Fatalf("应转换 2 条，得到 %d err=%v", converted, err)
	}

	var f models.FallenComic
	if err := db.First(&f, "dedupe_key = ?", "gid:100").Error; err != nil {
		t.Fatalf("gid=100 应转阵亡: %v", err)
	}
	if f.Reason != models.FallenReasonPathRemoved {
		t.Fatalf("原因应为 path-removed，得到 %q", f.Reason)
	}
	if !f.Dismissed {
		t.Fatalf("已有 dismissed 状态应保留")
	}

	var candCount int64
	db.Model(&models.RecoveryCandidate{}).Count(&candCount)
	if candCount != 0 {
		t.Fatalf("候选应清空，实际 %d 条", candCount)
	}
}

// TestDeleteScanPathRecoveryBoundary 移除路径边界：恢复中拒绝；有候选需确认；确认后候选转阵亡
func TestDeleteScanPathRecoveryBoundary(t *testing.T) {
	withoutXpCloud(t)
	db := newRefsTestDB(t)
	if err := db.AutoMigrate(&models.ExtraScanPath{}); err != nil {
		t.Fatalf("迁移路径表失败: %v", err)
	}
	db.Create(&models.ExtraScanPath{ID: "p1", Path: "Z:/c"})
	db.Create(&models.OfflineComic{ID: "c1", Title: "x", LocalPath: "Z:/c/1", ScanPathID: "p1"})
	db.Create(&models.RecoveryCandidate{GID: "100", Token: "t", Title: "x", ScanPathID: "p1", ComicID: "c1"})

	// 1) 有候选未确认 → ErrScanPathHasCandidates
	if _, err := DeleteScanPathWithComics(db, "p1", false); !errors.Is(err, ErrScanPathHasCandidates) {
		t.Fatalf("有候选未确认应拒绝移除，得到 %v", err)
	}

	// 2) 恢复中 → ErrScanPathRecoveryInProgress
	db.Create(&models.DownloadTask{
		ID: "r1", GID: "100", Token: "t",
		Mode: models.DownloadModeGallery, Status: models.DownloadQueued, Group: RecoveryGroup,
	})
	if _, err := DeleteScanPathWithComics(db, "p1", true); !errors.Is(err, ErrScanPathRecoveryInProgress) {
		t.Fatalf("恢复中应拒绝移除，得到 %v", err)
	}

	// 3) 任务取消后确认移除 → 候选转阵亡 + 路径与记录清理
	db.Model(&models.DownloadTask{}).Where("id = ?", "r1").Update("status", models.DownloadCancelled)
	removed, err := DeleteScanPathWithComics(db, "p1", true)
	if err != nil || removed != 1 {
		t.Fatalf("确认移除应清理 1 本，得到 removed=%d err=%v", removed, err)
	}
	var f models.FallenComic
	if err := db.First(&f, "dedupe_key = ?", "gid:100").Error; err != nil {
		t.Fatalf("候选应转阵亡条目: %v", err)
	}
	if f.Reason != models.FallenReasonPathRemoved {
		t.Fatalf("阵亡原因应为 path-removed，得到 %q", f.Reason)
	}
	var candCount, pathCount int64
	db.Model(&models.RecoveryCandidate{}).Count(&candCount)
	db.Model(&models.ExtraScanPath{}).Count(&pathCount)
	if candCount != 0 || pathCount != 0 {
		t.Fatalf("候选应清空、路径应删除（候选=%d 路径=%d）", candCount, pathCount)
	}
}
