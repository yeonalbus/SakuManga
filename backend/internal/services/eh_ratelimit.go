package services

import (
	"sync"
	"time"
)

// ─────────────────────────────────────────────────────────────
// Round26 性能优化 A：E 站请求自适应限流
//
// 背景：更新检测 / 维护查重 3a / GID 回填 三处原为固定 1.2s 睡眠
// （offline.go 与 backfillGIDOnline），3305 本全量 ≈ 1 小时+。
//
// 方案：成功提速（连续 5 次成功 → 间隔减半，下限 500ms ≈ 2 req/s），
// 失败退避（×2，上限 5s）。429/503/网络错误自动放慢，避免触发 E 站限流惩罚。
// 初始 800ms（介于旧 1.2s 与激进并发之间，Ex 会员通常可承受 1.5~2 req/s）。
//
// 注：StartOfflineTask 保证同一时刻只有一个离线任务，包级单例即可。
// ─────────────────────────────────────────────────────────────

const (
	rateLimitMin     = 500 * time.Millisecond // 下限（提速到 2 req/s 封顶）
	rateLimitMax     = 5 * time.Second        // 上限（退避封顶）
	rateLimitInit    = 800 * time.Millisecond // 初始间隔
	rateLimitStreak  = 5                      // 连续成功次数（达到后减半）
)

// EHRateLimiter 自适应请求间隔控制器
type EHRateLimiter struct {
	mu        sync.Mutex
	interval  time.Duration
	successes int
}

var ehRateLimiter = &EHRateLimiter{interval: rateLimitInit}

// Wait 请求前等待当前间隔（并发安全）
func (r *EHRateLimiter) Wait() {
	r.mu.Lock()
	d := r.interval
	r.mu.Unlock()
	if d > 0 {
		time.Sleep(d)
	}
}

// Mark 请求结果反馈：ok=true 连续 rateLimitStreak 次 → 间隔减半（下限 rateLimitMin）；
// ok=false → 间隔翻倍（上限 rateLimitMax）。removed/copyright 等确定性错误也按失败退避一次，
// 后续成功会恢复间隔，影响可忽略。
func (r *EHRateLimiter) Mark(ok bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if ok {
		r.successes++
		if r.successes >= rateLimitStreak && r.interval > rateLimitMin {
			r.interval /= 2
			if r.interval < rateLimitMin {
				r.interval = rateLimitMin
			}
			r.successes = 0
		}
		return
	}
	r.successes = 0
	r.interval *= 2
	if r.interval > rateLimitMax {
		r.interval = rateLimitMax
	}
}
