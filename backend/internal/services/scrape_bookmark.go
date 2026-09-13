package services

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"SakuManga/internal/models"

	"gorm.io/gorm"
)

// ─────────────────────────────────────────────────────────────
// Round28：搜刮书签服务（后端化，按用户隔离）
//
// 数据模型：config / anchor 以 JSON 字符串落库，对外统一以
// json.RawMessage 透传原始 JSON（前端自行解析 + 兜底），
// 避免「服务端强校验把脏数据拒之门外」造成书签丢失（BUG2 教训：
// 原 localStorage 恢复校验过严会静默丢弃整条书签）。
// ─────────────────────────────────────────────────────────────

// ErrBookmarkNotFound 书签不存在（或不属于当前用户）
var ErrBookmarkNotFound = errors.New("书签不存在")

// ScrapeBookmarkDTO 对外书签结构（config/anchor 原始 JSON 透传）
type ScrapeBookmarkDTO struct {
	ID        uint            `json:"id"`
	Name      string          `json:"name"`
	Type      string          `json:"type"`
	Keyword   string          `json:"keyword"`
	Config    json.RawMessage `json:"config"`
	Anchor    json.RawMessage `json:"anchor"`
	SortKey   float64         `json:"sortKey"` // Round37：LexoRank 权值（0 = 未赋权老数据）
	CreatedAt time.Time       `json:"createdAt"`
}

// ScrapeBookmarkInput 创建入参
type ScrapeBookmarkInput struct {
	Name    string          `json:"name"`
	Type    string          `json:"type"`
	Keyword string          `json:"keyword"`
	Config  json.RawMessage `json:"config"`
	Anchor  json.RawMessage `json:"anchor"`
}

// normalizeScrapeBookmark 宽松归一化：坏字段兜底，不拒绝（防丢数据）。
func normalizeScrapeBookmark(in ScrapeBookmarkInput) (name, typ string, config, anchor string) {
	// Round29：名称允许为空——留空时前端改展示「位置 + 发布时间」，不再自动生成默认名
	name = strings.TrimSpace(in.Name)
	typ = in.Type
	if typ != "search" {
		typ = "home" // 仅 home/search 两种，其余归 home
	}
	// config 必须是合法 JSON 对象；解析失败回退最小合法值（不丢书签）
	config = string(in.Config)
	if len(config) > 0 {
		var probe map[string]any
		if err := json.Unmarshal(in.Config, &probe); err != nil || probe == nil {
			config = "{}"
		}
	} else {
		config = "{}"
	}
	// anchor 允许 null 或 {gid,...}；解析失败置 null
	anchor = string(in.Anchor)
	if len(anchor) > 0 && anchor != "null" {
		var probe map[string]any
		if err := json.Unmarshal(in.Anchor, &probe); err != nil || probe == nil {
			anchor = "null"
		}
	} else {
		anchor = "null"
	}
	return name, typ, config, anchor
}

// toDTO 模型 → 对外结构
func toScrapeBookmarkDTO(m *models.ScrapeBookmark) ScrapeBookmarkDTO {
	return ScrapeBookmarkDTO{
		ID:        m.ID,
		Name:      m.Name,
		Type:      m.Type,
		Keyword:   m.Keyword,
		Config:    json.RawMessage(m.Config),
		Anchor:    json.RawMessage(m.Anchor),
		SortKey:   m.SortKey,
		CreatedAt: m.CreatedAt,
	}
}

// ListScrapeBookmarks 读取当前用户全部书签。
//
// Round37：改为按 LexoRank 权值升序（侧栏拖动排序），同权值（含升级前 sort_key=0
// 的老数据）按 id 升序兜底——老数据全部为 0 时即原「id ASC」顺序，
// 因此升级前后侧栏顺序完全一致。
func ListScrapeBookmarks(db *gorm.DB, userID uint) ([]ScrapeBookmarkDTO, error) {
	var marks []models.ScrapeBookmark
	if err := db.Where("user_id = ?", userID).Order("sort_key ASC, id ASC").Find(&marks).Error; err != nil {
		return nil, err
	}
	out := make([]ScrapeBookmarkDTO, 0, len(marks))
	for i := range marks {
		out = append(out, toScrapeBookmarkDTO(&marks[i]))
	}
	return out, nil
}

// nextBookmarkSortKey 计算下一个 LexoRank 权值（末项 +1000）。
//
// Round37：新建书签「追加到末尾」——既有书签全为 0（升级后从未拖动）时返回 1000，
// 同样落在末位；取不到最大值时保守返回 0（退回按 id 兜底排序，不阻断创建）。
func nextBookmarkSortKey(db *gorm.DB, userID uint) float64 {
	var row struct {
		MaxKey float64
	}
	err := db.Model(&models.ScrapeBookmark{}).
		Where("user_id = ?", userID).
		Select("COALESCE(MAX(sort_key), 0) AS max_key").
		Scan(&row).Error
	if err != nil {
		return 0
	}
	return row.MaxKey + 1000
}

