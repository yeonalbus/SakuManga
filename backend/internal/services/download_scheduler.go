package services

import (
	"log"
	"math"
	"time"

	"SakuManga/internal/models"
)

// ─────────────────────────────────────────────────────────────
// 统一调度器（Round45）
//
// 取代旧的「16 个 worker + 容量 256 的 channel」模型：
//   - 任务入队 = 进入无界 pendingQueue（永不丢弃，Enqueue 无失败分支）；
//   - 单一调度协程按「优先级降序 → 创建时间升序」挑候选，**非阻塞**预留资源后才发牌；
//   - 资源 = 优先级串并行门控 + 归档并发窗口（MaxArchiveConcurrency）+ 全局线程配额（10）；
//     资源不足则本轮停下等唤醒信号（任务释放 / 设置变更 / 1s 兜底），不占用执行协程；
//   - 执行协程只负责跑引擎与收尾，结束即释放预留并唤醒调度器。
//
// 并发语义（与 E-Hentai 官方一致）：
//   - downloadAllGalleriesSamePriority=false：全局串行，同一时刻仅 1 个任务；
//   - true：同一优先级可并行；低优先级任务等待所有更高优先级任务结束。
// ─────────────────────────────────────────────────────────────

// schedulerIdlePoll 调度器兜底轮询间隔（防唤醒信号丢失导致停滞）
const schedulerIdlePoll = time.Second

// schedulerLoop 统一调度主循环
func (m *DownloadManager) schedulerLoop() {
	log.Printf("%s [sched] 统一调度器已启动", dlLogTag)
	for {
		m.dispatch()
		select {
		case <-m.wakeCh:
		case <-m.quit:
			log.Printf("%s [sched] 调度器退出", dlLogTag)
			return
		case <-time.After(schedulerIdlePoll):
		}
	}
}

// Wake 唤醒调度器（非阻塞，容量 1 合并重复信号）
func (m *DownloadManager) Wake() {
	select {
	case m.wakeCh <- struct{}{}:
	default:
	}
}

// dispatch 尽量发牌：队首任务拿到资源就执行，拿不到就停（等下次唤醒）。
func (m *DownloadManager) dispatch() {
	for {
		cand, ok := m.pending.Peek()
		if !ok {
			return
		}
		// 快照可能过期（用户暂停/取消/抢占改状态）：回查 DB 后自愈清理
		var latest models.DownloadTask
		if err := m.db.First(&latest, "id = ?", cand.ID).Error; err != nil {
			m.pending.Remove(cand.ID)
			continue
		}
		if latest.Status != models.DownloadQueued {
			log.Printf("%s [sched] 任务 %s 状态为 %s（非 queued），移出待调度队列",
				dlWarnTag, latest.ID, latest.Status)
			m.pending.Remove(latest.ID)
			continue
		}

		res, ok := m.tryReserve(&latest)
		if !ok {
			return // 资源不足：任务保留在队列，等释放信号
		}
		m.pending.Remove(latest.ID)
		go m.executeTask(&latest, res)
	}
}

// tryReserve 非阻塞预留资源（优先级门控 + 归档并发窗口 + 全局线程配额）。
// 成功返回 reservation 并完成运行中记账；失败返回 false（任务保留在队列）。
func (m *DownloadManager) tryReserve(task *models.DownloadTask) (*reservation, bool) {
	if task == nil {
		return nil, false
	}
	setting := m.GetSettings()

	m.schedMu.Lock()
	defer m.schedMu.Unlock()

	// 防重复：同一任务已在执行中
	if _, running := m.reserved[task.ID]; running {
		return nil, false
	}

	// ① 优先级串并行门控
	if !m.slotAvailable(task, setting) {
		return nil, false
	}

	// ② 线程额度：归档受并发窗口限制；画廊尽力取余量（最少 1）
	res := &reservation{taskID: task.ID, priority: task.Priority}
	if task.Mode == models.DownloadModeArchive {
		res.archive = true
		threads := setting.ArchiveThreads
		if threads < 1 {
			threads = 1
		}
		if setting.ControlArchiveConcurrency {
			got := m.archivePool.tryArchiveSlot(task.ID, threads, setting.MaxArchiveConcurrency)
			if got <= 0 {
				return nil, false
			}
			res.threads = got
			res.pooled = true
		} else {
			// 不受控：按旧行为拿满线程，不占全局配额池
			res.threads = threads
		}
	} else {
		want := setting.ConcurrentImageDownloads
		if want <= 0 {
			want = 1
		}
		got := m.archivePool.tryAcquirePartial(task.ID, want)
		if got <= 0 {
			return nil, false
		}
		res.threads = got
		res.pooled = true
	}

	// ③ 运行中记账
	m.runningTotal++
	if task.Mode == models.DownloadModeArchive {
		m.runningArchive++
	}
	m.runningByPri[task.Priority]++
	m.runningTasks[task.ID] = task.Priority
	m.reserved[task.ID] = res
	log.Printf("%s [sched] 任务 %s 预留成功（优先级=%d mode=%s 线程=%d runningTotal=%d runningArchive=%d）",
		dlLogTag, task.ID, task.Priority, task.Mode, res.threads, m.runningTotal, m.runningArchive)
	return res, true
}

