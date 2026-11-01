package services

import (
	"testing"

	"SakuManga/internal/models"
)

// ─────────────────────────────────────────────────────────────
// Round50：维护查重口径修复（G）+ 删除后定向裁剪（A）
//
// 覆盖：
//   - 本地查重候选集保留老化 / 失效本，仍排除「已关闭离线维护」的额外路径；
//   - 联网候选集仍排除老化 / 失效本（分层语义不被破坏）；
//   - 老化本之间同名 → 名称级疑似重复能被检出（G 的回归）；
//   - 删除后定向裁剪：成员收缩、不足 2 本整簇消失、簇顺序与 ID 不变、items 成对项处理。
// ─────────────────────────────────────────────────────────────

// TestFilterOfflineLocalCandidatesKeepsAged 本地候选 vs 联网候选的分层语义
func TestFilterOfflineLocalCandidatesKeepsAged(t *testing.T) {
	db := newSyncTestDB(t)
	for _, p := range []models.ExtraScanPath{
		{ID: "p-on", Path: "Z:/on", EnableOfflineUpdate: true},
		{ID: "p-off", Path: "Z:/off", EnableOfflineUpdate: false},
	} {
		pth := p
		if err := db.Create(&pth).Error; err != nil {
			t.Fatalf("创建路径失败: %v", err)
		}
		// GORM 的 `default:true` 会把零值 false 覆盖成 true（并在 Create 后回填到结构体），
		// 因此按字面量再回写一次启用状态
		want := p.EnableOfflineUpdate
		if err := db.Model(&models.ExtraScanPath{}).Where("id = ?", pth.ID).
			Update("enable_offline_update", want).Error; err != nil {
			t.Fatalf("回写启用状态失败: %v", err)
		}
	}

	comics := []models.OfflineComic{
		{ID: "dl", LocalPath: "Z:/dl/1"},
		{ID: "aged", LocalPath: "Z:/dl/2", AgedStatus: true},
		{ID: "removed", LocalPath: "Z:/dl/3", RemovedStatus: true},
		{ID: "on", LocalPath: "Z:/on/1", ScanPathID: "p-on"},
		{ID: "off", LocalPath: "Z:/off/1", ScanPathID: "p-off"},
		{ID: "off-aged", LocalPath: "Z:/off/2", ScanPathID: "p-off", AgedStatus: true},
	}
	clone := func() []models.OfflineComic {
		return append([]models.OfflineComic(nil), comics...)
	}

	local := filterOfflineLocalCandidates(db, clone())
	gotLocal := map[string]bool{}
	for _, c := range local {
		gotLocal[c.ID] = true
	}
	for _, id := range []string{"dl", "aged", "removed", "on"} {
		if !gotLocal[id] {
			t.Errorf("本地候选应保留 %q", id)
		}
	}
	for _, id := range []string{"off", "off-aged"} {
		if gotLocal[id] {
			t.Errorf("本地候选不应保留 %q（额外路径已关闭离线维护）", id)
		}
	}

	online := filterOfflineUpdateEnabled(db, clone())
	gotOnline := map[string]bool{}
	for _, c := range online {
		gotOnline[c.ID] = true
	}
	for _, id := range []string{"dl", "on"} {
		if !gotOnline[id] {
			t.Errorf("联网候选应保留 %q", id)
		}
	}
	for _, id := range []string{"aged", "removed", "off", "off-aged"} {
		if gotOnline[id] {
			t.Errorf("联网候选不应保留 %q", id)
		}
	}
}

// TestMaintainClustersIncludeAged 老化本之间的同名重复必须能被检出（G 回归）
func TestMaintainClustersIncludeAged(t *testing.T) {
	resetMaintainResultForTest(t)
	db := newSyncTestDB(t)
	// 迁移查重设置表（否则联网复核读取会打 "no such table" 噪音日志，语义上等价于默认关闭）
	if err := db.AutoMigrate(&models.DedupSetting{}); err != nil {
		t.Fatalf("迁移 dedup_settings 失败: %v", err)
	}
	c1 := mkComic("aged-1", "[サークル] エイジングテスト", []string{"artist:てすと", "language:japanese"}, 30)
	c2 := mkComic("aged-2", "[サークル] エイジングテスト (English)", []string{"artist:てすと", "language:english"}, 31)
	for _, c := range []*models.OfflineComic{&c1, &c2} {
		c.Source = models.SourceOffline
		c.AgedStatus = true // 两本都已老化
		c.LocalPath = "/nonexistent/" + c.ID
		if err := db.Create(c).Error; err != nil {
			t.Fatalf("创建漫画失败: %v", err)
		}
	}

	res, err := MaintainDedupWithProgress(db, nil, nil, false)
	if err != nil {
		t.Fatalf("维护查重失败: %v", err)
	}
	if len(res.Clusters) != 1 {
		t.Fatalf("老化本应参与名称级聚类并产出 1 簇，得到 %d 簇", len(res.Clusters))
	}
	if len(res.Clusters[0].Members) != 2 {
		t.Errorf("簇成员应为 2，得到 %d", len(res.Clusters[0].Members))
	}
}

