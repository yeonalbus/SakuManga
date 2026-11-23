package services

import (
	"testing"

	"SakuManga/internal/models"
)

// ─────────────────────────────────────────────────────────────
// Round53 阶段三：D16 离线维护开关语义锁定
//
// EnableOfflineUpdate=false 的路径下记录：
//   - 退出「联网维度」（更新检测/老化/在线回填/在线父子发现）
//   - 同时退出「本地维度」（同 GID/hash/签名/名称级查重）
// 下载导入（ScanPathID 空）不受开关影响。
// ─────────────────────────────────────────────────────────────

func TestEnableOfflineUpdateExcludesBothDimensions(t *testing.T) {
	mgr := newTestDownloadManager(t)
	db := mgr.db
	if err := db.AutoMigrate(&models.ExtraScanPath{}, &models.OfflineComic{}); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	// 用 map 创建：GORM 对 bool 零值 + default:true 会落默认 true，必须显式写 false
	for _, p := range []struct {
		id      string
		path    string
		enabled bool
	}{
		{"p-on", "Z:/on", true},
		{"p-off", "Z:/off", false},
	} {
		if err := db.Model(&models.ExtraScanPath{}).Create(map[string]interface{}{
			"id":                    p.id,
			"path":                  p.path,
			"include_subfolders":    true,
			"enable_offline_update": p.enabled,
		}).Error; err != nil {
			t.Fatalf("创建路径失败: %v", err)
		}
	}

	base := []models.OfflineComic{
		{ID: "dl", GID: "1", LocalPath: "/x/dl"},
		{ID: "on", GID: "2", LocalPath: "/x/on", ScanPathID: "p-on"},
		{ID: "off", GID: "3", LocalPath: "/x/off", ScanPathID: "p-off"},
	}
	clone := func() []models.OfflineComic {
		out := make([]models.OfflineComic, len(base))
		copy(out, base)
		return out
	}

	assertSet := func(name string, got []models.OfflineComic, want map[string]bool) {
		t.Helper()
		if len(got) != len(want) {
			t.Fatalf("%s 应保留 %d 条，实际 %d 条: %+v", name, len(want), len(got), ids(got))
		}
		seen := map[string]bool{}
		for _, c := range got {
			seen[c.ID] = true
		}
		for id := range want {
			if !seen[id] {
				t.Errorf("%s 应保留 %s，实际被排除（结果: %v）", name, id, ids(got))
			}
		}
	}

	// 联网维度：只有关闭维护路径被排除
	assertSet("filterOfflineUpdateEnabled", filterOfflineUpdateEnabled(db, clone()), map[string]bool{"dl": true, "on": true})
	// 本地维度：同样只有关闭维护路径被排除（下载导入与开启路径都参与）
	assertSet("filterOfflineLocalCandidates", filterOfflineLocalCandidates(db, clone()), map[string]bool{"dl": true, "on": true})
}

func ids(comics []models.OfflineComic) []string {
	out := make([]string, 0, len(comics))
	for _, c := range comics {
		out = append(out, c.ID)
	}
	return out
}
