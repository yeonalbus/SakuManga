package services

// Round20-Bug1/Bug4：离线漫画记录删除或替换（换新 id）后，清理/迁移各用户引用。
//
// 背景：离线漫画 ID = md5(本地路径)。更新替换（AutoUpdateDeleteOriginal 删旧版）、
// 扫描归档升级（压缩包→文件夹）都会产生「同 gid 不同 id」的新旧记录：
//   - 旧记录删除后，历史表（HistoryRecord）、书架 comicIds、离线阅读清单（ReadingList）
//     中仍残留旧 id → 历史网格重复显示同一本子（Bug1）、点击旧 id 404 报「找不到该漫画」（Bug4）。
//
// 策略（决策 D1=A / D2=A）：
//   - newID != ""（存在同 gid 替换记录）→ 引用迁移到新 id；
//   - newID == "" → 删除对应离线历史行、从书架 comicIds 剔除、从离线阅读清单剔除。
//
// 说明：在线来源的 HistoryRecord 不受影响（comic_id 即 gid，不随本地路径变化）。

import (
	"encoding/json"
	"log"

	"gorm.io/gorm"

	"SakuManga/internal/models"
)

// refsWarnTag 引用清理日志前缀
const refsWarnTag = "[REFS-WARN]"

func logWarn(format string, args ...interface{}) {
	log.Printf(refsWarnTag+" "+format, args...)
}

// parseComicIDsJSON 解析书架 comicIds JSON 数组（脏数据回退空数组）
func parseComicIDsJSON(raw string) []string {
	if raw == "" {
		return []string{}
	}
	var ids []string
	if err := json.Unmarshal([]byte(raw), &ids); err != nil {
		return []string{}
	}
	return ids
}