// TestPruneMaintainDedupMembers 删除后定向裁剪（A）：不重算、不重排、不重编号
func TestPruneMaintainDedupMembers(t *testing.T) {
	resetMaintainResultForTest(t)

	mkMember := func(id string) ClusterMember {
		return ClusterMember{
			Comic:     toMaintainComicView(mkComic(id, "t-"+id, []string{"artist:x"}, 10)),
			PageCount: 10,
		}
	}

	pairKeep := mkComic("item-keep", "keep", nil, 10)
	pairVictim := mkComic("item-victim", "victim", nil, 10)

	offlineTaskMu.Lock()
	offlineMaintainRes = &DedupResult{
		Items: []DedupItem{
			// 被删项 + 其成对对象（成对对象也是保留项）→ 两者都该移出（组已处理完毕）
			{Comic: toMaintainComicView(pairVictim), Keep: false, Rule: "gid", PairComic: toMaintainComicViewPtr(pairKeep)},
			{Comic: toMaintainComicView(pairKeep), Keep: true, PairComic: toMaintainComicViewPtr(pairVictim)},
			{Comic: toMaintainComicView(mkComic("item-solo", "solo", nil, 10)), Keep: false, Rule: "hash"},
		},
		Clusters: []DedupCluster{
			// A 组：3 本，删其中 1 本 → 收缩为 2 本，ID 与位置不变
			{ID: "cluster-A", TitleKey: "a", Members: []ClusterMember{mkMember("a1"), mkMember("a2"), mkMember("victim-member")}},
			// B 组：2 本，删其中 1 本 → 整簇消失
			{ID: "cluster-B", TitleKey: "b", Members: []ClusterMember{mkMember("b1"), mkMember("victim-member-2")}},
			// C 组：无关，保持原样
			{ID: "cluster-C", TitleKey: "c", Members: []ClusterMember{mkMember("c1"), mkMember("c2")}},
		},
		FinishedAt: 1000,
		Stale:      true,
	}
	offlineTaskMu.Unlock()

	PruneMaintainDedupMembers([]string{"victim-member", "victim-member-2", "item-victim"})

	got := GetMaintainDedupResult()
	if got == nil {
		t.Fatal("裁剪后结果缓存不应为空")
	}
	if got.Stale {
		t.Error("裁剪后不应残留 stale 标记")
	}
	if got.FinishedAt <= 1000 {
		t.Errorf("裁剪后应前移 FinishedAt（清除未同步信号），得到 %d", got.FinishedAt)
	}

	// items：item-victim 与其成对项 item-keep 一并移出，只剩 item-solo
	if len(got.Items) != 1 || got.Items[0].Comic.ID != "item-solo" {
		ids := make([]string, 0, len(got.Items))
		for _, it := range got.Items {
			ids = append(ids, it.Comic.ID)
		}
		t.Errorf("items 应只剩 item-solo，得到 %v", ids)
	}

	// clusters：A（2 本，ID/顺序不变）、C（不变）；B 消失
	if len(got.Clusters) != 2 {
		t.Fatalf("裁剪后应有 2 组（A、C），得到 %d", len(got.Clusters))
	}
	if got.Clusters[0].ID != "cluster-A" || len(got.Clusters[0].Members) != 2 {
		t.Errorf("A 组应保留且 ID 不变、成员收缩为 2，得到 id=%s members=%d",
			got.Clusters[0].ID, len(got.Clusters[0].Members))
	}
	for _, m := range got.Clusters[0].Members {
		if m.Comic.ID == "victim-member" {
			t.Error("A 组不应残留被删成员")
		}
	}
	if got.Clusters[1].ID != "cluster-C" || len(got.Clusters[1].Members) != 2 {
		t.Errorf("C 组应保持原样（顺序不变），得到 id=%s members=%d",
			got.Clusters[1].ID, len(got.Clusters[1].Members))
	}
}

// TestPruneMaintainDedupMembersNilResult 缓存为空时裁剪应安全返回
func TestPruneMaintainDedupMembersNilResult(t *testing.T) {
	resetMaintainResultForTest(t)
	PruneMaintainDedupMembers([]string{"whatever"})
	PruneMaintainDedupMembers(nil)
	if GetMaintainDedupResult() != nil {
		t.Error("空缓存裁剪后仍应为空")
	}
}
