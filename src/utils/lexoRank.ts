// src/utils/lexoRank.ts
// Round22：书架列表 / 书架内本子排序的 LexoRank 浮点分段权值工具。
//
// 策略：
// - 初始权值 index * 1000（0, 1000, 2000, …），旧数据首次进入排序/拖动时按当前顺序惰性赋权；
// - 插入邻项 a、b 之间 → 中点 (a+b)/2（如 1000 与 2000 之间 → 1500）；
// - 插入顶部 → 首项权值 - 1；插入底部 → 末项权值 + 1；
// - 中点保留 6 位小数；两邻项 gap < MIN_GAP 时中点会被舍入吞掉 → 精度用尽，
//   由调用方触发一次异步全量重置（reweightAll 重新赋 1000*i）。
// 单次移动只更新一项权值，取代旧的全量数组重写，极大减少频繁排序的写入开销。

export const INITIAL_STEP = 1000
/** 中点保留的小数位数（6 位） */
const MID_PRECISION = 1e6
/** 精度用尽阈值：gap 小于该值即无法再插入 */
export const MIN_GAP = 1e-6

/** 权值保留 6 位小数（避免多次折半后浮点噪音无限累积） */
export const round6 = (n: number): number => Math.round(n * MID_PRECISION) / MID_PRECISION

/**
 * 计算两个邻项之间的中点权值。
 * @param a 前项权值；undefined = 无限小端（插入到最前）
 * @param b 后项权值；undefined = 无限大端（插入到最后）
 */
export function between(a: number | undefined, b: number | undefined): number {
  if (a === undefined && b === undefined) return INITIAL_STEP
  if (a === undefined) return round6((b as number) - 1)
  if (b === undefined) return round6(a + 1)
  return round6((a + b) / 2)
}

/** 精度是否已用尽：两邻项之间取整后无法再插入 */
export function needsReweight(a: number, b: number): boolean {
  return b - a < MIN_GAP
}

/** 计算目标插入位置需要的权值（含精度用尽判定与自动兜底） */
export function weightForInsert(
  orderedWeights: number[],
  insertIndex: number,
): { weight: number; needsFullReweight: boolean } {
  const prev = insertIndex > 0 ? orderedWeights[insertIndex - 1] : undefined
  const next = insertIndex < orderedWeights.length ? orderedWeights[insertIndex] : undefined
  if (prev !== undefined && next !== undefined && needsReweight(prev, next)) {
    return { weight: between(prev, next), needsFullReweight: true }
  }
  return { weight: between(prev, next), needsFullReweight: false }
}

/**
 * 全量重置权值：按给定顺序赋 1000*i（i 从 1 起）。
 * 返回 {ids: 顺序数组, weights: Map<id, weight>}。
 */
export function reweightAll<T>(ids: T[]): { ids: T[]; weights: Map<T, number> } {
  const weights = new Map<T, number>()
  ids.forEach((id, i) => weights.set(id, (i + 1) * INITIAL_STEP))
  return { ids, weights }
}

/**
 * 由权值表推导展示顺序：有权值的按权值升序在前，无权值（旧数据/新增）保持原数组顺序排后。
 * 与后端 sortedComicIDs 语义一致。
 */
export function orderByWeights<T>(ids: T[], weightOf: (id: T) => number | undefined): T[] {
  const keyed: { id: T; w: number }[] = []
  const unkeyed: T[] = []
  for (const id of ids) {
    const w = weightOf(id)
    if (w !== undefined) keyed.push({ id, w })
    else unkeyed.push(id)
  }
  keyed.sort((x, y) => x.w - y.w)
  return [...keyed.map((k) => k.id), ...unkeyed]
}
