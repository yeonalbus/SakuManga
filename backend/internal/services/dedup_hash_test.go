package services

import (
	"os"
	"path/filepath"
	"testing"

	"SakuManga/internal/models"
)

// ─────────────────────────────────────────────────────────────
// Round53 阶段三：D19 规则 2（归档 hash）输出「建议保留」
//
// 与规则 4 对称：同 hash 组内的保留项必须写入 keepSet，否则维护页只看到
// 「删除重复项」而看不到「建议保留」，用户无法确认哪一份是基准。
// ─────────────────────────────────────────────────────────────

func TestHashRuleEmitsKeep(t *testing.T) {
	db := newRefsTestDB(t)
	withoutXpCloud(t)

	dir := t.TempDir()
	content := []byte("identical-archive-bytes") // 只校验字节 hash，不要求合法 zip
	p1 := filepath.Join(dir, "alpha.zip")
	p2 := filepath.Join(dir, "beta.zip")
	for _, p := range []string{p1, p2} {
		if err := os.WriteFile(p, content, 0o644); err != nil {
			t.Fatalf("写归档失败: %v", err)
		}
	}
	for _, c := range []*models.OfflineComic{
		{ID: generateID(p1), Title: "alpha", LocalPath: p1, SourceMode: "archive"},
		{ID: generateID(p2), Title: "beta", LocalPath: p2, SourceMode: "archive"},
	} {
		if err := db.Create(c).Error; err != nil {
			t.Fatalf("创建记录失败: %v", err)
		}
	}

	res, err := maintainDedupWithProgress(db, nil, nil, true, false)
	if err != nil {
		t.Fatalf("维护查重失败: %v", err)
	}

	keepCount, removeCount := 0, 0
	for _, item := range res.Items {
		if item.Rule != "hash" {
			continue
		}
		if item.Keep {
			keepCount++
		} else {
			removeCount++
		}
	}
	if keepCount != 1 || removeCount != 1 {
		t.Fatalf("同 hash 组应输出 1 条建议保留 + 1 条删除，实际 keep=%d remove=%d（items=%+v）",
			keepCount, removeCount, res.Items)
	}
}
