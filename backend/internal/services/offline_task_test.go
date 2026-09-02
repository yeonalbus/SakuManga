package services

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ─────────────────────────────────────────────────────────────
// Round29：离线任务暂停 / 继续 / 取消 状态机单元测试
// 覆盖：pause → checkpoint 挂起（进度不再增长）→ resume 恢复 →
//      running 中 cancel → 检查点返回 ErrOfflineTaskCancelled → cancelled 收尾
// ─────────────────────────────────────────────────────────────

// resetOfflineTaskStateForTest 复位单槽位任务全局状态（测试隔离）
func resetOfflineTaskStateForTest() {
	offlineTaskMu.Lock()
	defer offlineTaskMu.Unlock()
	offlineTaskState = OfflineTaskState{Status: OfflineTaskIdle}
	offlineTaskCancel = false
}

// TestOfflineTaskPauseResume 暂停后任务在检查点挂起（进度停止），继续后恢复
func TestOfflineTaskPauseResume(t *testing.T) {
	resetOfflineTaskStateForTest()
	defer resetOfflineTaskStateForTest()

	if !StartOfflineTask(OfflineTaskMaintain) {
		t.Fatal("启动任务失败（单槽位被占用？）")
	}

	var processed int32
	var wg sync.WaitGroup
	wg.Add(1)
	stop := make(chan struct{})
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			// 与业务循环一致：处理每项前经过检查点
			if err := OfflineTaskCheckpoint(); err != nil {
				return // 取消/异常 → 退出
			}
			atomic.AddInt32(&processed, 1)
			time.Sleep(5 * time.Millisecond) // 模拟处理一项
		}
	}()

	time.Sleep(40 * time.Millisecond) // 任务已处理若干项
	if !PauseOfflineTask() {
		t.Fatal("暂停请求失败")
	}
	// 暂停后：任务应在检查点挂起，processed 停止增长（预留 30ms 让当前项完成并进入挂起）
	time.Sleep(40 * time.Millisecond)
	before := atomic.LoadInt32(&processed)
	time.Sleep(80 * time.Millisecond)
	if after := atomic.LoadInt32(&processed); after != before {
		t.Fatalf("暂停后进度仍在增长：before=%d after=%d", before, after)
	}
	if st := GetOfflineTaskProgress(); st.Status != OfflineTaskPaused {
		t.Fatalf("暂停后状态=%s，期望 paused", st.Status)
	}

	// 继续：进度恢复增长
	if !ResumeOfflineTask() {
		t.Fatal("继续请求失败")
	}
	time.Sleep(80 * time.Millisecond)
	if after := atomic.LoadInt32(&processed); after <= before {
		t.Fatalf("继续后进度未恢复：before=%d after=%d", before, after)
	}

	// 收尾：停任务 + 取消收尾为 cancelled
	close(stop)
	if !CancelOfflineTask() {
		t.Fatal("取消请求失败（任务应仍为 running）")
	}
	FinishOfflineTask(ErrOfflineTaskCancelled)
	if st := GetOfflineTaskProgress(); st.Status != OfflineTaskCancelled {
		t.Fatalf("取消收尾后状态=%s，期望 cancelled", st.Status)
	}
	wg.Wait()
}

// TestOfflineTaskCancelWhileRunning running 中取消 → 下一个检查点返回取消错误
func TestOfflineTaskCancelWhileRunning(t *testing.T) {
	resetOfflineTaskStateForTest()
	defer resetOfflineTaskStateForTest()

	if !StartOfflineTask(OfflineTaskUpdate) {
		t.Fatal("启动任务失败（单槽位被占用？）")
	}

	errCh := make(chan error, 1)
	go func() {
		// 第一遍检查点正常通过，模拟处理一项后再次进入检查点
		if err := OfflineTaskCheckpoint(); err != nil {
			errCh <- err
			return
		}
		time.Sleep(30 * time.Millisecond)
		errCh <- OfflineTaskCheckpoint()
	}()

	time.Sleep(10 * time.Millisecond) // 已通过第一遍检查点
	if !CancelOfflineTask() {
		t.Fatal("取消请求失败")
	}
	select {
	case err := <-errCh:
		if !errors.Is(err, ErrOfflineTaskCancelled) {
			t.Fatalf("期望 ErrOfflineTaskCancelled，实际得到 %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("取消后检查点未返回，任务未退出（超时）")
	}

	FinishOfflineTask(ErrOfflineTaskCancelled)
	if st := GetOfflineTaskProgress(); st.Status != OfflineTaskCancelled {
		t.Fatalf("取消收尾后状态=%s，期望 cancelled", st.Status)
	}
}

// TestOfflineTaskCancelOnPaused 暂停状态取消 → 立即唤醒挂起 goroutine 并退出
func TestOfflineTaskCancelOnPaused(t *testing.T) {
	resetOfflineTaskStateForTest()
	defer resetOfflineTaskStateForTest()

	if !StartOfflineTask(OfflineTaskMaintain) {
		t.Fatal("启动任务失败（单槽位被占用？）")
	}

	entered := make(chan struct{})
	goAgain := make(chan struct{})
	errCh := make(chan error, 1)
	go func() {
		// 第一遍检查点：任务 running，直接通过
		if err := OfflineTaskCheckpoint(); err != nil {
			errCh <- err
			return
		}
		close(entered)
		<-goAgain // 等待主协程完成暂停后再进第二遍
		// 第二遍检查点：任务已 paused → 挂起，直到 cancel 唤醒退出
		errCh <- OfflineTaskCheckpoint()
	}()

	<-entered
	if !PauseOfflineTask() {
		t.Fatal("暂停请求失败")
	}
	close(goAgain)
	time.Sleep(40 * time.Millisecond) // 确保 goroutine 已进入挂起等待
	if st := GetOfflineTaskProgress(); st.Status != OfflineTaskPaused {
		t.Fatalf("状态=%s，期望 paused", st.Status)
	}

	// paused 状态取消：应唤醒挂起 goroutine 使其返回取消错误
	if !CancelOfflineTask() {
		t.Fatal("取消请求失败")
	}
	select {
	case err := <-errCh:
		if !errors.Is(err, ErrOfflineTaskCancelled) {
			t.Fatalf("期望 ErrOfflineTaskCancelled，实际得到 %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("暂停态取消后 goroutine 未唤醒（超时）")
	}

	FinishOfflineTask(ErrOfflineTaskCancelled)
	if st := GetOfflineTaskProgress(); st.Status != OfflineTaskCancelled {
		t.Fatalf("取消收尾后状态=%s，期望 cancelled", st.Status)
	}
}

// TestOfflineTaskStartConflict paused 挂起期间禁止启动新任务（单槽位）
func TestOfflineTaskStartConflict(t *testing.T) {
	resetOfflineTaskStateForTest()
	defer resetOfflineTaskStateForTest()

	if !StartOfflineTask(OfflineTaskUpdate) {
		t.Fatal("启动任务失败")
	}
	if !PauseOfflineTask() {
		t.Fatal("暂停失败")
	}
	if StartOfflineTask(OfflineTaskMaintain) {
		t.Fatal("paused 挂起期间不应允许启动新任务")
	}
	// 正常收尾后允许再启动
	CancelOfflineTask()
	FinishOfflineTask(ErrOfflineTaskCancelled)
	if !StartOfflineTask(OfflineTaskMaintain) {
		t.Fatal("收尾后应允许启动新任务")
	}
	FinishOfflineTask(nil)
	if st := GetOfflineTaskProgress(); st.Status != OfflineTaskSuccess {
		t.Fatalf("成功收尾后状态=%s，期望 success", st.Status)
	}
}
