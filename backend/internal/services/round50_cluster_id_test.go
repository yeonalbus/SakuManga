package services

import (
	"strings"
	"testing"

	"SakuManga/internal/models"
)

// ─────────────────────────────────────────────────────────────
// Round50 B：疑似重复簇的排序与 ID 稳定化
//
// 覆盖：
//   - 同输入重复聚类 → 组顺序与 ID 完全一致（确定性）；
//   - 删除簇内一个成员 → 该簇 ID 保持不变（ID 由内容键派生，与位置/成员数无关）；
//   - 成员数相同的簇按「置信度 → 核心名 → 画师」稳定排序。
// ─────────────────────────────────────────────────────────────

// mkClusterComic 构造簇测试用漫画（同标题同画师 → 归入同一簇）
func mkClusterComic(id, title, artist string, pages int, aged bool) models.OfflineComic {
	c := mkComic(id, title, []string{"artist:" + artist, "language:japanese"}, pages)
	c.Source = models.SourceOffline
	c.AgedStatus = aged
	c.LocalPath = "/nonexistent/" + id
	return c
}

// TestClusterIDStableAcrossRuns 同输入重复聚类：顺序与 ID 必须完全一致
func TestClusterIDStableAcrossRuns(t *testing.T) {
	resetMaintainResultForTest(t)
	db := newSyncTestDB(t)
	comics := []models.OfflineComic{
		mkClusterComic("p1", "[サークル] アルファ", "a1", 20, false),
		mkClusterComic("p2", "[サークル] アルファ (English)", "a1", 21, false),
		mkClusterComic("p3", "[サークル] アルファ 汉化版", "a1", 22, false),
		mkClusterComic("q1", "[サークル] ベータ", "a1", 30, false),
		mkClusterComic("q2", "[サークル] ベータ (English)", "a1", 31, false),
	}
	idx := LoadIgnoreIndex(db)

	first := detectTitleClusters(comics, idx)
	if len(first) != 2 {
		t.Fatalf("预期 2 簇，得到 %d", len(first))
	}
	sig := func(cls []DedupCluster) string {
		var b strings.Builder
		for _, c := range cls {
			b.WriteString(c.ID + ":" + c.TitleKey + ";")
		}
		return b.String()
	}
	want := sig(first)
	for i := 0; i < 2; i++ {
		if got := sig(detectTitleClusters(comics, idx)); got != want {
			t.Fatalf("第 %d 次重算的顺序/ID 不一致:\n want %s\n  got %s", i+2, want, got)
		}
	}
	for _, c := range first {
		if !strings.HasPrefix(c.ID, "cluster-") {
			t.Errorf("ID 应以 cluster- 开头，得到 %q", c.ID)
		}
		if strings.Contains(c.ID, "cluster-0") || strings.Contains(c.ID, "cluster-1") {
			t.Errorf("ID 不应再按下标生成（避免重排后 key 全变），得到 %q", c.ID)
		}
	}
}

// TestClusterIDStableAfterMemberRemoved 删掉簇内一个成员后，该簇 ID 必须不变
func TestClusterIDStableAfterMemberRemoved(t *testing.T) {
	resetMaintainResultForTest(t)
	db := newSyncTestDB(t)
	comics := []models.OfflineComic{
		mkClusterComic("p1", "[サークル] アルファ", "a1", 20, false),
		mkClusterComic("p2", "[サークル] アルファ (English)", "a1", 21, false),
		mkClusterComic("p3", "[サークル] アルファ 汉化版", "a1", 22, false),
	}
	idx := LoadIgnoreIndex(db)

	before := detectTitleClusters(comics, idx)
	if len(before) != 1 {
		t.Fatalf("预期 1 簇，得到 %d", len(before))
	}
	idBefore := before[0].ID

	// 去掉 p3 后重算（模拟删除后的全量重算；A 的定向裁剪路径同样保持 ID）
	remain := []models.OfflineComic{comics[0], comics[1]}
	after := detectTitleClusters(remain, idx)
	if len(after) != 1 {
		t.Fatalf("预期删成员后仍有 1 簇，得到 %d", len(after))
	}
	if after[0].ID != idBefore {
		t.Errorf("删成员后簇 ID 应保持不变（前端 :key 与展开态依赖它）: before=%s after=%s",
			idBefore, after[0].ID)
	}
	if len(after[0].Members) != 2 {
		t.Errorf("成员数应为 2，得到 %d", len(after[0].Members))
	}
}

// TestClusterSortStableTieBreak 成员数相同的簇按「置信度 → 核心名」稳定排序
func TestClusterSortStableTieBreak(t *testing.T) {
	resetMaintainResultForTest(t)
	db := newSyncTestDB(t)
	comics := []models.OfflineComic{
		// zeta 簇（2 本，标题 z 开头）
		mkClusterComic("z1", "[サークル] ゼータ", "a2", 40, false),
		mkClusterComic("z2", "[サークル] ゼータ (English)", "a2", 41, false),
		// alpha 簇（2 本，标题 a 开头）→ 应排在 zeta 之前
		mkClusterComic("a1", "[サークル] アルファ", "a1", 20, false),
		mkClusterComic("a2c", "[サークル] アルファ (English)", "a1", 21, false),
	}
	idx := LoadIgnoreIndex(db)
	cls := detectTitleClusters(comics, idx)
	if len(cls) != 2 {
		t.Fatalf("预期 2 簇，得到 %d", len(cls))
	}
	if cls[0].TitleKey > cls[1].TitleKey {
		t.Errorf("同成员数的簇应按核心名升序排列，得到 %q、%q", cls[0].TitleKey, cls[1].TitleKey)
	}
}
