// cmd_debug/e2eseed：Round26 实机验证用一次性数据注入工具
// 向指定 sqlite 库插入测试漫画（id 前缀 e2e-，可重复执行：先清后插）。
// ⚠️ 仅用于隔离的临时库（go run 的 %TEMP% 库），禁止指向 backend/manga.db 真实库。
package main

import (
	"fmt"
	"os"
	"time"

	"SakuManga/internal/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: e2eseed <manga.db path>")
		return
	}
	db, err := gorm.Open(sqlite.Open(os.Args[1]), &gorm.Config{})
	if err != nil {
		panic("打开库失败: " + err.Error())
	}
	if err := db.AutoMigrate(&models.OfflineComic{}); err != nil {
		panic("迁移失败: " + err.Error())
	}

	now := time.Now()
	seeds := []models.OfflineComic{
		// 疑似重复组 A：3 成员（同标题同画师，页数差 ≤5，gid 各不相同）
		{ID: "e2e-a1", Title: "[サークル] テストタイトル", OnlineTags: `["artist:testart","language:japanese"]`, PageCount: 30, OriginalPageCount: 30, GID: "9001", Token: "t1", Source: models.SourceOffline, SourceMode: "gallery", LocalPath: "Z:\\e2e\\テストタイトル", AddedAt: now, UpdatedAt: now},
		{ID: "e2e-a2", Title: "[サークル] テストタイトル (English)", OnlineTags: `["artist:testart","language:english"]`, PageCount: 31, OriginalPageCount: 31, GID: "9002", Token: "t2", Source: models.SourceOffline, SourceMode: "gallery", LocalPath: "Z:\\e2e\\テストタイトル (English)", AddedAt: now, UpdatedAt: now},
		{ID: "e2e-a3", Title: "[サークル] テストタイトル 汉化版", OnlineTags: `["artist:testart","language:chinese"]`, PageCount: 32, OriginalPageCount: 32, GID: "9003", Token: "t3", Source: models.SourceOffline, SourceMode: "gallery", LocalPath: "Z:\\e2e\\テストタイトル 汉化版", AddedAt: now, UpdatedAt: now},
		// 续集 B：卷号不同 → 不应聚类
		{ID: "e2e-b1", Title: "[サークル] テストタイトル Vol.2", OnlineTags: `["artist:testart"]`, PageCount: 28, OriginalPageCount: 28, GID: "9004", Token: "t4", Source: models.SourceOffline, SourceMode: "gallery", LocalPath: "Z:\\e2e\\テストタイトル Vol.2", AddedAt: now, UpdatedAt: now},
		// 撞名 C：画师不同 → 不应聚类
		{ID: "e2e-c1", Title: "テストタイトル", OnlineTags: `["artist:someoneelse"]`, PageCount: 30, OriginalPageCount: 30, GID: "9005", Token: "t5", Source: models.SourceOffline, SourceMode: "gallery", LocalPath: "Z:\\e2e\\テストタイトル solo", AddedAt: now, UpdatedAt: now},
	}

	// 幂等：清掉旧 e2e- 数据
	if err := db.Where("id LIKE 'e2e-%'").Delete(&models.OfflineComic{}).Error; err != nil {
		panic("清理失败: " + err.Error())
	}
	created := 0
	for i := range seeds {
		if err := db.Create(&seeds[i]).Error; err != nil {
			fmt.Println("create err:", err)
			continue
		}
		created++
	}
	fmt.Printf("seeded: %d 条（库: %s）\n", created, os.Args[1])
}
