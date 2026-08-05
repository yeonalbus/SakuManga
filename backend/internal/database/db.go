// database/db.go
package database

import (
	"SakuHentai/internal/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

var DB *gorm.DB

func InitDB() {
	var err error
	DB, err = gorm.Open(sqlite.Open("manga.db"), &gorm.Config{})
	if err != nil {
		panic("无法连接数据库: " + err.Error())
	}

	DB.AutoMigrate(
		&models.AccountSetting{},
		&models.User{},
		&models.UserSession{},
		&models.ServerSetting{},
		&models.EHSetting{},
		&models.EHProfile{},
		&models.ExtraScanPath{},
		&models.OfflineComic{},
		&models.FavoriteState{},
		&models.Bookshelf{},
		&models.HistoryRecord{},
		&models.ComicRating{},
		&models.ReadingList{},
		&models.DownloadTask{},
		&models.DownloadSetting{},
		&models.TagMaintainSetting{},
	)

	migrateFavoriteStateTable()
}

// favoritePkRow PRAGMA table_info 结果行
type favoritePkRow struct {
	Name string `gorm:"column:name"`
	PK   int    `gorm:"column:pk"`
}

// hasCompositeFavoritePk 判断 favorite_states 表是否已具备 (user_id, g_id) 复合主键
func hasCompositeFavoritePk() bool {
	var cols []favoritePkRow
	if err := DB.Raw("PRAGMA table_info(favorite_states)").Scan(&cols).Error; err != nil {
		return false
	}
	userIDPk := false
	gidPk := false
	for _, c := range cols {
		switch c.Name {
		case "user_id":
			userIDPk = c.PK == 1
		case "g_id":
			gidPk = c.PK == 1
		}
	}
	return userIDPk && gidPk
}

// migrateFavoriteStateTable 修复历史版本遗留的 favorite_states 表结构。
//
// 背景：早期版本仅把 g_id 作为单列主键，且 user_id 写入 NULL（全局收藏），
// 与当前模型声明的 (user_id, g_id) 复合主键不一致。SQLite 无法直接 ALTER
// 增加复合主键，因此所有 ON CONFLICT (user_id, g_id) 的收藏写入都会报
// "ON CONFLICT clause does not match any PRIMARY KEY or UNIQUE constraint"，
// 导致收藏状态无法落库、首页/热门等处的收藏角标永不显示。
//
// 方案：采用"重建表"迁移——旧表改名 → 按当前模型重建 → 拷贝数据
// （历史遗留的 NULL user_id 归属到最早创建的用户）→ 删除旧表。
// 该函数幂等：表不存在或已是正确复合主键时直接返回。
func migrateFavoriteStateTable() {
	oldExists := DB.Migrator().HasTable("favorite_states_old")
	curExists := DB.Migrator().HasTable("favorite_states")

	// 处理上次中断的迁移
	if oldExists {
		if !curExists {
			// 中断点 1：刚改完名、尚未重建 → 改回原名重新走迁移
			DB.Migrator().RenameTable("favorite_states_old", "favorite_states")
			oldExists = false
		} else if hasCompositeFavoritePk() {
			// 中断点 2：新表已重建，仅剩删除旧表 → 补拷数据并收尾
			var oldCnt, newCnt int64
			DB.Table("favorite_states_old").Count(&oldCnt)
			DB.Table("favorite_states").Count(&newCnt)
			if newCnt < oldCnt {
				DB.Exec(`INSERT OR IGNORE INTO favorite_states (user_id, g_id, token, fav_cat, updated_at)
				         SELECT COALESCE(user_id, (SELECT MIN(id) FROM users)), g_id, token, fav_cat, updated_at
				         FROM favorite_states_old`)
			}
			DB.Migrator().DropTable("favorite_states_old")
			return
		}
	}

	// 表由 AutoMigrate 全新创建（复合主键正确）或已迁移完成 → 无需处理
	if !curExists {
		return
	}
	if hasCompositeFavoritePk() {
		return
	}

	// 异常残留的旧表（favorite_states 与 old 同时存在且 new 结构错误）先清理
	if oldExists {
		DB.Migrator().DropTable("favorite_states_old")
	}

	// 正式迁移：改名 → 重建 → 拷数据 → 删旧表
	if err := DB.Migrator().RenameTable("favorite_states", "favorite_states_old"); err != nil {
		return
	}
	if err := DB.AutoMigrate(&models.FavoriteState{}); err != nil {
		return
	}
	DB.Exec(`INSERT INTO favorite_states (user_id, g_id, token, fav_cat, updated_at)
	         SELECT COALESCE(user_id, (SELECT MIN(id) FROM users)), g_id, token, fav_cat, updated_at
	         FROM favorite_states_old`)
	DB.Migrator().DropTable("favorite_states_old")
}