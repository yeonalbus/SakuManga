package services

import (
	"log"
	"time"

	"SakuManga/internal/models"

	"gorm.io/gorm"
)

// ─────────────────────────────────────────────────────────────
// 归档 ↔ 节点绑定
//
// 用途：重试时若报价为 GP（说明出口 IP 已变、需重新扣费），
// 先尝试切回"当初解锁这个画廊的节点"，恢复到免费解锁状态，省下一次 GP。
//
// 主键是节点名而非 IP：Clash 只能按节点名切换；IP 仅作校验/审计。
// ─────────────────────────────────────────────────────────────

// archiveBindingTTL 绑定有效期：与 E 站"同 IP 24h 内重试免费"对齐，保守取 20h。
const archiveBindingTTL = 20 * time.Hour

// UpsertArchiveBinding 写入或刷新某个画廊的绑定（同 gid + user 只保留最新一条）。
func UpsertArchiveBinding(db *gorm.DB, b *models.ArchiveBinding) error {
	if db == nil || b == nil || b.GID == "" {
		return nil
	}
	now := time.Now()
	if b.CreatedAt.IsZero() {
		b.CreatedAt = now
	}
	if b.ExpiresAt.IsZero() {
		b.ExpiresAt = now.Add(archiveBindingTTL)
	}

	// 覆盖式更新：同一画廊同一账号只保留最新一次解锁的节点
	if err := db.Where("g_id = ? AND user_id = ?", b.GID, b.UserID).
		Delete(&models.ArchiveBinding{}).Error; err != nil {
		log.Printf("%s 清理旧绑定失败（gid=%s）：%v", clashLogTag, b.GID, err)
	}
	b.ID = 0
	if err := db.Create(b).Error; err != nil {
		log.Printf("%s 写入绑定失败（gid=%s node=%q）：%v", clashLogTag, b.GID, b.Node, err)
		return err
	}
	return nil
}

// FindActiveBinding 查询某个画廊当前仍可用的绑定（未过期、未消费）。
// 没有则返回 (nil, nil)。
func FindActiveBinding(db *gorm.DB, gid string, userID uint) (*models.ArchiveBinding, error) {
	if db == nil || gid == "" {
		return nil, nil
	}
	var b models.ArchiveBinding
	err := db.Where("g_id = ? AND user_id = ? AND consumed = ? AND expires_at > ?",
		gid, userID, false, time.Now()).
		Order("id DESC").First(&b).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &b, nil
}

// ConsumeBinding 标记某个画廊的绑定已消费（下载成功，无需再切回）。
func ConsumeBinding(db *gorm.DB, gid string, userID uint) {
	if db == nil || gid == "" {
		return
	}
	if err := db.Model(&models.ArchiveBinding{}).
		Where("g_id = ? AND user_id = ?", gid, userID).
		Update("consumed", true).Error; err != nil {
		log.Printf("%s 标记绑定已消费失败（gid=%s）：%v", clashLogTag, gid, err)
	}
}

// GCBindings 清理过期或已消费超过 1 小时的绑定，返回删除条数。
// 惰性调用（挂在归档调度 tick 上），不引入独立定时器。
func GCBindings(db *gorm.DB) int64 {
	if db == nil {
		return 0
	}
	now := time.Now()
	res := db.Where("expires_at < ? OR (consumed = ? AND created_at < ?)",
		now, true, now.Add(-time.Hour)).
		Delete(&models.ArchiveBinding{})
	if res.Error != nil {
		log.Printf("%s 清理绑定失败：%v", clashLogTag, res.Error)
		return 0
	}
	return res.RowsAffected
}
