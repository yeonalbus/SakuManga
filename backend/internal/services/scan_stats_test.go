package services

import (
	"path/filepath"
	"testing"

	"SakuManga/internal/models"
)

// ─────────────────────────────────────────────────────────────
// Round53 阶段三：D17 扫描统计细分
// found = 写入成功；skipped 按原因细分（已存在/同 gid/空目录/不支持归档）；failed 单列。
// ─────────────────────────────────────────────────────────────

func TestSaveComicResultReasons(t *testing.T) {
	db := setupScanPolicyDB(t)

	// 1) 新记录 → Created
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "1.jpg"), "x")
	if got := saveComicWithResult(dir, true, false, ""); got != SaveComicCreated {
		t.Fatalf("新记录应为 Created，实际 %v", got)
	}

	// 2) 增量模式已存在 → SkippedExisting
	if got := saveComicWithResult(dir, true, true, ""); got != SaveComicSkippedExisting {
		t.Fatalf("增量已存在应为 SkippedExisting，实际 %v", got)
	}

	// 3) 同 gid 冲突（已存在文件夹形态）→ SkippedGid
	galleryDir := t.TempDir()
	mustWriteFile(t, filepath.Join(galleryDir, "1.jpg"), "x")
	if err := db.Create(&models.OfflineComic{
		ID: generateID(galleryDir), GID: "100", LocalPath: galleryDir, SourceMode: "gallery",
	}).Error; err != nil {
		t.Fatalf("创建画廊记录失败: %v", err)
	}
	zipDir := t.TempDir()
	zipPath := filepath.Join(zipDir, "x.zip")
	mustWriteZipContent(t, zipPath, map[string]string{
		"1.jpg":    "x",
		"metadata": `{"gid":"100","token":"t"}`,
	})
	if got := saveComicWithResult(zipPath, false, false, ""); got != SaveComicSkippedGid {
		t.Fatalf("同 gid 压缩包应为 SkippedGid，实际 %v", got)
	}
}

func TestScanStatsCountsReasons(t *testing.T) {
	db := setupScanPolicyDB(t)
	_ = db

	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "A", "1.jpg"), "x")
	mustWriteFile(t, filepath.Join(root, "B", "1.jpg"), "x")
	mustWriteFile(t, filepath.Join(root, "empty", ".placeholder"), "x") // 无图 → empty

	stats, err := scanDirectory(root, true, "full", nil, "")
	if err != nil {
		t.Fatalf("扫描失败: %v", err)
	}
	if stats.Found != 2 {
		t.Errorf("Found 应为 2，实际 %d", stats.Found)
	}
	if stats.SkippedEmpty != 1 {
		t.Errorf("SkippedEmpty 应为 1，实际 %d", stats.SkippedEmpty)
	}
	if stats.Failed != 0 || stats.SkippedExisting != 0 || stats.SkippedGid != 0 {
		t.Errorf("不应有失败/已存在/同 gid 计数，实际 failed=%d existing=%d gid=%d",
			stats.Failed, stats.SkippedExisting, stats.SkippedGid)
	}
}

// TestPageCountBaseline D18：更新检测页数基准 = OriginalPageCount 优先，否则 PageCount。
func TestPageCountBaseline(t *testing.T) {
	cases := []struct {
		name string
		c    *models.OfflineComic
		want int
	}{
		{"nil", nil, 0},
		{"物理页优先", &models.OfflineComic{PageCount: 5, OriginalPageCount: 10}, 10},
		{"无物理页退回有效页", &models.OfflineComic{PageCount: 5}, 5},
		{"均为零", &models.OfflineComic{}, 0},
	}
	for _, c := range cases {
		if got := pageCountBaseline(c.c); got != c.want {
			t.Errorf("%s: pageCountBaseline = %d，期望 %d", c.name, got, c.want)
		}
	}
}
