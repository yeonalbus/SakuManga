// cmd_debug/xpseed：Round32 实机验证用一次性数据注入工具
// 向指定 sqlite 库插入/重置一个测试用户（bcrypt 哈希），供隔离库登录验证。
// ⚠️ 仅用于隔离的临时库副本，禁止指向 backend/manga.db 真实库。
// 用法：go run ./cmd_debug/xpseed <manga.db 路径> <用户名> <明文密码>
package main

import (
	"fmt"
	"os"

	"SakuManga/internal/models"
	"SakuManga/internal/services"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func main() {
	if len(os.Args) < 4 {
		fmt.Println("usage: xpseed <manga.db path> <username> <plain password>")
		return
	}
	dbPath, username, password := os.Args[1], os.Args[2], os.Args[3]

	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		panic("打开库失败: " + err.Error())
	}
	if err := db.AutoMigrate(&models.User{}); err != nil {
		panic("迁移失败: " + err.Error())
	}

	hash, err := services.HashPassword(password)
	if err != nil {
		panic("哈希失败: " + err.Error())
	}

	var user models.User
	err = db.Where("username = ?", username).First(&user).Error
	if err != nil {
		user = models.User{Username: username, Role: "admin", AllowDownload: true, PasswordHash: hash}
		if err := db.Create(&user).Error; err != nil {
			panic("创建用户失败: " + err.Error())
		}
		fmt.Printf("created user %s (id=%d)\n", username, user.ID)
		return
	}

	user.PasswordHash = hash
	user.Role = "admin"
	if err := db.Save(&user).Error; err != nil {
		panic("更新用户失败: " + err.Error())
	}
	fmt.Printf("updated user %s (id=%d)\n", username, user.ID)
}
