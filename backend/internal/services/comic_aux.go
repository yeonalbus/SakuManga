package services

import (
	"log"

	"gorm.io/gorm"

	"SakuManga/internal/models"
)

// CleanupComicAuxiliaryBatch 清理/迁移离线漫画的辅助数据（D14）。
//
// 与 CleanupComicReferences（历史/书架/阅读清单）互补，覆盖此前删除链遗漏的表：
//   - ComicRating（复合主键 user_id+comic_id）：有替代记录则迁移（目标已有评分则删旧保留目标）；无则删除
//   - ComicBookmark / ComicChapter：直接删除（按物理页索引绑定旧本，不迁移）
func CleanupComicAuxiliaryBatch(db *gorm.DB, oldIDs []string, replacement map[string]string) {
	if db == nil || len(oldIDs) == 0 {
		return
	}
	for _, oldID := range oldIDs {
		if oldID == "" {
			continue
		}
		newID := ""
		if replacement != nil {
			newID = replacement[oldID]
		}

		if newID != "" {
			// 评分迁移：逐用户；目标已有评分时删除旧行（保留用户在新记录上的评分）
			var ratings []models.ComicRating
			if err := db.Where("comic_id = ?", oldID).Find(&ratings).Error; err == nil {
				for _, r := range ratings {
					var exists int64
					if err := db.Model(&models.ComicRating{}).
						Where("user_id = ? AND comic_id = ?", r.UserID, newID).
						Count(&exists).Error; err != nil {
						continue
					}
					if exists > 0 {
						db.Where("user_id = ? AND comic_id = ?", r.UserID, oldID).Delete(&models.ComicRating{})
						continue
					}
					db.Model(&models.ComicRating{}).
						Where("user_id = ? AND comic_id = ?", r.UserID, oldID).
						Update("comic_id", newID)
				}
			}
		} else {
			if err := db.Where("comic_id = ?", oldID).Delete(&models.ComicRating{}).Error; err != nil {
				log.Printf("%s [aux] 清理评分失败 comic=%s: %v", dlWarnTag, oldID, err)
			}
		}

		// 书签/章节：与旧本的物理页绑定，迁移无意义 → 直接删除
		if err := db.Where("comic_id = ?", oldID).Delete(&models.ComicBookmark{}).Error; err != nil {
			log.Printf("%s [aux] 清理书签失败 comic=%s: %v", dlWarnTag, oldID, err)
		}
		if err := db.Where("comic_id = ?", oldID).Delete(&models.ComicChapter{}).Error; err != nil {
			log.Printf("%s [aux] 清理章节失败 comic=%s: %v", dlWarnTag, oldID, err)
		}
	}
}
