/**
 * 额外扫描路径 Store：与 Go 后端 /scan-paths 接口交互
 * 由原 appStore 拆分而来
 */
import { ref } from 'vue'
import { API_BASE } from '@/config/api'

/** 额外扫描路径 */
export interface ExtraScanPath {
  id: string
  path: string
  includeSubfolders: boolean
  lastScanned?: number
  comicCount?: number
}

/** 扫描路径列表 */
export const scanPaths = ref<ExtraScanPath[]>([])

/** 从 Go 后端拉取所有扫描路径 */
export const fetchScanPaths = async () => {
  try {
    const res = await fetch(`${API_BASE}/scan-paths`)
    if (res.ok) {
      scanPaths.value = await res.json()
    }
  } catch (err) {
    console.error('连接 Go 后端失败，请检查 backend 服务是否启动:', err)
  }
}

/** 添加新路径，成功返回 true */
export const addScanPath = async (path: string): Promise<boolean> => {
  try {
    const res = await fetch(`${API_BASE}/scan-paths`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ path: path.trim() }),
    })
    if (res.ok) {
      const newPath = await res.json()
      scanPaths.value.push(newPath)
      return true
    }
  } catch (err) {
    console.error('添加路径失败:', err)
  }
  return false
}

/** 切换是否包含子文件夹 */
export const toggleSubfolders = async (id: string, includeSubfolders: boolean) => {
  const item = scanPaths.value.find((p) => p.id === id)
  if (item) {
    item.includeSubfolders = includeSubfolders
    try {
      await fetch(`${API_BASE}/scan-paths/${id}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ includeSubfolders }),
      })
    } catch (err) {
      console.error('更新子文件夹状态失败:', err)
    }
  }
}

/** 移除指定路径 */
export const removeScanPath = async (id: string) => {
  scanPaths.value = scanPaths.value.filter((p) => p.id !== id)
  try {
    await fetch(`${API_BASE}/scan-paths/${id}`, {
      method: 'DELETE',
    })
  } catch (err) {
    console.error('删除路径失败:', err)
  }
}

/** 触发指定路径的后端扫描并回填统计 */
export const updateScanPathStats = async (id: string) => {
  try {
    const res = await fetch(`${API_BASE}/scan-paths/${id}/scan`, {
      method: 'POST',
    })
    if (res.ok) {
      const data = await res.json()
      const item = scanPaths.value.find((p) => p.id === id)
      if (item) {
        item.lastScanned = data.lastScanned
        item.comicCount = data.comicCount
      }
    }
  } catch (err) {
    console.error('触发扫描失败:', err)
  }
}
