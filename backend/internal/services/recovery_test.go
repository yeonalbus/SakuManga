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
		&models.DownloadSetting{}, &models.DownloadTask{},
		&models.Bookshelf{}, &models.HistoryRecord{}, &models.ReadingList{},
		&models.ComicRating{}, &models.ComicBookmark{}, &models.ComicChapter{}); err != nil {
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

// ─────────────────────────────────────────────────────────────
// 阶段3：完成/失败钩子 + 阵亡名单查询
// ─────────────────────────────────────────────────────────────

// TestClassifyRecoveryFailure 恢复失败分类：确定性（removed/解压失败）vs 可重试
func TestClassifyRecoveryFailure(t *testing.T) {
	cases := []struct {
		msg     string
		want    string
		wantAny bool
	}{
		{"获取画廊页面列表失败: 画廊已被删除或不可用（This gallery has been removed or is unavailable）", models.FallenReasonRemoved, true},
		{"Gallery has been removed", models.FallenReasonRemoved, true},
		{"copyright violation", models.FallenReasonRemoved, true},
		// archiver.php 对已删除/被替换画廊返回 404：确定性失败（E 站实装错误文本）
		{"archiver.php 返回状态码 404", models.FallenReasonRemoved, true},
		{"HTTP 404 Not Found", models.FallenReasonRemoved, true},
		// 解压失败可重试（zip 损坏/断点不完整/磁盘错误），不判阵亡
		{"解压失败: zip: not a valid zip file", "", false},
		{"下载连接超时: EOF", "", false},
		{"有 3 张图片下载失败（已完成 10 张已断点保存，可重试跳过）", "", false},
		{"入库失败: no such file", "", false},
	}
	for _, c := range cases {
		reason, deterministic := classifyRecoveryFailure(c.msg)
		if reason != c.want || deterministic != c.wantAny {
			t.Errorf("classifyRecoveryFailure(%q) = (%q,%v)，期望 (%q,%v)", c.msg, reason, deterministic, c.want, c.wantAny)
		}
	}
}

// TestFailTaskRecoveryDeterministic 恢复任务确定性失败 → 阵亡 + cancelled
func TestFailTaskRecoveryDeterministic(t *testing.T) {
	db := newRecoveryTestDB(t)
	mgr := NewDownloadManager(db, nil)
	db.Create(&models.RecoveryCandidate{GID: "100", Token: "t", Title: "a", ComicID: "c1"})
	db.Create(&models.DownloadTask{
		ID: "r1", UserID: 1, GID: "100", Token: "t", Title: "a",
		Mode: models.DownloadModeGallery, Status: models.DownloadDownloading, Group: RecoveryGroup,
	})

	var task models.DownloadTask
	db.First(&task, "id = ?", "r1")
	mgr.failTask(&task, "获取画廊页面列表失败: 画廊已被删除或不可用（This gallery has been removed or is unavailable）")

	db.First(&task, "id = ?", "r1")
	if task.Status != models.DownloadCancelled {
		t.Fatalf("确定性失败应置 cancelled，得到 %s", task.Status)
	}
	var f models.FallenComic
	if err := db.First(&f, "dedupe_key = ?", "gid:100").Error; err != nil {
		t.Fatalf("确定性失败应记入阵亡: %v", err)
	}
	if f.Reason != models.FallenReasonRemoved {
		t.Fatalf("阵亡原因应为 removed，得到 %q", f.Reason)
	}
	var candCount int64
	db.Model(&models.RecoveryCandidate{}).Count(&candCount)
	if candCount != 0 {
		t.Fatalf("确定性失败后候选应移除，实际 %d", candCount)
	}
}

// TestFailTaskRecoveryRetryable 恢复任务可重试失败 → 保持 error，不写阵亡
func TestFailTaskRecoveryRetryable(t *testing.T) {
	db := newRecoveryTestDB(t)
	mgr := NewDownloadManager(db, nil)
	db.Create(&models.RecoveryCandidate{GID: "200", Token: "t", Title: "b", ComicID: "c2"})
	db.Create(&models.DownloadTask{
		ID: "r2", UserID: 1, GID: "200", Token: "t", Title: "b",
		Mode: models.DownloadModeGallery, Status: models.DownloadDownloading, Group: RecoveryGroup,
	})

	var task models.DownloadTask
	db.First(&task, "id = ?", "r2")
	mgr.failTask(&task, "连接超时: EOF")

	db.First(&task, "id = ?", "r2")
	if task.Status != models.DownloadError {
		t.Fatalf("可重试失败应保持 error，得到 %s", task.Status)
	}
	var fallenCount int64
	db.Model(&models.FallenComic{}).Count(&fallenCount)
	if fallenCount != 0 {
		t.Fatalf("可重试失败不应写阵亡，实际 %d 条", fallenCount)
	}
}

// TestReclassifyRecoveryTasks 存量异常任务重分类：404 → 阵亡 + cancelled；网络错误保留
func TestReclassifyRecoveryTasks(t *testing.T) {
	db := newRecoveryTestDB(t)
	mgr := NewDownloadManager(db, nil)
	db.Create(&models.RecoveryCandidate{GID: "100", Token: "t", Title: "a", ComicID: "c1"})
	db.Create(&models.RecoveryCandidate{GID: "200", Token: "t", Title: "b", ComicID: "c2"})

	// 旧逻辑失败的 404 error 任务
	db.Create(&models.DownloadTask{
		ID: "r1", GID: "100", Token: "t", Title: "a",
		Mode: models.DownloadModeArchive, Status: models.DownloadError, Group: RecoveryGroup,
		Error: "archiver.php 返回状态码 404",
	})
	// 网络类错误任务（可重试）→ 保留
	db.Create(&models.DownloadTask{
		ID: "r2", GID: "200", Token: "t", Title: "b",
		Mode: models.DownloadModeArchive, Status: models.DownloadError, Group: RecoveryGroup,
		Error: "连接超时: EOF",
	})
	// 用户取消的 404 任务（无候选快照，走任务信息兜底）
	db.Create(&models.DownloadTask{
		ID: "r3", GID: "300", Token: "t", Title: "c",
		Mode: models.DownloadModeArchive, Status: models.DownloadCancelled, Group: RecoveryGroup,
		Error: "archiver.php 返回状态码 404",
	})

	res, err := mgr.ReclassifyRecoveryTasks()
	if err != nil {
		t.Fatalf("重分类失败: %v", err)
	}
	if res.Fallen != 2 || res.Left != 1 {
		t.Fatalf("应转阵亡 2、保留 1，得到 %+v", res)
	}

	// 阵亡：gid 100 / 300；gid 200 不应有
	// 注意：GORM 复用结构体查询时会把旧主键拼进条件，必须用新变量
	var f100 models.FallenComic
	if err := db.First(&f100, "dedupe_key = ?", "gid:100").Error; err != nil {
		t.Fatalf("gid=100 应转阵亡: %v", err)
	}
	if f100.Reason != models.FallenReasonRemoved {
		t.Fatalf("阵亡原因应为 removed，得到 %q", f100.Reason)
	}
	var f300 models.FallenComic
	if err := db.First(&f300, "dedupe_key = ?", "gid:300").Error; err != nil {
		t.Fatalf("用户取消的 gid=300 也应转阵亡: %v", err)
	}
	var cnt int64
	db.Model(&models.FallenComic{}).Where("dedupe_key = ?", "gid:200").Count(&cnt)
	if cnt != 0 {
		t.Fatalf("网络错误任务不应进阵亡")
	}

	// 任务状态：r1/r3 → cancelled；r2 保持 error
	var t1, t2, t3 models.DownloadTask
	db.First(&t1, "id = ?", "r1")
	db.First(&t2, "id = ?", "r2")
	db.First(&t3, "id = ?", "r3")
	if t1.Status != models.DownloadCancelled || t3.Status != models.DownloadCancelled {
		t.Fatalf("404 任务应置 cancelled（r1=%s r3=%s）", t1.Status, t3.Status)
	}
	if t2.Status != models.DownloadError {
		t.Fatalf("网络错误任务应保持 error，得到 %s", t2.Status)
	}

	// 候选：100 已清理，200 保留
	var candCount int64
	db.Model(&models.RecoveryCandidate{}).Count(&candCount)
	if candCount != 1 {
		t.Fatalf("候选应只剩 gid=200，实际 %d", candCount)
	}
}

// TestFinalizeUpdateRecovery 恢复任务完成钩子：防御校验 + 删旧记录 + 清候选/阵亡
func TestFinalizeUpdateRecovery(t *testing.T) {
	db := newRecoveryTestDB(t)
	mgr := NewDownloadManager(db, nil)

	root := t.TempDir()
	oldPath := filepath.Join(root, "old-gone") // 旧路径不存在（被删）
	newPath := filepath.Join(root, "new")
	if err := os.MkdirAll(newPath, 0o755); err != nil {
		t.Fatalf("创建新路径失败: %v", err)
	}

	db.Create(&models.OfflineComic{ID: "old1", GID: "300", Token: "t", Title: "old", LocalPath: oldPath, ScanPathID: "p1"})
	db.Create(&models.RecoveryCandidate{GID: "300", Token: "t", Title: "old", ComicID: "old1", ScanPathID: "p1"})
	db.Create(&models.FallenComic{DedupeKey: "gid:300", GID: "300", Title: "old"})
	// 新记录（ingest 产物）：同 gid、文件存在
	db.Create(&models.OfflineComic{ID: "new1", GID: "300", Token: "t", Title: "old", LocalPath: newPath})

	task := &models.DownloadTask{
		ID: "r3", UserID: 1, GID: "300", Token: "t", Title: "old",
		Mode: models.DownloadModeGallery, Status: models.DownloadCompleted,
		Group: RecoveryGroup, UpdateForComicID: "old1", ForceDeleteOriginal: true,
	}
	mgr.finalizeUpdate(task)

	var oldCount int64
	db.Model(&models.OfflineComic{}).Where("id = ?", "old1").Count(&oldCount)
	if oldCount != 0 {
		t.Fatalf("旧记录应被删除，实际 %d", oldCount)
	}
	var candCount, fallenCount int64
	db.Model(&models.RecoveryCandidate{}).Count(&candCount)
	db.Model(&models.FallenComic{}).Count(&fallenCount)
	if candCount != 0 || fallenCount != 0 {
		t.Fatalf("恢复完成应清空候选/阵亡（候选=%d 阵亡=%d）", candCount, fallenCount)
	}
}

// TestFinalizeUpdateRecoveryDefensive 防御校验：新记录未就绪 → 保留旧记录与候选/阵亡
func TestFinalizeUpdateRecoveryDefensive(t *testing.T) {
	db := newRecoveryTestDB(t)
	mgr := NewDownloadManager(db, nil)

	db.Create(&models.OfflineComic{ID: "old2", GID: "400", Token: "t", Title: "old", LocalPath: "Z:/gone/400", ScanPathID: "p1"})
	db.Create(&models.RecoveryCandidate{GID: "400", Token: "t", Title: "old", ComicID: "old2", ScanPathID: "p1"})
	db.Create(&models.FallenComic{DedupeKey: "gid:400", GID: "400", Title: "old"})

	task := &models.DownloadTask{
		ID: "r4", UserID: 1, GID: "400", Token: "t", Title: "old",
		Mode: models.DownloadModeGallery, Status: models.DownloadCompleted,
		Group: RecoveryGroup, UpdateForComicID: "old2", ForceDeleteOriginal: true,
	}
	mgr.finalizeUpdate(task)

	var oldCount int64
	db.Model(&models.OfflineComic{}).Where("id = ?", "old2").Count(&oldCount)
	if oldCount != 1 {
		t.Fatalf("新记录未就绪时旧记录应保留")
	}
	var candCount int64
	db.Model(&models.RecoveryCandidate{}).Count(&candCount)
	if candCount != 1 {
		t.Fatalf("防御校验拦截时候选应保留，实际 %d", candCount)
	}
}

// TestListFallenComicsOrder 阵亡名单排序：未 dismissed 在前，dismissed 置底
func TestListFallenComicsOrder(t *testing.T) {
	db := newRecoveryTestDB(t)
	db.Create(&models.FallenComic{DedupeKey: "gid:1", GID: "1", Title: "a"})
	db.Create(&models.FallenComic{DedupeKey: "gid:2", GID: "2", Title: "b", Dismissed: true})
	db.Create(&models.FallenComic{DedupeKey: "gid:3", GID: "3", Title: "c"})

	items, total, err := ListFallenComics(db, FallenListParams{Page: 1, Size: 10})
	if err != nil {
		t.Fatalf("查询阵亡名单失败: %v", err)
	}
	if total != 3 || len(items) != 3 {
		t.Fatalf("应返回 3 条，实际 total=%d len=%d", total, len(items))
	}
	// 未 dismissed（a、c）在前，dismissed（b）在最后
	if items[2].GID != "2" {
		t.Fatalf("dismissed 条目应排最后，实际顺序 %v/%v/%v", items[0].GID, items[1].GID, items[2].GID)
	}
	for _, it := range items[:2] {
		if it.Dismissed {
			t.Fatalf("前段不应出现 dismissed 条目")
		}
	}
}

// TestSetFallenDismissedToggle 阵亡条目 x toggle：设置/撤销 dismissed
func TestSetFallenDismissedToggle(t *testing.T) {
	db := newRecoveryTestDB(t)
	db.Create(&models.FallenComic{DedupeKey: "gid:500", GID: "500", Title: "x"})
	var f models.FallenComic
	db.First(&f, "dedupe_key = ?", "gid:500")

	if err := SetFallenDismissed(db, f.ID, true); err != nil {
		t.Fatalf("dismiss 失败: %v", err)
	}
	db.First(&f, "dedupe_key = ?", "gid:500")
	if !f.Dismissed || f.DismissedAt == nil {
		t.Fatalf("dismissed 应置 true 且记录 dismissed_at")
	}

	if err := SetFallenDismissed(db, f.ID, false); err != nil {
		t.Fatalf("撤销 dismiss 失败: %v", err)
	}
	// 用全新变量查询：GORM 复用结构体时 NULL 列不会覆盖旧值（会残留上次读到的 dismissed_at）
	var after models.FallenComic
	db.First(&after, "dedupe_key = ?", "gid:500")
	if after.Dismissed || after.DismissedAt != nil {
		t.Fatalf("撤销后 dismissed 应 false 且清空 dismissed_at（dismissed=%v at=%v）", after.Dismissed, after.DismissedAt)
	}
}

// TestReconcileSkipsFallenGID 已在阵亡名单的 gid 失效时不重入候选
func TestReconcileSkipsFallenGID(t *testing.T) {
	db := newRecoveryTestDB(t)
	root := t.TempDir()

	db.Create(&models.OfflineComic{ID: "c1", GID: "600", Token: "t", Title: "a", LocalPath: filepath.Join(root, "gone"), ScanPathID: "p1"})
	db.Create(&models.FallenComic{DedupeKey: "gid:600", GID: "600", Title: "a", Reason: models.FallenReasonRemoved})

	res, err := ReconcileScanPathAgainstDisk(db, "p1", root, nil)
	if err != nil {
		t.Fatalf("对账失败: %v", err)
	}
	if res.Candidates != 0 {
		t.Fatalf("阵亡 gid 不应重入候选，实际候选 %d", res.Candidates)
	}
	var f models.FallenComic
	if err := db.First(&f, "dedupe_key = ?", "gid:600").Error; err != nil {
		t.Fatalf("阵亡条目应保留: %v", err)
	}
}

// ─────────────────────────────────────────────────────────────
// Round55：双轨失效生命周期测试
// ─────────────────────────────────────────────────────────────

// TestRound55DualTrackReconcileAndHeal 验证双轨对账：
//   - 可恢复缺失 → file_missing=true 且保留书架/历史引用
//   - 阵亡缺失 → 从 offline_comics 彻底删除并清理书架/历史引用
//   - 文件恢复后自愈 → file_missing 恢复为 false
func TestRound55DualTrackReconcileAndHeal(t *testing.T) {
	db := newRecoveryTestDB(t)
	root := t.TempDir()
	recDir := filepath.Join(root, "rec-gone")
	deadDir := filepath.Join(root, "dead-gone")

	db.Create(&models.OfflineComic{ID: "rec1", GID: "1001", Token: "t1", Title: "rec", LocalPath: recDir, ScanPathID: "p1"})
	db.Create(&models.OfflineComic{ID: "dead1", GID: "1002", Token: "t2", Title: "dead", LocalPath: deadDir, ScanPathID: "p1", RemovedStatus: true})
	db.Create(&models.Bookshelf{ID: "s1", UserID: 1, Name: "shelf", ComicIDs: `["rec1","dead1"]`, Count: 2})
	db.Create(&models.HistoryRecord{UserID: 1, ComicID: "rec1", GID: "1001", Source: models.SourceOffline, ComicTitle: "rec"})
	db.Create(&models.HistoryRecord{UserID: 1, ComicID: "dead1", GID: "1002", Source: models.SourceOffline, ComicTitle: "dead"})

	res, err := ReconcileScanPathAgainstDisk(db, "p1", root, nil)
	if err != nil {
		t.Fatalf("对账失败: %v", err)
	}
	if res.Candidates != 1 || res.Fallen != 1 {
		t.Fatalf("统计不符: %+v", res)
	}

	// 可恢复记录保留但 file_missing=true
	var rec models.OfflineComic
	if err := db.First(&rec, "id = ?", "rec1").Error; err != nil {
		t.Fatalf("可恢复记录应保留: %v", err)
	}
	if !rec.FileMissing {
		t.Fatalf("可恢复记录应置 file_missing=true")
	}

	// 阵亡记录应被物理删除
	var deadCount int64
	db.Model(&models.OfflineComic{}).Where("id = ?", "dead1").Count(&deadCount)
	if deadCount != 0 {
		t.Fatalf("阵亡记录应被物理删除，实际 %d", deadCount)
	}

	// 书架与历史：保留 rec1，剔除 dead1
	var shelf models.Bookshelf
	db.First(&shelf, "id = ?", "s1")
	if shelf.ComicIDs != `["rec1"]` {
		t.Fatalf("书架应保留 rec1 剔除 dead1，实际 %s", shelf.ComicIDs)
	}
	var histCount int64
	db.Model(&models.HistoryRecord{}).Where("comic_id = ?", "rec1").Count(&histCount)
	if histCount != 1 {
		t.Fatalf("rec1 历史应保留")
	}
	db.Model(&models.HistoryRecord{}).Where("comic_id = ?", "dead1").Count(&histCount)
	if histCount != 0 {
		t.Fatalf("dead1 历史应被清理")
	}

	// 文件重新存在 → 自愈恢复 file_missing=false
	if err := os.MkdirAll(recDir, 0o755); err != nil {
		t.Fatalf("创建自愈目录失败: %v", err)
	}
	res2, err := ReconcileScanPathAgainstDisk(db, "p1", root, nil)
	if err != nil {
		t.Fatalf("自愈对账失败: %v", err)
	}
	if res2.Healed != 1 {
		t.Fatalf("应自愈 1 条，实际 %d", res2.Healed)
	}
	db.First(&rec, "id = ?", "rec1")
	if rec.FileMissing {
		t.Fatalf("自愈后 file_missing 应恢复为 false")
	}
}

// TestRound55RecordRecoveryFallenPurgesDeadComic 恢复确定性失败记入阵亡时同步删死记录
func TestRound55RecordRecoveryFallenPurgesDeadComic(t *testing.T) {
	db := newRecoveryTestDB(t)
	mgr := NewDownloadManager(db, nil)
	gone := filepath.Join(t.TempDir(), "gone-2001")

	db.Create(&models.OfflineComic{ID: "c2001", GID: "2001", Token: "t", Title: "gone", LocalPath: gone, FileMissing: true})
	db.Create(&models.RecoveryCandidate{GID: "2001", Token: "t", Title: "gone", ComicID: "c2001", LocalPath: gone})
	db.Create(&models.Bookshelf{ID: "s1", UserID: 1, Name: "shelf", ComicIDs: `["c2001"]`, Count: 1})

	task := &models.DownloadTask{ID: "t2001", GID: "2001", Token: "t", Title: "gone", Group: RecoveryGroup, UpdateForComicID: "c2001"}
	if err := mgr.recordRecoveryFallen(task, models.FallenReasonRemoved); err != nil {
		t.Fatalf("recordRecoveryFallen 失败: %v", err)
	}

	var count int64
	db.Model(&models.OfflineComic{}).Where("id = ?", "c2001").Count(&count)
	if count != 0 {
		t.Fatalf("阵亡后死记录应被删除，实际 %d", count)
	}
	var shelf models.Bookshelf
	db.First(&shelf, "id = ?", "s1")
	if shelf.ComicIDs != "[]" {
		t.Fatalf("死记录书架引用应被清理，实际 %s", shelf.ComicIDs)
	}
}

// TestRound55ClearRecoveryCandidates 清空候选转阵亡并删死记录；恢复中返回 409 错误
func TestRound55ClearRecoveryCandidates(t *testing.T) {
	db := newRecoveryTestDB(t)
	gone := filepath.Join(t.TempDir(), "gone-3001")

	db.Create(&models.OfflineComic{ID: "c3001", GID: "3001", Token: "t", Title: "ab", LocalPath: gone, FileMissing: true})
	db.Create(&models.RecoveryCandidate{GID: "3001", Token: "t", Title: "ab", ComicID: "c3001", LocalPath: gone})

	// 恢复中拒绝清空
	db.Create(&models.DownloadTask{ID: "tk1", GID: "3001", Group: RecoveryGroup, Status: models.DownloadPaused})
	if _, err := ClearRecoveryCandidates(db); !errors.Is(err, ErrRecoveryInProgress) {
		t.Fatalf("恢复中应返回 ErrRecoveryInProgress，实际 %v", err)
	}
	db.Where("id = ?", "tk1").Delete(&models.DownloadTask{})

	res, err := ClearRecoveryCandidates(db)
	if err != nil {
		t.Fatalf("ClearRecoveryCandidates 失败: %v", err)
	}
	if res.Converted != 1 || res.Purged != 1 {
		t.Fatalf("统计异常: %+v", res)
	}
	var candCount, comicCount int64
	db.Model(&models.RecoveryCandidate{}).Count(&candCount)
	db.Model(&models.OfflineComic{}).Count(&comicCount)
	if candCount != 0 || comicCount != 0 {
		t.Fatalf("候选与死记录应均清零，实际 cand=%d comic=%d", candCount, comicCount)
	}
	var f models.FallenComic
	if err := db.First(&f, "dedupe_key = ?", "gid:3001").Error; err != nil || f.Reason != models.FallenReasonAbandoned {
		t.Fatalf("应写入 abandoned 阵亡记录: err=%v reason=%q", err, f.Reason)
	}
}

// TestRound55ReconcileDefaultDownloadComics 默认下载目录（scan_path_id=''）失效对账
func TestRound55ReconcileDefaultDownloadComics(t *testing.T) {
	db := newRecoveryTestDB(t)
	root := t.TempDir()
	alive := filepath.Join(root, "alive")
	if err := os.MkdirAll(alive, 0o755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}

	db.Create(&models.OfflineComic{ID: "d1", GID: "4001", Token: "t1", Title: "ok", LocalPath: alive, ScanPathID: ""})
	db.Create(&models.OfflineComic{ID: "d2", GID: "4002", Token: "t2", Title: "miss", LocalPath: filepath.Join(root, "gone"), ScanPathID: ""})
	db.Create(&models.OfflineComic{ID: "d3", GID: "4003", Token: "t3", Title: "extra", LocalPath: filepath.Join(root, "extra-gone"), ScanPathID: "p-other"})

	res, err := ReconcileDefaultDownloadComics(db, root)
	if err != nil {
		t.Fatalf("默认目录对账失败: %v", err)
	}
	if res.Total != 2 || res.Missing != 1 || res.Candidates != 1 {
		t.Fatalf("应仅检查 scan_path_id='' 的 2 条记录，实际 %+v", res)
	}
	var extra models.OfflineComic
	db.First(&extra, "id = ?", "d3")
	if extra.FileMissing {
		t.Fatalf("其他扫描路径记录不应被默认目录对账修改")
	}
}

// TestRound55SyncRecoveryAndFallenOnStartup 启动同步幂等隐藏候选并清理阵亡死记录
func TestRound55SyncRecoveryAndFallenOnStartup(t *testing.T) {
	db := newRecoveryTestDB(t)
	root := t.TempDir()
	aliveDir := filepath.Join(root, "still-here")
	if err := os.MkdirAll(aliveDir, 0o755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}

	db.Create(&models.OfflineComic{ID: "cand1", GID: "5001", Token: "t", Title: "c", LocalPath: filepath.Join(root, "g1")})
	db.Create(&models.RecoveryCandidate{GID: "5001", Token: "t", Title: "c", ComicID: "cand1"})

	db.Create(&models.OfflineComic{ID: "dead1", GID: "5002", Token: "t", Title: "d", LocalPath: filepath.Join(root, "g2")})
	db.Create(&models.FallenComic{DedupeKey: "gid:5002", GID: "5002", ComicID: "dead1", Title: "d"})

	// 磁盘文件实际存在时不得误删
	db.Create(&models.OfflineComic{ID: "alive1", GID: "5003", Token: "t", Title: "a", LocalPath: aliveDir})
	db.Create(&models.FallenComic{DedupeKey: "gid:5003", GID: "5003", ComicID: "alive1", Title: "a"})

	hidden, purged := SyncRecoveryAndFallenOnStartup(db)
	if hidden != 1 || purged != 1 {
		t.Fatalf("启动同步应 hidden=1 purged=1，实际 hidden=%d purged=%d", hidden, purged)
	}
	// 再次执行应幂等
	hidden2, purged2 := SyncRecoveryAndFallenOnStartup(db)
	if hidden2 != 0 || purged2 != 0 {
		t.Fatalf("二次启动同步应幂等为 0/0，实际 %d/%d", hidden2, purged2)
	}
	var aliveCount int64
	db.Model(&models.OfflineComic{}).Where("id = ?", "alive1").Count(&aliveCount)
	if aliveCount != 1 {
		t.Fatalf("磁盘仍存在的记录不得被误删")
	}
}

// TestRound55FilterFileMissingQueries 验证在线已下载/替换查找排除 file_missing=true
func TestRound55FilterFileMissingQueries(t *testing.T) {
	db := newRecoveryTestDB(t)
	db.Create(&models.OfflineComic{ID: "m1", GID: "6001", Token: "t", Title: "missing", LocalPath: "Z:/g/6001", FileMissing: true})
	db.Create(&models.OfflineComic{ID: "a1", GID: "6002", Token: "t", Title: "alive", LocalPath: "Z:/g/6002", FileMissing: false})

	dtos := AttachDownloadStates(db, []OnlineComicDTO{{ID: "6001"}, {ID: "6002"}})
	if dtos[0].IsDownloaded || !dtos[1].IsDownloaded {
		t.Fatalf("AttachDownloadStates 应忽略 file_missing=true: %+v", dtos)
	}
	detail := AttachDetailDownloadState(db, &GalleryDetailResult{ID: "6001"})
	if detail.IsDownloaded {
		t.Fatalf("AttachDetailDownloadState 应忽略 file_missing=true")
	}
	if rep := FindReplacementByGID(db, "6001", "other"); rep != "" {
		t.Fatalf("FindReplacementByGID 不应返回 file_missing=true 的记录: %q", rep)
	}
}
