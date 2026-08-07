package services

import (
	"log"
	"sync"
	"time"

	"SakuHentai/internal/models"

	"gorm.io/gorm"
)

// ─────────────────────────────────────────────────────────────
// 需求4：书库变更感知的自动增量维护查重
//
// 设计要点（用户确认方案）：
//   - 不拆分接口：复用既有维护查重（MaintainDedupWithProgress）。
//   - 增量查重仅在「下载队列全部为空」后触发：
//       ① 用户自主进入维护界面（前端触发，见 OfflineMaintain.vue）
//       ② 队列为空的时间 > 1min（本文件，后台定时器触发）
//   - 变更时间戳去重：lastLibraryChange 记录最近一次书库变更
//     （下载完成 / 更新完成-保留或删除旧版 / 删除记录），
//     仅当 lastLibraryChange 晚于查重结果生成时间（FinishedAt）时才需要重新扫描。
//   - 单槽位互斥：复用 StartOfflineTask，与维护/更新任务互斥。
//   - 增量扫描：forceFull=false，只核对未检查（parent_checked_at=0）的漫画；
//     全量在线核对保留为手动按钮。
// ─────────────────────────────────────────────────────────────

var (
	libraryChangeMu   sync.Mutex
	lastLibraryChange int64 // 最近一次书库变更时间戳(ms)，0 表示进程内尚未记录任何变更
)

// MarkLibraryChanged 记录书库变更时间戳（下载完成 / 更新完成-保留或删除旧版 / 删除记录时调用）。
// 维护查重据此判断是否存在「尚未反映到查重结果」的变更。
func MarkLibraryChanged() {
	libraryChangeMu.Lock()
	lastLibraryChange = time.Now().UnixMilli()
	libraryChangeMu.Unlock()
}

// GetLastLibraryChange 读取最近一次书库变更时间戳(ms)
func GetLastLibraryChange() int64 {
	libraryChangeMu.Lock()
	defer libraryChangeMu.Unlock()
	return lastLibraryChange
}

// HasUnsyncedLibraryChanges 判断是否存在尚未反映到维护查重结果的变更：
// 变更时间戳晚于查重结果生成时间（FinishedAt），或结果缓存为空。
func HasUnsyncedLibraryChanges() bool {
	chg := GetLastLibraryChange()
	if chg == 0 {
		return false // 进程内尚无任何变更记录，无需自动扫描
	}
	res := GetMaintainDedupResult()
	if res == nil {
		return true // 尚无查重结果，变更必然未反映
	}
	return chg > res.FinishedAt
}

// MarkMaintainDedupReplacement 定向写入「旧版被新版取代，建议删除」到维护查重结果缓存（需求4）。
//
// 更新下载完成但选择「保留旧版」（AutoUpdateDeleteOriginal=false 或旧版无本地文件）时调用：
// 无需等待重新全量扫描，即可在维护界面瞬时看到该旧版「建议删除」，并清除 stale
// （否则删除/更新操作后结果被标记 stale，前端会一直提示重新扫描）。
//
// 注意：本函数只定向追加建议项，不更新 FinishedAt —— 由调用方另行 MarkLibraryChanged，
// 保证「存在未反映变更」仍为真，下载队列空闲后后台仍会执行一次真实增量查重。
func MarkMaintainDedupReplacement(db *gorm.DB, old *models.OfflineComic, newGID string) {
	if old == nil {
		return
	}

	// 新版漫画（已下载入库，gid 匹配）作为成对对象，对比视图双列展示 新版↔旧版
	var pair *models.OfflineComic
	if newGID != "" {
		var nc models.OfflineComic
		if err := db.Where("g_id = ?", newGID).Order("updated_at DESC").First(&nc).Error; err == nil {
			pair = &nc
		}
	}

	offlineTaskMu.Lock()
	defer offlineTaskMu.Unlock()

	if offlineMaintainRes == nil {
		offlineMaintainRes = &DedupResult{}
	}
	res := offlineMaintainRes

	// 删除标记优先：先移除旧版可能存在的旧条目（保留/删除标记均清除），再以「建议删除」写入
	kept := res.Items[:0]
	for _, it := range res.Items {
		if it.Comic.ID == old.ID {
			continue
		}
		kept = append(kept, it)
	}
	res.Items = append(kept, DedupItem{
		Comic:      *old,
		Reason:     "旧版被新版取代，建议删除",
		Keep:       false,
		PairComic:  pair,
	})
	res.Stale = false // 定向写入后结果即时可信，前端无需提示重新扫描

	log.Printf("%s [auto-maintain] 定向写入「旧版被新版取代，建议删除」comic=%s（newGID=%s）",
		dlLogTag, old.ID, newGID)
}

// AutoRunMaintainDedup 后台自动触发一次增量维护查重（需求4，队列空闲>1min 的触发点）。
//
// 返回 true 表示已启动扫描（实际扫描在 goroutine 中异步执行）；false 表示跳过：
//   - 书库无未反映变更（变更时间戳去重，避免空转）；
//   - 已有离线任务（维护/更新）在运行（单槽位互斥）；
//   - ehService 为空时退化为纯本地查重（由 MaintainDedupWithProgress 内部处理）。
func AutoRunMaintainDedup(db *gorm.DB, ehService *EHService) bool {
	if !HasUnsyncedLibraryChanges() {
		log.Printf("%s [auto-maintain] 书库无未反映变更（lastLibraryChange=%d），跳过自动增量查重",
			dlLogTag, GetLastLibraryChange())
		return false
	}
	if !StartOfflineTask(OfflineTaskMaintain) {
		log.Printf("%s [auto-maintain] 已有离线维护任务在运行，跳过自动增量查重", dlWarnTag)
		return false
	}

	log.Printf("%s [auto-maintain] 下载队列空闲>1min 且书库存在未反映变更，自动启动增量维护查重", dlLogTag)
	go func() {
		result, err := MaintainDedupWithProgress(db, ehService, OfflineMaintainProgressSink, false)
		if err != nil {
			FinishOfflineTask(err)
			return
		}
		StoreMaintainDedupResult(result)
		FinishOfflineTask(nil)
	}()
	return true
}

// MaintainUnsyncedStatus 书库变更与查重结果的同步状态（需求4，前端进入维护界面时判断是否自动增量查重）
type MaintainUnsyncedStatus struct {
	LastLibraryChange int64 `json:"lastLibraryChange"` // 书库最近一次变更时间戳(ms)，0 表示进程内尚无变更记录
	ResultFinishedAt  int64 `json:"resultFinishedAt"`  // 最近一次查重结果生成时间戳(ms)，0 表示尚无结果
	HasUnsynced       bool  `json:"hasUnsynced"`       // 是否存在尚未反映到查重结果的变更（true 时建议自动增量查重）
}

// GetMaintainUnsyncedStatus 读取书库变更同步状态（前端入口自动触发的依据）。
func GetMaintainUnsyncedStatus() MaintainUnsyncedStatus {
	res := GetMaintainDedupResult()
	var finished int64
	if res != nil {
		finished = res.FinishedAt
	}
	return MaintainUnsyncedStatus{
		LastLibraryChange: GetLastLibraryChange(),
		ResultFinishedAt:  finished,
		HasUnsynced:       HasUnsyncedLibraryChanges(),
	}
}
