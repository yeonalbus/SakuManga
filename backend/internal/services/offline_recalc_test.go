package services

import (
	"os"
	"path/filepath"
	"testing"

	"SakuManga/internal/models"
)

// ─────────────────────────────────────────────────────────────
// Round53 阶段三：D20 恢复忽略后的本地重算（不联网）
//
// RecalculateDedupLocal 只跑本地规则：跳过在线回填 GID、规则 3a 在线父子关系
// 发现与 Tier2 联网复核；本地规则 1/2/4/5 与 3b 本地父画廊检查照常执行。
// ─────────────────────────────────────────────────────────────

func TestRecalculateDedupLocal(t *testing.T) {
	db := newRefsTestDB(t)
	withoutXpCloud(t)
	if err := db.AutoMigrate(&models.MaintainDedupSnapshot{}, &models.DedupSetting{}); err != nil {
		t.Fatalf("迁移快照表失败: %v", err)
	}

	// 规则 1（同 GID）：本地规则，不联网；保留文件夹形态、删除归档形态
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "1.jpg"), "x")
	zipPath := filepath.Join(t.TempDir(), "x.zip")
	if err := os.WriteFile(zipPath, []byte("zip-bytes"), 0o644); err != nil {
		t.Fatalf("写归档失败: %v", err)
	}
	for _, c := range []*models.OfflineComic{
		{ID: generateID(dir), GID: "100", Title: "A", LocalPath: dir, PageCount: 1, SourceMode: "gallery"},
		{ID: generateID(zipPath), GID: "100", Title: "B", LocalPath: zipPath, PageCount: 1, SourceMode: "archive"},
	} {
		if err := db.Create(c).Error; err != nil {
			t.Fatalf("创建记录失败: %v", err)
		}
	}

	res, err := RecalculateDedupLocal(db)
	if err != nil {
		t.Fatalf("本地重算失败: %v", err)
	}
	keepCount, removeCount := 0, 0
	for _, item := range res.Items {
		if item.Rule != "gid" {
			continue
		}
		if item.Keep {
			keepCount++
		} else {
			removeCount++
		}
	}
	if keepCount != 1 || removeCount != 1 {
		t.Fatalf("同 GID 组应输出 1 条建议保留 + 1 条删除，实际 keep=%d remove=%d（items=%+v）",
			keepCount, removeCount, res.Items)
	}
	// 本地重算不应产出 parent 规则项（那需要联网或本地 parent_gid）
	for _, item := range res.Items {
		if item.Rule == "parent" {
			t.Errorf("本地重算不应产出 parent 规则项: %+v", item)
		}
	}
}

// TestRecalculateDedupLocalIgnoresOnlineService 传 nil ehService 也不应报错（本地路径安全）。
func TestRecalculateDedupLocalWithNilService(t *testing.T) {
	db := newRefsTestDB(t)
	withoutXpCloud(t)
	if err := db.AutoMigrate(&models.MaintainDedupSnapshot{}, &models.DedupSetting{}); err != nil {
		t.Fatalf("迁移快照表失败: %v", err)
	}
	if _, err := RecalculateDedupLocal(db); err != nil {
		t.Fatalf("空库本地重算失败: %v", err)
	}
}