// releaseReservation 释放预留资源并唤醒调度器（任务结束/让位/失败统一走这里）。
func (m *DownloadManager) releaseReservation(res *reservation) {
	if res == nil {
		return
	}
	m.schedMu.Lock()
	if res.pooled {
		if res.archive {
			m.archivePool.releaseArchive(res.taskID)
		} else {
			m.archivePool.releaseAll(res.taskID)
		}
	}
	if m.runningTotal > 0 {
		m.runningTotal--
	}
	if res.archive && m.runningArchive > 0 {
		m.runningArchive--
	}
	if n := m.runningByPri[res.priority]; n > 0 {
		if n == 1 {
			delete(m.runningByPri, res.priority)
		} else {
			m.runningByPri[res.priority] = n - 1
		}
	}
	delete(m.runningTasks, res.taskID)
	delete(m.reserved, res.taskID)
	m.schedMu.Unlock()

	log.Printf("%s [sched] 任务 %s 释放预留（优先级=%d 线程=%d runningTotal=%d）",
		dlLogTag, res.taskID, res.priority, res.threads, m.runningTotal)
	m.Wake()
}

// slotAvailable 判断任务当前是否可执行（须在持有 schedMu 时调用）。
// 归档任务的「线程并发」由全局线程配额池管理，此处不按归档线程数限制任务数。
func (m *DownloadManager) slotAvailable(task *models.DownloadTask, setting *models.DownloadSetting) bool {
	if setting.DownloadAllGalleriesSamePriority {
		// 并行模式：低优先级任务必须等待所有更高优先级任务结束
		return task.Priority >= m.maxRunningPriority()
	}
	// 串行模式：同一时刻仅运行 1 个任务
	return m.runningTotal == 0
}

// maxRunningPriority 返回当前正在运行任务中的最高优先级（无运行任务时返回 math.MinInt）
func (m *DownloadManager) maxRunningPriority() int {
	max := math.MinInt
	for pri := range m.runningByPri {
		if pri > max {
			max = pri
		}
	}
	return max
}

// ─────────────────────────────────────────────────────────────
// 抢占式调度（计划书 5.5.2）：高优先级任务让低优先级任务让路
//
// 触发时机：新任务入队（CreateTask / ResumeTask / RetryTask / UnlockTask）或
// 优先级提升（SetTaskPriority）时调用 preemptLowerPriority(newPriority)。
// 被抢占任务状态置回 queued（进度保留，可断点续传），由 preemptOne 重新入队。
// ─────────────────────────────────────────────────────────────

// preemptLowerPriority 停止所有正在运行且优先级低于 newPriority 的任务。
func (m *DownloadManager) preemptLowerPriority(newPriority int) {
	m.schedMu.Lock()
	var victims []string
	for id, pri := range m.runningTasks {
		if pri < newPriority {
			victims = append(victims, id)
		}
	}
	m.schedMu.Unlock()

	if len(victims) == 0 {
		return
	}

	log.Printf("%s [sched] 抢占调度：优先级 %d 触发，抢占 %d 个低优先级任务 %v",
		dlLogTag, newPriority, len(victims), victims)
	for _, id := range victims {
		m.preemptOne(id)
	}
	// 释放的线程额度唤醒调度器重新分配
	m.archivePool.wakeAll()
	m.Wake()
}

// preemptOne 将单个正在运行的任务置回 queued 并立即中断引擎（进度保留）。
// 引擎 stopped() 检测到本地停止标记后中止收尾，不会覆盖 queued 状态。
func (m *DownloadManager) preemptOne(taskID string) {
	var task models.DownloadTask
	if err := m.db.First(&task, "id = ?", taskID).Error; err != nil {
		return
	}
	if task.Status != models.DownloadDownloading {
		return // 仅抢占正在下载的任务（排队中的任务无需打断）
	}
	task.Status = models.DownloadQueued
	task.UpdatedAt = time.Now()
	if err := m.db.Save(&task).Error; err != nil {
		log.Printf("%s [sched] 抢占任务 %s 置回 queued 失败: %v", dlErrTag, taskID, err)
		return
	}
	log.Printf("%s [sched] 任务 %s 已被高优先级任务抢占，置回 queued（进度 %d/%d 保留，等待重新调度）",
		dlLogTag, taskID, task.DoneFiles, task.TotalFiles)
	// 立即中断运行中的引擎（归档分块请求 / 画廊图片请求取消，.part 保留供恢复续传）
	m.stopArchiveDownload(taskID)
	m.stopGalleryDownload(taskID)
	// 等待被抢占引擎完全退出（释放线程配额与槽位），避免高优先级任务启动时
	// 仍占用线程配额导致其排队等待（"线程不足"）。
	m.waitArchiveStopped(taskID)
	m.waitGalleryStopped(taskID)
	// 重新入队：旧执行协程退出时释放预留，调度器随后重新分配
	m.Enqueue(taskID)
}
