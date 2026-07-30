import { defineStore } from 'pinia'
import { ref } from 'vue'

export interface TagItem {
  namespace: string
  key: string
  name: string
  intro?: string
}

export const useTagStore = defineStore('tag', () => {
  // key 格式: "female:cat ears" -> TagItem
  const tagMap = ref<Map<string, TagItem>>(new Map())
  const isLoaded = ref(false)

  // 初始化时从后端拉取完整翻译字典
  const fetchTagDictionary = async () => {
    try {
      const res = await fetch('/api/v1/tags/dictionary')
      if (res.ok) {
        const data: TagItem[] = await res.json()
        const map = new Map<string, TagItem>()
        data.forEach((item) => {
          const ns = (item.namespace || 'other').toLowerCase()
          const k = item.key.toLowerCase().replace(/_/g, ' ')
          map.set(`${ns}:${k}`, item)
        })
        tagMap.value = map
        isLoaded.value = true
      }
    } catch (err) {
      console.error('加载标签字典失败:', err)
    }
  }

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
    fetchTagDictionary,
    translate,
  }
})
