package services

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"SakuManga/internal/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ─────────────────────────────────────────────────────────────
// Round45：查重规则4（文件夹内容签名）缓存基准测试
//
// 背景：旧快路径用「目录 mtime <= file_modified_at」判定，而 file_modified_at 被本规则
// 写成「目录内图片最大 mtime」→ 目录 mtime 恒略晚，缓存恒失效（实测命中率 3.6%），
// 每次维护都递归 stat 全部文件夹。现改以「算出签名时的目录 mtime」（sig_dir_mtime）为基准。
//
// 覆盖：
//   - 首次运行：算出签名 + 记录目录 mtime 基准
//   - 目录 mtime 未变 → 复用缓存签名（不重算）
//   - 目录 mtime 变化 → 重算（内容变化被感知）
//   - forceFull（全量核对）→ 忽略缓存强制重算（兜底「同名覆盖写」盲区）
// ─────────────────────────────────────────────────────────────

func newSigCacheTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
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

// mkGalleryDir 建一个 gallery 形态的漫画文件夹（含实际图片文件，签名算法按「相对路径|大小」计算）
func mkGalleryDir(t *testing.T, root, name string, files map[string]string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}
	for fn, content := range files {
		if err := os.WriteFile(filepath.Join(dir, fn), []byte(content), 0o644); err != nil {
			t.Fatalf("写入文件失败: %v", err)
		}
	}
	return dir
}

// seedGalleryComics 入库两本 gallery 漫画（GID 各自唯一 → 不触发规则1/3）
func seedGalleryComics(t *testing.T, db *gorm.DB, idA, pathA, idB, pathB string) {
	t.Helper()
	for _, m := range []models.OfflineComic{
		mkSignatureComic(idA, pathA),
		mkSignatureComic(idB, pathB),
	} {
		c := m
		if err := db.Create(&c).Error; err != nil {
			t.Fatalf("创建漫画失败: %v", err)
		}
	}
}

func mkSignatureComic(id, path string) models.OfflineComic {
	return models.OfflineComic{
		ID:         id,
		Title:      "[テスト] サンプル " + id,
		Source:     models.SourceOffline,
		SourceMode: "gallery",
		LocalPath:  path,
		GID:        "gid-" + id,
		Token:      "tok-" + id,
		PageCount:  2,
		FileSize:   30,
	}
}

// reread 读取一条漫画记录（用于断言字段落库结果）
func reread(t *testing.T, db *gorm.DB, id string) models.OfflineComic {
	t.Helper()
	var got models.OfflineComic
	if err := db.Where("id = ?", id).First(&got).Error; err != nil {
		t.Fatalf("读取漫画 %s 失败: %v", id, err)
	}
	return got
}

// mustStat 目录自身 mtime（断言用：应与库里记录的 sig_dir_mtime 精确相等）
func mustStat(t *testing.T, path string) time.Time {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat 目录失败: %v", err)
	}
	return fi.ModTime()
}

// TestFolderSignatureCacheWarmupAndReuse 首次预热 → 目录未变则复用缓存（不重算）
func TestFolderSignatureCacheWarmupAndReuse(t *testing.T) {
	resetMaintainResultForTest(t)
	db := newSigCacheTestDB(t)
	root := t.TempDir()

	// 两个文件夹内容不同（文件名不同 → 签名不同），避免被规则4 判为互相重复
	dirA := mkGalleryDir(t, root, "A", map[string]string{"01.jpg": "aaaaaaaaaa", "02.jpg": "bbbbbbbbbbbbbb"})
	dirB := mkGalleryDir(t, root, "B", map[string]string{"01.jpg": "cccccccccccccc", "02.jpg": "dddddd"})
	seedGalleryComics(t, db, "sig-a", dirA, "sig-b", dirB)

	// ── 首次运行：无缓存基准 → 预热，算出签名并记录目录 mtime ──
	if _, err := MaintainDedupWithProgress(db, nil, nil, false); err != nil {
		t.Fatalf("首次维护查重失败: %v", err)
	}
	a := reread(t, db, "sig-a")
	if a.FileHash == "" {
		t.Fatalf("首次运行应算出内容签名，实际为空")
	}
	if a.SigDirMtime.IsZero() {
		t.Fatalf("首次运行应记录目录 mtime 基准，实际为零值")
	}
	if a.SigComputedAt == 0 {
		t.Fatalf("首次运行应记录签名计算时间戳，实际为 0")
	}
	hashA := a.FileHash
	if !a.SigDirMtime.Equal(mustStat(t, dirA)) {
		t.Fatalf("记录的 sig_dir_mtime(%v) 应与目录 mtime(%v) 一致", a.SigDirMtime, mustStat(t, dirA))
	}

	// ── 目录未变：把签名替换为标记值，二次运行应「复用」而不重算 ──
	if err := db.Model(&models.OfflineComic{}).Where("id = ?", "sig-a").
		Update("file_hash", "cached-marker").Error; err != nil {
		t.Fatalf("写入标记签名失败: %v", err)
	}
	if _, err := MaintainDedupWithProgress(db, nil, nil, false); err != nil {
		t.Fatalf("二次维护查重失败: %v", err)
	}
	got := reread(t, db, "sig-a")
	if got.FileHash != "cached-marker" {
		t.Fatalf("目录 mtime 未变时应复用缓存签名（cached-marker），实际被重算为 %q（原签名 %q）",
			got.FileHash, hashA)
	}
	if !got.SigDirMtime.Equal(mustStat(t, dirA)) {
		t.Fatalf("复用分支不应改动 sig_dir_mtime")
	}
}

