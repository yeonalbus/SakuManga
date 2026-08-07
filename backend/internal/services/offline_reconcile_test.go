package services

import (
	"os"
	"path/filepath"
	"testing"

	"SakuHentai/internal/models"

	"gorm.io/gorm"
)

// ─────────────────────────────────────────────────────────────
// 需求 3(1)：下载完成后主动对账数据库（ReconcileOfflineAfterDownload）
//
// 四步对账：
//  1. GID 去重：同 GID 且 local_path 不同 → 生成去重建议（保留文件夹形态）
//  2. ParentGID 回写：metadata parent_gid 非空且本地为空 → 回写
//  3. PageCount 校正：metadata filecount > 本地 PageCount → 更新
//  4. Aged 复位：AgedStatus=true → 复位 AgedStatus/AgedCheckedAt
// ─────────────────────────────────────────────────────────────

// writeTestMetadata 在目录内写入 metadata 文件（平铺格式，兼容 parseEHJSONMetadata）。
func writeTestMetadata(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "metadata"), []byte(content), 0o644); err != nil {
		t.Fatalf("写入 metadata 失败: %v", err)
	}
}

func TestReconcileOfflineAfterDownload(t *testing.T) {
	mgr := newTestDownloadManager(t)
	if err := mgr.db.AutoMigrate(&models.OfflineComic{}); err != nil {
		t.Fatalf("迁移 OfflineComic 失败: %v", err)
	}

	// 落地目录 A：含 metadata（parent_gid=500, filecount=30）
	dirA := t.TempDir()
	writeTestMetadata(t, dirA, `{"gid":"100","token":"t100","parent_gid":"500","filecount":30}`)
	// 落地目录 B：无 metadata
	dirB := t.TempDir()

	recordA := &models.OfflineComic{
		ID:            "reconcile-dir-a",
		GID:           "100",
		Title:         "A",
		LocalPath:     dirA,
		SourceMode:    "gallery",
		PageCount:     20, // < metadata 30 → 校正
		ParentGID:     "", // 空 → 回写 500
		AgedStatus:    true,
		AgedCheckedAt: 123,
	}
	recordB := &models.OfflineComic{
		ID:         "reconcile-dir-b",
		GID:        "100",
		Title:      "B",
		LocalPath:  dirB,
		SourceMode: "gallery",
		PageCount:  10,
	}
	for _, c := range []*models.OfflineComic{recordA, recordB} {
		if err := mgr.db.Create(c).Error; err != nil {
			t.Fatalf("创建漫画 %q 失败: %v", c.Title, err)
		}
	}

	task := &models.DownloadTask{GID: "100"}
	result, err := ReconcileOfflineAfterDownload(mgr.db, task)
	if err != nil {
		t.Fatalf("ReconcileOfflineAfterDownload 失败: %v", err)
	}

	// 四步对账统计
	if result.ParentGIDWritten != 1 {
		t.Errorf("ParentGID 应回写 1 条，得到 %d", result.ParentGIDWritten)
	}
	if result.PageCountCorrected != 1 {
		t.Errorf("PageCount 应校正 1 条，得到 %d", result.PageCountCorrected)
	}
	if result.AgedReset != 1 {
		t.Errorf("Aged 应复位 1 条，得到 %d", result.AgedReset)
	}
	// GID 去重：两条同 gid=100 → 建议删除 B（保留 A）
	if len(result.DedupItems) != 1 {
		t.Fatalf("应生成 1 条去重建议，得到 %d", len(result.DedupItems))
	}
	di := result.DedupItems[0]
	if di.Comic.ID != "reconcile-dir-b" || di.Keep {
		t.Errorf("去重建议应指向 B 且 Keep=false，得到 %q keep=%v", di.Comic.ID, di.Keep)
	}
	if di.PairComic == nil || di.PairComic.ID != "reconcile-dir-a" {
		t.Errorf("去重建议成对对象应为 A，得到 %+v", di.PairComic)
	}

	// 落库验证 A：ParentGID=500 / PageCount=30 / Aged 复位
	var gotA models.OfflineComic
	if err := mgr.db.First(&gotA, "id = ?", "reconcile-dir-a").Error; err != nil {
		t.Fatalf("读取 A 失败: %v", err)
	}
	if gotA.ParentGID != "500" {
		t.Errorf("A.ParentGID 应为 500，得到 %q", gotA.ParentGID)
	}
	if gotA.PageCount != 30 {
		t.Errorf("A.PageCount 应为 30，得到 %d", gotA.PageCount)
	}
	if gotA.AgedStatus || gotA.AgedCheckedAt != 0 {
		t.Errorf("A.Aged 应复位（status=false checkedAt=0），得到 %v / %d", gotA.AgedStatus, gotA.AgedCheckedAt)
	}

	// 落库验证 B：无 metadata，字段对账不变（GID 去重不落库，仅建议）
	var gotB models.OfflineComic
	if err := mgr.db.First(&gotB, "id = ?", "reconcile-dir-b").Error; err != nil {
		t.Fatalf("读取 B 失败: %v", err)
	}
	if gotB.ParentGID != "" || gotB.PageCount != 10 {
		t.Errorf("B 字段对账应无变化，得到 parentGID=%q pageCount=%d", gotB.ParentGID, gotB.PageCount)
	}
}

