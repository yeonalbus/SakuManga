package services

import (
	"encoding/json"
	"log"
	"strings"

	"SakuManga/internal/models"

	"gorm.io/gorm"
)

// ─────────────────────────────────────────────────────────────
// Round49：书签新增字段的一次性回填（幂等）
//
// 背景：书签页改造后，每条书签独立持有 role / 已囤货 / 置顶 / 更新开关
// 等字段（书签项级，无跨书签联动）。老书签没有这些字段，AutoMigrate 只会
// 建列并填默认值，语义信息（原名字里手写的"最新/最旧/已下载完"）需在此
// 提取一次作为初值，否则用户要手动重填 38 条。
//
// 规则（仅迁移期使用一次，非运行时判据）：
//   - 名字含"最旧"   → role=oldest
//   - 名字含"最新"   → role=newest（同时含"已下载完"则一并置 bulkDone）
//   - 名字含"已下载完" → role=newest 且 bulkDone=true（该命名语义即"已无差别囤过"）
//   - 其余          → role=interest
//
// pinned / updateWatch 一律保持 false：只能由用户手动开启，不自动置顶、不自动探测。
// 同时清除 anchor JSON 中的 invalid 键——失效检测已剪枝（M7），残留标记无人清理，
// 不清掉会在书签页上永久挂一个无法消除的失效标记。
//
// 幂等性依赖 FieldsMigrated 标记：处理完成后置 true，重启不会重复覆盖
// 用户后续手动修改过的 role / bulkDone。
// ─────────────────────────────────────────────────────────────

// MigrateScrapeBookmarkFields 回填老书签的新字段，返回处理条数。
func MigrateScrapeBookmarkFields(db *gorm.DB) int {
	var marks []models.ScrapeBookmark
	if err := db.Where("fields_migrated = ?", false).Find(&marks).Error; err != nil {
		log.Printf("[BOOKMARK] 书签字段回填查询失败: %v", err)
		return 0
	}
	if len(marks) == 0 {
		return 0
	}

	n := 0
	for i := range marks {
		m := &marks[i]
		role, bulkDone := inferBookmarkRoleFromName(m.Name)
		updates := map[string]any{
			"role":            role,
			"bulk_done":       bulkDone,
			"fields_migrated": true,
		}
		if cleaned := stripAnchorInvalid(m.Anchor); cleaned != m.Anchor {
			updates["anchor"] = cleaned
		}
		if err := db.Model(&models.ScrapeBookmark{}).
			Where("id = ?", m.ID).
			Updates(updates).Error; err != nil {
			log.Printf("[BOOKMARK] 书签 [%d] 字段回填失败: %v", m.ID, err)
			continue
		}
		n++
	}
	return n
}

// inferBookmarkRoleFromName 从旧名字推断标记与已囤货初值（仅迁移期使用）。
func inferBookmarkRoleFromName(name string) (role string, bulkDone bool) {
	s := strings.TrimSpace(name)
	switch {
	case strings.Contains(s, "最旧"):
		return "oldest", false
	case strings.Contains(s, "最新"):
		return "newest", strings.Contains(s, "已下载完")
	case strings.Contains(s, "已下载完"):
		return "newest", true
	default:
		return "interest", false
	}
}

// stripAnchorInvalid 删除 anchor JSON 中的 invalid 键，其余键原样保留。
// anchor 为空 / "null" / 非法 JSON / 本就没有 invalid 时原样返回。
func stripAnchorInvalid(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" || s == "null" {
		return raw
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(s), &obj); err != nil {
		return raw
	}
	if _, ok := obj["invalid"]; !ok {
		return raw
	}
	delete(obj, "invalid")
	out, err := json.Marshal(obj)
	if err != nil {
		return raw
	}
	return string(out)
}