// joinComicIDsJSON 序列化书架 comicIds 数组
func joinComicIDsJSON(ids []string) string {
	b, _ := json.Marshal(ids)
	return string(b)
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// CleanupComicReferences 漫画记录删除/替换后的引用清理与迁移。
// oldID 必填；newID 为空 = 删除引用，非空 = 迁移到新 id。
func CleanupComicReferences(db *gorm.DB, oldID, newID string) {
	if oldID == "" {
		return
	}
	var migrate map[string]string
	if newID != "" {
		migrate = map[string]string{oldID: newID}
	}
	CleanupComicReferencesBatch(db, []string{oldID}, migrate)
}

// CleanupComicReferencesBatch 批量版引用清理/迁移（语义与 CleanupComicReferences 一致）。
//
// 存在的理由：删除「额外的画廊扫描路径」会一次删掉成百上千本离线记录，
// 逐本调用单本版会对书架与阅读清单各做一次全表扫描（O(N 本 × 全表)），
// 本函数把两处全表扫描各合并为一次。历史表则按 id 分为「迁移」与「批量删除」两批。
//
// migrate：oldID → 替换记录 id；未命中的 id 直接从引用中剔除。
func CleanupComicReferencesBatch(db *gorm.DB, oldIDs []string, migrate map[string]string) {
	if db == nil || len(oldIDs) == 0 {
		return
	}
	oldSet := make(map[string]bool, len(oldIDs))
	for _, id := range oldIDs {
		if id != "" {
			oldSet[id] = true
		}
	}
	if len(oldSet) == 0 {
		return
	}
	replacementOf := func(id string) string {
		if migrate == nil {
			return ""
		}
		return migrate[id]
	}

	// 1. 离线历史：有替换记录 → 迁移（保留进度与阅读时间）；否则计入批量删除（孤儿行）
	toDelete := make([]string, 0, len(oldSet))
	for id := range oldSet {
		newID := replacementOf(id)
		if newID == "" {
			toDelete = append(toDelete, id)
			continue
		}
		if err := db.Model(&models.HistoryRecord{}).
			Where("source = ? AND comic_id = ?", models.SourceOffline, id).
			Update("comic_id", newID).Error; err != nil {
			logWarn("CleanupComicReferencesBatch: 迁移历史失败 old=%s new=%s: %v", id, newID, err)
		}
	}
	if len(toDelete) > 0 {
		if err := db.Where("source = ? AND comic_id IN ?", models.SourceOffline, toDelete).
			Delete(&models.HistoryRecord{}).Error; err != nil {
			logWarn("CleanupComicReferencesBatch: 删除孤儿历史失败 count=%d: %v", len(toDelete), err)
		}
	}

	// 2. 书架 comicIds（全用户扫描，数量级小）
	//    Round38：同时同步手动指定的封面引用（cover_comic_id）——替换则迁移，删除则清空回退自动封面
	var shelves []models.Bookshelf
	if err := db.Find(&shelves).Error; err == nil {
		for i := range shelves {
			ids := parseComicIDsJSON(shelves[i].ComicIDs)
			changed := false
			out := make([]string, 0, len(ids))
			for _, id := range ids {
				if oldSet[id] {
					changed = true
					if newID := replacementOf(id); newID != "" && !containsString(out, newID) {
						out = append(out, newID)
					}
					continue
				}
				out = append(out, id)
			}
			if changed {
				shelves[i].ComicIDs = joinComicIDsJSON(out)
				shelves[i].Count = len(out)
			}
			// 封面引用同步（Round38）
			if cid := shelves[i].CoverComicID; cid != "" && oldSet[cid] {
				if newID := replacementOf(cid); newID != "" {
					shelves[i].CoverComicID = newID
				} else {
					shelves[i].CoverComicID = "" // 本子已删除 → 回退自动封面
				}
				changed = true
			}
			// D14：同步清理书架权重表 sortKeys（被删 id 删除；有替代记录则迁移原权重）
			if keysRaw := shelves[i].SortKeys; keysRaw != "" {
				var keys map[string]float64
				if err := json.Unmarshal([]byte(keysRaw), &keys); err == nil {
					changedKeys := false
					for id := range oldSet {
						w, ok := keys[id]
						if !ok {
							continue
						}
						delete(keys, id)
						if newID := replacementOf(id); newID != "" {
							if _, exists := keys[newID]; !exists {
								keys[newID] = w
							}
						}
						changedKeys = true
					}
					if changedKeys {
						if b, err := json.Marshal(keys); err == nil {
							shelves[i].SortKeys = string(b)
							changed = true
						}
					}
				}
			}
			if changed {
				if err := db.Save(&shelves[i]).Error; err != nil {
					logWarn("CleanupComicReferencesBatch: 更新书架失败 shelf=%s: %v", shelves[i].ID, err)
				}
			}
		}
	}

	// 3. 离线阅读清单（每用户一条 JSON 快照，迁移时同步改写 id 字段）
	var lists []models.ReadingList
	if err := db.Where("source = ?", models.SourceOffline).Find(&lists).Error; err == nil {
		for i := range lists {
			var items []map[string]interface{}
			if err := json.Unmarshal([]byte(lists[i].Items), &items); err != nil {
				continue
			}
			changed := false
			out := make([]map[string]interface{}, 0, len(items))
			for _, it := range items {
				id, _ := it["id"].(string)
				if oldSet[id] {
					changed = true
					if newID := replacementOf(id); newID != "" {
						it["id"] = newID
						out = append(out, it)
					}
					continue
				}
				out = append(out, it)
			}
			if changed {
				b, _ := json.Marshal(out)
				lists[i].Items = string(b)
				if err := db.Save(&lists[i]).Error; err != nil {
					logWarn("CleanupComicReferencesBatch: 更新阅读清单失败 user=%d: %v", lists[i].UserID, err)
				}
			}
		}
	}
}

// FindReplacementByGID 查找同 gid 的其他离线漫画记录（更新替换/删除时作为引用迁移目标）。
// 找不到返回空串。
func FindReplacementByGID(db *gorm.DB, gid, excludeID string) string {
	if db == nil || gid == "" {
		return ""
	}
	var id string
	if err := db.Model(&models.OfflineComic{}).
		Where("g_id = ? AND id != ?", gid, excludeID).
		Limit(1).
		Pluck("id", &id).Error; err != nil {
		return ""
	}
	return id
}
