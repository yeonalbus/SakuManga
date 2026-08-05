// 临时调试工具：打印 favorite_states / offline_comics 表的实际结构
package main

import (
	"fmt"

	"SakuHentai/internal/database"
	"SakuHentai/internal/models"
	"SakuHentai/internal/services"

	"gorm.io/gorm/clause"
)

func main() {
	// 调用生产初始化逻辑（AutoMigrate + favorite_states 表结构迁移）
	database.InitDB()
	db := database.DB

	tables := []string{"favorite_states", "offline_comics"}
	for _, t := range tables {
		fmt.Printf("===== schema of %s =====\n", t)
		var cols []map[string]interface{}
		if err := db.Raw(fmt.Sprintf("PRAGMA table_info(%s)", t)).Scan(&cols).Error; err != nil {
			fmt.Println("table_info error:", err)
			continue
		}
		for _, c := range cols {
			fmt.Printf("  %-20v %-12v pk=%v notnull=%v dflt=%v\n",
				c["name"], c["type"], c["pk"], c["notnull"], c["dflt_value"])
		}
		var idx []map[string]interface{}
		if err := db.Raw(fmt.Sprintf("PRAGMA index_list(%s)", t)).Scan(&idx).Error; err != nil {
			fmt.Println("index_list error:", err)
			continue
		}
		fmt.Println("  -- indexes --")
		for _, i := range idx {
			fmt.Printf("  idx=%v unique=%v\n", i["name"], i["unique"])
		}
	}

	fmt.Println("===== favorite_states count =====")
	var favCount int64
	db.Table("favorite_states").Count(&favCount)
	fmt.Println("count:", favCount)

	fmt.Println("===== favorite_states by user / fav_cat =====")
	var dist []map[string]interface{}
	db.Raw("SELECT user_id, fav_cat, COUNT(*) AS n FROM favorite_states GROUP BY user_id, fav_cat").Scan(&dist)
	for _, d := range dist {
		fmt.Printf("  user_id=%v fav_cat=%v n=%v\n", d["user_id"], d["fav_cat"], d["n"])
	}

	fmt.Println("===== users =====")
	var users []map[string]interface{}
	db.Table("users").Find(&users)
	for _, u := range users {
		fmt.Printf("  id=%v username=%v\n", u["id"], u["username"])
	}

	fmt.Println("===== offline_comics sample (g_id) =====")
	type row struct {
		GID string `gorm:"column:g_id"`
	}
	var rows []row
	db.Table("offline_comics").Select("g_id").Limit(10).Find(&rows)
	for _, r := range rows {
		fmt.Println("g_id:", r.GID)
	}
	var offCount int64
	db.Table("offline_comics").Count(&offCount)
	fmt.Println("offline_comics count:", offCount)

	fmt.Println("===== 验证 ON CONFLICT (user_id,g_id) upsert =====")
	// 复用第一条既有 g_id 做更新式 upsert（不新增数据，仅验证不再报 SQL 错误）
	var first models.FavoriteState
	if err := db.Order("g_id").First(&first).Error; err == nil {
		up := models.FavoriteState{
			UserID: 1,
			GID:    first.GID,
			Token:  first.Token,
			FavCat: first.FavCat,
		}
		err = db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_id"}, {Name: "g_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"fav_cat", "token", "updated_at"}),
		}).Create(&up).Error
		fmt.Printf("upsert(gid=%s): err=%v\n", first.GID, err)
	}

	fmt.Println("===== 验证 AttachFavoriteStates / AttachDownloadStates =====")
	samples := []services.OnlineComicDTO{
		{ID: first.GID, IsFavorite: false, IsDownloaded: false},
		{ID: "4098402", IsFavorite: false, IsDownloaded: false},
		{ID: "999999999999", IsFavorite: false, IsDownloaded: false},
	}
	res := services.AttachFavoriteStates(db, 1, samples)
	res = services.AttachDownloadStates(db, res)
	for _, c := range res {
		favIdx := "-"
		if c.FavIndex != nil {
			favIdx = fmt.Sprintf("%d", *c.FavIndex)
		}
		fmt.Printf("gid=%s isFavorite=%v favIndex=%s isDownloaded=%v\n",
			c.ID, c.IsFavorite, favIdx, c.IsDownloaded)
	}
}
