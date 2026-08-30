package services

import (
	"errors"
	"testing"

	"SakuManga/internal/models"
)

// ─────────────────────────────────────────────────────────────
// 3a 父链上溯（climbParentChain）单测——隔代祖孙发现
// 本地只保存更新链两端（中间版本缺失）时，3b 直接父检查失效，
// 需沿父画廊链逐层上溯命中本地版本。
// ─────────────────────────────────────────────────────────────

// fakeChainFetcher 用内存 map 模拟详情抓取：gid → 该画廊的父画廊 gid。
// parent=="" 表示链头；unknown 的 gid 返回错误（模拟网络失败）。
func fakeChainFetcher(chain map[string]string) parentDetailFetcher {
	return func(gid, token string) (*GalleryDetailResult, error) {
		parent, ok := chain[gid]
		if !ok {
			return nil, errors.New("模拟网络失败")
		}
		return &GalleryDetailResult{ID: gid, Token: token, ParentGID: parent}, nil
	}
}

// localMap 从 (gid, id) 对构建 gidToComic 索引
func localMap(gidIDs ...string) map[string]*models.OfflineComic {
	m := map[string]*models.OfflineComic{}
	for i := 0; i+1 < len(gidIDs); i += 2 {
		gid, id := gidIDs[i], gidIDs[i+1]
		m[gid] = &models.OfflineComic{ID: id, GID: gid, Title: "gid=" + gid}
	}
	return m
}

// TestClimbParentChainGrandparent 核心场景：链 A→B→C→D，本地只有 B 与 D（隔代）。
// 从 D 的直接父 C 开始上溯，应命中 B（C 不在本地，A 不在本地）。
func TestClimbParentChainGrandparent(t *testing.T) {
	// 链：A(100) → B(101) → C(102) → D(103)，即 103 的父是 102，102 的父是 101，101 的父是 100
	chain := map[string]string{"100": "", "101": "100", "102": "101", "103": "102"}
	fetch := fakeChainFetcher(chain)
	// 本地：B=101（旧版）、D=103（新版）
	gidToComic := localMap("101", "local-b", "103", "local-d")

	// 从 D 的直接父 C=102 上溯 → 期望命中 B=101
	hits := climbParentChain(fetch, "102", "t102", gidToComic)
	if len(hits) != 1 {
		t.Fatalf("期望命中 1 个本地版本，得到 %d 个", len(hits))
	}
	if hits[0].ID != "local-b" {
		t.Errorf("命中应为 local-b(gid=101)，得到 %q", hits[0].ID)
	}
}

// TestClimbParentChainMultipleLocal 链上多个本地版本：A→B→C→D，本地有 A、C、D。
// 从 D 的直接父 C 上溯 → 应命中 C（近）与 A（远，隔 C 的父 B）。
func TestClimbParentChainMultipleLocal(t *testing.T) {
	chain := map[string]string{"100": "", "101": "100", "102": "101", "103": "102"}
	fetch := fakeChainFetcher(chain)
	gidToComic := localMap("100", "local-a", "102", "local-c", "103", "local-d")

	hits := climbParentChain(fetch, "102", "t102", gidToComic)
	if len(hits) != 2 {
		t.Fatalf("期望命中 2 个本地版本（C、A），得到 %d 个", len(hits))
	}
	if hits[0].ID != "local-c" {
		t.Errorf("最近命中应为 local-c(gid=102)，得到 %q", hits[0].ID)
	}
	if hits[1].ID != "local-a" {
		t.Errorf("次近命中应为 local-a(gid=100)，得到 %q", hits[1].ID)
	}
}

// TestClimbParentChainDirectParent 直接父在本地（常规 3b 场景）也走通。
func TestClimbParentChainDirectParent(t *testing.T) {
	chain := map[string]string{"100": "", "101": "100"}
	fetch := fakeChainFetcher(chain)
	gidToComic := localMap("100", "local-a", "101", "local-b")
	hits := climbParentChain(fetch, "100", "t100", gidToComic)
	if len(hits) != 1 || hits[0].ID != "local-a" {
		t.Fatalf("期望命中 local-a，得到 %+v", hits)
	}
}

// TestClimbParentChainDepthLimit 层数上限：超长链（>maxParentClimb）截断不越界。
func TestClimbParentChainDepthLimit(t *testing.T) {
	chain := map[string]string{}
	// 构造 100 → 101 → 102 → ... → 100+maxParentClimb+5 的长链
	for i := 0; i < maxParentClimb+5; i++ {
		chain[itoa(i)] = itoa(i + 1) // gid=i 的父是 gid=i+1
	}
	// 本地只有链尾（最旧端）——在深度上限之外，不应命中
	gidToComic := localMap(itoa(maxParentClimb+5), "local-tail")
	fetch := fakeChainFetcher(chain)
	hits := climbParentChain(fetch, "0", "t0", gidToComic)
	if len(hits) != 0 {
		t.Errorf("超深度上限的本地版本不应命中，得到 %d 个", len(hits))
	}
}

// TestClimbParentChainCycle 环检测：A→B→C→A 不应死循环。
func TestClimbParentChainCycle(t *testing.T) {
	chain := map[string]string{"100": "102", "101": "100", "102": "101"} // 100→102→101→100 成环
	fetch := fakeChainFetcher(chain)
	gidToComic := localMap("100", "local-a")
	hits := climbParentChain(fetch, "102", "t102", gidToComic)
	// 最多执行 maxParentClimb 层，且 A=100 在第 2 层命中
	if len(hits) != 1 || hits[0].ID != "local-a" {
		t.Fatalf("环内命中 local-a 一次，得到 %+v", hits)
	}
}

// TestClimbParentChainFetchFail 某层抓取失败 → 截断，已收集命中仍保留。
func TestClimbParentChainFetchFail(t *testing.T) {
	chain := map[string]string{"100": "", "101": "100"}
	// 本地有 A=100；上溯先抓 C=102（失败）再抓 A —— C 失败即截断，不应命中 A
	gidToComic := localMap("100", "local-a")
	fetch := fakeChainFetcher(chain) // 102 不在 chain 中 → 失败
	hits := climbParentChain(fetch, "102", "t102", gidToComic)
	if len(hits) != 0 {
		t.Errorf("首层失败应截断且无命中，得到 %d 个", len(hits))
	}
}

// itoa 极简 int→string（测试内联，避免 strconv 噪音）
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
