package services

import (
	"testing"
	"time"

	"SakuManga/internal/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("打开内存数据库失败: %v", err)
	}
	// 内存库固定单连接：glebarez/sqlite 的 :memory: 是「每连接一个独立库」
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("获取底层连接失败: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	if err := db.AutoMigrate(&models.ArchiveBinding{}, &models.NetworkSetting{}); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	return db
}

func TestArchiveCircuitTripsAfterThreeNetworkFailures(t *testing.T) {
	c := &archiveCircuit{}
	if c.Tripped() {
		t.Fatal("初始不应熔断")
	}
	c.Record("100", true)
	c.Record("101", true)
	if c.Tripped() {
		t.Fatal("两个画廊网络失败不应熔断")
	}
	c.Record("102", true)
	if !c.Tripped() {
		t.Fatal("连续三个画廊网络失败应熔断")
	}
}

func TestArchiveCircuitSuccessBreaksStreak(t *testing.T) {
	c := &archiveCircuit{}
	c.Record("1", true)
	c.Record("2", true)
	c.Record("3", false) // 成功打断
	c.Record("4", true)
	if c.Tripped() {
		t.Fatal("成功应打断连续性，不应熔断")
	}
}

func TestArchiveCircuitNonNetworkNotCounted(t *testing.T) {
	c := &archiveCircuit{}
	// 画廊自身问题（403/404）记录为 networkFail=false
	c.Record("1", true)
	c.Record("2", false)
	c.Record("3", true)
	if c.Tripped() {
		t.Fatal("混入非网络失败时不应熔断")
	}
}

func TestArchiveCircuitSameGIDDoesNotFillWindow(t *testing.T) {
	c := &archiveCircuit{}
	c.Record("same", true)
	c.Record("same", true)
	c.Record("same", true)
	if c.Tripped() {
		t.Fatal("同一画廊多次重试不应撑满窗口（计数单位是画廊）")
	}
	if len(c.Snapshot()) != 1 {
		t.Fatalf("同一 gid 应只保留一条记录，实际 %d", len(c.Snapshot()))
	}
}

func TestArchiveCircuitReset(t *testing.T) {
	c := &archiveCircuit{}
	c.Record("1", true)
	c.Record("2", true)
	c.Record("3", true)
	if !c.Tripped() {
		t.Fatal("应已熔断")
	}
	c.Reset()
	if c.Tripped() {
		t.Fatal("Reset 后不应熔断")
	}
}

func TestArchiveBindingLifecycle(t *testing.T) {
	db := newTestDB(t)

	b := &models.ArchiveBinding{GID: "123", UserID: 7, Group: "Ehentai", Node: "HK 01", GPCost: "1952 GP"}
	if err := UpsertArchiveBinding(db, b); err != nil {
		t.Fatalf("写入绑定失败: %v", err)
	}

	found, err := FindActiveBinding(db, "123", 7)
	if err != nil || found == nil {
		t.Fatalf("应能查到绑定: %v", err)
	}
	if found.Node != "HK 01" || found.GPCost != "1952 GP" {
		t.Fatalf("绑定内容错误: %+v", found)
	}
	if found.ExpiresAt.Before(time.Now()) {
		t.Fatal("有效期应晚于当前时间")
	}

	// 其它用户/其它画廊不应命中
	if other, _ := FindActiveBinding(db, "123", 8); other != nil {
		t.Fatal("其它用户不应命中该绑定")
	}
	if other, _ := FindActiveBinding(db, "999", 7); other != nil {
		t.Fatal("其它画廊不应命中该绑定")
	}

	// 同一画廊重复解锁 → 覆盖为最新节点
	if err := UpsertArchiveBinding(db, &models.ArchiveBinding{
		GID: "123", UserID: 7, Group: "Ehentai", Node: "JP 02", GPCost: "Free!",
	}); err != nil {
		t.Fatalf("覆盖绑定失败: %v", err)
	}
	var count int64
	db.Model(&models.ArchiveBinding{}).Where("g_id = ? AND user_id = ?", "123", 7).Count(&count)
	if count != 1 {
		t.Fatalf("同一画廊+账号应只保留一条，实际 %d", count)
	}
	found, _ = FindActiveBinding(db, "123", 7)
	if found.Node != "JP 02" {
		t.Fatalf("应覆盖为最新节点，实际 %q", found.Node)
	}

	// 消费后不再命中
	ConsumeBinding(db, "123", 7)
	if b2, _ := FindActiveBinding(db, "123", 7); b2 != nil {
		t.Fatal("已消费的绑定不应命中")
	}
}

func TestArchiveBindingGC(t *testing.T) {
	db := newTestDB(t)

	// 过期记录
	oldB := &models.ArchiveBinding{
		GID: "old", UserID: 1, Node: "X",
		ExpiresAt: time.Now().Add(-time.Hour),
	}
	db.Create(oldB)

	// 未过期记录
	liveB := &models.ArchiveBinding{
		GID: "live", UserID: 1, Node: "Y",
		ExpiresAt: time.Now().Add(time.Hour),
	}
	db.Create(liveB)

	removed := GCBindings(db)
	if removed != 1 {
		t.Fatalf("应清理 1 条过期记录，实际 %d", removed)
	}
	if b, _ := FindActiveBinding(db, "live", 1); b == nil {
		t.Fatal("未过期记录不应被清理")
	}
}

func TestNetworkSettingDefaultsAndValidation(t *testing.T) {
	db := newTestDB(t)

	setting := GetNetworkSetting(db)
	if setting.ClashController != "127.0.0.1:9090" {
		t.Fatalf("默认控制面地址错误: %q", setting.ClashController)
	}
	if setting.ClashEnabled() {
		t.Fatal("默认应关闭联动")
	}

	// 关闭开关时允许空值保存
	empty := &models.NetworkSetting{}
	if err := ValidateNetworkSetting(empty); err != nil {
		t.Fatalf("关闭开关时不应校验: %v", err)
	}

	// 打开开关后必填校验
	on := &models.NetworkSetting{AdvancedProxyEnabled: true}
	if err := ValidateNetworkSetting(on); err == nil {
		t.Fatal("开启联动且未填地址应报错")
	}
	on.ClashController = "127.0.0.1:9090"
	if err := ValidateNetworkSetting(on); err == nil {
		t.Fatal("未填代理组应报错")
	}
	on.GroupEGallery = "Ehentai"
	if err := ValidateNetworkSetting(on); err == nil {
		t.Fatal("里站组必填")
	}
	on.GroupEGalleryEx = "Ehentai"
	if err := ValidateNetworkSetting(on); err != nil {
		t.Fatalf("必填齐全应通过: %v", err)
	}

	// GroupForSite 回退逻辑
	site := &models.NetworkSetting{GroupEGallery: "A", GroupEGalleryEx: "B"}
	if got := site.GroupForSite(false); got != "A" {
		t.Fatalf("表站组错误: %q", got)
	}
	if got := site.GroupForSite(true); got != "B" {
		t.Fatalf("里站组错误: %q", got)
	}
	onlyA := &models.NetworkSetting{GroupEGallery: "A"}
	if got := onlyA.GroupForSite(true); got != "A" {
		t.Fatalf("里站组缺失时应回退表站: %q", got)
	}
}
