package services

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"

	"SakuManga/internal/database"
	"SakuManga/internal/models"
)

// ─────────────────────────────────────────────────────────────
// Round53 阶段一：D1/D2/D3/D4 扫描口径
//
// D1 一个扫描路径 = 一层：直接子目录各一本；根自身含图则根一本（子目录不再切）；
//    根下直接归档各一本；遍历跳过 "." 开头的隐藏项。
// D2 直接子目录递归图片数为 0 → 不建本（即使只有归档）。
// D3 页数 = 递归页表长度（与阅读器同一实现）。
// D4 仅 zip/cbz 可入库；rar/7z 跳过并告警。
// ─────────────────────────────────────────────────────────────

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("写文件失败: %v", err)
	}
}

// mustWriteZip 生成包含给定条目名的 zip 文件（内容任意）
func mustWriteZip(t *testing.T, path string, entries ...string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("创建 zip 失败: %v", err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	for _, name := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("写 zip 条目失败: %v", err)
		}
		if _, err := w.Write([]byte("x")); err != nil {
			t.Fatalf("写 zip 内容失败: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("关闭 zip 失败: %v", err)
	}
}

func candidatePaths(cands []ScanCandidate) []string {
	out := make([]string, 0, len(cands))
	for _, c := range cands {
		out = append(out, c.Path)
	}
	return out
}

func skipReasons(skips []ScanSkip) []ScanSkipReason {
	out := make([]ScanSkipReason, 0, len(skips))
	for _, s := range skips {
		out = append(out, s.Reason)
	}
	return out
}

func TestRootLevelChildrenAreBooks(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "A", "1.jpg"), "x")
	mustWriteFile(t, filepath.Join(root, "B", "2.png"), "x")
	mustWriteFile(t, filepath.Join(root, "note.txt"), "x")

	cands, skips, err := CollectScanCandidates(root, true)
	if err != nil {
		t.Fatalf("收集失败: %v", err)
	}
	if len(cands) != 2 {
		t.Fatalf("应得 2 本，实际 %d: %v", len(cands), candidatePaths(cands))
	}
	if len(skips) != 0 {
		t.Errorf("不应有跳过项: %v", skipReasons(skips))
	}
}

// TestNestedArchivesOnlyDirIsNotABook 反例：子目录只有归档、无散图 → 不建 0 页空本。
func TestNestedArchivesOnlyDirIsNotABook(t *testing.T) {
	root := t.TempDir()
	mustWriteZip(t, filepath.Join(root, "作者合集", "vol1.zip"), "1.jpg")
	mustWriteZip(t, filepath.Join(root, "作者合集", "vol2.zip"), "1.jpg")

	cands, skips, err := CollectScanCandidates(root, true)
	if err != nil {
		t.Fatalf("收集失败: %v", err)
	}
	if len(cands) != 0 {
		t.Fatalf("纯归档子目录不应建本，实际: %v", candidatePaths(cands))
	}
	if len(skips) != 1 || skips[0].Reason != SkipReasonEmpty {
		t.Fatalf("应记 1 条 empty 跳过，实际: %+v", skips)
	}
}

// TestMixedDirWithDirectAndNestedImagesIsOneBook 父目录有图 + 子目录有图 → 父一本，子图计入页。
func TestMixedDirWithDirectAndNestedImagesIsOneBook(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "A", "1.jpg"), "x")
	mustWriteFile(t, filepath.Join(root, "A", "sub", "2.jpg"), "x")

	cands, _, err := CollectScanCandidates(root, true)
	if err != nil {
		t.Fatalf("收集失败: %v", err)
	}
	if len(cands) != 1 || cands[0].Path != filepath.Join(root, "A") {
		t.Fatalf("应为 A 一本，实际: %v", candidatePaths(cands))
	}
}