func TestReconcileOfflineAfterDownloadZipDuplicate(t *testing.T) {
	mgr := newTestDownloadManager(t)
	if err := mgr.db.AutoMigrate(&models.OfflineComic{}); err != nil {
		t.Fatalf("迁移 OfflineComic 失败: %v", err)
	}

	dirA := t.TempDir()
	writeTestMetadata(t, dirA, `{"gid":"200","token":"t200","parent_gid":"999","filecount":5}`)
	zipB := filepath.Join(t.TempDir(), "dup.zip") // 压缩包形态（本地文件不存在也可，仅验证去重判定）

	recordA := &models.OfflineComic{
		ID:         "reconcile-zip-dir-a",
		GID:        "200",
		Title:      "folder",
		LocalPath:  dirA,
		SourceMode: "gallery",
		PageCount:  3,
	}
	recordB := &models.OfflineComic{
		ID:         "reconcile-zip-b",
		GID:        "200",
		Title:      "archive",
		LocalPath:  zipB,
		SourceMode: "archive",
		PageCount:  4,
	}
	for _, c := range []*models.OfflineComic{recordA, recordB} {
		if err := mgr.db.Create(c).Error; err != nil {
			t.Fatalf("创建漫画 %q 失败: %v", c.Title, err)
		}
	}

	result, err := ReconcileOfflineAfterDownload(mgr.db, &models.DownloadTask{GID: "200"})
	if err != nil {
		t.Fatalf("ReconcileOfflineAfterDownload 失败: %v", err)
	}
	if len(result.DedupItems) != 1 {
		t.Fatalf("应生成 1 条去重建议（删除压缩包 B），得到 %d", len(result.DedupItems))
	}
	di := result.DedupItems[0]
	if di.Comic.ID != "reconcile-zip-b" || di.Keep {
		t.Errorf("应建议删除压缩包 B（Keep=false），得到 %q keep=%v", di.Comic.ID, di.Keep)
	}
	if di.PairComic == nil || di.PairComic.ID != "reconcile-zip-dir-a" {
		t.Errorf("成对对象应为文件夹 A，得到 %+v", di.PairComic)
	}
	// ParentGID 回写仍生效（A 目录有 metadata parent_gid=999）
	if result.ParentGIDWritten != 1 {
		t.Errorf("ParentGID 应回写 1 条，得到 %d", result.ParentGIDWritten)
	}
	var gotA models.OfflineComic
	if err := mgr.db.First(&gotA, "id = ?", "reconcile-zip-dir-a").Error; err != nil {
		t.Fatalf("读取 A 失败: %v", err)
	}
	if gotA.ParentGID != "999" {
		t.Errorf("A.ParentGID 应为 999，得到 %q", gotA.ParentGID)
	}
}

func TestReconcileOfflineAfterDownloadNoDedupSingle(t *testing.T) {
	mgr := newTestDownloadManager(t)
	if err := mgr.db.AutoMigrate(&models.OfflineComic{}); err != nil {
		t.Fatalf("迁移 OfflineComic 失败: %v", err)
	}
	dirA := t.TempDir()
	writeTestMetadata(t, dirA, `{"gid":"300","token":"t300","filecount":8}`)
	if err := mgr.db.Create(&models.OfflineComic{
		ID:        "reconcile-single",
		GID:       "300",
		Title:     "single",
		LocalPath: dirA,
		PageCount: 2,
	}).Error; err != nil {
		t.Fatalf("创建漫画失败: %v", err)
	}

	result, err := ReconcileOfflineAfterDownload(mgr.db, &models.DownloadTask{GID: "300"})
	if err != nil {
		t.Fatalf("ReconcileOfflineAfterDownload 失败: %v", err)
	}
	if len(result.DedupItems) != 0 {
		t.Errorf("单条记录不应生成去重建议，得到 %d", len(result.DedupItems))
	}
	if result.PageCountCorrected != 1 {
		t.Errorf("PageCount 应校正 1 条，得到 %d", result.PageCountCorrected)
	}
}

func TestReconcileOfflineAfterDownloadEdge(t *testing.T) {
	mgr := newTestDownloadManager(t)
	if err := mgr.db.AutoMigrate(&models.OfflineComic{}); err != nil {
		t.Fatalf("迁移 OfflineComic 失败: %v", err)
	}

	// db=nil / task=nil / gid 空 / 无匹配记录 → 返回空结果，无错误
	for _, tc := range []struct {
		db   *gorm.DB
		task *models.DownloadTask
	}{
		{nil, &models.DownloadTask{GID: "1"}},
		{mgr.db, nil},
		{mgr.db, &models.DownloadTask{GID: ""}},
		{mgr.db, &models.DownloadTask{GID: "not-exist-gid"}},
	} {
		result, err := ReconcileOfflineAfterDownload(tc.db, tc.task)
		if err != nil {
			t.Fatalf("边界场景应无错误: %v", err)
		}
		if result == nil || len(result.DedupItems) != 0 || result.ParentGIDWritten != 0 {
			t.Errorf("边界场景应返回空结果，得到 %+v", result)
		}
	}
}
