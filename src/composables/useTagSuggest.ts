import { ref, watch, onUnmounted } from 'vue'
import { http } from '@/utils/request'
import type { TagItem } from '@/stores/tagStore'

export interface TagSuggestion extends TagItem {
  /** 是否为负向联想（输入以「- 」开头时结果标记为负向） */
  isNegative: boolean
  /** 完整展示用 key：带命名空间为 namespace:key，否则仅 key */
  displayKey: string
  /** 压入队列的文本：负向项带「- 」前缀 */
  insertText: string
}

/**
 * Round3-任务5：tag 联想 composable
 * - 复用后端 /tags/suggest 热度联想接口（与 SearchBar 一致）
 * - 支持负向前缀「- 」：输入以「- 」开头时，查询自动去掉前缀，结果标记为负向
 * - 统一清洗 / 截断 / 限制条数，避免异常条目导致渲染崩溃（对齐 SearchBar.safeSuggestedTags 的防御逻辑）
 *
 * @param source 输入源 getter（如 () => inputKeyword.value），内部 watch 其变化并防抖
 * @param limit 联想结果上限
 * @param debounce 防抖毫秒数
 */
export function useTagSuggest(source: () => string, limit = 8, debounce = 150) {
  const suggestions = ref<TagSuggestion[]>([])
  const loading = ref(false)
  let timer: number | null = null

  // 判断是否负向输入：「- 」前缀（- 后紧跟非空白字符）
  const isNegativeInput = (raw: string) => /^-\s*\S/.test(raw)

  const sanitize = (data: TagItem[], isNegative: boolean): TagSuggestion[] => {
    if (!Array.isArray(data)) return []
    return data
      .filter((t): t is TagItem => !!t && typeof t === 'object')
      .map((t) => {
        const namespace = typeof t.namespace === 'string' ? t.namespace : 'other'
        const key = typeof t.key === 'string' ? t.key : ''
        const hasNs = namespace !== 'other' && namespace !== ''
        const displayKey = hasNs ? `${namespace}:${key}` : key
        return {
          namespace,
          key,
          name: typeof t.name === 'string' ? t.name : '',
          intro: typeof t.intro === 'string' ? t.intro : undefined,
          count: typeof t.count === 'number' && Number.isFinite(t.count) ? t.count : 0,
          isNegative,
          displayKey,
          insertText: `${isNegative ? '- ' : ''}${displayKey}`,
        }
      })
      .filter((t) => t.key !== '')
      .slice(0, limit)
  }

  const fetchSuggestions = async () => {
    const raw = source().trim()
    const isNegative = isNegativeInput(raw)
    const q = isNegative ? raw.replace(/^-\s*/, '').trim() : raw
    if (!q) {
      suggestions.value = []
      loading.value = false
      return
    }
    loading.value = true
    try {
      const data = await http<TagItem[]>('/tags/suggest', { params: { q, limit } })
      suggestions.value = sanitize(data, isNegative)
    } catch (e) {
      console.error('获取标签联想失败:', e)
      suggestions.value = []
    } finally {
      loading.value = false
    }
  }

  watch(source, () => {
    if (timer) clearTimeout(timer)
    timer = window.setTimeout(fetchSuggestions, debounce)
  })

  /** 立即触发一次联想（供打开面板 / 输入非空时主动调用） */
  const refresh = () => {
    if (timer) clearTimeout(timer)
    fetchSuggestions()
  }

  /** 清空联想结果（提交 / 失焦 / 关闭时调用） */
  const clear = () => {
    if (timer) clearTimeout(timer)
    suggestions.value = []
    loading.value = false
  }

  onUnmounted(() => {
    if (timer) clearTimeout(timer)
  })

  return { suggestions, loading, refresh, clear }
}
