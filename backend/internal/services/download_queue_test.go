package services

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"SakuManga/internal/models"
)

// ─────────────────────────────────────────────────────────────
// Round45：无界待调度队列单测
//
// 覆盖：无上限入队不丢 / 幂等去重 / 排序（优先级→创建时间→ID）/ 移除 / 并发安全
// ─────────────────────────────────────────────────────────────

func newQueueTask(id string, priority int, created time.Time) *models.DownloadTask {
	return &models.DownloadTask{
		ID:        id,
		GID:       "g-" + id,
		Status:    models.DownloadQueued,
		Priority:  priority,
		CreatedAt: created,
	}
}

// TestPendingQueueUnboundedPush 队列无上限：2000 条全部保留（旧实现 256 条后即丢弃）。
func TestPendingQueueUnboundedPush(t *testing.T) {
	q := newPendingQueue()
	base := time.Now()
	for i := 0; i < 2000; i++ {
		id := fmt.Sprintf("task-%04d", i)
		added := q.Push(newQueueTask(id, 0, base.Add(time.Duration(i)*time.Millisecond)))
		if !added {
			t.Fatalf("第 %d 条任务应为新增", i+1)
		}
	}
	if got := q.Len(); got != 2000 {
		t.Fatalf("队列长度期望 2000，得到 %d", got)
	}
}

// TestPendingQueuePushIdempotent 重复 Push 同一任务只保留一份，并更新排序键。
func TestPendingQueuePushIdempotent(t *testing.T) {
	q := newPendingQueue()
	created := time.Now()
	q.Push(newQueueTask("dup", 0, created))
	if added := q.Push(newQueueTask("dup", 0, created)); added {
		t.Fatalf("重复入队应返回 false")
	}
	if got := q.Len(); got != 1 {
		t.Fatalf("重复入队后长度期望 1，得到 %d", got)
	}

	// 更新优先级后应参与排序
	q.Push(newQueueTask("dup", 99, created))
	q.Push(newQueueTask("other", 0, created))
	head, ok := q.Peek()
	if !ok || head.ID != "dup" {
		t.Fatalf("提升优先级后队首应为 dup，得到 %v", head)
	}
}

// TestPendingQueueOrder 排序：优先级降序 → 创建时间升序 → ID 兜底。
func TestPendingQueueOrder(t *testing.T) {
	q := newPendingQueue()
	base := time.Now()
	q.Push(newQueueTask("low", 0, base))
	q.Push(newQueueTask("high", 10, base.Add(2*time.Second)))
	q.Push(newQueueTask("mid-early", 5, base))
	q.Push(newQueueTask("mid-late", 5, base.Add(time.Second)))
	q.Push(newQueueTask("same-a", 5, base))
	q.Push(newQueueTask("same-b", 5, base))

	want := []string{"high", "mid-early", "same-a", "same-b", "mid-late", "low"}
	for i, id := range want {
		got, ok := q.Pop()
		if !ok {
			t.Fatalf("第 %d 次出队失败", i+1)
		}
		if got.ID != id {
			t.Fatalf("第 %d 次出队期望 %s，得到 %s", i+1, id, got.ID)
		}
	}
	if _, ok := q.Pop(); ok {
		t.Fatalf("队列应已空")
	}
}

// TestPendingQueueRemoveAndContains 移除后不可见，Contains 反映实时状态。
func TestPendingQueueRemoveAndContains(t *testing.T) {
	q := newPendingQueue()
	base := time.Now()
	q.Push(newQueueTask("a", 0, base))
	q.Push(newQueueTask("b", 0, base.Add(time.Millisecond)))

	if !q.Contains("a") {
		t.Fatalf("a 应在队列中")
	}
	if !q.Remove("a") {
		t.Fatalf("移除 a 应返回 true")
	}
	if q.Remove("a") {
		t.Fatalf("重复移除 a 应返回 false")
	}
	if q.Contains("a") {
		t.Fatalf("a 不应仍在队列中")
	}
	head, ok := q.Peek()
	if !ok || head.ID != "b" {
		t.Fatalf("移除后队首应为 b，得到 %v", head)
	}
}

// TestPendingQueueConcurrent 并发 Push/Remove/Pop 不 panic、无数据竞争（配合 -race）。
func TestPendingQueueConcurrent(t *testing.T) {
	q := newPendingQueue()
	base := time.Now()
	var wg sync.WaitGroup
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				id := fmt.Sprintf("w%d-%d", worker, i)
				q.Push(newQueueTask(id, i%5, base.Add(time.Duration(i)*time.Millisecond)))
				_ = q.Contains(id)
				if i%3 == 0 {
					q.Remove(id)
				}
				if i%7 == 0 {
					q.Pop()
					q.Peek()
				}
				_ = q.Len()
			}
		}(w)
	}
	wg.Wait()
	// 只要求不 panic / 不崩溃；残留数量取决于并发交错，不做精确断言
	if q.Len() < 0 {
		t.Fatalf("长度异常")
	}
}
