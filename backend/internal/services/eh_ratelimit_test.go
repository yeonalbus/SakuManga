package services

import "testing"

// ─────────────────────────────────────────────────────────────
// Round26 性能 A：EHRateLimiter 自适应限流单测
// 验证：初始间隔 / 连续成功提速至下限 / 失败退避至上限 / streak 重置
// ─────────────────────────────────────────────────────────────

func TestEHRateLimiterAdaptive(t *testing.T) {
	r := &EHRateLimiter{interval: rateLimitInit}

	// 初始间隔
	if r.interval != rateLimitInit {
		t.Fatalf("初始间隔应为 %v，得到 %v", rateLimitInit, r.interval)
	}

	// 连续成功 rateLimitStreak 次 → 减半，但不得低于下限
	for i := 0; i < rateLimitStreak; i++ {
		r.Mark(true)
	}
	if r.interval != rateLimitMin {
		t.Errorf("连续 %d 次成功后间隔应为下限 %v，得到 %v", rateLimitStreak, rateLimitMin, r.interval)
	}

	// 持续成功不得突破下限
	for i := 0; i < 20; i++ {
		r.Mark(true)
	}
	if r.interval != rateLimitMin {
		t.Errorf("持续成功后间隔不得低于下限 %v，得到 %v", rateLimitMin, r.interval)
	}

	// 一次失败 → 翻倍
	r.Mark(false)
	if r.interval != rateLimitMin*2 {
		t.Errorf("失败后间隔应翻倍为 %v，得到 %v", rateLimitMin*2, r.interval)
	}

	// 连续失败 → 上限封顶
	for i := 0; i < 20; i++ {
		r.Mark(false)
	}
	if r.interval != rateLimitMax {
		t.Errorf("连续失败后间隔应封顶为 %v，得到 %v", rateLimitMax, r.interval)
	}

	// 失败清零成功计数：失败后单次成功不立即降间隔（未达 streak）
	r2 := &EHRateLimiter{interval: 2 * rateLimitMin, successes: 0}
	r2.Mark(true)
	if r2.interval != 2*rateLimitMin {
		t.Errorf("单次成功未达 streak 不应降间隔，得到 %v", r2.interval)
	}
}

func TestEHRateLimiterStreakReset(t *testing.T) {
	// 成功→失败 打乱 streak：失败必须清零连续成功计数，之后需重新累积；
	// 退避后恢复是「每次达标减半」的渐进过程（2s → 1s → 500ms），而非一次跳回下限。
	r := &EHRateLimiter{interval: 2 * rateLimitMin}
	r.Mark(true)
	r.Mark(true)
	r.Mark(false) // 清零，interval 1s → 2s
	r.Mark(true)  // 重新计数 1
	if r.interval != 2*rateLimitMin*2 {
		t.Errorf("失败清零后单次成功不应降间隔，得到 %v", r.interval)
	}
	r.Mark(true) // 计数 2
	r.Mark(true) // 计数 3
	r.Mark(true) // 计数 4
	if r.interval != 2*rateLimitMin*2 {
		t.Errorf("未达 streak(5) 不应降间隔，得到 %v", r.interval)
	}
	r.Mark(true) // 计数 5 → 减半：2s → 1s
	if r.interval != 2*rateLimitMin {
		t.Errorf("连续 5 次成功应减半到 %v，得到 %v", 2*rateLimitMin, r.interval)
	}
	// 再 5 次成功 → 1s → 500ms（下限）
	for i := 0; i < rateLimitStreak; i++ {
		r.Mark(true)
	}
	if r.interval != rateLimitMin {
		t.Errorf("继续 5 次成功应降到下限 %v，得到 %v", rateLimitMin, r.interval)
	}
}
