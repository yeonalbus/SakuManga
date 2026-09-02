package services

import (
	"errors"
	"log"
	"sync"
	"time"
)

// ─────────────────────────────────────────────────────────────
// 离线维护任务进度管理（问题3：海量抓取无反馈 / 长任务同步阻塞）
//
// 本地书库维护查重（MaintainDedup）与离线更新检测（CheckUpdates）都是
// 逐画廊联网抓取的长任务（每本限流 ~1.2s，数千本需要数十分钟）。此前它们
// 在 HTTP handler 中同步执行：前端只能看到「正在扫描本地书库查重...」，
// 无法感知进度，请求也极易超时。
//
// 本文件提供全局任务状态（单槽位：维护/更新任务互斥，避免同时联网触发限流），
// handler 改为「异步启动 → 前端轮询进度 → 完成后再拉取结果」。
//
// Round29：任务支持中途暂停 / 继续 / 取消（协作式）——
//   - 状态机由 idle|running|success|error 扩展 paused|cancelled；
//   - 任务 goroutine 在业务循环的检查点（OfflineTaskCheckpoint，逐本漫画粒度）
//     挂起/退出，当前正在处理的一本完成后才真正暂停（≤2s，超大 hash 文件除外），
//     不强行中断在途网络请求，保证数据一致；
//   - 暂停/继续/取消通过 sync.Cond 通知任务 goroutine，等待期间不占用锁、不占 CPU。
// ─────────────────────────────────────────────────────────────

// OfflineTaskKind 离线维护任务类型
type OfflineTaskKind string

const (
	OfflineTaskMaintain OfflineTaskKind = "maintain" // 本地书库维护查重
	OfflineTaskUpdate   OfflineTaskKind = "update"   // 离线更新检测
)

// OfflineTaskStatus 离线维护任务状态
type OfflineTaskStatus string

const (
	OfflineTaskIdle      OfflineTaskStatus = "idle"
	OfflineTaskRunning   OfflineTaskStatus = "running"
	OfflineTaskPaused    OfflineTaskStatus = "paused"    // Round29：用户暂停（等待继续/取消）
	OfflineTaskSuccess   OfflineTaskStatus = "success"
	OfflineTaskError     OfflineTaskStatus = "error"
	OfflineTaskCancelled OfflineTaskStatus = "cancelled" // Round29：用户取消（任务已终止）
)

// ErrOfflineTaskCancelled 用户取消任务的哨兵错误：
// 任务循环检查点收到后向上返回，由 FinishOfflineTask 收尾为 cancelled 状态。
var ErrOfflineTaskCancelled = errors.New("任务已被用户取消")

// OfflineTaskState 离线维护任务进度快照（返回给前端轮询）
type OfflineTaskState struct {
	Type         OfflineTaskKind   `json:"type"`                  // 任务类型 maintain | update
	Status       OfflineTaskStatus `json:"status"`                // idle | running | paused | success | error | cancelled
	Phase        string            `json:"phase,omitempty"`       // 当前阶段说明（如「在线父子关系发现」「归档 Hash 计算」）
	Total        int               `json:"total"`                 // 当前阶段总数
	Done         int               `json:"done"`                  // 当前阶段已完成数
	CurrentTitle string            `json:"currentTitle,omitempty"` // 当前处理的漫画标题
	Message      string            `json:"message,omitempty"`     // 附加提示（如「共 2760 个，仅对含 gid 的联网」）
	StartedAt    int64             `json:"startedAt,omitempty"`   // 开始时间戳(ms)
	FinishedAt   int64             `json:"finishedAt,omitempty"`  // 结束时间戳(ms)
	Error        string            `json:"error,omitempty"`       // 失败原因（status=error 时）
}

// OfflineProgressFn 离线任务阶段进度回调
// done/total 为当前阶段的进度；title 为当前处理的漫画；phase 为阶段说明。
type OfflineProgressFn func(done, total int, title, phase string)

var (
	offlineTaskMu      sync.Mutex
	offlineTaskState   = OfflineTaskState{Status: OfflineTaskIdle}
	offlineTaskCond    = sync.NewCond(&offlineTaskMu) // Round29：暂停/恢复/取消唤醒
	offlineTaskCancel  bool                           // Round29：取消请求标志（任务级，结束后复位）
	offlineMaintainRes *DedupResult                   // 最近一次维护查重结果缓存
	offlineUpdateRes   *UpdateCheckResult             // 最近一次更新检测结果缓存
)

