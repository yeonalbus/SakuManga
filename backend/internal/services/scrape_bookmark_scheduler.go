package services

import (
	"log"
	"time"

	"SakuManga/internal/models"

	"gorm.io/gorm"
)

// ─────────────────────────────────────────────────────────────
// Round49：书签更新探测调度器（每日一轮）
//
// 触发时刻复用 Tag 维护设置里的「每日刷新时刻」RefreshHour（默认 6:00），
// 不新增独立设置项；但**不读 EnableDailyRefresh 开关**——书签探测有自己的
// 门禁（每条书签的 updateWatch），关掉 Tag 刷新不应连带静默关掉书签探测。
//
// 实现沿用 update_scheduler / tag_scheduler 的 time.Sleep(time.Until(next))
// 范式，不引入 cron 依赖；时区与 Tag 维护一致（tagMaintainLoc）。
//
// 每轮遍历所有用户：未绑定 E 站凭据的用户跳过；其余按各自凭据串行探测。
// ─────────────────────────────────────────────────────────────

// StartScrapeBookmarkProbeScheduler 启动每日书签更新探测（goroutine 内运行，永不返回）
func StartScrapeBookmarkProbeScheduler(db *gorm.DB, eh *EHService) {
	go func() {
		for {
			next := nextBookmarkProbeTrigger(db)
			log.Printf("%s [BOOKMARK] 下次书签更新探测: %s", dlLogTag,
				next.Format("2006-01-02 15:04:05 MST"))
			time.Sleep(time.Until(next))
			runBookmarkProbeRound(db, eh)
		}
	}()
}

// nextBookmarkProbeTrigger 下次触发时刻：本地时区 RefreshHour 的下一整点日。
func nextBookmarkProbeTrigger(db *gorm.DB) time.Time {
	setting := LoadTagMaintainSetting(db)
	now := time.Now().In(tagMaintainLoc)
	next := time.Date(now.Year(), now.Month(), now.Day(), setting.RefreshHour, 0, 0, 0, tagMaintainLoc)
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

// runBookmarkProbeRound 遍历全部用户，探测各自开启了更新开关的书签。
func runBookmarkProbeRound(db *gorm.DB, eh *EHService) {
	var users []models.User
	if err := db.Find(&users).Error; err != nil {
		log.Printf("%s [BOOKMARK] 读取用户列表失败: %v", dlErrTag, err)
		return
	}

	total, updated, failed := 0, 0, 0
	for i := range users {
		u := &users[i]
		if u.IPBMemberID == "" || u.IPBPassHash == "" {
			continue // 未绑定 E 站凭据
		}
		account := &models.AccountSetting{
			ID:          u.ID,
			IPBMemberID: u.IPBMemberID,
			IPBPassHash: u.IPBPassHash,
			Igneous:     u.Igneous,
			SK:          u.SK,
			IsEx:        u.IsEx,
		}
		setting := loadEHSettingForUser(db, u.ID)
		results, err := ProbeScrapeBookmarks(db, eh, u.ID, account, setting, nil)
		if err != nil {
			log.Printf("%s [BOOKMARK] 用户 %s 探测失败: %v", dlErrTag, u.Username, err)
			failed++
			continue
		}
		for _, r := range results {
			total++
			switch {
			case r.Status == BookmarkProbeOK && r.HasUpdate:
				updated++
			case r.Status != BookmarkProbeOK:
				failed++
			}
		}
	}
	if total > 0 || failed > 0 {
		log.Printf("%s [BOOKMARK] 每日更新探测完成：%d 条，有更新 %d 条，未判定 %d 条",
			dlLogTag, total, updated, failed)
	}
}

// loadEHSettingForUser 读取用户 EHSetting；无记录时返回默认值（不落库，避免探测时写库）。
func loadEHSettingForUser(db *gorm.DB, userID uint) *models.EHSetting {
	var s models.EHSetting
	if err := db.Where("user_id = ?", userID).First(&s).Error; err != nil {
		return &models.EHSetting{UserID: userID, Site: "e-hentai", PreferRedirect: true}
	}
	return &s
}