// CreateScrapeBookmark 新增书签，返回带 id 的完整结构
func CreateScrapeBookmark(db *gorm.DB, userID uint, in ScrapeBookmarkInput) (*ScrapeBookmarkDTO, error) {
	name, typ, config, anchor := normalizeScrapeBookmark(in)
	m := models.ScrapeBookmark{
		UserID:    userID,
		Name:      name,
		Type:      typ,
		Keyword:   strings.TrimSpace(in.Keyword),
		Config:    config,
		Anchor:    anchor,
		SortKey:   nextBookmarkSortKey(db, userID), // Round37：追加到末尾
		CreatedAt: time.Now(),
	}
	if err := db.Create(&m).Error; err != nil {
		return nil, err
	}
	dto := toScrapeBookmarkDTO(&m)
	return &dto, nil
}

// RenameScrapeBookmark 重命名书签（空名忽略，保持原名）
func RenameScrapeBookmark(db *gorm.DB, userID uint, id uint, name string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return nil
	}
	res := db.Model(&models.ScrapeBookmark{}).
		Where("id = ? AND user_id = ?", id, userID).
		Update("name", trimmed)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrBookmarkNotFound
	}
	return nil
}

// ErrBookmarkInvalidAnchor 锚点数据不合法
var ErrBookmarkInvalidAnchor = errors.New("锚点数据不合法")

// UpdateScrapeBookmarkAnchor 更新书签锚点（Round33：迁移写回 / 失效标记写回）。
// 关键：锚点 JSON **原样存储**（不做 struct 往返），保证 invalid / migratedFrom /
// listIndex 等扩展字段不丢失；仅校验合法性（null 或含非空 gid 的对象）。
func UpdateScrapeBookmarkAnchor(db *gorm.DB, userID uint, id uint, anchor json.RawMessage) error {
	s := strings.TrimSpace(string(anchor))
	if s == "" {
		return ErrBookmarkInvalidAnchor
	}
	if s != "null" {
		var probe map[string]any
		if err := json.Unmarshal(anchor, &probe); err != nil {
			return ErrBookmarkInvalidAnchor
		}
		gid, _ := probe["gid"].(string)
		if strings.TrimSpace(gid) == "" {
			return ErrBookmarkInvalidAnchor
		}
	}
	res := db.Model(&models.ScrapeBookmark{}).
		Where("id = ? AND user_id = ?", id, userID).
		Update("anchor", s)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrBookmarkNotFound
	}
	return nil
}

// DeleteScrapeBookmark 删除书签（仅限本人）
func DeleteScrapeBookmark(db *gorm.DB, userID uint, id uint) error {
	res := db.Where("id = ? AND user_id = ?", id, userID).Delete(&models.ScrapeBookmark{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrBookmarkNotFound
	}
	return nil
}

// ─────────────────────────────────────────────────────────────
// Round37：侧栏拖动排序（LexoRank，与 Round22 书架列表同构）
// ─────────────────────────────────────────────────────────────

// MoveScrapeBookmarkPosition 单点移动：只更新该书的 LexoRank 权值
// （权值由前端 between(prev, next) 算出，取代全量重写）。
func MoveScrapeBookmarkPosition(db *gorm.DB, userID uint, id uint, sortKey float64) error {
	res := db.Model(&models.ScrapeBookmark{}).
		Where("id = ? AND user_id = ?", id, userID).
		Update("sort_key", sortKey)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrBookmarkNotFound
	}
	return nil
}

// ReorderScrapeBookmarks 全量重置权值：ids 数组下标即新顺序，赋 1000*(i+1)。
//
// 两个用途（与书架 ensureShelfWeights / 精度用尽兜底一致）：
//  1. 惰性赋权——升级前老书签 sort_key 全为 0，首次拖动时按当前顺序一次性赋权；
//  2. LexoRank 精度用尽（相邻两项 gap < 1e-6）时重排。
//
// 非本人书签 id 静默跳过（RowsAffected 不多做校验），避免跨用户写入。
func ReorderScrapeBookmarks(db *gorm.DB, userID uint, ids []uint) error {
	for i, id := range ids {
		if err := db.Model(&models.ScrapeBookmark{}).
			Where("id = ? AND user_id = ?", id, userID).
			Update("sort_key", float64((i+1)*1000)).Error; err != nil {
			return err
		}
	}
	return nil
}
