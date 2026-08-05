package services

import (
	"log"
	"time"

	"gorm.io/gorm"
)

// ─────────────────────────────────────────────────────────────
// Tag 维护定时调度器
//
// 调度规则（东八区 Asia/Shanghai）：
//   - 每日 RefreshHour 点（默认 6:00）：联网刷新全部含 gid 漫画的 OnlineTags
//   - 每周 WritebackWeekday（0=周日）的 WritebackHour 点（默认 6:00）：反向写回 ComicInfo
//
// 实现复用 ToplistService.StartScheduler 的 time.Sleep(time.Until(next)) 模式，
// 不引入 cron 依赖；Windows 下通过 time.LoadLocation 或 FixedZone 保证东八区。
// ─────────────────────────────────────────────────────────────

// tagMaintainTrigger 下一次触发信息
type tagMaintainTrigger struct {
	At   time.Time
	Kind string // "daily" | "weekly"
}

// StartTagMaintainScheduler 启动 Tag 维护调度器（goroutine 内运行，永不返回）
func StartTagMaintainScheduler(db *gorm.DB, svc *TagMaintainService) {
	go func() {
		// 启动时执行一次旧数据迁移（Tags → OnlineTags）
		if _, err := MigrateLegacyTags(db); err != nil {
			log.Printf("%s [tagm] 旧 Tags 迁移失败: %v", dlErrTag, err)
		}
		for {
			next := nextTagMaintainTrigger(db)
			log.Printf("%s [tagm] 下次 Tag 维护触发: %s（%s）", dlLogTag,
				next.At.In(tagMaintainLoc).Format("2006-01-02 15:04:05 MST"), next.Kind)
			time.Sleep(time.Until(next.At))

			setting := LoadTagMaintainSetting(db)

			if next.Kind == "daily" && setting.EnableDailyRefresh {
				log.Printf("%s [tagm] 触发每日 Tag 刷新（东八区 %02d:00）", dlLogTag, setting.RefreshHour)
				if _, err := svc.RefreshAllTags(); err != nil {
					log.Printf("%s [tagm] 每日 Tag 刷新失败: %v", dlErrTag, err)
				}
			} else if setting.EnableWeeklyWriteback {
				log.Printf("%s [tagm] 触发每周反向写回 ComicInfo（东八区 周%d %02d:00）",
					dlLogTag, setting.WritebackWeekday, setting.WritebackHour)
				if _, err := svc.WritebackComicInfo(); err != nil {
					log.Printf("%s [tagm] 每周反向写回失败: %v", dlErrTag, err)
				}
			}
		}
	}()
}

// nextTagMaintainTrigger 计算下一次需要执行的时刻与类型。
// 取「每日刷新时刻」与「每周写回时刻」中更近的一个。
func nextTagMaintainTrigger(db *gorm.DB) tagMaintainTrigger {
	setting := LoadTagMaintainSetting(db)
	now := time.Now().In(tagMaintainLoc)

	// 每日刷新时刻
	daily := time.Date(now.Year(), now.Month(), now.Day(), setting.RefreshHour, 0, 0, 0, tagMaintainLoc)
	if !daily.After(now) {
		daily = daily.AddDate(0, 0, 1)
	}

	// 每周写回时刻：本周 WritebackWeekday 的 WritebackHour
	weekday := time.Weekday(setting.WritebackWeekday)
	daysUntil := (int(weekday) - int(now.Weekday()) + 7) % 7
	weekly := time.Date(now.Year(), now.Month(), now.Day(), setting.WritebackHour, 0, 0, 0, tagMaintainLoc).AddDate(0, 0, daysUntil)
	if !weekly.After(now) {
		weekly = weekly.AddDate(0, 0, 7)
	}

	if daily.Before(weekly) {
		return tagMaintainTrigger{At: daily, Kind: "daily"}
	}
	return tagMaintainTrigger{At: weekly, Kind: "weekly"}
}
