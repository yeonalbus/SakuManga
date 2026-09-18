import { defineStore } from 'pinia'
import { ref } from 'vue'
import { http } from '@/utils/request'

export interface TagItem {
  namespace: string
  key: string
  name: string
  intro?: string
  count?: number
}

/**
 * 词典加载重试退避（毫秒）：
 * 词典是「全站标签汉化」的唯一数据源，加载失败会导致所有标签静默退化为英文原文
 * （TagChip 查不到中文名就显示 key），因此失败必须自动重试而不是一次性放弃。
 */
const RETRY_DELAYS_MS = [2000, 5000, 10000]

export const useTagStore = defineStore('tag', () => {
  // key 格式: "female:cat ears" -> TagItem
  const tagMap = ref<Map<string, TagItem>>(new Map())
  const isLoaded = ref(false)
  /** 是否正在加载（并发去重：多处同时触发只发一次请求） */
  const isLoading = ref(false)
  /** 加载失败标记：供 UI 提示（此前失败只打 console，用户完全无感） */
  const loadFailed = ref(false)
  /** 最近一次失败原因（诊断用） */
  const lastError = ref('')

  let inflight: Promise<boolean> | null = null
  let retryTimer: number | null = null
  let retryAttempt = 0
  /** 词典代次：reset 后自增，用于丢弃「清空之前发出的」过期响应（防止 switch 关闭后又把中文塞回来） */
  let generation = 0

  const applyDictionary = (data: TagItem[]) => {
    const map = new Map<string, TagItem>()
    data.forEach((item) => {
      const ns = (item.namespace || 'other').toLowerCase()
      const k = item.key.toLowerCase().replace(/_/g, ' ')
      map.set(`${ns}:${k}`, item)
    })
    tagMap.value = map
    isLoaded.value = true
    loadFailed.value = false
    lastError.value = ''
  }

  /** 取消排队中的重试（加载成功后调用，避免多余的重复请求） */
  const clearRetry = () => {
    if (retryTimer !== null) {
      window.clearTimeout(retryTimer)
      retryTimer = null
    }
    retryAttempt = 0
  }

  /**
   * 拉取标签词典（并发去重 + 失败退避重试）。
   *
   * 关键修复（标签中文化失效根因）：本方法在【已登录】时才会真正请求。
   * /tags/dictionary 是鉴权接口，登录页 boot 阶段无 token 必然 401；
   * 此前 App 挂载即无条件请求且失败不重试，导致「在登录页输入账密登录」这条路径下
   * 整个会话的词典恒为空、标签全部显示英文，只有整页刷新才恢复。
   *
   * @param force       强制重新请求（如开关切换后需要拿新词典）
   * @param isRetryCall 内部标记：退避重试自身发起的调用（不重置退避计数，避免无限重试）
   * @returns 是否加载成功（永不 reject，失败返回 false 并已安排重试）
   */
  const ensureDictionary = async (force = false, isRetryCall = false): Promise<boolean> => {
    if (isLoaded.value && !force) return true
    if (inflight) return inflight

    // 登录态守卫：无 token 不发请求（避免登录页 401 噪声，也让调用方可安全早调）
    const token = localStorage.getItem('saku_token')
    if (!token) return false

    // 外部（人工/登录触发）调用：重置退避计数，保证每次新尝试都有完整的重试预算
    if (!isRetryCall) retryAttempt = 0

    loadFailed.value = false
    isLoading.value = true
    const gen = generation
    inflight = (async () => {
      try {
        const data = await http<TagItem[]>('/tags/dictionary')
        if (!Array.isArray(data) || data.length === 0) {
          throw new Error('词典响应为空')
        }
        // 过期响应（期间已登出 / 已关闭中文翻译）：丢弃，不得写回词典
        if (gen !== generation) return false
        applyDictionary(data)
        clearRetry()
        return true
      } catch (err) {
        if (gen !== generation) return false // 已 reset：不再报错/重试
        const msg = err instanceof Error ? err.message : String(err)
        loadFailed.value = true
        lastError.value = msg
        console.error('加载标签字典失败:', msg)
        // 退避重试：网络抖动 / 服务端瞬时 401 时不至于整站标签一直是英文
        if (retryAttempt < RETRY_DELAYS_MS.length) {
          const delay = RETRY_DELAYS_MS[retryAttempt]
          retryAttempt += 1
          if (retryTimer !== null) window.clearTimeout(retryTimer)
          retryTimer = window.setTimeout(() => {
            retryTimer = null
            void ensureDictionary(true, true)
          }, delay)
        }
        return false
      } finally {
        if (gen === generation) {
          isLoading.value = false
          inflight = null
        }
      }
    })()

    return inflight
  }

  /**
   * 清空词典（中文翻译开关关闭 / 登出时调用）。
   * 清空后 TagChip 查不到中文名 → 立即按英文原文渲染，无需刷新页面。
   */
  const resetDictionary = () => {
    clearRetry()
    generation += 1 // 使在途请求的响应作废
    tagMap.value = new Map()
    isLoaded.value = false
    loadFailed.value = false
    lastError.value = ''
    inflight = null
    isLoading.value = false
  }

  // 兼容旧调用名（App.vue 启动加载）
  const fetchTagDictionary = () => ensureDictionary()

  // 常见命名空间降级搜索顺序
  const fallbackNamespaces = [
    'female',
    'male',
    'character',
    'artist',
    'parody',
    'group',
    'mixed',
    'language',
    'reclass',
    'other',
  ]

  // 输入原始字符串（如 "female:twintails" 或 "cat_ears"），查表返回结构化数据
  const translate = (rawTag: string): TagItem => {
    if (!rawTag) return { namespace: 'other', key: '', name: '' }

    const raw = rawTag.trim()
    const colonIndex = raw.indexOf(':')

    let ns = 'other'
    let key = raw

    if (colonIndex !== -1) {
      ns = raw.slice(0, colonIndex).trim().toLowerCase()
      key = raw.slice(colonIndex + 1).trim()
    }

    const keyClean = key.toLowerCase().replace(/_/g, ' ')

    // 1. 带 namespace 查找
    if (colonIndex !== -1) {
      const lookupKey = `${ns}:${keyClean}`
      if (tagMap.value.has(lookupKey)) {
        const item = tagMap.value.get(lookupKey)!
        return { namespace: item.namespace, key, name: item.name || key }
      }
    } else {
      // 2. 不带 namespace 时，依次搜寻常用分类
      for (const tryNS of fallbackNamespaces) {
        const lookupKey = `${tryNS}:${keyClean}`
        if (tagMap.value.has(lookupKey)) {
          const item = tagMap.value.get(lookupKey)!
          return { namespace: item.namespace, key, name: item.name || key }
        }
      }
    }

    // 未查到翻译时返回原名
    return { namespace: ns, key, name: '' }
  }

  return {
    tagMap,
    isLoaded,
    isLoading,
    loadFailed,
    lastError,
    fetchTagDictionary,
    ensureDictionary,
    resetDictionary,
    translate,
  }
})
