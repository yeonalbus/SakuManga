package services

import (
	"log"
	"time"

	"gorm.io/gorm"

	"SakuHentai/internal/models"
)

// ─────────────────────────────────────────────────────────────
// 每周自动更新扫描调度器（Round4 任务四）
//
// 调度规则（系统本地时区）：
//   - 每周 ScanWeekday（0=周日）的 ScanHour 点（默认 6:00）：
//     执行一轮完整更新扫描（常规更新检测 + 老化判定），见 RunUpdateScanWithProgress。
//
// 实现复用 TagMaintain 的 time.Sleep(time.Until(next)) 模式，不引入 cron 依赖；
// 基准时区使用 time.Local（系统本地时区），与计算机时间保持一致。
// ─────────────────────────────────────────────────────────────

// updateScanLoc 周扫描系统本地时区（与 TagMaintain 保持一致）
var updateScanLoc = time.Local

// LoadUpdateScanSetting 读取单例设置（不存在则创建默认值）
func LoadUpdateScanSetting(db *gorm.DB) *models.UpdateScanSetting {
	var setting models.UpdateScanSetting
	if err := db.First(&setting, 1).Error; err != nil {
		setting = models.UpdateScanSetting{
			ID:               1,
			EnableWeeklyScan: false,
			ScanWeekday:      0,
			ScanHour:         6,
		}
		if err := db.Create(&setting).Error; err != nil {
			log.Printf("%s 初始化 UpdateScanSetting 失败: %v", dlErrTag, err)
		}
	}
	return &setting
}

// SaveUpdateScanSetting 保存设置并返回最新值
func SaveUpdateScanSetting(db *gorm.DB, setting *models.UpdateScanSetting) (*models.UpdateScanSetting, error) {
	setting.ID = 1
	setting.UpdatedAt = time.Now()
	if err := db.Save(setting).Error; err != nil {
		return nil, err
	}
	return setting, nil
}

// nextUpdateScanTrigger 计算下一次周扫描触发时刻（本地 ScanWeekday 的 ScanHour）
func nextUpdateScanTrigger(db *gorm.DB) time.Time {
	setting := LoadUpdateScanSetting(db)
	now := time.Now().In(updateScanLoc)

	weekday := time.Weekday(setting.ScanWeekday)
	daysUntil := (int(weekday) - int(now.Weekday()) + 7) % 7
	next := time.Date(now.Year(), now.Month(), now.Day(), setting.ScanHour, 0, 0, 0, updateScanLoc).AddDate(0, 0, daysUntil)
	if !next.After(now) {
		next = next.AddDate(0, 0, 7)
	}
	return next
}

// StartUpdateScanScheduler 启动每周自动更新扫描调度器（goroutine 内运行，永不返回）。
// 到点后先检查 EnableWeeklyScan 开关，开启才执行；关闭则直接进入下一轮等待。
func StartUpdateScanScheduler(db *gorm.DB, ehService *EHService, manager *DownloadManager) {
	go func() {
		for {
			next := nextUpdateScanTrigger(db)
			log.Printf("%s [update] 下次周扫描触发: %s", dlLogTag, next.In(updateScanLoc).Format("2006-01-02 15:04:05 MST"))
			time.Sleep(time.Until(next))

			setting := LoadUpdateScanSetting(db)
			if !setting.EnableWeeklyScan {
				continue
			}

			log.Printf("%s [update] 触发每周自动更新扫描（本地 周%d %02d:00）",
				dlLogTag, setting.ScanWeekday, setting.ScanHour)
			runWeeklyUpdateScan(db, ehService, manager)
		}
	}()
}

// runWeeklyUpdateScan 执行一轮周扫描：占用单槽位 → 常规更新检测 + 老化判定 →
// 自动更新入队（若开启）→ 记录上次执行时间。
func runWeeklyUpdateScan(db *gorm.DB, ehService *EHService, manager *DownloadManager) {
	if !StartOfflineTask(OfflineTaskUpdate) {
		log.Printf("%s [update] 周扫描被跳过：已有离线维护任务正在运行", dlWarnTag)
		return
	}

	result, _, err := RunUpdateScanWithProgress(db, ehService, OfflineUpdateProgressSink)
	if err != nil {
		FinishOfflineTask(err)
		return
	}
	StoreUpdateCheckResult(result)

	// 自动更新画廊（与手动触发一致，按 autoUpdateGallery 开关）
	if manager != nil && manager.GetSettings().AutoUpdateGallery {
		enqueued, skipped := AutoEnqueueUpdates(db, manager, result)
		log.Printf("%s [update] 周扫描完成：需要更新 %d 个，自动入队 %d 个，跳过 %d 个（autoUpdateGallery=true）",
			dlLogTag, len(result.NeedsUpdate), enqueued, skipped)
	}

	FinishOfflineTask(nil)

	setting := LoadUpdateScanSetting(db)
	setting.LastWeeklyScanAt = time.Now().UnixMilli()
	if _, err := SaveUpdateScanSetting(db, setting); err != nil {
		log.Printf("%s [update] 更新周扫描执行时间失败: %v", dlErrTag, err)
	}
	log.Printf("%s [update] 周扫描执行完毕", dlLogTag)
}
