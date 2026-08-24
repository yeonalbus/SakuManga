// src/composables/useShelfQuickAdd.ts
// Round22：离线多选「快捷加入书架」共享逻辑。
// 复用面：离线首页 / 书架内部 / 离线历史 / 手气不错（离线结果）。
// 封装：selectMode / selectedIds / 长按进入（仅离线漫画）/ 点选 / 全选本页 / 书架浮层 / 批量加入 / 退出。
// 配合 ShelfQuickAddToolbar.vue 使用；页面额外动作（如书架内「移出书架」）由页面自行实现后调用 exitSelectMode。
import { ref } from 'vue'
import type { ComicItem } from '@/types/comic'
import { addComicsToShelf } from '@/stores/bookshelfStore'
import { useUI } from '@/composables/useUI'

export function useShelfQuickAdd(getPageItems: () => ComicItem[]) {
  const { toast } = useUI()

  const selectMode = ref(false)
  const selectedIds = ref<string[]>([])
  const showShelfPicker = ref(false)

  const toggleSelect = (comic: ComicItem) => {
    const idx = selectedIds.value.indexOf(comic.id)
    if (idx >= 0) selectedIds.value.splice(idx, 1)
    else selectedIds.value.push(comic.id)
  }

  /** 长按卡片进入选择模式并选中（仅离线漫画可加入书架） */
  const handleLongPress = (comic: ComicItem) => {
    if (comic.source !== 'offline') return
    selectMode.value = true
    toggleSelect(comic)
  }

  /** 选择模式下点击卡片切换选中（仅离线漫画） */
  const handleSelect = (comic: ComicItem) => {
    if (comic.source !== 'offline') return
    toggleSelect(comic)
  }

  const exitSelectMode = () => {
    selectMode.value = false
    selectedIds.value = []
    showShelfPicker.value = false
  }

  const openShelfPicker = () => {
    if (selectedIds.value.length === 0) return
    showShelfPicker.value = true
  }

  /** 全选 / 取消全选当前页可见项（仅可加入的离线项） */
  const toggleSelectAllPage = () => {
    const pageIds = getPageItems()
      .filter((c) => c.source === 'offline')
      .map((c) => c.id)
    if (pageIds.length === 0) return
    const allSelected = pageIds.every((id) => selectedIds.value.includes(id))
    if (allSelected) {
      selectedIds.value = selectedIds.value.filter((id) => !pageIds.includes(id))
    } else {
      selectedIds.value = Array.from(new Set([...selectedIds.value, ...pageIds]))
    }
  }

  /** 书架浮层 add 回调：批量加入并退出选择模式 */
  const handleAddToShelf = async (shelfId: string) => {
    const ids = [...selectedIds.value]
    showShelfPicker.value = false
    if (ids.length === 0) return
    const { added, skipped } = await addComicsToShelf(shelfId, ids)
    if (added > 0 || skipped > 0) {
      toast.success(`已加入书架 ${added} 本${skipped > 0 ? `（跳过 ${skipped} 本已在书架）` : ''}`)
    } else {
      toast.warning('所选作品均已在该书架中')
    }
    exitSelectMode()
  }

  return {
    selectMode,
    selectedIds,
    showShelfPicker,
    toggleSelect,
    handleLongPress,
    handleSelect,
    toggleSelectAllPage,
    openShelfPicker,
    handleAddToShelf,
    exitSelectMode,
  }
}
