package services

import (
	"sync"

	"SakuManga/internal/models"
)

// ─────────────────────────────────────────────────────────────
// 无界待调度队列（Round45）
//
// 设计目标：取代旧的 `queue chan string`（容量 256，满即丢弃）。
//   - 无容量上限：批量创建 1000+ 任务也不会丢；
//   - Push 幂等：同一任务重复入队只保留一份（更新排序键），避免被执行两次；
//   - 出队顺序：优先级降序 → 创建时间升序 → ID 升序（稳定兜底）；
//   - 线程安全：所有方法内部持锁，调度器可并发调用。
//
// 状态以数据库为准：队列只保存任务快照用于排序与去重，
// 调度器出队前会回查 DB 状态（非 queued 直接丢弃）。
// ─────────────────────────────────────────────────────────────

// pendingQueue 无界待调度任务队列
type pendingQueue struct {
	mu    sync.Mutex
	items map[string]*models.DownloadTask
}

// newPendingQueue 构造空队列
func newPendingQueue() *pendingQueue {
	return &pendingQueue{items: make(map[string]*models.DownloadTask)}
}

// lessPendingTask 出队排序：优先级高者在前；同优先级按创建时间早者在前；再按 ID 稳定排序。
func lessPendingTask(a, b *models.DownloadTask) bool {
	if a.Priority != b.Priority {
		return a.Priority > b.Priority
	}
	if !a.CreatedAt.Equal(b.CreatedAt) {
		return a.CreatedAt.Before(b.CreatedAt)
	}
	return a.ID < b.ID
}

// Push 入队（幂等）：已存在时更新快照（优先级/创建时间可能被修改）。
// 返回 true 表示新增，false 表示已存在并更新。
func (q *pendingQueue) Push(task *models.DownloadTask) bool {
	if task == nil || task.ID == "" {
		return false
	}
	snapshot := *task // 拷贝：避免调用方后续修改污染队列排序键
	q.mu.Lock()
	defer q.mu.Unlock()
	_, existed := q.items[task.ID]
	q.items[task.ID] = &snapshot
	return !existed
}

// Remove 移除任务（暂停/取消/抢占时调用）。返回是否确实移除。
func (q *pendingQueue) Remove(id string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if _, ok := q.items[id]; !ok {
		return false
	}
	delete(q.items, id)
	return true
}

// Contains 判断任务是否在队列中
func (q *pendingQueue) Contains(id string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	_, ok := q.items[id]
	return ok
}

// Len 返回队列长度
func (q *pendingQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items)
}

// Peek 返回队首任务（不删除）
func (q *pendingQueue) Peek() (*models.DownloadTask, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	best := q.peekLocked()
	return best, best != nil
}

// Pop 取出队首任务（删除并返回）
func (q *pendingQueue) Pop() (*models.DownloadTask, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	best := q.peekLocked()
	if best == nil {
		return nil, false
	}
	delete(q.items, best.ID)
	return best, true
}

// Snapshot 返回当前队列中的任务 ID 快照（诊断/日志用，顺序不保证）
func (q *pendingQueue) Snapshot() []string {
	q.mu.Lock()
	defer q.mu.Unlock()
	ids := make([]string, 0, len(q.items))
	for id := range q.items {
		ids = append(ids, id)
	}
	return ids
}

// peekLocked 选出队首（调用方须持有 q.mu）
func (q *pendingQueue) peekLocked() *models.DownloadTask {
	var best *models.DownloadTask
	for _, t := range q.items {
		if best == nil || lessPendingTask(t, best) {
			best = t
		}
	}
	return best
}
