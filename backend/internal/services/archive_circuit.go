package services

import "sync"

// ─────────────────────────────────────────────────────────────
// 归档下载熔断器（画廊级）
//
// 语义（对齐定稿方案"阶梯 3"）：
//   - 计数单位是**画廊**，不是节点：同一画廊的多次重试只算一次结局
//   - 连续 N 个画廊都因**网络类错误**（EOF / 超时 / 连接重置 / IP-lock）判错 → 判定当前代理组完全失效
//   - 画廊自身问题（HTTP 403/404、页面结构异常、解压失败）**不计数**，直接跳过
//   - 任何一个画廊成功解锁 → 打断连续性
//
// 进程内状态，重启归零；用户手动重试时可通过 Reset 解除。
// ─────────────────────────────────────────────────────────────

// archiveCircuitWindow 连续多少个画廊网络失败后判定代理组失效
const archiveCircuitWindow = 3

// circuitEntry 单个画廊的最终结局
type circuitEntry struct {
	gid         string
	networkFail bool
}

// archiveCircuit 画廊级连续网络失败熔断器
type archiveCircuit struct {
	mu    sync.Mutex
	trail []circuitEntry
}

// globalArchiveCircuit 进程级单例
var globalArchiveCircuit = &archiveCircuit{}

// ArchiveCircuit 返回全局归档熔断器
func ArchiveCircuit() *archiveCircuit { return globalArchiveCircuit }

// Record 记录一个画廊的最终结局（成功或失败都应调用）：
//   - networkFail=true：该画廊因网络类错误判错
//   - networkFail=false：该画廊成功解锁（打断连续性）
//
// 同一 gid 重复记录只保留最新一条，避免多次重试把窗口撑满。
func (c *archiveCircuit) Record(gid string, networkFail bool) {
	if gid == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	kept := c.trail[:0]
	for _, e := range c.trail {
		if e.gid != gid {
			kept = append(kept, e)
		}
	}
	c.trail = append(kept, circuitEntry{gid: gid, networkFail: networkFail})
	if len(c.trail) > archiveCircuitWindow {
		c.trail = c.trail[len(c.trail)-archiveCircuitWindow:]
	}
}

// Tripped 最近 archiveCircuitWindow 个画廊是否全部为网络类失败（= 代理组判定失效）。
func (c *archiveCircuit) Tripped() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.trail) < archiveCircuitWindow {
		return false
	}
	for _, e := range c.trail {
		if !e.networkFail {
			return false
		}
	}
	return true
}

// Reset 清空熔断状态（手动重试 / 配置变更 / 成功换到可用节点时调用）。
func (c *archiveCircuit) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.trail = nil
}

// Snapshot 返回当前窗口快照（供日志/调试）
func (c *archiveCircuit) Snapshot() []circuitEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]circuitEntry, len(c.trail))
	copy(out, c.trail)
	return out
}
