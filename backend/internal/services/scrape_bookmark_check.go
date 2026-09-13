package services

import (
	"encoding/json"
	"errors"
	"log"
	"strings"

	"SakuManga/internal/models"

	"gorm.io/gorm"
)

// ─────────────────────────────────────────────────────────────
// Round33：搜刮书签失效探测（主动检测）
//
// 判定依据（实测 E 站行为 + 项目既有识别能力）：
//   - 410 Gone / ErrGalleryUnavailable{removed}        → removed（已删除/不可用）
//   - 403 Forbidden / ErrGalleryUnavailable{copyright} → copyright（版权下架）
//   - 200 但 title / 封面皆空                           → invalid（gid 不存在）
//   - 200 且 NewVersionGID 非空                         → replaced（已被新版本取代，可精确迁移）
//   - 200 正常                                          → ok（顺带回传刷新后的 title/postedAt）
//   - 其他错误（网络 / 限流 / 会话失效）                 → error（不判定失效，避免误杀）
// ─────────────────────────────────────────────────────────────

// 书签探测状态
const (
	BookmarkStatusOK        = "ok"
	BookmarkStatusRemoved   = "removed"
	BookmarkStatusCopyright = "copyright"
	BookmarkStatusInvalid   = "invalid"
	BookmarkStatusReplaced  = "replaced"
	BookmarkStatusError     = "error"
)

// BookmarkAnchorInfo 锚点信息（探测结果中回传的候选 / 刷新数据）
type BookmarkAnchorInfo struct {
	GID      string `json:"gid"`
	Token    string `json:"token,omitempty"`
	Title    string `json:"title,omitempty"`
	PostedAt string `json:"postedAt,omitempty"`
}

// BookmarkCheckResult 单条书签探测结果
type BookmarkCheckResult struct {
	ID         uint                `json:"id"`
	Status     string              `json:"status"`
	Message    string              `json:"message,omitempty"`
	NewVersion *BookmarkAnchorInfo `json:"newVersion,omitempty"` // replaced 时的精确迁移目标
	Refreshed  *BookmarkAnchorInfo `json:"refreshed,omitempty"`  // ok 时刷新后的元信息
}

// bookmarkAnchorJSON 锚点 JSON（与前端 ScrapeBookmark['anchor'] 对齐）
type bookmarkAnchorJSON struct {
	GID      string `json:"gid"`
	Token    string `json:"token,omitempty"`
	Title    string `json:"title,omitempty"`
	PostedAt string `json:"postedAt,omitempty"`
}

// parseAnchorJSON 解析锚点（null / 空 / 非法一律返回 nil）
func parseAnchorJSON(raw string) (*bookmarkAnchorJSON, error) {
	s := strings.TrimSpace(raw)
	if s == "" || s == "null" {
		return nil, nil
	}
	var a bookmarkAnchorJSON
	if err := json.Unmarshal([]byte(s), &a); err != nil {
		log.Printf("[BOOKMARK] 锚点 JSON 解析失败: %v", err)
		return nil, err
	}
	if a.GID == "" {
		return nil, nil
	}
	return &a, nil
}

// CheckScrapeBookmarks 批量探测书签锚定画廊的有效性。
// ids 为空表示检测当前用户全部「有锚点」的书签；逐条串行探测（复用 E 站自适应限流）。
func CheckScrapeBookmarks(
	db *gorm.DB,
	eh *EHService,
	userID uint,
	account *models.AccountSetting,
	setting *models.EHSetting,
	ids []uint,
) ([]BookmarkCheckResult, error) {
	q := db.Where("user_id = ?", userID)
	if len(ids) > 0 {
		q = q.Where("id IN ?", ids)
	}
	var marks []models.ScrapeBookmark
	if err := q.Order("id ASC").Find(&marks).Error; err != nil {
		return nil, err
	}

	results := make([]BookmarkCheckResult, 0, len(marks))
	for i := range marks {
		bm := marks[i]
		res := BookmarkCheckResult{ID: bm.ID}

		// 无锚点 / 锚点无效：未锚定书签不参与失效判定
		anchor, err := parseAnchorJSON(bm.Anchor)
		if err != nil || anchor == nil {
			continue
		}
		if anchor.Token == "" {
			// 缺 token 无法探测（老书签可能没有）：记为错误，不判定失效
			res.Status = BookmarkStatusError
			res.Message = "书签缺少 token，无法探测"
			results = append(results, res)
			continue
		}

		detail, ferr := eh.FetchGalleryDetail(account, anchor.GID, anchor.Token, setting)
		if ferr != nil {
			var gErr *ErrGalleryUnavailable
			if errors.As(ferr, &gErr) {
				if gErr.Kind == "copyright" {
					res.Status = BookmarkStatusCopyright
				} else {
					res.Status = BookmarkStatusRemoved
				}
				res.Message = gErr.Error()
			} else {
				// 网络 / 限流 / 会话失效：不判定失效，交给用户重试
				res.Status = BookmarkStatusError
				res.Message = ferr.Error()
			}
			results = append(results, res)
			continue
		}

		// 空详情（标题与封面皆空）= gid 不存在（实测 E 站对无效 gid 返回 200 + 空字段）
		if strings.TrimSpace(detail.Title) == "" && strings.TrimSpace(detail.CoverURL) == "" {
			res.Status = BookmarkStatusInvalid
			res.Message = "画廊不存在或已失效"
			results = append(results, res)
			continue
		}

		// 已被新版本取代：前端按「时间锚迁移」处理（新版本位置已变，不做换 gid 式替代）
		if detail.NewVersionGID != "" {
			res.Status = BookmarkStatusReplaced
			res.Message = "画廊已被新版本取代（原锚点位置可能已变化）"
			res.NewVersion = &BookmarkAnchorInfo{
				GID:   detail.NewVersionGID,
				Token: detail.NewVersionToken,
			}
			results = append(results, res)
			continue
		}

		// 有效：回传刷新后的元信息（标题可能改名、发布时间补齐）
		res.Status = BookmarkStatusOK
		res.Refreshed = &BookmarkAnchorInfo{
			GID:      anchor.GID,
			Token:    anchor.Token,
			Title:    detail.Title,
			PostedAt: detail.UpdatedAt,
		}
		results = append(results, res)
	}
	return results, nil
}
