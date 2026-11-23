package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

// ─────────────────────────────────────────────────────────────
// D14：两阶段原子删除的文件阶段
//
// 目标：Windows 下文件被占用（资源管理器预览/杀软扫描）时，必须在「动数据库之前」
// 拦截失败，且回滚成本为一次同盘 Rename（秒级）。
//
// 流程：stageToTrash（Rename 到同盘 .saku_trash）→ DB 事务删记录 → Commit 成功后
// cleanupTrash；Commit 失败则 restoreFromTrash 回滚，回滚再失败写 pending 清单，
// 由启动期 RecoverPendingTrash 处理。
// ─────────────────────────────────────────────────────────────

const (
	// trashDirName 回收目录名（放在目标文件所在目录下，保证同盘 Rename 原子）
	trashDirName = ".saku_trash"
)

// pendingTrashFile 回滚失败清单（相对运行目录；var 便于测试注入临时路径）
var pendingTrashFile = "data/trash_pending.json"

// pendingTrashEntry 一条待恢复的回收项
type pendingTrashEntry struct {
	TrashPath string `json:"trashPath"`
	LocalPath string `json:"localPath"`
}

// stageToTrash 第一阶段：把目标 Rename 到同盘回收目录 <父目录>/.saku_trash/<comicID>-<ts>。
// 目标不存在（死记录/已被手动删除）→ 返回 ("", nil)，视为第一阶段成功。
func stageToTrash(localPath, comicID string) (string, error) {
	if localPath == "" {
		return "", nil
	}
	if _, err := os.Stat(localPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil // 磁盘文件已不在：直接允许删库
		}
		return "", err
	}
	parent := filepath.Dir(localPath)
	trashRoot := filepath.Join(parent, trashDirName)
	if err := os.MkdirAll(trashRoot, 0o755); err != nil {
		return "", fmt.Errorf("创建回收目录失败: %v", err)
	}
	trashPath := filepath.Join(trashRoot, fmt.Sprintf("%s-%d", comicID, time.Now().UnixMilli()))
	if err := os.Rename(localPath, trashPath); err != nil {
		return "", fmt.Errorf("移动文件到回收目录失败（可能被占用）: %v", err)
	}
	return trashPath, nil
}

// restoreFromTrash 回滚：把回收目录 Rename 回原路径。
func restoreFromTrash(trashPath, localPath string) error {
	if trashPath == "" || localPath == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		return err
	}
	return os.Rename(trashPath, localPath)
}

// cleanupTrash 收尾：删除回收目录（失败仅留垃圾文件，不影响数据一致性）。
func cleanupTrash(trashPath string) {
	if trashPath == "" {
		return
	}
	if err := os.RemoveAll(trashPath); err != nil {
		log.Printf("%s [trash] 清理回收目录失败 %q: %v", dlWarnTag, trashPath, err)
	}
}

func readPendingTrash() []pendingTrashEntry {
	b, err := os.ReadFile(pendingTrashFile)
	if err != nil {
		return nil
	}
	var entries []pendingTrashEntry
	if err := json.Unmarshal(b, &entries); err != nil {
		log.Printf("%s [trash] 解析 pending 清单失败: %v", dlWarnTag, err)
		return nil
	}
	return entries
}

// recordPendingTrash 回滚失败时落盘，交由启动期 RecoverPendingTrash 恢复。
func recordPendingTrash(entries []pendingTrashEntry) {
	if len(entries) == 0 {
		return
	}
	all := append(readPendingTrash(), entries...)
	b, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		log.Printf("%s [trash] 序列化 pending 清单失败: %v", dlWarnTag, err)
		return
	}
	if err := os.MkdirAll(filepath.Dir(pendingTrashFile), 0o755); err != nil {
		log.Printf("%s [trash] 创建 pending 目录失败: %v", dlWarnTag, err)
		return
	}
	if err := os.WriteFile(pendingTrashFile, b, 0o644); err != nil {
		log.Printf("%s [trash] 写入 pending 清单失败: %v", dlWarnTag, err)
	}
}

// RecoverPendingTrash 启动期恢复中断的删除：把仍在回收目录的文件 Rename 回原路径。
// 返回成功恢复条数。原路径已存在时保留回收文件并告警（不覆盖）。
func RecoverPendingTrash() int {
	entries := readPendingTrash()
	if len(entries) == 0 {
		return 0
	}
	var remaining []pendingTrashEntry
	recovered := 0
	for _, e := range entries {
		if _, err := os.Stat(e.TrashPath); err != nil {
			// 回收文件已不存在（已清理/已恢复）→ 丢弃该条
			continue
		}
		if _, err := os.Stat(e.LocalPath); err == nil {
			// 原路径已被占用：保留回收文件，继续告警，不覆盖
			log.Printf("%s [trash] 恢复跳过：原路径已存在 %q（回收文件保留 %q）", dlWarnTag, e.LocalPath, e.TrashPath)
			remaining = append(remaining, e)
			continue
		}
		if err := restoreFromTrash(e.TrashPath, e.LocalPath); err != nil {
			log.Printf("%s [trash] 恢复失败 %q → %q: %v", dlWarnTag, e.TrashPath, e.LocalPath, err)
			remaining = append(remaining, e)
			continue
		}
		recovered++
	}
	if len(remaining) == 0 {
		_ = os.Remove(pendingTrashFile)
		return recovered
	}
	b, _ := json.MarshalIndent(remaining, "", "  ")
	_ = os.WriteFile(pendingTrashFile, b, 0o644)
	return recovered
}
