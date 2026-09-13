package services

import (
	"testing"

	"SakuManga/internal/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// 在线画廊「已下载」状态挂载时同步回填 LocalID（本地记录 ID）的回归测试。
// 前端据此把在线详情页的「⬇️ 下载」替换为「📚 本地」并跳转本地画廊详情。

func newDownloadStateTestDB(t *testing.T) *gorm.DB {
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
	if err := db.AutoMigrate(&models.OfflineComic{}); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	return db
}

func seedOfflineComic(t *testing.T, db *gorm.DB, id, gid, localPath string) {
	t.Helper()
	if err := db.Create(&models.OfflineComic{
		ID:        id,
		GID:       gid,
		Title:     "t-" + id,
		LocalPath: localPath,
	}).Error; err != nil {
		t.Fatalf("写入本地漫画失败: %v", err)
	}
}

// 详情页：本地存在同 GID → IsDownloaded=true 且回填 LocalID
func TestAttachDetailDownloadState_FillsLocalID(t *testing.T) {
	db := newDownloadStateTestDB(t)
	seedOfflineComic(t, db, "md5-aaa", "1001", "Z:\\Comics\\a")

	detail := AttachDetailDownloadState(db, &GalleryDetailResult{ID: "1001"})
	if !detail.IsDownloaded {
		t.Fatal("本地存在同 GID 记录时 IsDownloaded 应为 true")
	}
	if detail.LocalID != "md5-aaa" {
		t.Fatalf("LocalID 应回填为本地记录 ID，得到 %q", detail.LocalID)
	}
}

// 详情页：本地无同 GID → 不标记已下载、不回填 LocalID
func TestAttachDetailDownloadState_NoLocalCopy(t *testing.T) {
	db := newDownloadStateTestDB(t)

	detail := AttachDetailDownloadState(db, &GalleryDetailResult{ID: "2002"})
	if detail.IsDownloaded {
		t.Fatal("本地无同 GID 记录时 IsDownloaded 不应为 true")
	}
	if detail.LocalID != "" {
		t.Fatalf("LocalID 应为空，得到 %q", detail.LocalID)
	}
}

// 详情页：nil 与空 ID 输入不得 panic
func TestAttachDetailDownloadState_NilSafety(t *testing.T) {
	db := newDownloadStateTestDB(t)
	if out := AttachDetailDownloadState(db, nil); out != nil {
		t.Fatal("nil 详情应原样返回 nil")
	}
	if out := AttachDetailDownloadState(db, &GalleryDetailResult{ID: ""}); out.LocalID != "" {
		t.Fatal("空 ID 详情不应回填 LocalID")
	}
}

// 列表页：仅已下载项回填 LocalID，其余保持原值
func TestAttachDownloadStates_FillsLocalID(t *testing.T) {
	db := newDownloadStateTestDB(t)
	seedOfflineComic(t, db, "md5-bbb", "3003", "Z:\\Comics\\b")

	comics := AttachDownloadStates(db, []OnlineComicDTO{
		{ID: "3003"},
		{ID: "4004"},
	})
	if len(comics) != 2 {
		t.Fatalf("期望 2 条，得到 %d", len(comics))
	}
	if !comics[0].IsDownloaded || comics[0].LocalID != "md5-bbb" {
		t.Fatalf("已下载项应回填 localId=md5-bbb，得到 isDownloaded=%v localId=%q",
			comics[0].IsDownloaded, comics[0].LocalID)
	}
	if comics[1].IsDownloaded || comics[1].LocalID != "" {
		t.Fatalf("未下载项不应被标记，得到 isDownloaded=%v localId=%q",
			comics[1].IsDownloaded, comics[1].LocalID)
	}
}

// 同一 GID 存在多条本地记录（重复扫描/替换残留）时，取 ID 升序首条，保证跳转目标稳定
func TestAttachDownloadStates_MultipleLocalRows_PicksFirstByID(t *testing.T) {
	db := newDownloadStateTestDB(t)
	seedOfflineComic(t, db, "bbbb", "5005", "Z:\\Comics\\b1")
	seedOfflineComic(t, db, "aaaa", "5005", "Z:\\Comics\\b2")

	comics := AttachDownloadStates(db, []OnlineComicDTO{{ID: "5005"}})
	if comics[0].LocalID != "aaaa" {
		t.Fatalf("多条本地记录应按 ID 升序取首条 aaaa，得到 %q", comics[0].LocalID)
	}

	detail := AttachDetailDownloadState(db, &GalleryDetailResult{ID: "5005"})
	if detail.LocalID != "aaaa" {
		t.Fatalf("详情页同 GID 多条本地记录应取 aaaa，得到 %q", detail.LocalID)
	}
}
