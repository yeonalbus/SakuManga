package services

import (
	"log"
	"strings"

	"SakuManga/internal/models"

	"gorm.io/gorm"
)

// stripTagAliasSuffix 去掉单个 tag 的 " | 别名/提示" 后缀，只保留规范 key。
//
// 例：
//
//	"language:japanese | definition revised please read the wiki" → "language:japanese"
//	"character:cure answer | anna akechi"                        → "character:cure answer"
//
// 不含 "|" 的 tag 原样返回；截断后为空（异常数据）也原样返回，避免误删。
func stripTagAliasSuffix(raw string) string {
	i := strings.Index(raw, "|")
	if i < 0 {
		return raw
	}
	left := strings.TrimSpace(raw[:i])
	if left == "" {
		return raw
	}
	return left
}

// repairTagAliasSuffix 清洗历史脏 tag（Round46 存量自愈）。
//
// 背景：旧版 FetchGalleryDetail 取 E-H 页面 <a> 的显示文本入库，而 E-H 对部分 tag 显示
// "主 tag | 别名/wiki 提示"（如 "japanese | definition revised please read the wiki"、
// "cure answer | anna akechi"），导致 online_tags 被污染 → 词典翻译全 miss、
// 点击 tag 生成 `ns:"key | alias$"` 搜不到内容、XP 词云出现脏词条。
//
// 本迁移幂等：
//   - 只处理含 "|" 的 tag 元素（两侧空格的 " | " 只是其中一种形态，统一按第一个 "|" 截断）；
//   - 覆盖 online_tags 与旧 tags 两列（后者在 online_tags 为空时被列表接口回退使用）；
//   - 仅在确有变化时写库，且不触碰 updated_at（避免影响按更新时间排序）。
//
// 返回：修复的漫画数、修复的 tag 条数。
func repairTagAliasSuffix(db *gorm.DB) (comicsFixed, tagsFixed int, err error) {
	var comics []models.OfflineComic
	if err = db.Find(&comics).Error; err != nil {
		return 0, 0, err
	}

	for i := range comics {
		c := &comics[i]
		updates := make(map[string]interface{}, 2)
		comicChanged := false

		columns := []struct {
			name string
			val  *string
		}{
			{"online_tags", &c.OnlineTags},
			{"tags", &c.Tags},
		}
		for _, col := range columns {
			raw := strings.TrimSpace(*col.val)
			if raw == "" || !strings.Contains(raw, "|") {
				continue
			}
			tags := UnmarshalTagSlice(raw)
			if len(tags) == 0 {
				continue
			}
			changed := 0
			for j, tag := range tags {
				fixed := stripTagAliasSuffix(tag)
				if fixed != tag {
					tags[j] = fixed
					changed++
				}
			}
			if changed == 0 {
				continue
			}
			*col.val = MarshalTagSlice(tags)
			updates[col.name] = *col.val
			comicChanged = true
			tagsFixed += changed
		}

		if !comicChanged {
			continue
		}
		if uerr := db.Model(c).Updates(updates).Error; uerr != nil {
			log.Printf("%s [tagm] 清洗漫画 %s 的 Tag 别名后缀失败: %v", dlErrTag, c.ID, uerr)
			continue
		}
		comicsFixed++
	}
	return comicsFixed, tagsFixed, nil
}
