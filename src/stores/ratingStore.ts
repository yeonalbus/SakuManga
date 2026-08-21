/**
 * 个人评分 Store：按登录用户隔离，存储于后端 /ratings API
 * 提供评分映射的加载、读取与设置（1-5 星，0 分表示清除）。
 */
import { ref } from 'vue'
import type { ComicItem } from '@/types/comic'
import { http } from '@/utils/request'

/** 个人评分映射：comicId -> score（1-5 星，缺失表示未评分） */
export const myRatings = ref<Record<string, number>>({})

/** 从后端加载当前用户的全部评分 */
export const loadMyRatings = async () => {
  try {
    const data = await http<{ ratings: Record<string, number> }>('/ratings')
    myRatings.value = data.ratings || {}
  } catch (e) {
    console.error('加载个人评分失败:', e)
  }
}

/** 读取某作品的个人评分（未评分返回 0） */
export const getMyRating = (comicId: string): number => myRatings.value[comicId] || 0

/**
 * 生效评分（Round10-Bug2/3）：卡片展示与离线星级筛选统一使用的评分口径。
 * - 离线漫画：优先个人评分（详情页 1-5 星，myRatings），未评分回退社区评分 comic.rating；
 * - 在线漫画：保持社区评分 comic.rating。
 * 修复「详情页打分后卡片仍显示 —」「星级筛选按社区评分永远筛不出个人评分」两个问题。
 */
export const getEffectiveRating = (
  comic: Pick<ComicItem, 'id' | 'source' | 'rating'>,
): number => {
  if (comic.source === 'offline') {
    const mine = myRatings.value[comic.id]
    if (mine && mine > 0) return mine
  }
  const r = Number(comic.rating)
  return Number.isFinite(r) && r > 0 ? r : 0
}

/** 设置/清除某作品的个人评分（score <= 0 视为清除） */
export const setMyRating = async (comicId: string, score: number) => {
  if (score <= 0) {
    delete myRatings.value[comicId]
    try {
      await http(`/ratings/${comicId}`, { method: 'DELETE' })
    } catch (e) {
      console.error('删除评分失败:', e)
    }
    return
  }
  myRatings.value[comicId] = score
  try {
    await http(`/ratings/${comicId}`, {
      method: 'PUT',
      body: JSON.stringify({ score }),
    })
  } catch (e) {
    console.error('保存评分失败:', e)
  }
}