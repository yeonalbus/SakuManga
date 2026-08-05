package services

import (
	"fmt"

	"SakuHentai/internal/models"

	"gorm.io/gorm"
)

// EnsureInitialAdmin 当 users 表为空时自动创建 admin / admin123（E 站凭证置空），
// 并在启动日志与控制台明确打印初始账密，避免管理员登录不进去。
// 同时把既有全局 EHSetting / EHProfile 数据迁移给 admin（admin 继承当前生效配置）。
func EnsureInitialAdmin(db *gorm.DB) error {
	var count int64
	if err := db.Model(&models.User{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	hash, err := HashPassword("admin123")
	if err != nil {
		return err
	}
	admin := models.User{
		Username:     "admin",
		PasswordHash: hash,
		Role:         RoleAdmin,
	}
	if err := db.Create(&admin).Error; err != nil {
		return err
	}

	// 迁移既有全局 EHSetting / EHProfile 给 admin（管理员继承当前生效配置）
	migrateGlobalEHSetting(db, admin.ID)

	fmt.Print(initialAdminBanner)
	return nil
}

const initialAdminBanner = "\n============================================\n" +
	"  首次启动：已创建初始管理员账号\n" +
	"  用户名: admin\n" +
	"  默认密码: admin123\n" +
	"  ⚠️ 请登录后立即在「安全设置」中修改密码\n" +
	"  ⚠️ 请登录后自行绑定 E 站凭证\n" +
	"============================================\n"

// migrateGlobalEHSetting 把旧版全局 EHSetting（ID=1）与全部 EHProfile 归属到 admin
func migrateGlobalEHSetting(db *gorm.DB, adminID uint) {
	var setting models.EHSetting
	if err := db.First(&setting, 1).Error; err == nil {
		setting.ID = 0 // 重新生成自增 ID
		setting.UserID = adminID
		db.Create(&setting)
		db.Delete(&models.EHSetting{}, 1)
	}
	// 全局 EHProfile 无 UserID（旧数据 UserID=0），迁移给 admin
	db.Model(&models.EHProfile{}).
		Where("user_id = ?", 0).
		Update("user_id", adminID)
}

// LoadAdminAccount 加载 admin 用户的 E 站凭证，构造 AccountSetting（后台维护任务专用）。
// 返回的 AccountSetting.ID 承载 admin 的用户 ID，便于后台任务按用户加载其 EHSetting。
func LoadAdminAccount(db *gorm.DB) *models.AccountSetting {
	var admin models.User
	if err := db.Where("role = ?", RoleAdmin).Order("id ASC").First(&admin).Error; err != nil {
		return &models.AccountSetting{}
	}
	return &models.AccountSetting{
		ID:          admin.ID,
		IPBMemberID: admin.IPBMemberID,
		IPBPassHash: admin.IPBPassHash,
		Igneous:     admin.Igneous,
		SK:          admin.SK,
		IsEx:        admin.IsEx,
	}
}

// LoadAdminUserID 返回 admin 用户 ID（后台任务加载其 EHSetting / 凭证使用）
func LoadAdminUserID(db *gorm.DB) uint {
	var admin models.User
	if err := db.Where("role = ?", RoleAdmin).Order("id ASC").First(&admin).Error; err != nil {
		return 0
	}
	return admin.ID
}
