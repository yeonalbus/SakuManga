package services

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"SakuManga/internal/database"
	"SakuManga/internal/models"

	"gorm.io/gorm"
)

// ─────────────────────────────────────────────────────────────
// Round53 阶段二：D7 扫描字段三类互斥白名单
//
// 背景：saveComic 原用 DB.Save 整行覆盖，一次全量扫描会把状态位
// （AgedStatus/RemovedStatus/NeedsUpdate/FileHash…）与用户数据
// （Remark/OriginalTitle/HiddenPages/OnlineTags…）写回零值。
// ─────────────────────────────────────────────────────────────

func setupScanPolicyDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := newRefsTestDB(t)
	old := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = old })
	withoutXpCloud(t)
	return db
}

// seedComicDir 建临时目录（含 1.jpg）与对应离线记录（ID 必须是 generateID(localPath)）
func seedComicDir(t *testing.T, db *gorm.DB, fn func(*models.OfflineComic)) (*models.OfflineComic, string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "1.jpg"), []byte("x"), 0o644); err != nil {
		t.Fatalf("写图片失败: %v", err)
	}
	c := &models.OfflineComic{
		ID:        generateID(dir),
		Title:     "原",
		LocalPath: dir,
		PageCount: 1,
	}
	if fn != nil {
		fn(c)
	}
	if err := db.Create(c).Error; err != nil {
		t.Fatalf("创建记录失败: %v", err)
	}
	return c, dir
}

func TestFullScanPreservesProtectedFields(t *testing.T) {
	db := setupScanPolicyDB(t)
	addedAt := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	seed, dir := seedComicDir(t, db, func(c *models.OfflineComic) {
		c.Title = "原"
		c.OriginalTitle = "原"
		c.AgedStatus = true
		c.AgedCheckedAt = 111
		c.RemovedStatus = true
		c.RemovedAt = 222
		c.NeedsUpdate = true
		c.NewGID = "9"
		c.UpdateNote = "note"
		c.FileHash = "h"
		c.ReadCount = 5
		c.ClickCount = 7
		c.Remark = "r"
		c.ScanPathID = "path-a"
		c.DownloadScheme = "gallery"
		c.AddedAt = addedAt
		c.OnlineTags = `["x:1"]`
		c.OfflineAddTags = `["a:2"]`
		c.OfflineRemoveTags = `["x:1"]`
		c.HiddenPages = `["1.jpg"]` // 隐藏唯一页 → 有效页数 0
	})

	if !saveComic(dir, true, false, "") {
		t.Fatalf("saveComic 应成功")
	}
	var got models.OfflineComic
	if err := db.First(&got, "id = ?", seed.ID).Error; err != nil {
		t.Fatalf("读取记录失败: %v", err)
	}

	if !got.AgedStatus || got.AgedCheckedAt != 111 || !got.RemovedStatus || got.RemovedAt != 222 {
		t.Errorf("老化/失效状态被扫描覆盖: aged=%v/%d removed=%v/%d",
			got.AgedStatus, got.AgedCheckedAt, got.RemovedStatus, got.RemovedAt)
	}
	if !got.NeedsUpdate || got.NewGID != "9" || got.UpdateNote != "note" {
		t.Errorf("更新标记被扫描覆盖: needs=%v newGID=%q note=%q", got.NeedsUpdate, got.NewGID, got.UpdateNote)
	}
	if got.FileHash != "h" || got.ReadCount != 5 || got.ClickCount != 7 || got.Remark != "r" {
		t.Errorf("签名/计数/备注被扫描覆盖: hash=%q read=%d click=%d remark=%q",
			got.FileHash, got.ReadCount, got.ClickCount, got.Remark)
	}
	if got.ScanPathID != "path-a" || got.DownloadScheme != "gallery" {
		t.Errorf("来源/下载方案被扫描覆盖: scanPathID=%q scheme=%q", got.ScanPathID, got.DownloadScheme)
	}
	if !got.AddedAt.Equal(addedAt) {
		t.Errorf("AddedAt 被扫描覆盖: %v", got.AddedAt)
	}
	if got.OnlineTags != `["x:1"]` || got.OfflineAddTags != `["a:2"]` || got.OfflineRemoveTags != `["x:1"]` {
		t.Errorf("三态标签源被扫描覆盖: online=%q add=%q remove=%q",
			got.OnlineTags, got.OfflineAddTags, got.OfflineRemoveTags)
	}
	if got.HiddenPages != `["1.jpg"]` {
		t.Errorf("隐藏页被扫描覆盖: %q", got.HiddenPages)
	}
	if got.PageCount != 0 || got.OriginalPageCount != 1 {
		t.Errorf("页数应为有效 0 / 物理 1，实际 %d/%d", got.PageCount, got.OriginalPageCount)
	}
}

