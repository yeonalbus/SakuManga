package services

import (
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"SakuManga/internal/database"
	"SakuManga/internal/models"
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
		// Round54：根路径不存在时不再直接报错——标记「路径缺失」并继续执行对账，
		// 把该路径下全部记录判失效（误删整条目录的恢复/阵亡触发场景）。
		scanned := ScanStats{}
		var scanErr error
		if _, statErr := os.Stat(path); statErr != nil && os.IsNotExist(statErr) {
			progress.setPathMissing(true)
		} else {
			scanned, scanErr = scanDirectory(path, includeSubfolders, mode, progress, pathID)
		}

		// Round54：全量扫描成功（或路径缺失）后执行失效对账；增量维持原有语义不做对账。
		var recon *ScanReconcileResult
		if mode == "full" && scanErr == nil {
			progress.setPhase(ScanPhaseReconciling)
			recon, scanErr = ReconcileScanPathAgainstDisk(database.DB, pathID, path, progress)
			if scanErr != nil {
				// 对账失败不阻断扫描本身（扫描已完成入库）：仅记录日志，进度仍按成功收尾
				log.Printf("%s [scan] 路径 %q 失效对账失败（不影响扫描结果）: %v", dlErrTag, path, scanErr)
				scanErr = nil
			} else if recon != nil {
				progress.setReconcileResult(*recon)
			}
		}

		progress.finish(scanErr)
		if scanErr == nil {
			// 扫描成功 → 回写路径记录的统计（供列表直接展示）
			now := time.Now().UnixMilli()
			database.DB.Model(&models.ExtraScanPath{}).Where("id = ?", pathID).Updates(map[string]interface{}{
				"last_scanned": now,
				"comic_count":  scanned.Found,
			})
			reconNote := ""
			if recon != nil {
				reconNote = fmt.Sprintf("；对账：失效 %d / 候选 %d / 阵亡 %d / 自愈 %d",
					recon.Missing, recon.Candidates, recon.Fallen, recon.Healed)
			}
			log.Printf("%s [scan] 路径 %q 扫描完成：入库 %d 本（跳过：已存在 %d / 同gid %d / 空目录 %d / 不支持归档 %d；失败 %d，mode=%s%s）",
				dlLogTag, path, scanned.Found, scanned.SkippedExisting, scanned.SkippedGid,
				scanned.SkippedEmpty, scanned.SkippedUnsupported, scanned.Failed, mode, reconNote)
			// Round32：扫描会成批新增/更新/删除本地漫画 → 异步全量重建 XP 统计表
			XpRebuildAsync("扫描")
		} else {
			log.Printf("%s [scan] 路径 %q 扫描失败: %v", dlErrTag, path, scanErr)
		}
	}()

	return progress, nil
}

// IsScanning 指定路径当前是否正在扫描（未完成）。
// 删除扫描路径前用它拒绝并发删除：否则扫描 goroutine 会继续把漫画
// 以即将被删除的路径 ID 落库，留下新的孤儿记录。
func (m *ScanManager) IsScanning(pathID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.running[pathID]
	return ok && !p.IsDone()
}

// GetProgress 获取指定路径的扫描进度快照；若该路径从未启动过扫描返回 nil
func (m *ScanManager) GetProgress(pathID string) *ScanProgress {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.running[pathID]
	if !ok {
		return nil
	}
	return p.Snapshot()
}

// GetAllProgress 返回所有路径的扫描进度快照（含已完成任务，供前端一次轮询恢复状态）。
// 返回指针切片：ScanProgress 含 sync.Mutex，值切片 append 会拷贝锁（go vet 告警）。
func (m *ScanManager) GetAllProgress() []*ScanProgress {
	m.mu.Lock()
	defer m.mu.Unlock()
	list := make([]*ScanProgress, 0, len(m.running))
	for _, p := range m.running {
		list = append(list, p.Snapshot())
	}
	return list
}
