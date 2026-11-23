package services

import "fmt"

// ─────────────────────────────────────────────────────────────
// D21：下载入库契约
//
// 下载引擎的落地目录必须满足「本子形态」：
//   - 画廊下载：destDir 内图片直接平铺（本子根模式）；
//   - 归档下载：extractDir 内图片直接平铺（extractArchive 已剥离单一顶层目录）。
//
// 入库扫描失败或未入库任何本子 → 返回 error，由引擎把任务置为 error（可重试），
// 不得标记 completed（消灭「下载完成但离线库没有」的困惑形态）。
// ─────────────────────────────────────────────────────────────

// ingestDownloadDir 下载落地后的入库步骤（D21）。
// 扫描口径固定为 includeSubfolders=false（单本根模式，见 §2.6）。
func ingestDownloadDir(dir string) (int, error) {
	if dir == "" {
		return 0, fmt.Errorf("入库失败：落地目录为空")
	}
	found, err := ScanAndSaveDirectory(dir, false)
	if err != nil {
		return 0, fmt.Errorf("入库扫描失败: %v", err)
	}
	if found == 0 {
		return 0, fmt.Errorf("入库扫描未发现任何本子（目录 %q）", dir)
	}
	return found, nil
}
