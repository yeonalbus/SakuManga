package services

import (
	"SakuManga/internal/models"
	"errors"
	"sort"

	"gorm.io/gorm"
)

// ─────────────────────────────────────────────────────────────
// Round24：书签 / 章节标记（仅本地阅读器）
//
// 标记绑定「物理页索引」（0-based 原文件索引），与隐藏页同一锚点体系：
// 隐藏某物理页 → 该页书签 / 章节标记联动清除。
// ─────────────────────────────────────────────────────────────

var (
	ErrMarkInvalidLevel    = errors.New("章节层级仅支持 1~3")
	ErrMarkPageOutOfRange  = errors.New("起始页超出物理页数")
	ErrMarkRootHasParent   = errors.New("一级章节为根节点，不能选择父级")
	ErrMarkNoParent        = errors.New("二、三级章节必须选择父级")
	ErrMarkParentNotFound  = errors.New("父章节不存在")
	ErrMarkParentLevel     = errors.New("父章节层级必须为当前层级减一")
	ErrMarkChapterNotFound = errors.New("章节不存在")
	ErrMarkParentCycle     = errors.New("父章节不能是自己或自己的后代")
	ErrMarkHasChildren     = errors.New("该章节下还有子级，无法修改层级（可先调整或删除子级）")
)

// normalizeMarkPages 归一化书签页索引：排序去重、裁剪越界（limit<0 不裁剪）
func normalizeMarkPages(pages []int, limit int) []int {
	set := map[int]bool{}
	out := []int{}
	for _, p := range pages {
		if p < 0 {
			continue
		}
		if limit >= 0 && p >= limit {
			continue
		}
		if set[p] {
			continue
		}
		set[p] = true
		out = append(out, p)
	}
	sort.Ints(out)
	return out
}

// GetComicBookmarks 读取本地漫画的书签物理页索引（排序去重）
func GetComicBookmarks(db *gorm.DB, comicID string) ([]int, error) {
	var marks []models.ComicBookmark
	if err := db.Where("comic_id = ?", comicID).Order("page_index ASC").Find(&marks).Error; err != nil {
		return nil, err
	}
	out := make([]int, 0, len(marks))
	for _, m := range marks {
		out = append(out, m.PageIndex)
	}
	return normalizeMarkPages(out, -1), nil
}

// ReplaceComicBookmarks 整体覆盖书签列表（先清后插；越界索引裁剪）
func ReplaceComicBookmarks(db *gorm.DB, comicID string, pages []int, limit int) ([]int, error) {
	norm := normalizeMarkPages(pages, limit)
	if err := db.Where("comic_id = ?", comicID).Delete(&models.ComicBookmark{}).Error; err != nil {
		return nil, err
	}
	for _, p := range norm {
		if err := db.Create(&models.ComicBookmark{ComicID: comicID, PageIndex: p}).Error; err != nil {
			return nil, err
		}
	}
	return norm, nil
}

// FixOrphanChapters 修复历史宽松规则遗留的孤儿节点（parentId=0 但 level>1）：
// 幂等迁移——挂靠到「物理页码 ≤ 自身、层级 = level-1」的最近节点下；
// 无合适父级则保持为根。先修二级再修三级，保证父子链逐级收敛。
func FixOrphanChapters(db *gorm.DB, comicID string) (fixed int, err error) {
	var orphans []models.ComicChapter
	if err := db.Where("comic_id = ? AND parent_id = 0 AND level > 1", comicID).
		Order("level ASC, id ASC").Find(&orphans).Error; err != nil {
		return 0, err
	}
	for _, o := range orphans {
		var cand models.ComicChapter
		err := db.Where("comic_id = ? AND level = ? AND page_index <= ?",
			comicID, o.Level-1, o.PageIndex).
			Order("page_index DESC, id DESC").First(&cand).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue // 无合适父级，保持为根
			}
			return fixed, err
		}
		if err := db.Model(&o).Update("parent_id", cand.ID).Error; err != nil {
			return fixed, err
		}
		fixed++
	}
	return fixed, nil
}

// ListComicChapters 读取章节标记（同级按物理页码升序，id 作次级稳定序）
func ListComicChapters(db *gorm.DB, comicID string) ([]models.ComicChapter, error) {
	if _, err := FixOrphanChapters(db, comicID); err != nil {
		return nil, err
	}
	var list []models.ComicChapter
	err := db.Where("comic_id = ?", comicID).Order("page_index ASC, id ASC").Find(&list).Error
	return list, err
}

// ChapterInput 章节创建 / 更新入参
type ChapterInput struct {
	ParentID  uint   `json:"parentId"`
	Level     int    `json:"level"`
	Title     string `json:"title"`
	PageIndex int    `json:"pageIndex"`
	OrderNo   int    `json:"orderNo"`
}

// validateChapter 校验章节层级 / 父节点 / 起始页
// 严格顺序：一级必为根（不可选父级）；二级父级必为一级；三级父级必为二级。
func validateChapter(db *gorm.DB, comicID string, parentID uint, level, pageIndex, physicalCount int) error {
	if level < 1 || level > 3 {
		return ErrMarkInvalidLevel
	}
	if pageIndex < 0 || (physicalCount >= 0 && pageIndex >= physicalCount) {
		return ErrMarkPageOutOfRange
	}
	if level == 1 {
		if parentID != 0 {
			return ErrMarkRootHasParent
		}
		return nil
	}
	// 二 / 三级：必须选择父级，且父级层级严格为 level-1
	if parentID == 0 {
		return ErrMarkNoParent
	}
	var parent models.ComicChapter
	if err := db.Where("id = ? AND comic_id = ?", parentID, comicID).First(&parent).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrMarkParentNotFound
		}
		return err
	}
	if parent.Level != level-1 {
		return ErrMarkParentLevel
	}
	return nil
}

