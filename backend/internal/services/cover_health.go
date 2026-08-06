package services

import (
	"log"
	"sync"
	"sync/atomic"
	"time"
)

// 封面代理健康统计与动态降级。
//
// cover-proxy 依赖 VPN/代理隧道访问 E 站图片 CDN。隧道抖动时，大量并发请求会
// 进一步撑爆隧道导致成片 EOF。这里维护一个滑动窗口统计封面请求成败：
//   - 失败率高（>= 30%）→ 自动降级：封面并发 6→2、退避拉长（500ms 级 → 秒级）；
//   - 连续成功足够多（>= 10 次）→ 恢复：并发与退避回弹。
//
// 进入降级与恢复的阈值不同（滞回），避免在阈值附近反复振荡。

const (
	// coverWindowSize 健康统计滑动窗口大小（最近 N 个有效样本）
	coverWindowSize = 20
	// coverDegradeFailRatio 触发降级的失败率阈值
	coverDegradeFailRatio = 0.30
	// coverRecoverConsecutiveOK 从降级恢复所需的连续成功次数
	coverRecoverConsecutiveOK = 10
	// coverConcurrencyNormal / coverConcurrencyDegraded 正常/降级时的封面并发上限
	coverConcurrencyNormal   = 6
	coverConcurrencyDegraded = 2
)

var (
	coverHealthMu            sync.Mutex
	coverHealthResults       []bool // 最近窗口结果，true=成功；仅统计网络层/5xx/429 样本
	coverHealthDegraded      bool
	coverHealthConsecutiveOK int

	// 封面并发闸门（应用层限流）：保证同一时刻在飞的封面请求不超过 coverLimit。
	coverGateMu   sync.Mutex
	coverGateCond = sync.NewCond(&coverGateMu)
	coverLimit    atomic.Int64
	coverActive   atomic.Int64
)

func init() {
	coverLimit.Store(int64(coverConcurrencyNormal))
}

// RecordCoverResult 记录一次封面代理请求结果（ok=是否成功）并据此更新降级状态。
// 只有与隧道/源站健康相关的样本才应传入：网络错误、5xx、429 传 false；成功传 true。
// 403（凭证/风控）、404/410（图不存在）等确定性结果不应调用，避免污染统计。
func RecordCoverResult(ok bool) {
	coverHealthMu.Lock()

	// 更新滑动窗口
	if len(coverHealthResults) >= coverWindowSize {
		copy(coverHealthResults, coverHealthResults[1:])
		coverHealthResults = coverHealthResults[:coverWindowSize-1]
	}
	coverHealthResults = append(coverHealthResults, ok)

	// 连续成功计数（用于滞回恢复）
	if ok {
		coverHealthConsecutiveOK++
	} else {
		coverHealthConsecutiveOK = 0
	}

	// 滞回状态机：进入降级看失败率，恢复看连续成功
	newDegraded := coverHealthDegraded
	if !newDegraded {
		if coverFailRatioLocked() >= coverDegradeFailRatio {
			newDegraded = true
		}
	} else if coverHealthConsecutiveOK >= coverRecoverConsecutiveOK {
		newDegraded = false
	}

	if newDegraded != coverHealthDegraded {
		coverHealthDegraded = newDegraded
		concurrency := coverConcurrencyFor(newDegraded)
		setCoverConcurrency(concurrency)
		if newDegraded {
			log.Printf("[COVER-HEALTH] 封面代理进入降级模式: 并发=%d, 退避拉长", concurrency)
		} else {
			log.Printf("[COVER-HEALTH] 封面代理恢复正常: 并发=%d", concurrency)
		}
	}

	coverHealthMu.Unlock()
}

// coverFailRatioLocked 计算当前窗口失败率（调用方需持有 coverHealthMu）。
func coverFailRatioLocked() float64 {
	if len(coverHealthResults) == 0 {
		return 0
	}
	fails := 0
	for _, ok := range coverHealthResults {
		if !ok {
			fails++
		}
	}
	return float64(fails) / float64(len(coverHealthResults))
}

// coverConcurrencyFor 返回给定降级状态下的封面并发上限。
func coverConcurrencyFor(degraded bool) int {
	if degraded {
		return coverConcurrencyDegraded
	}
	return coverConcurrencyNormal
}

// IsCoverDegraded 返回当前是否处于封面降级模式。
func IsCoverDegraded() bool {
	coverHealthMu.Lock()
	defer coverHealthMu.Unlock()
	return coverHealthDegraded
}

// CoverBackoffFor 返回第 attempt 次重试前的退避间隔（attempt 从 1 开始，最多 3 次）。
// 降级时使用秒级退避，避免隧道抖动期间继续高频打请求。
func CoverBackoffFor(attempt int) time.Duration {
	if IsCoverDegraded() {
		switch attempt {
		case 1:
			return 2 * time.Second
		case 2:
			return 4 * time.Second
		default:
			return 8 * time.Second
		}
	}
	switch attempt {
	case 1:
		return 500 * time.Millisecond
	case 2:
		return 1200 * time.Millisecond
	default:
		return 2500 * time.Millisecond
	}
}

// AcquireCoverSlot 获取一个封面请求并发槽位，返回释放函数。
// 超过当前并发上限的请求在此排队（而非并发打向隧道），排队即天然错峰。
func AcquireCoverSlot() func() {
	coverGateMu.Lock()
	for coverActive.Load() >= coverLimit.Load() {
		coverGateCond.Wait()
	}
	coverActive.Add(1)
	coverGateMu.Unlock()

	return func() {
		coverGateMu.Lock()
		coverActive.Add(-1)
		coverGateCond.Signal()
		coverGateMu.Unlock()
	}
}

// setCoverConcurrency 动态调整封面并发上限（正常 6 / 降级 2），并唤醒排队请求。
func setCoverConcurrency(n int) {
	coverLimit.Store(int64(n))
	coverGateCond.Broadcast()
}
