package services

import (
	"testing"
	"time"

	"SakuManga/internal/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ─────────────────────────────────────────────────────────────
// 画质升级：有效方案解析 + 候选列表筛选
//
// 判定链：download_scheme（新数据回填）→ download_tasks 最近完成任务方案
//（下载列表展示的「归档·原图/归档·压缩/画廊」）→ 落地目录名推断
//（archive - 前缀 = 归档视为原图；普通目录 = 画廊）。
// ─────────────────────────────────────────────────────────────

// TestResolveEffectiveDownloadScheme 覆盖三级判定链
func TestResolveEffectiveDownloadScheme(t *testing.T) {
	// 任务索引：gid → 方案
	taskIdx := map[string]string{
		"200": string(models.DefaultSchemeArchiveOriginal), // 任务：归档原图
		"300": string(models.DefaultSchemeGallery),         // 任务：画廊
		"400": string(models.DefaultSchemeArchiveResample), // 任务：归档压缩
	}
	cases := []struct {
		name  string
		comic *models.OfflineComic
		idx   map[string]string
		want  string
	}{
		// ① 新数据：download_scheme 直接生效
		{"新数据归档压缩", &models.OfflineComic{DownloadScheme: string(models.DefaultSchemeArchiveResample)}, taskIdx, string(models.DefaultSchemeArchiveResample)},
		{"新数据归档原图", &models.OfflineComic{DownloadScheme: string(models.DefaultSchemeArchiveOriginal)}, taskIdx, string(models.DefaultSchemeArchiveOriginal)},
		{"新数据画廊", &models.OfflineComic{DownloadScheme: string(models.DefaultSchemeGallery)}, taskIdx, string(models.DefaultSchemeGallery)},
		// ② 存量：任务索引命中（gid 匹配最近完成任务）
		{"存量任务归档原图", &models.OfflineComic{GID: "200", LocalPath: `Z:\Comics\archive - 200 - x`}, taskIdx, string(models.DefaultSchemeArchiveOriginal)},
		{"存量任务画廊", &models.OfflineComic{GID: "300", LocalPath: `Z:\Comics\300 - x`}, taskIdx, string(models.DefaultSchemeGallery)},
		{"存量任务归档压缩", &models.OfflineComic{GID: "400", LocalPath: `Z:\Comics\archive - 400 - x`}, taskIdx, string(models.DefaultSchemeArchiveResample)},
		// ③ 存量：任务索引未命中 → 目录名推断
		{"存量无任务归档目录推断原图", &models.OfflineComic{GID: "500", LocalPath: `Z:\Comics\archive - 500 - title`}, taskIdx, string(models.DefaultSchemeArchiveOriginal)},
		{"存量无任务普通目录推断画廊", &models.OfflineComic{GID: "600", LocalPath: `Z:\Comics\600 - title`}, taskIdx, string(models.DefaultSchemeGallery)},
		// ④ 边界
		{"nil 返回空", nil, taskIdx, ""},
		{"空索引回退目录推断", &models.OfflineComic{GID: "700", LocalPath: `D:\data\archive - 700 - t`}, nil, string(models.DefaultSchemeArchiveOriginal)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ResolveEffectiveDownloadScheme(tc.comic, tc.idx); got != tc.want {
				t.Fatalf("ResolveEffectiveDownloadScheme() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestBuildTaskSchemeIndex 验证任务索引构建：仅 completed、同 gid 取最新
func TestBuildTaskSchemeIndex(t *testing.T) {
	db := newUpgradeTestDB(t)
	base := time.Now().Add(-time.Hour)
	tasks := []*models.DownloadTask{
		{ID: "t1", GID: "10", Mode: models.DownloadModeGallery, Status: models.DownloadCompleted, UpdatedAt: base},                                      // 画廊（旧）
		{ID: "t2", GID: "10", Mode: models.DownloadModeArchive, ArchiveType: models.ArchiveTypeOriginal, Status: models.DownloadCompleted, UpdatedAt: base.Add(10 * time.Minute)}, // 归档原图（新）→ 应胜出
		{ID: "t3", GID: "20", Mode: models.DownloadModeArchive, ArchiveType: models.ArchiveTypeResample, Status: models.DownloadCompleted, UpdatedAt: base.Add(20 * time.Minute)},
		{ID: "t4", GID: "30", Mode: models.DownloadModeArchive, ArchiveType: models.ArchiveTypeOriginal, Status: models.DownloadCancelled, UpdatedAt: base.Add(30 * time.Minute)}, // cancelled 不计
	}
	for _, tsk := range tasks {
		if err := db.Create(tsk).Error; err != nil {
			t.Fatalf("插入任务失败: %v", err)
		}
	}
	idx := BuildTaskSchemeIndex(db)
	if idx["10"] != string(models.DefaultSchemeArchiveOriginal) {
		t.Fatalf("gid=10 应取最新归档原图任务，实际 %q", idx["10"])
	}
	if idx["20"] != string(models.DefaultSchemeArchiveResample) {
		t.Fatalf("gid=20 应为归档压缩，实际 %q", idx["20"])
	}
	if _, ok := idx["30"]; ok {
		t.Fatalf("gid=30 的任务被取消，不应进入索引")
	}
}

// TestListUpgradeCandidates 验证候选筛选（元数据为唯一标准）：
//   - 纳入：存量画廊目录、任务为画廊/归档压缩、已知非归档原图
//   - 排除：存量归档目录（推断原图）、任务为归档原图、已知归档原图、
//     额外路径导入、无 gid、被删/移除
func TestListUpgradeCandidates(t *testing.T) {
	db := newUpgradeTestDB(t)
	now := time.Now()

	// 下载任务记录（gid → 方案；覆盖部分存量）
	tasks := []*models.DownloadTask{
		{ID: "t-arch-orig", GID: "200", Mode: models.DownloadModeArchive, ArchiveType: models.ArchiveTypeOriginal, Status: models.DownloadCompleted, UpdatedAt: now},
		{ID: "t-gallery", GID: "300", Mode: models.DownloadModeGallery, Status: models.DownloadCompleted, UpdatedAt: now},
		{ID: "t-arch-res", GID: "400", Mode: models.DownloadModeArchive, ArchiveType: models.ArchiveTypeResample, Status: models.DownloadCompleted, UpdatedAt: now},
	}
	for _, tsk := range tasks {
		if err := db.Create(tsk).Error; err != nil {
			t.Fatalf("插入任务失败: %v", err)
		}
	}

	comics := []*models.OfflineComic{
		{ID: "c1", Title: "存量画廊目录", GID: "11", Token: "t", LocalPath: `Z:\Comics\11 - x`, SourceMode: "gallery"},                                                              // 纳入（无任务 + 普通目录 → 画廊）
		{ID: "c2", Title: "存量归档目录", GID: "12", Token: "t", LocalPath: `Z:\Comics\archive - 12 - x`, SourceMode: "gallery"},                                                   // 排除（无任务 + archive 目录 → 推断原图）
		{ID: "c3", Title: "任务归档原图", GID: "200", Token: "t", LocalPath: `Z:\Comics\200 - x`, SourceMode: "gallery"},                                                           // 排除（任务=归档原图）
		{ID: "c4", Title: "任务画廊", GID: "300", Token: "t", LocalPath: `Z:\Comics\300 - x`, SourceMode: "gallery"},                                                               // 纳入（任务=画廊）
		{ID: "c5", Title: "任务归档压缩", GID: "400", Token: "t", LocalPath: `Z:\Comics\archive - 400 - x`, SourceMode: "gallery"},                                                 // 纳入（任务=归档压缩）
		{ID: "c6", Title: "已知归档压缩", GID: "13", Token: "t", LocalPath: `Z:\Comics\13 - x`, SourceMode: "gallery", DownloadScheme: string(models.DefaultSchemeArchiveResample)}, // 纳入（新数据）
		{ID: "c7", Title: "已知归档原图", GID: "14", Token: "t", LocalPath: `Z:\Comics\14 - x`, SourceMode: "gallery", DownloadScheme: string(models.DefaultSchemeArchiveOriginal)}, // 排除（新数据已达标）
		{ID: "c8", Title: "额外路径导入", GID: "16", Token: "t", LocalPath: `Z:\X\16 - x`, SourceMode: "gallery", ScanPathID: "sp1"},                                               // 排除（非 SakuManga 下载）
		{ID: "c9", Title: "无 gid", Token: "t", LocalPath: `Z:\Comics\17 - x`, SourceMode: "gallery"},                                                                              // 排除（无 E 站元数据）
		{ID: "c10", Title: "已被移除", GID: "18", Token: "t", LocalPath: `Z:\Comics\18 - x`, SourceMode: "gallery", RemovedStatus: true},                                           // 排除（被删无法下载）
	}
	for _, c := range comics {
		if err := db.Create(c).Error; err != nil {
			t.Fatalf("插入测试数据失败 %s: %v", c.ID, err)
		}
	}

	items, err := ListUpgradeCandidates(db)
	if err != nil {
		t.Fatalf("ListUpgradeCandidates 失败: %v", err)
	}

	got := map[string]string{}
	for _, it := range items {
		got[it.ID] = it.DownloadScheme
	}
	// 期望纳入及其有效方案
	wantIn := map[string]string{
		"c1": string(models.DefaultSchemeGallery),
		"c4": string(models.DefaultSchemeGallery),
		"c5": string(models.DefaultSchemeArchiveResample),
		"c6": string(models.DefaultSchemeArchiveResample),
	}
	for id, scheme := range wantIn {
		if got[id] != scheme {
			t.Fatalf("%s 应纳入且方案=%q，实际 %q（候选: %v）", id, scheme, got[id], got)
		}
	}
	// 期望排除
	for _, id := range []string{"c2", "c3", "c7", "c8", "c9", "c10"} {
		if _, ok := got[id]; ok {
			t.Fatalf("%s 不应成为候选，实际候选: %v", id, got)
		}
	}
}

// newUpgradeTestDB 构造内存 sqlite（单连接）并迁移测试所需模型
func newUpgradeTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("打开内存库失败: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("获取底层连接失败: %v", err)
	}
	sqlDB.SetMaxOpenConns(1) // 内存库每连接独立，固定单连接
	if err := db.AutoMigrate(&models.OfflineComic{}, &models.DownloadTask{}); err != nil {
		t.Fatalf("迁移模型失败: %v", err)
	}
	return db
}
