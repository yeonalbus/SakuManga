package services

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"SakuManga/internal/models"

	"gorm.io/gorm"
)

// ─────────────────────────────────────────────────────────────
// Round42 决策 D6：维护查重结果持久化（结果快照）
//
// 现状问题：结果只存进程内存（offlineMaintainRes），重启即丢；用户进维护页若缓存为空，
// 只能整份重扫（数千本、数十分钟联网核对）。
//
// 方案：单行快照（固定 ID=1 覆盖写）落库；内存缓存缺失时自动从库回填。
//   - StoreMaintainDedupResult（内存）之后调用 PersistMaintainSnapshot 落库；
//   - handler 读结果前调用 EnsureMaintainResultLoaded，缓存为空则从库加载；
//   - SyncMaintainDedupClusters（定向重算簇）之后同样落库，保持内存/DB 一致。
// ─────────────────────────────────────────────────────────────

// maintainSnapshotID 单行快照固定主键
const maintainSnapshotID uint = 1

// SaveMaintainSnapshot 把结果写入快照表（覆盖写；res 为 nil 时删除快照）
func SaveMaintainSnapshot(db *gorm.DB, res *DedupResult, forceFull bool) error {
	if db == nil {
		return nil
	}
	if res == nil {
		return db.Where("id = ?", maintainSnapshotID).Delete(&models.MaintainDedupSnapshot{}).Error
	}
	payload, err := json.Marshal(res)
	if err != nil {
		return err
	}
	snap := models.MaintainDedupSnapshot{
		ID:          maintainSnapshotID,
		Payload:     string(payload),
		ForceFull:   forceFull,
		FinishedAt:  res.FinishedAt,
		GeneratedAt: time.Now().UnixMilli(),
	}
	// 覆盖写：先删后插，避免依赖各数据库的 upsert 语法差异
	if err := db.Where("id = ?", maintainSnapshotID).Delete(&models.MaintainDedupSnapshot{}).Error; err != nil {
		return err
	}
	return db.Create(&snap).Error
}

// LoadMaintainSnapshot 读取快照（无记录时 ok=false）
func LoadMaintainSnapshot(db *gorm.DB) (*DedupResult, bool, error) {
	if db == nil {
		return nil, false, nil
	}
	var snap models.MaintainDedupSnapshot
	if err := db.Where("id = ?", maintainSnapshotID).First(&snap).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, false, nil
		}
		return nil, false, err
	}
	if snap.Payload == "" {
		return nil, false, nil
	}
	var res DedupResult
	if err := json.Unmarshal([]byte(snap.Payload), &res); err != nil {
		return nil, false, err
	}
	res.forceFull = snap.ForceFull
	return &res, true, nil
}

// PersistMaintainSnapshot 把当前内存缓存落库（供 StoreMaintainDedupResult 的调用方在之后调用）。
// 内存为空时删除快照，避免 DB 与内存长期不一致。
func PersistMaintainSnapshot(db *gorm.DB) {
	if db == nil {
		return
	}
	res := GetMaintainDedupResult()
	if res == nil {
		if err := SaveMaintainSnapshot(db, nil, false); err != nil {
			log.Printf("%s [maintain] 清理结果快照失败: %v", dlWarnTag, err)
		}
		return
	}
	if err := SaveMaintainSnapshot(db, res, res.forceFull); err != nil {
		log.Printf("%s [maintain] 保存结果快照失败: %v", dlWarnTag, err)
	}
}

// EnsureMaintainResultLoaded 确保结果可用：内存缓存为空时从 DB 快照回填（重启后秒开）。
// 返回值仅用于日志/调试，不改变调用方语义（仍以 GetMaintainDedupResult 读取）。
func EnsureMaintainResultLoaded(db *gorm.DB) bool {
	if GetMaintainDedupResult() != nil {
		return false // 内存已有结果（本次进程扫描过），无需回填
	}
	res, ok, err := LoadMaintainSnapshot(db)
	if err != nil {
		log.Printf("%s [maintain] 读取结果快照失败: %v", dlWarnTag, err)
		return false
	}
	if !ok || res == nil {
		return false
	}
	// 回填内存缓存（不再前移 FinishedAt：快照时间即结果生成时间，前端据此显示"数据时间"）
	offlineTaskMu.Lock()
	offlineMaintainRes = res
	offlineTaskMu.Unlock()
	log.Printf("%s [maintain] 已从结果快照回填缓存（生成于 %d）", dlLogTag, res.FinishedAt)
	return true
}

// PublishLocalDedupSnapshot 本地判重先行（Round42 D5/D6）。
//
// 名称级疑似重复是**纯本地、毫秒级**计算，而整次维护扫查的主要耗时在逐本联网核对
// （每本 ~1.2s 限流，增量几十本、全量上千本）。本函数把判重结果提前产出并入缓存 + 落库，
// 使前端进维护页即可秒级看到「疑似重复」，不必等待联网阶段。
//
// 候选口径：直接用传入的 comics（尚未扣除规则 1/2/4 的建议删除项）——
// 本地阶段判定略宽松，联网阶段结束后会用完整结果（含规则 1/2/3/4）覆盖本快照。
//
// 返回本次产出的簇数（仅用于日志/测试）。
func PublishLocalDedupSnapshot(comics []models.OfflineComic, ignoreIdx *IgnoreIndex, forceFull bool, onProgress OfflineProgressFn) int {
	if onProgress != nil {
		onProgress(0, 1, "", "本地判重（名称级疑似重复）")
	}
	clusters := detectTitleClusters(comics, ignoreIdx, forceFull)
	res := &DedupResult{Items: []DedupItem{}, Clusters: clusters}
	StoreMaintainDedupResult(res, forceFull) // 内部同步落库（D6）
	if onProgress != nil {
		onProgress(1, 1, "", fmt.Sprintf("本地判重完成（疑似重复 %d 组），转入联网核对", len(clusters)))
	}
	log.Printf("%s [maintain] 本地判重先行：疑似重复 %d 组已发布（联网核对继续，完成后覆盖为完整结果）",
		dlLogTag, len(clusters))
	return len(clusters)
}
