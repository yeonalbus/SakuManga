package services

import (
	"log"
	"strings"
	"time"

	"SakuManga/internal/models"

	"gorm.io/gorm"
)

// ─────────────────────────────────────────────────────────────
// Round49：书签更新探测（书签项级）
//
// 语义：对「开启了更新开关」的每条书签，用其创建时的检索条件
// （config 快照）请求**第一页**列表（不带 seek / next），取列表首条
// 发布时间，与书签锚定的发布时间比较：
//
//	有更新 ⟺ 列表首条 postedAt > 书签.anchor.postedAt
//
// 判据与书签失效检测（已剪枝）不同：不扫描、不找 gid，只问"现在的最新
// 一条是否比我锚的那一刻更晚"。因此每条书签只花 1 次请求。
//
// 失败处理：请求失败只记 updateError，**不改动 hasUpdate**——一次网络抖动
// 不该把红点抹掉。缺少锚定画廊或发布时间的书签无法探测，记为 skipped。
//
// 开关关闭时红点由 UpdateScrapeBookmarkFields 顺带清掉（见该函数注释）。
// ─────────────────────────────────────────────────────────────

// 探测结果状态
const (
	BookmarkProbeOK      = "ok"      // 完成判定
	BookmarkProbeSkipped = "skipped" // 信息不全，未判定
	BookmarkProbeError   = "error"   // 请求失败，未判定
)

// BookmarkProbeResult 单条探测结果
type BookmarkProbeResult struct {
	ID           uint   `json:"id"`
	Status       string `json:"status"`
	HasUpdate    bool   `json:"hasUpdate"`
	HeadPostedAt string `json:"headPostedAt,omitempty"` // 本次看到的列表首条发布时间
	Message      string `json:"message,omitempty"`
	CheckedAt    int64  `json:"checkedAt"`
}

// ProbeScrapeBookmarks 逐条探测开启更新开关的书签。
//
// ids 为空表示探测当前用户全部 updateWatch=true 的书签；逐条串行（复用 E 站自适应限流）。
// 未开启开关的书签即使出现在 ids 里也会被跳过（开关是唯一门禁）。
func ProbeScrapeBookmarks(
	db *gorm.DB,
	eh *EHService,
	userID uint,
	account *models.AccountSetting,
	setting *models.EHSetting,
	ids []uint,
) ([]BookmarkProbeResult, error) {
	q := db.Where("user_id = ? AND update_watch = ?", userID, true)
	if len(ids) > 0 {
		q = q.Where("id IN ?", ids)
	}
	var marks []models.ScrapeBookmark
	if err := q.Order("id ASC").Find(&marks).Error; err != nil {
		return nil, err
	}

	results := make([]BookmarkProbeResult, 0, len(marks))
	for i := range marks {
		bm := marks[i]
		res := BookmarkProbeResult{ID: bm.ID, CheckedAt: time.Now().UnixMilli()}

		anchor, err := parseAnchorJSON(bm.Anchor)
		if err != nil || anchor == nil ||
			strings.TrimSpace(anchor.GID) == "" || strings.TrimSpace(anchor.PostedAt) == "" {
			res.Status = BookmarkProbeSkipped
			res.Message = "缺少锚定画廊或发布时间，无法探测"
			saveProbeState(db, bm.ID, res.CheckedAt, nil, res.Message)
			results = append(results, res)
			continue
		}

		cfg := parseBookmarkConfig(bm.Config)
		params := cfg.toSearchParams()
		// 探测只看「当前最新一页」：清掉游标类参数，避免带出历史 seek/翻页状态
		params.Seek = ""
		params.Next = ""
		params.Prev = ""
		params.Page = 1

		ehRateLimiter.Wait()
		r, ferr := eh.FetchGalleryList(account, params, setting)
		ehRateLimiter.Mark(ferr == nil)
		if ferr != nil {
			res.Status = BookmarkProbeError
			res.Message = ferr.Error()
			saveProbeState(db, bm.ID, res.CheckedAt, nil, res.Message)
			results = append(results, res)
			log.Printf("[BOOKMARK] 探测书签 [%d] 请求失败: %v", bm.ID, ferr)
			continue
		}

		head := ""
		if r != nil && len(r.Comics) > 0 {
			head = r.Comics[0].UpdatedAt
		}
		hasUpdate := postedAfter(head, anchor.PostedAt)
		res.Status = BookmarkProbeOK
		res.HasUpdate = hasUpdate
		res.HeadPostedAt = head
		saveProbeState(db, bm.ID, res.CheckedAt, &hasUpdate, "")
		results = append(results, res)
	}
	return results, nil
}

// saveProbeState 写回探测状态：hasUpdate 为 nil 表示"未判定"（失败/跳过），
// 只更新时间戳与错误原因，不动红点。
func saveProbeState(db *gorm.DB, id uint, checkedAt int64, hasUpdate *bool, errMsg string) {
	updates := map[string]any{
		"update_checked_at": checkedAt,
		"update_error":      errMsg,
	}
	if hasUpdate != nil {
		updates["has_update"] = *hasUpdate
	}
	if err := db.Model(&models.ScrapeBookmark{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		log.Printf("[BOOKMARK] 写回探测状态失败 [%d]: %v", id, err)
	}
}

// postedAfter 列表首条发布时间是否晚于锚定时间（相等不报）。
// 时间格式无法解析时返回 false（不亮红点，保守）。
func postedAfter(head, anchor string) bool {
	h := parseGalleryAddedAt(head)
	a := parseGalleryAddedAt(anchor)
	if h == nil || a == nil {
		return false
	}
	return h.After(*a)
}
