package services

import (
	"fmt"
	"log"
	"sync"
	"time"

	"SakuHentai/internal/database"
	"SakuHentai/internal/models"
)

// ─────────────────────────────────────────────────────────────
// 扫描任务管理器：内存态保存每个路径的扫描进度，支持异步轮询。
//
// 背景：原先 /scan-paths/:id/scan 是同步阻塞扫描，前端看不到进度；
// 且切页后前端请求上下文丢失，看起来像是“被截断”，但后端仍在跑。
// 现在改为：POST 启动后立即返回进度对象，扫描在 goroutine 中执行，
// 前端可通过 GET /scan-paths/:id/scan/progress 轮询实时进度。
// ─────────────────────────────────────────────────────────────

// ScanManager 扫描任务管理器
type ScanManager struct {
	mu      sync.Mutex
	running map[string]*ScanProgress // key: ExtraScanPath.ID
}

// globalScanManager 全局单例扫描管理器
var globalScanManager = &ScanManager{
	running: map[string]*ScanProgress{},
}

// GetScanManager 返回全局扫描管理器
func GetScanManager() *ScanManager {
	return globalScanManager
}

// StartScan 异步启动一个路径的扫描，立即返回进度对象。
// 若该路径正在扫描中则返回错误。
func (m *ScanManager) StartScan(pathID, path string, includeSubfolders bool, mode string) (*ScanProgress, error) {
	if mode != "incremental" {
		mode = "full"
	}

	m.mu.Lock()
	if p, ok := m.running[pathID]; ok && !p.IsDone() {
		m.mu.Unlock()
		return nil, fmt.Errorf("该路径正在扫描中，请等待完成")
	}
	progress := newScanProgress(pathID, mode)
	m.running[pathID] = progress
	m.mu.Unlock()

	go func() {
		count, err := scanDirectory(path, includeSubfolders, mode, progress)
		progress.finish(err)
		if err == nil {
			// 扫描成功 → 回写路径记录的统计（供列表直接展示）
			now := time.Now().UnixMilli()
			database.DB.Model(&models.ExtraScanPath{}).Where("id = ?", pathID).Updates(map[string]interface{}{
				"last_scanned": now,
				"comic_count":  count,
			})
			log.Printf("%s [scan] 路径 %q 扫描完成：发现 %d 本漫画（mode=%s）", dlLogTag, path, count, mode)
		} else {
			log.Printf("%s [scan] 路径 %q 扫描失败: %v", dlErrTag, path, err)
		}
	}()

	return progress, nil
}

// GetProgress 获取指定路径的扫描进度快照；若该路径从未启动过扫描返回 nil
func (m *ScanManager) GetProgress(pathID string) *ScanProgress {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.running[pathID]
	if !ok {
		return nil
	}
	sp := p.Snapshot()
	return &sp
}

// GetAllProgress 返回所有路径的扫描进度快照（含已完成任务，供前端一次轮询恢复状态）
func (m *ScanManager) GetAllProgress() []ScanProgress {
	m.mu.Lock()
	defer m.mu.Unlock()
	list := make([]ScanProgress, 0, len(m.running))
	for _, p := range m.running {
		sp := p.Snapshot()
		list = append(list, sp)
	}
	return list
}