func TestFullScanUpdatesScanOwnedFields(t *testing.T) {
	db := setupScanPolicyDB(t)
	seed, dir := seedComicDir(t, db, func(c *models.OfflineComic) {
		c.PageCount = 99
		c.OriginalPageCount = 99
		c.FileSize = 999
		c.Category = "OldCategory"
		c.SourceMode = "archive"
	})

	if !saveComic(dir, true, false, "") {
		t.Fatalf("saveComic 应成功")
	}
	var got models.OfflineComic
	if err := db.First(&got, "id = ?", seed.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.PageCount != 1 {
		t.Errorf("有效页数应更新为 1，实际 %d", got.PageCount)
	}
	// OriginalPageCount 语义：物理页变多时抬升、变少时保持（防误降），99 应保留
	if got.OriginalPageCount != 99 {
		t.Errorf("物理原页数变少时应保持 99，实际 %d", got.OriginalPageCount)
	}
	if got.FileSize != 1 {
		t.Errorf("文件大小应更新为 1，实际 %d", got.FileSize)
	}
	if got.Category != "Doujinshi" {
		t.Errorf("分类应更新为默认 Doujinshi，实际 %q", got.Category)
	}
	if got.SourceMode != "gallery" {
		t.Errorf("形态应更新为 gallery，实际 %q", got.SourceMode)
	}
}

func TestFullScanFillsEmptyMergeFields(t *testing.T) {
	db := setupScanPolicyDB(t)
	seed, dir := seedComicDir(t, db, func(c *models.OfflineComic) {
		c.GID = ""
		c.Token = ""
		c.ParentGID = ""
		c.CommunityRating = 0
		c.OnlineTags = ""
	})
	writeTestMetadata(t, dir, `{"gid":"100","token":"t100","parent_gid":"500","rating":7.5,"tags":["artist:me"]}`)

	if !saveComic(dir, true, false, "") {
		t.Fatalf("saveComic 应成功")
	}
	var got models.OfflineComic
	if err := db.First(&got, "id = ?", seed.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.GID != "100" || got.Token != "t100" || got.ParentGID != "500" {
		t.Errorf("空 GID/Token/ParentGID 应被侧车填充，实际 %q/%q/%q", got.GID, got.Token, got.ParentGID)
	}
	if got.CommunityRating != 7.5 {
		t.Errorf("社区评分应为 7.5，实际 %v", got.CommunityRating)
	}
	if got.OnlineTags == "" || got.OnlineTags == "[]" {
		t.Errorf("OnlineTags 空时应被侧车标签填充，实际 %q", got.OnlineTags)
	}
}

func TestFullScanKeepsNonEmptyMergeFields(t *testing.T) {
	db := setupScanPolicyDB(t)
	seed, dir := seedComicDir(t, db, func(c *models.OfflineComic) {
		c.GID = "100"
		c.Token = "t100"
		c.ParentGID = "p100"
		c.CommunityRating = 8
		c.OnlineTags = `["x:1"]`
	})
	writeTestMetadata(t, dir, `{"gid":"200","token":"t200","parent_gid":"p200","rating":3,"tags":["artist:other"]}`)

	if !saveComic(dir, true, false, "") {
		t.Fatalf("saveComic 应成功")
	}
	var got models.OfflineComic
	if err := db.First(&got, "id = ?", seed.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.GID != "100" || got.Token != "t100" || got.ParentGID != "p100" {
		t.Errorf("非空 GID/Token/ParentGID 不应被覆盖，实际 %q/%q/%q", got.GID, got.Token, got.ParentGID)
	}
	if got.CommunityRating != 8 {
		t.Errorf("非零社区评分不应被覆盖，实际 %v", got.CommunityRating)
	}
	if got.OnlineTags != `["x:1"]` {
		t.Errorf("非空 OnlineTags 不应被覆盖，实际 %q", got.OnlineTags)
	}
}

func TestFullScanKeepsUserEditedTitle(t *testing.T) {
	db := setupScanPolicyDB(t)
	seed, dir := seedComicDir(t, db, func(c *models.OfflineComic) {
		c.Title = "用户改的标题"
		c.OriginalTitle = "原标题"
	})

	if !saveComic(dir, true, false, "") {
		t.Fatalf("saveComic 应成功")
	}
	var got models.OfflineComic
	if err := db.First(&got, "id = ?", seed.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.Title != "用户改的标题" || got.OriginalTitle != "原标题" {
		t.Errorf("用户改过的标题不应被扫描覆盖，实际 title=%q original=%q", got.Title, got.OriginalTitle)
	}
}

// TestFullScanSyncsTitleBaseline 未手改时写入新标题必须同步基线，防止下次误判为手改锁定。
func TestFullScanSyncsTitleBaseline(t *testing.T) {
	db := setupScanPolicyDB(t)
	seed, dir := seedComicDir(t, db, func(c *models.OfflineComic) {
		c.Title = "原"
		c.OriginalTitle = "原"
	})

	if !saveComic(dir, true, false, "") {
		t.Fatalf("saveComic 应成功")
	}
	var got models.OfflineComic
	if err := db.First(&got, "id = ?", seed.ID).Error; err != nil {
		t.Fatal(err)
	}
	want := filepath.Base(dir)
	if got.Title != want {
		t.Fatalf("未手改时应写入新标题 %q，实际 %q", want, got.Title)
	}
	if got.OriginalTitle != want {
		t.Fatalf("写标题时必须同步基线 OriginalTitle=%q，实际 %q（下次会被误判手改）", want, got.OriginalTitle)
	}

	// 再扫一次：title == original → 仍视为未手改，可继续更新
	if !saveComic(dir, true, false, "") {
		t.Fatalf("二次 saveComic 应成功")
	}
	if err := db.First(&got, "id = ?", seed.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.Title != want || got.OriginalTitle != want {
		t.Errorf("二次扫描后标题基线应保持，实际 title=%q original=%q", got.Title, got.OriginalTitle)
	}
}

func TestFullScanSynthesizesTagsFromTriState(t *testing.T) {
	db := setupScanPolicyDB(t)
	seed, dir := seedComicDir(t, db, func(c *models.OfflineComic) {
		c.OnlineTags = `["a:1","a:2"]`
		c.OfflineAddTags = `["b:3"]`
		c.OfflineRemoveTags = `["a:2"]`
		c.Tags = `["old"]`
	})

	if !saveComic(dir, true, false, "") {
		t.Fatalf("saveComic 应成功")
	}
	var got models.OfflineComic
	if err := db.First(&got, "id = ?", seed.ID).Error; err != nil {
		t.Fatal(err)
	}
	var tags []string
	if err := json.Unmarshal([]byte(got.Tags), &tags); err != nil {
		t.Fatalf("解析 tags 失败: %v（raw=%q）", err, got.Tags)
	}
	sort.Strings(tags)
	want := []string{"a:1", "b:3"}
	if len(tags) != len(want) || tags[0] != want[0] || tags[1] != want[1] {
		t.Errorf("tags 应为三态合成结果 %v，实际 %v", want, tags)
	}
}

// TestSaveUpdateMarksDoesNotTouchUpdatedAt D7 配套：检测流程不得刷新 updated_at。
func TestSaveUpdateMarksDoesNotTouchUpdatedAt(t *testing.T) {
	db := setupScanPolicyDB(t)
	oldTime := time.Date(2021, 5, 6, 7, 8, 9, 0, time.UTC)
	seed, _ := seedComicDir(t, db, func(c *models.OfflineComic) {
		c.UpdatedAt = oldTime
	})

	comics := []models.OfflineComic{{ID: seed.ID, NeedsUpdate: true, NewGID: "999"}}
	saveUpdateMarks(db, comics, map[string]bool{seed.ID: true})

	var got models.OfflineComic
	if err := db.First(&got, "id = ?", seed.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !got.NeedsUpdate || got.NewGID != "999" {
		t.Errorf("更新标记应已保存，实际 needs=%v newGID=%q", got.NeedsUpdate, got.NewGID)
	}
	if !got.UpdatedAt.Equal(oldTime) {
		t.Errorf("updated_at 不应被检测刷新：期望 %v，实际 %v", oldTime, got.UpdatedAt)
	}
}

// ─────────────────────────────────────────────────────────────
// D8 来源不可变：仅首次入库写 ScanPathID；已存在记录一律不改写
// （避免「下载目录被设为扫描路径」把来源从「下载」翻成「额外路径」）
// ─────────────────────────────────────────────────────────────

func TestScanFillsSourceForNewRecords(t *testing.T) {
	db := setupScanPolicyDB(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "1.jpg"), []byte("x"), 0o644); err != nil {
		t.Fatalf("写图片失败: %v", err)
	}
	if !saveComic(dir, true, false, "path-a") {
		t.Fatalf("saveComic 应成功")
	}
	var got models.OfflineComic
	if err := db.First(&got, "local_path = ?", dir).Error; err != nil {
		t.Fatalf("读取记录失败: %v", err)
	}
	if got.ScanPathID != "path-a" {
		t.Errorf("首次入库应写来源 path-a，实际 %q", got.ScanPathID)
	}
}

func TestScanKeepsDownloadOrigin(t *testing.T) {
	db := setupScanPolicyDB(t)
	seed, dir := seedComicDir(t, db, func(c *models.OfflineComic) {
		c.ScanPathID = ""
	})
	if !saveComic(dir, true, false, "path-a") {
		t.Fatalf("saveComic 应成功")
	}
	var got models.OfflineComic
	if err := db.First(&got, "id = ?", seed.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.ScanPathID != "" {
		t.Errorf("下载导入记录被扫描命中时不应改写来源，实际 %q", got.ScanPathID)
	}
}

func TestScanKeepsExistingSourceOnPathChange(t *testing.T) {
	db := setupScanPolicyDB(t)
	seed, dir := seedComicDir(t, db, func(c *models.OfflineComic) {
		c.ScanPathID = "path-a"
	})
	if !saveComic(dir, true, false, "path-b") {
		t.Fatalf("saveComic 应成功")
	}
	var got models.OfflineComic
	if err := db.First(&got, "id = ?", seed.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.ScanPathID != "path-a" {
		t.Errorf("路径A→路径B 不应改写来源，实际 %q", got.ScanPathID)
	}
}

func TestIncrementalKeepsDownloadOrigin(t *testing.T) {
	db := setupScanPolicyDB(t)
	seed, dir := seedComicDir(t, db, func(c *models.OfflineComic) {
		c.ScanPathID = ""
	})
	// 增量模式：已存在 → 跳过（返回 false），且不得改写来源
	if saveComic(dir, true, true, "path-a") {
		t.Errorf("增量模式遇已存在记录应返回 false")
	}
	var got models.OfflineComic
	if err := db.First(&got, "id = ?", seed.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.ScanPathID != "" {
		t.Errorf("增量模式不应改写来源，实际 %q", got.ScanPathID)
	}
}

func TestFirstScanPopulatesOnlineTags(t *testing.T) {
	db := setupScanPolicyDB(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "1.jpg"), []byte("x"), 0o644); err != nil {
		t.Fatalf("写图片失败: %v", err)
	}
	metaJSON := `{"gid": 123456, "token": "abcdef", "title": "[shengtian] Test", "tags": ["artist:shengtian", "parody:blue archive"]}`
	if err := os.WriteFile(filepath.Join(dir, "metadata.json"), []byte(metaJSON), 0o644); err != nil {
		t.Fatalf("写 metadata.json 失败: %v", err)
	}

	if !saveComic(dir, true, false, "") {
		t.Fatalf("首次扫描 saveComic 应成功")
	}

	var got models.OfflineComic
	if err := db.First(&got, "id = ?", generateID(dir)).Error; err != nil {
		t.Fatalf("读取新入库记录失败: %v", err)
	}
	if got.OnlineTags == "" || got.OnlineTags == "[]" {
		t.Fatalf("首次扫描应同步写入 OnlineTags，实际为空: %q", got.OnlineTags)
	}
	if got.OnlineTags != got.Tags {
		t.Errorf("首次扫描 OnlineTags (%q) 应与 Tags (%q) 一致", got.OnlineTags, got.Tags)
	}
}
