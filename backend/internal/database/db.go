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
}