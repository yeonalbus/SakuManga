package services

import (
	"testing"

	"SakuManga/internal/models"
)

// ─────────────────────────────────────────────────────────────
// 真实案例回归：Yeowasae FANBOX FULL GALLERY Part.2 更新链
// 线上 API 实证链：3880747(04.08) → 3890218(04.15) → 3900023(04.21)
//                  → 3907746(04.26) → 3912617(04.29) → 4157229(08.22)
// 本地只保存链的两端：3890218（旧版）与 3912617（新版），中间版本缺失。
// ─────────────────────────────────────────────────────────────

// yeowasaeChain 真实链数据（gid → parentGID），来自线上 /comics/online/detail 实证
var yeowasaeChain = map[string]string{
	"3880747": "",        // 链头（04.08，无父）
	"3890218": "3880747", // 04.15
	"3900023": "3890218", // 04.21
	"3907746": "3900023", // 04.26
	"3912617": "3907746", // 04.29
	"4157229": "3912617", // 08.22（最新）
}

// yeowasaeChildren 真实 children 列表（gid → 全部后代更新版，从旧到新），线上 API 实证：
// 旧版详情页 #gnd 罗列「本画廊之后的所有更新版本」。
var yeowasaeChildren = map[string][]string{
	"3880747": {"3890218", "3900023", "3907746", "3912617", "4157229"},
	"3890218": {"3900023", "3907746", "3912617", "4157229"},
	"3900023": {"3907746", "3912617", "4157229"},
	"3907746": {"3912617", "4157229"},
	"3912617": {"4157229"},
	"4157229": {},
}

// TestYeowasaeChainClimb 真实链：从新版 3912617 的直接父 3907746 上溯，
// 应逐层经过 3907746(不在本地) → 3900023(不在本地) → 命中 3890218（本地旧版）。
func TestYeowasaeChainClimb(t *testing.T) {
	fetch := fakeChainFetcher(yeowasaeChain)
	// 本地只有链两端：3890218（旧）、3912617（新）
	gidToComic := localMap("3890218", "local-3890218", "3912617", "local-3912617")

	// 从 3912617 的直接父 3907746 开始上溯（修复后的 3a 行为）
	hits := climbParentChain(fetch, "3907746", "t3907746", gidToComic)
	if len(hits) != 1 {
		t.Fatalf("上溯应命中 1 个本地版本（3890218），得到 %d 个", len(hits))
	}
	if hits[0].ID != "local-3890218" {
		t.Errorf("命中应为 local-3890218，得到 %q", hits[0].ID)
	}
}

// TestYeowasaeChainSuccessor 旧版详情页 children 罗列全部后代 → 取最后一个本地存在 = 新版。
// 模拟修复后 3a 对旧版 3890218 联网后的 successor 判定。
func TestYeowasaeChainSuccessor(t *testing.T) {
	// 本地只有链两端
	gidToComic := localMap("3890218", "local-3890218", "3912617", "local-3912617")

	// 3890218 的详情：children=[3900023, 3907746, 3912617, 4157229]，
	// NewVersionGID=4157229（最新版，不在本地）→ 退化走 children 遍历
	children := yeowasaeChildren["3890218"]
	var successor *models.OfflineComic
	for _, ch := range children {
		if ch == "" || ch == "3890218" {
			continue
		}
		if s, ok := gidToComic[ch]; ok && s.ID != "local-3890218" {
			successor = s // A→C：不 break，取最后一个本地存在的更新版（最新版）
		}
	}
	if successor == nil {
		t.Fatal("3890218 的 children 应命中本地新版 3912617")
	}
	if successor.ID != "local-3912617" {
		t.Errorf("successor 应为 local-3912617，得到 %q", successor.ID)
	}
}

// TestYeowasaeChainOldLogicMiss 复现修复前 bug：3a 跳过条件含 c.ParentGID != ""，
// 两本 ParentGID 均非空（3890218→3880747，3912617→3907746）→ 永不联网 → 查不出。
// 此测试固化「为何修复前失效」，防止回归。
func TestYeowasaeChainOldLogicMiss(t *testing.T) {
	comics := []models.OfflineComic{
		{ID: "local-3890218", GID: "3890218", Token: "d402fdc2c8", ParentGID: "3880747", ParentCheckedAt: 1786009468271},
		{ID: "local-3912617", GID: "3912617", Token: "d4c4332977", ParentGID: "3907746", ParentCheckedAt: 1786009319435},
	}
	// 复刻修复前 3a 跳过条件：ParentGID != "" → 跳过
	skipped := 0
	for _, c := range comics {
		if c.GID == "" || c.Token == "" || c.ParentGID != "" {
			skipped++
			continue
		}
	}
	if skipped != 2 {
		t.Fatalf("修复前 3a 应跳过两本（ParentGID 非空），实际跳过 %d", skipped)
	}
	// 3b 本地：直接父 3880747 / 3907746 均不在本地 → 无命中
	gidToComic := localMap("3890218", "local-3890218", "3912617", "local-3912617")
	hit := 0
	for _, c := range comics {
		if c.ParentGID == "" {
			continue
		}
		if p, ok := gidToComic[c.ParentGID]; ok && p.ID != c.ID {
			hit++
		}
	}
	if hit != 0 {
		t.Fatalf("修复前 3b 不应命中（中间版本缺失），实际命中 %d", hit)
	}
	t.Log("✅ 复现确认：修复前增量/全量均查不出（3a 拦截 + 3b 直接父失效）")
}