// StartOfflineTask 尝试启动一个离线维护任务（单槽位互斥）。
// 已有任务在运行/暂停挂起（paused 的任务 goroutine 仍存活）时返回 false，调用方应返回 409。
func StartOfflineTask(kind OfflineTaskKind) bool {
	offlineTaskMu.Lock()
	defer offlineTaskMu.Unlock()
	if offlineTaskState.Status == OfflineTaskRunning || offlineTaskState.Status == OfflineTaskPaused {
		return false
	}
	offlineTaskCancel = false
	offlineTaskState = OfflineTaskState{
		Type:      kind,
		Status:    OfflineTaskRunning,
		Phase:     "准备中",
		StartedAt: time.Now().UnixMilli(),
	}
	if kind == OfflineTaskMaintain {
		offlineMaintainRes = nil
	} else {
		offlineUpdateRes = nil
	}
	return true
}

// PauseOfflineTask 请求暂停当前任务（Round29）。
// 仅 running 可暂停；暂停为协作式：任务 goroutine 在当前漫画处理完后于下一检查点挂起。
// 返回是否接受请求（无任务/任务已结束返回 false）。
func PauseOfflineTask() bool {
	offlineTaskMu.Lock()
	defer offlineTaskMu.Unlock()
	if offlineTaskState.Status != OfflineTaskRunning {
		return false
	}
	offlineTaskState.Status = OfflineTaskPaused
	offlineTaskState.Message = "已请求暂停，当前项处理完后生效"
	return true
}

// ResumeOfflineTask 继续被暂停的任务（Round29）。仅 paused 可继续。
func ResumeOfflineTask() bool {
	offlineTaskMu.Lock()
	defer offlineTaskMu.Unlock()
	if offlineTaskState.Status != OfflineTaskPaused {
		return false
	}
	offlineTaskState.Status = OfflineTaskRunning
	offlineTaskState.Message = ""
	offlineTaskCond.Broadcast() // 唤醒挂起中的任务 goroutine
	return true
}

// CancelOfflineTask 取消任务（Round29）。running / paused 均可取消：
// running 状态下任务会在下一检查点（当前项处理完后）退出；
// paused 状态下立即唤醒挂起 goroutine 退出。
// 返回是否接受请求；任务真正收尾为 cancelled 由 FinishOfflineTask 完成。
func CancelOfflineTask() bool {
	offlineTaskMu.Lock()
	defer offlineTaskMu.Unlock()
	switch offlineTaskState.Status {
	case OfflineTaskRunning, OfflineTaskPaused:
		offlineTaskCancel = true
		offlineTaskState.Message = "已请求取消，正在停止…"
		if offlineTaskState.Status == OfflineTaskPaused {
			offlineTaskState.Status = OfflineTaskRunning
			offlineTaskCond.Broadcast() // 唤醒等待继续的 goroutine → 检测取消 → 退出
		}
		return true
	default:
		return false
	}
}

// OfflineTaskCheckpoint 任务循环检查点（Round29）：
//   1. 任务被暂停时挂起当前 goroutine（不占锁/CPU），等待继续或取消；
//   2. 已请求取消时返回 ErrOfflineTaskCancelled，调用方应终止任务并向
//      FinishOfflineTask 传递该错误（收尾为 cancelled 状态）。
// 应放在业务循环每本处理前调用（粒度：逐本漫画）。
func OfflineTaskCheckpoint() error {
	offlineTaskMu.Lock()
	defer offlineTaskMu.Unlock()
	for offlineTaskState.Status == OfflineTaskPaused && !offlineTaskCancel {
		offlineTaskCond.Wait()
	}
	if offlineTaskCancel {
		return ErrOfflineTaskCancelled
	}
	return nil
}

// UpdateOfflineTaskProgress 更新当前任务进度（由 WithProgress 变体内部调用）
func UpdateOfflineTaskProgress(done, total int, title, phase string) {
	offlineTaskMu.Lock()
	defer offlineTaskMu.Unlock()
	if offlineTaskState.Status != OfflineTaskRunning && offlineTaskState.Status != OfflineTaskPaused {
		return
	}
	if total > 0 {
		offlineTaskState.Total = total
	}
	if done >= 0 {
		offlineTaskState.Done = done
	}
	if title != "" {
		offlineTaskState.CurrentTitle = title
	}
	if phase != "" {
		offlineTaskState.Phase = phase
	}
}