// collectChapterSubtree 收集某章节的全部后代 id（不含自身）
func collectChapterSubtree(db *gorm.DB, comicID string, parentID uint, out *[]uint) {
	var children []models.ComicChapter
	if err := db.Where("comic_id = ? AND parent_id = ?", comicID, parentID).Find(&children).Error; err != nil {
		return
	}
	for _, ch := range children {
		*out = append(*out, ch.ID)
		collectChapterSubtree(db, comicID, ch.ID, out)
	}
}

// CreateComicChapter 创建章节标记
func CreateComicChapter(db *gorm.DB, comicID string, in ChapterInput, physicalCount int) (*models.ComicChapter, error) {
	if err := validateChapter(db, comicID, in.ParentID, in.Level, in.PageIndex, physicalCount); err != nil {
		return nil, err
	}
	ch := models.ComicChapter{
		ComicID:   comicID,
		ParentID:  in.ParentID,
		Level:     in.Level,
		Title:     in.Title,
		PageIndex: in.PageIndex,
		OrderNo:   in.OrderNo,
	}
	if err := db.Create(&ch).Error; err != nil {
		return nil, err
	}
	return &ch, nil
}

// UpdateComicChapter 更新章节标记（层级 / 父节点 / 名称 / 起始页 / 排序）
func UpdateComicChapter(db *gorm.DB, comicID string, id uint, in ChapterInput, physicalCount int) (*models.ComicChapter, error) {
	var ch models.ComicChapter
	if err := db.Where("id = ? AND comic_id = ?", id, comicID).First(&ch).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrMarkChapterNotFound
		}
		return nil, err
	}
	// 防成环：父节点不能是自己或自己的后代
	if in.ParentID != 0 {
		if in.ParentID == id {
			return nil, ErrMarkParentCycle
		}
		var subtree []uint
		collectChapterSubtree(db, comicID, id, &subtree)
		for _, s := range subtree {
			if s == in.ParentID {
				return nil, ErrMarkParentCycle
			}
		}
	}
	if err := validateChapter(db, comicID, in.ParentID, in.Level, in.PageIndex, physicalCount); err != nil {
		return nil, err
	}
	// 子级一致性：节点改层级时必须无子级（否则子级层级链断裂——子级固定为 level+1，
	// 父级层级变化后不再满足「父级层级 = 子级层级 - 1」）。层级不变时改父级/名称不受限。
	if in.Level != ch.Level {
		var childCount int64
		if err := db.Model(&models.ComicChapter{}).
			Where("comic_id = ? AND parent_id = ?", comicID, id).Count(&childCount).Error; err != nil {
			return nil, err
		}
		if childCount > 0 {
			return nil, ErrMarkHasChildren
		}
	}
	ch.ParentID = in.ParentID
	ch.Level = in.Level
	ch.Title = in.Title
	ch.PageIndex = in.PageIndex
	ch.OrderNo = in.OrderNo
	if err := db.Save(&ch).Error; err != nil {
		return nil, err
	}
	return &ch, nil
}

// DeleteComicChapter 删除章节及其全部后代子树
func DeleteComicChapter(db *gorm.DB, comicID string, id uint) error {
	var ch models.ComicChapter
	if err := db.Where("id = ? AND comic_id = ?", id, comicID).First(&ch).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrMarkChapterNotFound
		}
		return err
	}
	var ids []uint
	collectChapterSubtree(db, comicID, id, &ids)
	ids = append(ids, id)
	if err := db.Where("id IN ?", ids).Delete(&models.ComicChapter{}).Error; err != nil {
		return err
	}
	return nil
}

// PurgeHiddenMarks 隐藏页联动清理：
// 最终隐藏集合内的物理页，其书签删除；章节起始页被隐藏则删除该章节节点及其子树。
// 返回被清理的书签 / 章节数量。
func PurgeHiddenMarks(db *gorm.DB, comicID string, hidden []int) (removedBookmarks int, removedChapters int, err error) {
	if len(hidden) == 0 {
		return 0, 0, nil
	}
	res := db.Where("comic_id = ? AND page_index IN ?", comicID, hidden).Delete(&models.ComicBookmark{})
	if res.Error != nil {
		return 0, 0, res.Error
	}
	removedBookmarks = int(res.RowsAffected)

	var roots []models.ComicChapter
	if err := db.Where("comic_id = ? AND page_index IN ?", comicID, hidden).Find(&roots).Error; err != nil {
		return removedBookmarks, 0, err
	}
	seen := map[uint]bool{}
	for _, root := range roots {
		if seen[root.ID] {
			continue
		}
		var ids []uint
		collectChapterSubtree(db, comicID, root.ID, &ids)
		ids = append(ids, root.ID)
		for _, id := range ids {
			seen[id] = true
		}
		if err := db.Where("id IN ?", ids).Delete(&models.ComicChapter{}).Error; err != nil {
			return removedBookmarks, removedChapters, err
		}
		removedChapters += len(ids)
	}
	return removedBookmarks, removedChapters, nil
}