// TestRootWithDirectImageIsSingleBook 根自身直接含图 + 下挂子目录 → 仅根一本（D1 边界用例）。
func TestRootWithDirectImageIsSingleBook(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "1.jpg"), "x")
	mustWriteFile(t, filepath.Join(root, "sub", "2.jpg"), "x")

	cands, _, err := CollectScanCandidates(root, true)
	if err != nil {
		t.Fatalf("收集失败: %v", err)
	}
	if len(cands) != 1 || cands[0].Path != root {
		t.Fatalf("根自身含图应只产出根一本，实际: %v", candidatePaths(cands))
	}
}

// TestHiddenEntriesSkipped D1 全局忽略规则：.saku_trash / .hidden / .cover.jpg 不参与。
func TestHiddenEntriesSkipped(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, ".saku_trash", "x.jpg"), "x")
	mustWriteFile(t, filepath.Join(root, "A", ".hidden", "y.jpg"), "x")
	mustWriteFile(t, filepath.Join(root, "A", "1.jpg"), "x")
	mustWriteFile(t, filepath.Join(root, "A", ".cover.jpg"), "x")

	cands, _, err := CollectScanCandidates(root, true)
	if err != nil {
		t.Fatalf("收集失败: %v", err)
	}
	if len(cands) != 1 || cands[0].Path != filepath.Join(root, "A") {
		t.Fatalf("隐藏目录不应成候选，实际: %v", candidatePaths(cands))
	}

	// 隐藏项也不进页表
	pages, err := listImagesRecursive(filepath.Join(root, "A"))
	if err != nil {
		t.Fatalf("页表失败: %v", err)
	}
	if len(pages) != 1 || pages[0] != "1.jpg" {
		t.Fatalf("页表应只含 1.jpg，实际: %v", pages)
	}
}

// TestIncludeSubfoldersFalseScansRootAsSingleBook 单本入口模式：根即一本（含子目录图）。
func TestIncludeSubfoldersFalseScansRootAsSingleBook(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "1.jpg"), "x")
	mustWriteFile(t, filepath.Join(root, "sub", "2.jpg"), "x")

	cands, _, err := CollectScanCandidates(root, false)
	if err != nil {
		t.Fatalf("收集失败: %v", err)
	}
	if len(cands) != 1 || cands[0].Path != root {
		t.Fatalf("false 模式应为根一本，实际: %v", candidatePaths(cands))
	}
}

func TestIncludeSubfoldersFalseEmptyRootIsSkipped(t *testing.T) {
	root := t.TempDir()
	cands, skips, err := CollectScanCandidates(root, false)
	if err != nil {
		t.Fatalf("收集失败: %v", err)
	}
	if len(cands) != 0 {
		t.Fatalf("空根不应建本，实际: %v", candidatePaths(cands))
	}
	if len(skips) != 1 || skips[0].Reason != SkipReasonEmpty {
		t.Fatalf("应记 empty 跳过，实际: %+v", skips)
	}
}

// TestRar7zSkippedWithReason D4：rar/7z 不作候选，zip/cbz 正常。
func TestRar7zSkippedWithReason(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "a.rar"), "x")
	mustWriteFile(t, filepath.Join(root, "b.7z"), "x")
	mustWriteZip(t, filepath.Join(root, "c.zip"), "1.jpg")
	mustWriteZip(t, filepath.Join(root, "d.cbz"), "1.jpg")

	cands, skips, err := CollectScanCandidates(root, true)
	if err != nil {
		t.Fatalf("收集失败: %v", err)
	}
	if len(cands) != 2 {
		t.Fatalf("应只收 zip/cbz 共 2 本，实际: %v", candidatePaths(cands))
	}
	if len(skips) != 2 {
		t.Fatalf("应记 2 条 unsupported，实际: %+v", skips)
	}
	for _, s := range skips {
		if s.Reason != SkipReasonUnsupported {
			t.Errorf("跳过原因应为 unsupported，实际: %+v", s)
		}
	}
}

