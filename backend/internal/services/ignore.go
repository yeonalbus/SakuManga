package services

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"SakuHentai/internal/models"

	"gorm.io/gorm"
)

// ─────────────────────────────────────────────────────────────
// Round26 O2：忽略标记（疑似重复组 / 父画廊更新提示）
//
// 作用域：仅疑似重复判定 + 规则 3 父子画廊；规则 1/2/4（同 GID/hash/内容签名）
// 与更新检测不读取本表。
// 增量查重：命中忽略 → 直接跳过；
// 全量核对（forceFull）：不豁免，仍展示但带「已忽略」标记（前端折叠/徽标，可单独解除）。
// ─────────────────────────────────────────────────────────────

// ErrIgnoreNotFound 忽略条目不存在（恢复时容错）
var ErrIgnoreNotFound = errors.New("忽略条目不存在")

// CreateIgnore 新增忽略条目（幂等：同 type+key 已存在则返回现有条目）
func CreateIgnore(db *gorm.DB, typ, titleKey, artist, gid, comicID, note string) (*models.IgnoredIdentifier, error) {
	if db == nil {
		return nil, fmt.Errorf("非法参数：db 不能为空")
	}
	typ = strings.TrimSpace(typ)
	switch typ {
	case "title":
		titleKey = strings.TrimSpace(titleKey)
		if titleKey == "" {
			return nil, fmt.Errorf("非法参数：title 类型必须提供核心名")
		}
	case "gid":
		gid = strings.TrimSpace(gid)
		if gid == "" {
			return nil, fmt.Errorf("非法参数：gid 类型必须提供画廊 gid")
		}
	case "comic":
		comicID = strings.TrimSpace(comicID)
		if comicID == "" {
			return nil, fmt.Errorf("非法参数：comic 类型必须提供本地漫画 id")
		}
	default:
		return nil, fmt.Errorf("非法参数：忽略类型仅支持 title / gid / comic")
	}

	// 幂等去重：同类型同 key 不重复插入
	var existing models.IgnoredIdentifier
	q := db.Where("type = ?", typ)
	switch typ {
	case "title":
		q = q.Where("title_key = ? AND artist = ?", titleKey, artist)
	case "gid":
		q = q.Where("g_id = ?", gid)
	case "comic":
		q = q.Where("comic_id = ?", comicID)
	}
	if err := q.First(&existing).Error; err == nil {
		return &existing, nil
	}

	rec := &models.IgnoredIdentifier{
		ID:        fmt.Sprintf("ignore-%d", time.Now().UnixNano()),
		Type:      typ,
		TitleKey:  titleKey,
		Artist:    artist,
		GID:       gid,
		ComicID:   comicID,
		Note:      strings.TrimSpace(note),
		CreatedAt: time.Now(),
	}
	if err := db.Create(rec).Error; err != nil {
		return nil, fmt.Errorf("写入忽略条目失败: %v", err)
	}
	return rec, nil
}

// ListIgnores 忽略清单（按创建时间倒序）
func ListIgnores(db *gorm.DB) ([]models.IgnoredIdentifier, error) {
	if db == nil {
		return nil, fmt.Errorf("非法参数：db 不能为空")
	}
	var out []models.IgnoredIdentifier
	if err := db.Order("created_at desc").Find(&out).Error; err != nil {
		return nil, fmt.Errorf("读取忽略清单失败: %v", err)
	}
	return out, nil
}

// IgnoreItemView 忽略清单条目视图（comic 型附带漫画标题，便于界面展示）
type IgnoreItemView struct {
	models.IgnoredIdentifier
	ComicTitle string `json:"comicTitle,omitempty"`
}

// ListIgnoresWithTitles 忽略清单（comic 型补漫画标题；title/gid 型不变）
func ListIgnoresWithTitles(db *gorm.DB) ([]IgnoreItemView, error) {
	list, err := ListIgnores(db)
	if err != nil {
		return nil, err
	}
	out := make([]IgnoreItemView, 0, len(list))
	for _, ig := range list {
		v := IgnoreItemView{IgnoredIdentifier: ig}
		if ig.Type == "comic" && ig.ComicID != "" {
			var comic models.OfflineComic
			if err := db.Select("title").First(&comic, "id = ?", ig.ComicID).Error; err == nil {
				v.ComicTitle = comic.Title
			}
		}
		out = append(out, v)
	}
	return out, nil
}

// RestoreIgnore 恢复（删除）忽略条目；id 不存在时返回 ErrIgnoreNotFound（容错）
func RestoreIgnore(db *gorm.DB, id string) error {
	if db == nil {
		return fmt.Errorf("非法参数：db 不能为空")
	}
	res := db.Delete(&models.IgnoredIdentifier{}, "id = ?", id)
	if res.Error != nil {
		return fmt.Errorf("恢复忽略条目失败: %v", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrIgnoreNotFound
	}
	return nil
}

// ── 查重期内存索引（每次维护查重开始时加载一次） ──

// IgnoreIndex 忽略索引：title 型按「核心名+画师」匹配，gid 型按 gid 匹配，comic 型按漫画 id 匹配
type IgnoreIndex struct {
	titleKeys map[string]bool // key: titleIgnoreKey(TitleKey, Artist)
	gids      map[string]bool // key: gid
	comicIDs  map[string]bool // key: comicID（Round26-2：成员级忽略，聚类时剔除）
}

// LoadIgnoreIndex 加载全部忽略条目到内存索引
func LoadIgnoreIndex(db *gorm.DB) *IgnoreIndex {
	idx := &IgnoreIndex{
		titleKeys: map[string]bool{},
		gids:      map[string]bool{},
		comicIDs:  map[string]bool{},
	}
	if db == nil {
		return idx
	}
	var list []models.IgnoredIdentifier
	if err := db.Find(&list).Error; err != nil {
		return idx
	}
	for _, ig := range list {
		switch ig.Type {
		case "title":
			idx.titleKeys[titleIgnoreKey(ig.TitleKey, ig.Artist)] = true
		case "gid":
			idx.gids[ig.GID] = true
		case "comic":
			idx.comicIDs[ig.ComicID] = true
		}
	}
	return idx
}

func titleIgnoreKey(titleKey, artist string) string {
	return strings.ToLower(strings.TrimSpace(titleKey)) + "\x00" + strings.ToLower(strings.TrimSpace(artist))
}

// IsTitleIgnored title 型命中：核心名 + 画师双匹配（防撞名误伤）
func (idx *IgnoreIndex) IsTitleIgnored(titleKey, artist string) bool {
	return idx != nil && idx.titleKeys[titleIgnoreKey(titleKey, artist)]
}

// IsGIDIgnored gid 型命中（父画廊 gid，规则 3 例外）
func (idx *IgnoreIndex) IsGIDIgnored(gid string) bool {
	return idx != nil && idx.gids[gid]
}

// IsComicIgnored comic 型命中（成员级忽略：该漫画不参与名称级聚类）
func (idx *IgnoreIndex) IsComicIgnored(comicID string) bool {
	return idx != nil && idx.comicIDs[comicID]
}
