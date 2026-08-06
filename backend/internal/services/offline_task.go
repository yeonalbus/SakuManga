package services

import (
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
	OfflineTaskIdle    OfflineTaskStatus = "idle"
	OfflineTaskRunning OfflineTaskStatus = "running"
	OfflineTaskSuccess OfflineTaskStatus = "success"
	OfflineTaskError   OfflineTaskStatus = "error"
)

// OfflineTaskState 离线维护任务进度快照（返回给前端轮询）
type OfflineTaskState struct {
	Type         OfflineTaskKind   `json:"type"`                  // 任务类型 maintain | update
	Status       OfflineTaskStatus `json:"status"`                // idle | running | success | error
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
	offlineMaintainRes *DedupResult       // 最近一次维护查重结果缓存
	offlineUpdateRes   *UpdateCheckResult // 最近一次更新检测结果缓存
)

// StartOfflineTask 尝试启动一个离线维护任务（单槽位互斥）。
// 已有任务在运行/未结束时返回 false，调用方应返回 409。
func StartOfflineTask(kind OfflineTaskKind) bool {
	offlineTaskMu.Lock()
	defer offlineTaskMu.Unlock()
	if offlineTaskState.Status == OfflineTaskRunning {
		return false
	}
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

// UpdateOfflineTaskProgress 更新当前任务进度（由 WithProgress 变体内部调用）
func UpdateOfflineTaskProgress(done, total int, title, phase string) {
	offlineTaskMu.Lock()
	defer offlineTaskMu.Unlock()
	if offlineTaskState.Status != OfflineTaskRunning {
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

// FinishOfflineTask 结束任务（成功传 nil；失败传 error）。
// 调用方须在设置结果缓存后再调用本函数。
func FinishOfflineTask(err error) {
	offlineTaskMu.Lock()
	defer offlineTaskMu.Unlock()
	offlineTaskState.FinishedAt = time.Now().UnixMilli()
	offlineTaskState.CurrentTitle = ""
	if err != nil {
		offlineTaskState.Status = OfflineTaskError
		offlineTaskState.Error = err.Error()
	} else {
		offlineTaskState.Status = OfflineTaskSuccess
	}
}

// StoreMaintainDedupResult 缓存维护查重结果
func StoreMaintainDedupResult(res *DedupResult) {
	offlineTaskMu.Lock()
	defer offlineTaskMu.Unlock()
	offlineMaintainRes = res
}

// GetMaintainDedupResult 读取最近一次维护查重结果缓存
func GetMaintainDedupResult() *DedupResult {
	offlineTaskMu.Lock()
	defer offlineTaskMu.Unlock()
	return offlineMaintainRes
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