// TestArchivePageCountMatchesReader D3：归档页数 = 包内图片数，与页表一致。
func TestArchivePageCountMatchesReader(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "x.zip")
	mustWriteZip(t, zipPath, "1.jpg", "sub/2.png", ".hidden/3.jpg", "readme.txt")

	n, err := countImagesInZip(zipPath)
	if err != nil {
		t.Fatalf("计数失败: %v", err)
	}
	pages, err := listImagesInZip(zipPath)
	if err != nil {
		t.Fatalf("页表失败: %v", err)
	}
	if n != 2 || len(pages) != 2 {
		t.Fatalf("应得 2 页（隐藏段与 txt 排除），实际 n=%d pages=%v", n, pages)
	}
}

// TestCountImagesRecursiveMatchesPageList D3：目录页数 = 递归页表长度。
func TestCountImagesRecursiveMatchesPageList(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "1.jpg"), "x")
	mustWriteFile(t, filepath.Join(root, "sub", "2.png"), "x")
	mustWriteFile(t, filepath.Join(root, ".hidden", "3.jpg"), "x")

	n, err := countImagesRecursive(root)
	if err != nil {
		t.Fatalf("计数失败: %v", err)
	}
	pages, err := listImagesRecursive(root)
	if err != nil {
		t.Fatalf("页表失败: %v", err)
	}
	if n != len(pages) || n != 2 {
		t.Fatalf("页数与页表应一致为 2，实际 n=%d pages=%v", n, pages)
	}
	// 相对路径统一 "/" 分隔
	for _, p := range pages {
		if filepath.Separator == '\\' && filepath.ToSlash(p) != p {
			t.Errorf("页表路径应统一为 / 分隔: %q", p)
		}
	}
}

// TestScanEndToEndPageCountRecursive 端到端：真实扫描入库后，DB 页数 = 递归页表长度。
func TestScanEndToEndPageCountRecursive(t *testing.T) {
	withoutXpCloud(t)
	db := newRefsTestDB(t)
	oldDB := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = oldDB })

	root := t.TempDir()
	aaa := filepath.Join(root, "AAA")
	mustWriteFile(t, filepath.Join(aaa, "1.jpg"), "x")
	mustWriteFile(t, filepath.Join(aaa, "BBB", "2.jpg"), "x")
	// 根下直接归档：内部 2 图
	mustWriteZip(t, filepath.Join(root, "c.zip"), "1.jpg", "2.jpg")

	found, err := ScanAndSaveDirectory(root, true)
	if err != nil {
		t.Fatalf("扫描失败: %v", err)
	}
	if found != 2 {
		t.Fatalf("应入库 2 本（AAA + c.zip），实际 %d", found)
	}

	var dirComic models.OfflineComic
	if err := db.First(&dirComic, "local_path = ?", aaa).Error; err != nil {
		t.Fatalf("读取 AAA 记录失败: %v", err)
	}
	if dirComic.PageCount != 2 || dirComic.OriginalPageCount != 2 {
		t.Fatalf("AAA 页数应为递归 2 页，实际 %d/%d", dirComic.PageCount, dirComic.OriginalPageCount)
	}

	var zipComic models.OfflineComic
	if err := db.First(&zipComic, "local_path = ?", filepath.Join(root, "c.zip")).Error; err != nil {
		t.Fatalf("读取归档记录失败: %v", err)
	}
	if zipComic.PageCount != 2 || zipComic.OriginalPageCount != 2 {
		t.Fatalf("归档页数应为 2 页，实际 %d/%d", zipComic.PageCount, zipComic.OriginalPageCount)
	}

	// DB 页数必须与阅读器页表一致
	pages, err := listImagesRecursive(aaa)
	if err != nil {
		t.Fatalf("页表失败: %v", err)
	}
	if len(pages) != dirComic.PageCount {
		t.Fatalf("页表 %d 页与 DB PageCount %d 不一致", len(pages), dirComic.PageCount)
	}
}
