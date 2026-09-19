package services

import (
	"testing"

	"SakuManga/internal/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// ─────────────────────────────────────────────────────────────
// Round42 D2：查重设置（联网复核，默认关闭）+ 联网复核判定逻辑单测
// ─────────────────────────────────────────────────────────────

func newDedupSettingTestDB(t *testing.T) *gorm.DB {
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
	if err := db.AutoMigrate(&models.DedupSetting{}, &models.User{}, &models.AccountSetting{}); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	return db
}

func TestDedupSettingDefaultOff(t *testing.T) {
	db := newDedupSettingTestDB(t)
	// 无记录 → 返回默认值：联网复核关闭（D2 要求默认关闭）
	s := GetDedupSetting(db)
	if s.OnlineVerify {
		t.Error("无记录时联网复核应为关闭（默认 false）")
	}
	if s.ID != dedupSettingID {
		t.Errorf("默认设置 ID 应为 %d，得到 %d", dedupSettingID, s.ID)
	}
}

func TestSaveAndGetDedupSetting(t *testing.T) {
	db := newDedupSettingTestDB(t)
	if err := SaveDedupSetting(db, models.DedupSetting{OnlineVerify: true}); err != nil {
		t.Fatalf("保存失败: %v", err)
	}
	if s := GetDedupSetting(db); !s.OnlineVerify {
		t.Error("保存后应读回 true")
	}
	// 单行覆盖写：再存 false → 读回 false，且表中仍只有一行
	if err := SaveDedupSetting(db, models.DedupSetting{OnlineVerify: false}); err != nil {
		t.Fatalf("二次保存失败: %v", err)
	}
	if s := GetDedupSetting(db); s.OnlineVerify {
		t.Error("二次保存后应读回 false")
	}
	var count int64
	if err := db.Model(&models.DedupSetting{}).Count(&count).Error; err != nil {
		t.Fatalf("统计失败: %v", err)
	}
	if count != 1 {
		t.Errorf("设置表应始终单行，实际 %d 行", count)
	}
}

func TestMatchOnlineCandidate(t *testing.T) {
	results := []OnlineComicDTO{
		{ID: "1", Title: "[サークル] サンプルタイトル [中国翻訳]"},
		{ID: "2", Title: "[Circle] Unrelated Work"},
	}
	// 主标题归一后一致（版本标记差异被剥净）→ 通过
	if !matchOnlineCandidate("サンプルタイトル", results) {
		t.Error("主标题归一一致时应复核通过")
	}
	// 簇核心名含版本标记也能匹配（双向归一）
	if !matchOnlineCandidate("[サークル] サンプルタイトル [DL版]", results) {
		t.Error("带版本标记的核心名也应归一匹配")
	}
	// 无关标题 → 不通过
	if matchOnlineCandidate("まったく別の作品", results) {
		t.Error("无关标题不应通过")
	}
	// 跨语言翻译标题（罗马音）→ 不通过（宁缺毋滥）
	if matchOnlineCandidate("Sample Title", results) {
		t.Error("跨语言翻译标题不应通过（本地日文 vs 在线罗马音）")
	}
	// 空结果 / 空核心名 → 不通过
	if matchOnlineCandidate("サンプルタイトル", nil) {
		t.Error("空结果不应通过")
	}
	if matchOnlineCandidate("", results) {
		t.Error("空核心名不应通过")
	}
}

func TestApproxClusterTarget(t *testing.T) {
	// 仅近似层（Tier 2）产出的簇需要联网复核
	tier2 := DedupCluster{Reason: "标题/标签多重相似（最低配对分 0.78，阈值 0.85），疑似同作品不同版本，请人工确认"}
	if !approxClusterTarget(tier2) {
		t.Error("近似层簇应纳入联网复核")
	}
	tier1 := DedupCluster{Reason: "画师相同（artist:abc）+ 核心名相同 + 页数差 ≤5，疑似多语言/重传/不同版本"}
	if approxClusterTarget(tier1) {
		t.Error("Tier 1 精确归一键结果不应参与联网复核（本地证据已足够）")
	}
}

func TestVerifyApproxClustersOnlineSkipsWithoutService(t *testing.T) {
	db := newDedupSettingTestDB(t)
	clusters := []DedupCluster{{TitleKey: "k", Reason: "标题/标签多重相似，疑似同版本", Confidence: "medium"}}
	// ehService 为空 → 直接跳过，不报错、不修改置信度
	if n := VerifyApproxClustersOnline(db, nil, clusters, nil); n != 0 {
		t.Errorf("ehService 为空时应跳过，得到 %d", n)
	}
	if clusters[0].Confidence != "medium" {
		t.Error("跳过时不应修改置信度")
	}
	// 未绑定 IPB 账号（空库）→ 同样跳过
	if n := VerifyApproxClustersOnline(db, &EHService{}, clusters, nil); n != 0 {
		t.Errorf("未绑定账号时应跳过，得到 %d", n)
	}
	if clusters[0].Confidence != "medium" {
		t.Error("未绑定账号时不应修改置信度")
	}
}
