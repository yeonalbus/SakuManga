package services

import (
	"log"
	"time"

	"gorm.io/gorm"
)

// ─────────────────────────────────────────────────────────────
// Tag 维护定时调度器
//
// 调度规则（系统本地时区）：
//   - 每日 RefreshHour 点（默认 6:00）：联网刷新全部含 gid 漫画的 OnlineTags
//
// 实现复用 ToplistService.StartScheduler 的 time.Sleep(time.Until(next)) 模式，
// 不引入 cron 依赖；基准时区使用 time.Local（系统本地时区），与计算机时间保持一致。
// ─────────────────────────────────────────────────────────────

// StartTagMaintainScheduler 启动 Tag 维护调度器（goroutine 内运行，永不返回）
func StartTagMaintainScheduler(db *gorm.DB, svc *TagMaintainService) {
	go func() {
		// 启动时执行一次旧数据迁移（Tags → OnlineTags）
		if _, err := MigrateLegacyTags(db); err != nil {
			log.Printf("%s [tagm] 旧 Tags 迁移失败: %v", dlErrTag, err)
		}
		// Round46：清洗历史 Tag 别名后缀（旧解析把 E-H 显示文本当 key，
		// 如 "japanese | definition revised please read the wiki"、"cure answer | anna akechi"）。
		// 幂等：仅处理含 "|" 的项，无变更时不写库。
		if comicsFixed, tagsFixed, err := repairTagAliasSuffix(db); err != nil {
			log.Printf("%s [tagm] Tag 别名后缀清洗失败: %v", dlErrTag, err)
		} else if comicsFixed > 0 {
			log.Printf("%s [tagm] Tag 别名后缀清洗完成：修复 %d 个漫画 / %d 个 tag", dlLogTag, comicsFixed, tagsFixed)
			XpRebuildAsync("Tag 别名后缀清洗")
		}
		for {
			nextAt := nextTagMaintainTrigger(db)
			log.Printf("%s [tagm] 下次每日 Tag 刷新触发: %s", dlLogTag,
				nextAt.In(tagMaintainLoc).Format("2006-01-02 15:04:05 MST"))
			time.Sleep(time.Until(nextAt))

			setting := LoadTagMaintainSetting(db)
			if setting.EnableDailyRefresh {
				log.Printf("%s [tagm] 触发每日 Tag 刷新（本地 %02d:00）", dlLogTag, setting.RefreshHour)
				if _, err := svc.RefreshAllTags(); err != nil {
					log.Printf("%s [tagm] 每日 Tag 刷新失败: %v", dlErrTag, err)
				}
			}
		}
	}()
}

// nextTagMaintainTrigger 计算下一次每日刷新时刻。
func nextTagMaintainTrigger(db *gorm.DB) time.Time {
	setting := LoadTagMaintainSetting(db)
	now := time.Now().In(tagMaintainLoc)

	daily := time.Date(now.Year(), now.Month(), now.Day(), setting.RefreshHour, 0, 0, 0, tagMaintainLoc)
	if !daily.After(now) {
		daily = daily.AddDate(0, 0, 1)
	}
	return daily
}
