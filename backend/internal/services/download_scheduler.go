package services

import (
	"log"
	"math"
	"time"

	"SakuHentai/internal/models"
)

// ─────────────────────────────────────────────────────────────
// 调度门控：控制「同一优先级并发」
//
// 语义与 E-Hentai 官方一致：
//   - downloadAllGalleriesSamePriority=false：全局串行，同一时刻仅下载 1 个画廊（含归档）
//   - downloadAllGalleriesSamePriority=true：同一优先级可并行；低优先级任务需等待更高优先级任务结束
//
// 归档任务的「线程并发」不在此门控：由 archive_thread_pool 全局线程配额池统一管理
//（controlArchiveConcurrency=true 时，归档任务需先获取全局线程配额，不足则排队等待）。
//
// worker 池数量（NewDownloadManager 的 workers）仅作并发上限兜底，
// 真实并发由这里按下载设置决定。
// ─────────────────────────────────────────────────────────────

// acquireSlot 按下载设置等待可执行槽位（阻塞轮询）。
// 等待期间若任务被用户暂停/取消/删除（状态不再是 queued），立即返回且不占用槽位，
// 避免占着 worker 空等；调用方在拿到槽位后仍会重新校验任务状态。
// 每次轮询重读任务记录：SetTaskPriority 修改优先级后，等待中的任务按新优先级重新判断
//（实现计划书 5.5.1「出队排序」的等效语义——高优先级任务优先竞争槽位）。
func (m *DownloadManager) acquireSlot(task *models.DownloadTask) {
	for {
		// 重读任务：状态/优先级可能被暂停/取消/SetTaskPriority 修改
		var latest models.DownloadTask
		if err := m.db.First(&latest, "id = ?", task.ID).Error; err != nil {
			log.Printf("%s [sched] 任务 %s 不存在或已被删除: %v", dlWarnTag, task.ID, err)
			return
		}
		if latest.Status != models.DownloadQueued {
			log.Printf("%s [sched] 任务 %s 等待槽位期间状态变为 %s，放弃获取槽位", dlWarnTag, task.ID, latest.Status)
			return
		}
		task = &latest // 用最新记录（含修改后的优先级）

		setting := m.GetSettings()

		m.schedMu.Lock()
		ok := m.slotAvailable(task, setting)
		if ok {
			m.runningTotal++
			if task.Mode == models.DownloadModeArchive {
				m.runningArchive++
			}
			m.runningByPri[task.Priority]++
			m.runningTasks[task.ID] = task.Priority // 记录运行中任务（供抢占）
			m.schedMu.Unlock()

			log.Printf("%s [sched] 任务 %s 获取槽位（优先级=%d mode=%s runningTotal=%d runningArchive=%d）",
				dlLogTag, task.ID, task.Priority, task.Mode, m.runningTotal, m.runningArchive)
			return
		}
		m.schedMu.Unlock()

		time.Sleep(400 * time.Millisecond)
	}
}

// releaseSlot 释放任务占用的槽位（defer 调用，须在 acquireSlot 成功后使用）
func (m *DownloadManager) releaseSlot(task *models.DownloadTask) {
	m.schedMu.Lock()
	defer m.schedMu.Unlock()

	if m.runningTotal > 0 {
		m.runningTotal--
	}
	if task.Mode == models.DownloadModeArchive && m.runningArchive > 0 {
		m.runningArchive--
	}
	if n := m.runningByPri[task.Priority]; n > 0 {
		if n == 1 {
			delete(m.runningByPri, task.Priority)
		} else {
			m.runningByPri[task.Priority] = n - 1
		}
	}
	delete(m.runningTasks, task.ID)

	log.Printf("%s [sched] 任务 %s 释放槽位（优先级=%d mode=%s runningTotal=%d runningArchive=%d）",
		dlLogTag, task.ID, task.Priority, task.Mode, m.runningTotal, m.runningArchive)
}

// slotAvailable 判断任务当前是否可执行（须在持有 schedMu 时调用）
func (m *DownloadManager) slotAvailable(task *models.DownloadTask, setting *models.DownloadSetting) bool {
	// 归档任务的线程并发由 archiveThreadPool 全局配额池管理（不足时在引擎内排队等待），
	// 此处不再按 archiveThreads 限制同时运行的归档任务数，多个归档任务可并发启动。

	if setting.DownloadAllGalleriesSamePriority {
		// 并行模式：低优先级任务必须等待所有更高优先级任务结束
		if task.Priority < m.maxRunningPriority() {
			return false
		}
	} else {
		// 串行模式：同一时刻仅运行 1 个任务
		if m.runningTotal > 0 {
			return false
		}
	}
	return true
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
// 被抢占任务状态置回 queued（进度保留，可断点续传），由 worker 在 runTask 退出时
// 自动重新入队；释放的全局线程额度经 wakeAll 唤醒排队任务竞争空位。
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
	// 释放的线程额度唤醒排队任务（含新入队的高优先级任务）竞争空位
	m.archivePool.wakeAll()
}

// preemptOne 将单个正在运行的任务置回 queued 并立即中断引擎（进度保留）。
// 引擎 stopped() 检测到本地停止标记后中止收尾，不会覆盖 queued 状态。
func (m *DownloadManager) preemptOne(taskID string) {
	var task models.DownloadTask
	if err := m.db.First(&task, "id = ?", taskID).Error; err != nil {
		return
	}
	if task.Status != models.DownloadDownloading {
		return // 仅抢占正在下载的任务（等待槽位/排队中的任务无需打断）
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
}
