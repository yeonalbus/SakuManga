package services

import (
	"log"
	"sync"
)

// ─────────────────────────────────────────────────────────────
// 全局归档下载线程配额池（线程配额控制 + 排队 + 动态唤醒）
//
// 语义对齐 JHentai 的 archive_download_service.dart：
//   - 每个归档任务需要 archiveThreads 个线程（对应其多 Isolate 的 isolateCount）
//   - 全局活跃线程总数上限 defaultMaxArchiveThreads（=10，超过将导致 E 站 410）
//   - 线程不足时任务在 acquire 处阻塞排队（对应 JHentai 的 ArchiveStatus.waitingIsolate）
//   - 任务结束/暂停释放线程后 Broadcast 唤醒等待中的任务（对应 _tryWakeWaitingTasks）
//   - 支持运行中动态调整每任务线程数（对应 _onIsolateCountChange → changeIsolateCount）
//
// 线程数语义：
//   archiveThreads 是「单个归档文件的分片下载线程数」，不是同时运行的归档任务数。
//   多个归档任务可同时运行（由 download_scheduler 的优先级/串并行门控决定），
//   但所有任务的活跃线程总和不得超过 10，超过部分在 acquire 处排队等待。
// ─────────────────────────────────────────────────────────────

// defaultMaxArchiveThreads 全局归档下载线程上限（E-Hentai 允许的最大并发线程数）
const defaultMaxArchiveThreads = 10

// archiveThreadPool 全局归档下载线程配额池
type archiveThreadPool struct {
	mu      sync.Mutex
	cond    *sync.Cond
	max     int            // 全局线程上限（默认 10）
	active  int            // 当前已分配的活跃线程总数
	perTask map[string]int // 任务ID -> 该任务当前持有的线程数
}

// newArchiveThreadPool 构造线程配额池
func newArchiveThreadPool(max int) *archiveThreadPool {
	if max <= 0 {
		max = defaultMaxArchiveThreads
	}
	p := &archiveThreadPool{max: max, perTask: make(map[string]int)}
	p.cond = sync.NewCond(&p.mu)
	return p
}

// acquire 为任务申请 n 个线程（全有或全无语义），配额不足时阻塞排队。
// stop 回调用于在等待期间检测任务被暂停/取消（返回 true 立即放弃并返回 0）。
func (p *archiveThreadPool) acquire(taskID string, n int, stop func() bool) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	for {
		if stop != nil && stop() {
			return 0
		}
		if n > 0 && p.active+n <= p.max {
			p.active += n
			p.perTask[taskID] += n
			log.Printf("%s [archive-thread] 任务 %s 获取 %d 个线程（active=%d/%d）",
				dlLogTag, taskID, n, p.active, p.max)
			return n
		}
		log.Printf("%s [archive-thread] 任务 %s 线程不足（active=%d/%d，需要 %d），排队等待…",
			dlWarnTag, taskID, p.active, p.max, n)
		p.cond.Wait()
	}
}

// release 释放任务 n 个线程并唤醒等待者（动态调小时调用）
func (p *archiveThreadPool) release(taskID string, n int) {
	p.mu.Lock()
	p.releaseLocked(taskID, n)
	p.cond.Broadcast()
	p.mu.Unlock()
}

// releaseAll 释放任务占用的全部线程并唤醒等待者（任务结束/暂停时调用）
func (p *archiveThreadPool) releaseAll(taskID string) {
	p.mu.Lock()
	p.releaseLocked(taskID, p.perTask[taskID])
	p.cond.Broadcast()
	p.mu.Unlock()
}

// wakeAll 唤醒所有等待线程配额的任务（暂停/取消时调用）：
// 让阻塞在 acquire 中排队等待的任务立即检查停止标记，避免被暂停的任务空占 worker。
func (p *archiveThreadPool) wakeAll() {
	p.mu.Lock()
	p.cond.Broadcast()
	p.mu.Unlock()
}

// adjust 将任务线程数调整为 target（尽力而为，可能部分成功）。
// 返回任务调整后实际持有的线程数（调用方据此同步 worker 数）。
func (p *archiveThreadPool) adjust(taskID string, target int) int {
	p.mu.Lock()
	defer p.mu.Unlock()

	cur := p.perTask[taskID]
	if target > cur {
		delta := target - cur
		avail := p.max - p.active
		if avail < 0 {
			avail = 0
		}
		got := delta
		if got > avail {
			got = avail
		}
		if got > 0 {
			p.active += got
			p.perTask[taskID] = cur + got
		}
		log.Printf("%s [archive-thread] 任务 %s 调大线程数 %d -> %d（实际 %d，全局余量 %d）",
			dlLogTag, taskID, cur, target, p.perTask[taskID], avail)
		return p.perTask[taskID]
	}
	if target < cur {
		p.releaseLocked(taskID, cur-target)
		p.cond.Broadcast()
		log.Printf("%s [archive-thread] 任务 %s 调小线程数 %d -> %d",
			dlLogTag, taskID, cur, target)
	}
	return target
}

// acquired 返回任务当前持有的线程数
func (p *archiveThreadPool) acquired(taskID string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.perTask[taskID]
}

// releaseLocked 释放内部实现（调用方须持有 p.mu）
func (p *archiveThreadPool) releaseLocked(taskID string, n int) {
	cur := p.perTask[taskID]
	if cur <= 0 || n <= 0 {
		return
	}
	if n > cur {
		n = cur
	}
	p.perTask[taskID] = cur - n
	p.active -= n
	if p.perTask[taskID] == 0 {
		delete(p.perTask, taskID)
	}
	log.Printf("%s [archive-thread] 任务 %s 释放 %d 个线程（active=%d/%d）",
		dlLogTag, taskID, n, p.active, p.max)
}
