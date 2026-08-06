// src/composables/useBatchSelection.ts
// 在线列表页「长按多选 → 批量下载」共享逻辑：
// 封装 selectMode / selectedIds / 全选本页 / 已选目标映射，供各在线列表页复用。
import { ref, computed } from 'vue'
import type { OnlineComic } from '@/types/comic'
import type { DownloadTarget } from '@/api/download'

/**
 * 在线列表页的批量选择组合式
 * @param getItems 返回当前页所有在线漫画（用于「全选本页」与「已选→下载目标」映射）
 */
export function useBatchSelection(getItems: () => OnlineComic[]) {
  const selectMode = ref(false)
  const selectedIds = ref<string[]>([])

  const toggleSelect = (comic: Pick<OnlineComic, 'id'>) => {
    const idx = selectedIds.value.indexOf(comic.id)
    if (idx >= 0) selectedIds.value.splice(idx, 1)
    else selectedIds.value.push(comic.id)
  }

  /** 长按卡片进入选择模式并选中该卡片 */
  const handleLongPress = (comic: Pick<OnlineComic, 'id'>) => {
    selectMode.value = true
    toggleSelect(comic)
  }

  /** 选择模式下点击卡片切换选中 */
  const handleSelect = (comic: Pick<OnlineComic, 'id'>) => {
    toggleSelect(comic)
  }

  const exitSelectMode = () => {
    selectMode.value = false
    selectedIds.value = []
  }

  /** 全选 / 取消全选当前页可见项 */
  const toggleSelectAll = () => {
    const items = getItems()
    const pageIds = items.map((c) => c.id)
    if (pageIds.length === 0) return
    const allSelected = pageIds.every((id) => selectedIds.value.includes(id))
    if (allSelected) {
      selectedIds.value = selectedIds.value.filter((id) => !pageIds.includes(id))
    } else {
      selectedIds.value = Array.from(new Set([...selectedIds.value, ...pageIds]))
    }
  }

  /**
   * 已选中的画廊 → 批量下载目标。
   * 仅包含带 token 的项（无 token 无法入队，如历史页经刷新后的记录），避免后端必败。
   */
  const selectedTargets = computed<DownloadTarget[]>(() => {
    const byId = new Map(getItems().map((c) => [c.id, c]))
    const targets: DownloadTarget[] = []
    for (const id of selectedIds.value) {
      const c = byId.get(id)
      if (!c || !c.token) continue
      targets.push({
        gid: c.id,
        token: c.token,
        title: c.title,
        coverUrl: c.coverUrl,
      })
    }
    return targets
  })

  /** BatchDownloadBar 关闭/完成后退出选择模式 */
  const handleBatchClose = () => {
    exitSelectMode()
  }

  return {
    selectMode,
    selectedIds,
    selectedTargets,
    toggleSelect,
    handleLongPress,
    handleSelect,
    toggleSelectAll,
    exitSelectMode,
    handleBatchClose,
  }
}