// TestFolderSignatureCacheRecomputeOnChange 目录 mtime 变化 → 重算且内容变化被感知
func TestFolderSignatureCacheRecomputeOnChange(t *testing.T) {
	resetMaintainResultForTest(t)
	db := newSigCacheTestDB(t)
	root := t.TempDir()

	dirA := mkGalleryDir(t, root, "A", map[string]string{"01.jpg": "aaaaaaaaaa"})
	dirB := mkGalleryDir(t, root, "B", map[string]string{"01.jpg": "cccccccccccccc"})
	seedGalleryComics(t, db, "sig-a", dirA, "sig-b", dirB)

	if _, err := MaintainDedupWithProgress(db, nil, nil, false); err != nil {
		t.Fatalf("首次维护查重失败: %v", err)
	}
	hashBefore := reread(t, db, "sig-a").FileHash

	// 目录内容变化：新增一个图片文件（目录条目变化 → 目录 mtime 更新）
	if err := os.WriteFile(filepath.Join(dirA, "03.jpg"), []byte("new-page"), 0o644); err != nil {
		t.Fatalf("新增图片失败: %v", err)
	}
	// 把签名替换为标记值，以证明本次确实发生重算
	if err := db.Model(&models.OfflineComic{}).Where("id = ?", "sig-a").
		Update("file_hash", "stale-marker").Error; err != nil {
		t.Fatalf("写入标记签名失败: %v", err)
	}

	if _, err := MaintainDedupWithProgress(db, nil, nil, false); err != nil {
		t.Fatalf("二次维护查重失败: %v", err)
	}
	got := reread(t, db, "sig-a")
	if got.FileHash == "stale-marker" {
		t.Fatalf("目录 mtime 已变化，签名应被重算，实际仍为缓存标记值")
	}
	if got.FileHash == hashBefore {
		t.Fatalf("目录新增图片后签名应变化，实际与变化前相同（%q）", got.FileHash)
	}
	if !got.SigDirMtime.Equal(mustStat(t, dirA)) {
		t.Fatalf("重算后 sig_dir_mtime 应更新为最新目录 mtime")
	}
}

// TestFolderSignatureCacheIgnoredOnForceFull forceFull（全量核对）必须忽略缓存强制重算
func TestFolderSignatureCacheIgnoredOnForceFull(t *testing.T) {
	resetMaintainResultForTest(t)
	db := newSigCacheTestDB(t)
	root := t.TempDir()

	dirA := mkGalleryDir(t, root, "A", map[string]string{"01.jpg": "aaaaaaaaaa"})
	dirB := mkGalleryDir(t, root, "B", map[string]string{"01.jpg": "cccccccccccccc"})
	seedGalleryComics(t, db, "sig-a", dirA, "sig-b", dirB)

	if _, err := MaintainDedupWithProgress(db, nil, nil, false); err != nil {
		t.Fatalf("首次维护查重失败: %v", err)
	}
	if err := db.Model(&models.OfflineComic{}).Where("id = ?", "sig-a").
		Update("file_hash", "cached-marker").Error; err != nil {
		t.Fatalf("写入标记签名失败: %v", err)
	}

	// forceFull=true：即使目录 mtime 与基准一致，也必须重算
	if _, err := MaintainDedupWithProgress(db, nil, nil, true); err != nil {
		t.Fatalf("全量维护查重失败: %v", err)
	}
	got := reread(t, db, "sig-a")
	if got.FileHash == "cached-marker" {
		t.Fatalf("forceFull=true 时应忽略缓存强制重算，实际仍为缓存标记值")
	}
	if got.FileHash == "" {
		t.Fatalf("重算后签名不应为空")
	}
}

// TestFolderSignatureDuplicateStillDetected 缓存生效后，内容相同的文件夹仍被判为重复
func TestFolderSignatureDuplicateStillDetected(t *testing.T) {
	resetMaintainResultForTest(t)
	db := newSigCacheTestDB(t)
	root := t.TempDir()

	// 两个目录内容完全一致（同名同大小）→ 规则4 应判「删除复制项」
	files := map[string]string{"01.jpg": "aaaaaaaaaa", "02.jpg": "bbbbbbbbbb"}
	dirA := mkGalleryDir(t, root, "A", files)
	dirB := mkGalleryDir(t, root, "B", files)
	seedGalleryComics(t, db, "dup-a", dirA, "dup-b", dirB)

	res, err := MaintainDedupWithProgress(db, nil, nil, false)
	if err != nil {
		t.Fatalf("首次维护查重失败: %v", err)
	}
	removed := map[string]bool{}
	for _, it := range res.Items {
		if !it.Keep {
			removed[it.Comic.ID] = true
		}
	}
	if !removed["dup-a"] && !removed["dup-b"] {
		t.Fatalf("内容相同的两个文件夹应有一本被判「删除复制项」，实际无删除项")
	}

	// 二次运行（走缓存复用分支）后结论不变
	res2, err := MaintainDedupWithProgress(db, nil, nil, false)
	if err != nil {
		t.Fatalf("二次维护查重失败: %v", err)
	}
	removed2 := map[string]bool{}
	for _, it := range res2.Items {
		if !it.Keep {
			removed2[it.Comic.ID] = true
		}
	}
	if !removed2["dup-a"] && !removed2["dup-b"] {
		t.Fatalf("复用缓存签名的二次运行仍应判出重复项，实际无删除项")
	}
}
