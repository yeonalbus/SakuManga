/**
 * 个人评分 Store：按登录用户隔离，存储于后端 /ratings API
 * 提供评分映射的加载、读取与设置（1-5 星，0 分表示清除）。
 */
import { ref } from 'vue'
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