// SetOfflineTaskMessage 更新附加提示文案（不覆盖阶段/进度）
func SetOfflineTaskMessage(msg string) {
	offlineTaskMu.Lock()
	defer offlineTaskMu.Unlock()
	offlineTaskState.Message = msg
}

// FinishOfflineTask 结束任务（成功传 nil；失败传 error；取消传 ErrOfflineTaskCancelled）。
// 调用方须在设置结果缓存后再调用本函数。
func FinishOfflineTask(err error) {
	offlineTaskMu.Lock()
	defer offlineTaskMu.Unlock()
	offlineTaskState.FinishedAt = time.Now().UnixMilli()
	offlineTaskState.CurrentTitle = ""
	if err != nil {
		if errors.Is(err, ErrOfflineTaskCancelled) {
			// Round29：用户取消 → cancelled 状态（不污染 error 字段，前端展示独立文案）
			offlineTaskState.Status = OfflineTaskCancelled
			offlineTaskState.Message = "任务已取消"
			offlineTaskState.Error = ""
		} else {
			offlineTaskState.Status = OfflineTaskError
			offlineTaskState.Error = err.Error()
		}
	} else {
		offlineTaskState.Status = OfflineTaskSuccess
		offlineTaskState.Message = ""
	}
	offlineTaskCancel = false // 取消标志随任务生命周期复位
}

// StoreMaintainDedupResult 缓存维护查重结果（记录生成时间并清除过期标记，
// 前端据此判断结果是否可信：删除操作后结果会被标记为过期）
func StoreMaintainDedupResult(res *DedupResult) {
	offlineTaskMu.Lock()
	defer offlineTaskMu.Unlock()
	if res != nil {
		res.FinishedAt = time.Now().UnixMilli()
		res.Stale = false
	}
	offlineMaintainRes = res
}

// GetMaintainDedupResult 读取最近一次维护查重结果缓存
func GetMaintainDedupResult() *DedupResult {
	offlineTaskMu.Lock()
	defer offlineTaskMu.Unlock()
	return offlineMaintainRes
}

// InvalidateMaintainDedupResult 使维护查重结果缓存失效（幽灵文件修复）：
// 删除漫画后调用，将已删除的 id 从结果 items 中移除并标记 stale=true，
// 同时清空更新检测结果缓存（被删漫画可能仍残留在更新列表中）。
// 前端下次读取结果时会发现 stale=true，提示用户重新扫描以获取一致的最新数据。
func InvalidateMaintainDedupResult(removedIDs []string) {
	offlineTaskMu.Lock()
	defer offlineTaskMu.Unlock()
	if offlineMaintainRes != nil {
		if len(removedIDs) > 0 {
			idSet := make(map[string]struct{}, len(removedIDs))
			for _, id := range removedIDs {
				idSet[id] = struct{}{}
			}
			kept := offlineMaintainRes.Items[:0]
			for _, item := range offlineMaintainRes.Items {
				if _, hit := idSet[item.Comic.ID]; !hit {
					kept = append(kept, item)
				}
			}
			offlineMaintainRes.Items = kept
		}
		offlineMaintainRes.Stale = true
	}
	offlineUpdateRes = nil
}

// StoreUpdateCheckResult 缓存更新检测结果
func StoreUpdateCheckResult(res *UpdateCheckResult) {
	offlineTaskMu.Lock()
	defer offlineTaskMu.Unlock()
	offlineUpdateRes = res
}

// GetUpdateCheckResult 读取最近一次更新检测结果缓存
func GetUpdateCheckResult() *UpdateCheckResult {
	offlineTaskMu.Lock()
	defer offlineTaskMu.Unlock()
	return offlineUpdateRes
}

// GetOfflineTaskProgress 读取当前离线任务进度快照（前端轮询）
func GetOfflineTaskProgress() OfflineTaskState {
	offlineTaskMu.Lock()
	defer offlineTaskMu.Unlock()
	return offlineTaskState
}

// OfflineMaintainProgressSink 维护查重的进度回调（写入全局状态）
func OfflineMaintainProgressSink(done, total int, title, phase string) {
	UpdateOfflineTaskProgress(done, total, title, phase)
	log.Printf("%s [maintain] 进度 %d/%d %s「%s」", dlLogTag, done, total, phase, title)
}

// OfflineUpdateProgressSink 更新检测的进度回调（写入全局状态）
func OfflineUpdateProgressSink(done, total int, title, phase string) {
	UpdateOfflineTaskProgress(done, total, title, phase)
	log.Printf("%s [update] 进度 %d/%d %s「%s」", dlLogTag, done, total, phase, title)
}
