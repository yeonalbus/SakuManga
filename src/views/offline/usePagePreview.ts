import { computed, reactive, ref } from 'vue'

/**
 * Round52：维护页「展开对比」的页预览状态（模块级单例，跨卡共享）。
 *
 * 切换规则（珱垣定稿）：
 * - 每本记住最后查看页（同本翻页只更新最后一次）；
 * - 在「最近打开过的两个本」之间来回切，各回各的页；
 * - 另一本从未打开过 → 切换按钮置灰；
 * - 三本以上只保留最近两个（A P1 → C P5 → B P6，切换＝C P5，再切＝B P6）；
 * - 禁止按页码对齐跳转。
 */

export interface PreviewTarget {
  label: string // 成员标签（A / B / C…）
  title: string
  pageCount: number
}

const visible = ref(false)
const current = reactive({ id: '', page: 1 })
const targets = reactive<Record<string, PreviewTarget>>({})
const lastPage = reactive<Record<string, number>>({})
const recent = ref<string[]>([])

const currentTarget = computed(() => targets[current.id])
const swapTargetId = computed(() => recent.value.find((id) => id !== current.id) ?? '')
const swapTarget = computed(() => (swapTargetId.value ? targets[swapTargetId.value] : undefined))

function touch(id: string, page: number) {
  lastPage[id] = page
  recent.value = [id, ...recent.value.filter((x) => x !== id)]
}

export function usePagePreview() {
  const openPreview = (id: string, page: number, target: PreviewTarget) => {
    targets[id] = target
    current.id = id
    current.page = page
    touch(id, page)
    visible.value = true
  }

  const closePreview = () => {
    visible.value = false
  }

  const movePreview = (delta: number) => {
    const max = targets[current.id]?.pageCount ?? 1
    const next = Math.min(max, Math.max(1, current.page + delta))
    current.page = next
    touch(current.id, next)
  }

  const swapPreview = () => {
    const id = swapTargetId.value
    if (!id) return
    current.id = id
    current.page = lastPage[id] ?? 1
    touch(id, current.page)
  }

  return {
    visible,
    current,
    targets,
    currentTarget,
    swapTargetId,
    swapTarget,
    lastPage,
    openPreview,
    closePreview,
    movePreview,
    swapPreview,
  }
}
