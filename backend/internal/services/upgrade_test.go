package services

import (
	"testing"

	"SakuHentai/internal/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ─────────────────────────────────────────────────────────────
// 画质升级：有效方案解析 + 候选列表筛选
// ─────────────────────────────────────────────────────────────

// TestResolveEffectiveDownloadScheme 覆盖「新数据直取 / 存量按形态推断」：
//   - 新数据：download_scheme 非空直接返回；
//   - 存量（空）：archive（压缩包形态）→ 归档原图（已达标排除），gallery（文件夹）→ 画廊下载。
func TestResolveEffectiveDownloadScheme(t *testing.T) {
	cases := []struct {
		name string
		comic *models.OfflineComic
		want string
	}{
		{"已知归档压缩", &models.OfflineComic{DownloadScheme: string(models.DefaultSchemeArchiveResample)}, string(models.DefaultSchemeArchiveResample)},
		{"已知归档原图", &models.OfflineComic{DownloadScheme: string(models.DefaultSchemeArchiveOriginal)}, string(models.DefaultSchemeArchiveOriginal)},
		{"已知画廊", &models.OfflineComic{DownloadScheme: string(models.DefaultSchemeGallery)}, string(models.DefaultSchemeGallery)},
		{"存量归档推断为原图", &models.OfflineComic{SourceMode: "archive"}, string(models.DefaultSchemeArchiveOriginal)},
		{"存量画廊推断为画廊下载", &models.OfflineComic{SourceMode: "gallery"}, string(models.DefaultSchemeGallery)},
		{"存量无形态兜底画廊", &models.OfflineComic{}, string(models.DefaultSchemeGallery)},
		{"nil 返回空", nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ResolveEffectiveDownloadScheme(tc.comic); got != tc.want {
				t.Fatalf("ResolveEffectiveDownloadScheme() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestListUpgradeCandidates 验证候选筛选条件（元数据为唯一标准）：
//   - 纳入：SakuHentai 下载（scan_path_id 空）且有 gid/token；已知方案非归档原图；存量画廊形态
//   - 排除：存量归档形态（推断为归档原图）、已知归档原图、额外路径导入、无 gid、被删/移除
func TestListUpgradeCandidates(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("打开内存库失败: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("获取底层连接失败: %v", err)
	}
	sqlDB.SetMaxOpenConns(1) // 内存库每连接独立，固定单连接
	if err := db.AutoMigrate(&models.OfflineComic{}); err != nil {
		t.Fatalf("迁移 OfflineComic 失败: %v", err)
	}

	comics := []*models.OfflineComic{
		{ID: "c1", Title: "存量画廊", GID: "11", Token: "t", LocalPath: "/g/11", SourceMode: "gallery"},                                             // 纳入（存量画廊 → 画廊下载）
		{ID: "c2", Title: "存量归档", GID: "12", Token: "t", LocalPath: "/a/12.zip", SourceMode: "archive"},                                           // 排除（存量归档 → 推断原图）
		{ID: "c3", Title: "已知归档压缩", GID: "13", Token: "t", LocalPath: "/a/13", SourceMode: "gallery", DownloadScheme: string(models.DefaultSchemeArchiveResample)}, // 纳入
		{ID: "c4", Title: "已知归档原图", GID: "14", Token: "t", LocalPath: "/a/14", SourceMode: "gallery", DownloadScheme: string(models.DefaultSchemeArchiveOriginal)}, // 排除（已达标）
		{ID: "c5", Title: "已知画廊", GID: "15", Token: "t", LocalPath: "/g/15", SourceMode: "gallery", DownloadScheme: string(models.DefaultSchemeGallery)},             // 纳入
		{ID: "c6", Title: "额外路径导入", GID: "16", Token: "t", LocalPath: "/x/16", SourceMode: "gallery", ScanPathID: "sp1"},                                          // 排除（非 SakuHentai 下载）
		{ID: "c7", Title: "无 gid", Token: "t", LocalPath: "/g/17", SourceMode: "gallery"},                                                                               // 排除（无 E 站元数据）
		{ID: "c8", Title: "已被移除", GID: "18", Token: "t", LocalPath: "/g/18", SourceMode: "gallery", RemovedStatus: true},                                             // 排除（被删无法下载）
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
	// 期望纳入：c1（推断画廊下载）、c3（归档压缩）、c5（画廊下载）
	for _, id := range []string{"c1", "c3", "c5"} {
		if _, ok := got[id]; !ok {
			t.Fatalf("候选缺失 %s（应纳入），实际候选: %v", id, got)
		}
	}
	// 期望排除：c2（存量归档推断原图）、c4（归档原图）、c6（额外路径）、c7（无 gid）、c8（被移除）
	for _, id := range []string{"c2", "c4", "c6", "c7", "c8"} {
		if _, ok := got[id]; ok {
			t.Fatalf("%s 不应成为候选，实际候选: %v", id, got)
		}
	}
	// 存量画廊推断后的方案应展示为「画廊下载」，而非空/未知
	if got["c1"] != string(models.DefaultSchemeGallery) {
		t.Fatalf("c1 存量画廊应推断为画廊下载，实际 %q", got["c1"])
	}
	if got["c3"] != string(models.DefaultSchemeArchiveResample) {
		t.Fatalf("c3 应展示归档压缩，实际 %q", got["c3"])
	}
}
