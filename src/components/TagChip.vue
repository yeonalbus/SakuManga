<script setup lang="ts">
import { computed } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useTagStore } from '@/stores/tagStore'
import { useModeStore } from '@/stores/modeStore'
// 🎯 引入全局 SearchConfig Store 状态
import { offlineSearchConfig } from '@/stores/searchStore'
// 🎯 f_search 标准语法格式化：在线按 E-Hentai 规范（namespace:"key$"），离线保持裸格式
import { formatFSearchTag } from '@/utils/tagFilter'

export interface TagData {
  namespace: string
  key: string
  name: string
}

const props = withDefaults(
  defineProps<{
    tag: TagData | string
    showTranslation?: boolean
    /** 禁用点击快捷搜索（供搜索联想下拉使用）：不拦截冒泡，把点击交给父级把 tag 插入输入框 */
    disableQuickSearch?: boolean
  }>(),
  {
    showTranslation: true,
    disableQuickSearch: false,
  },
)

const router = useRouter()
const route = useRoute()
const tagStore = useTagStore()
const modeStore = useModeStore()

// 统一转换为 TagData 结构
// 防御（问题8）：后端数据异常（如 name/key 非字符串或为 null）时强制归一化，
// 避免后续 .replace() 在非字符串上抛错，导致联想渲染整棵卸载白屏。
const tagData = computed<TagData>(() => {
  let base: TagData
  if (typeof props.tag === 'string') {
    // bug1 防御：若字符串含 EH 翻译库的 markdown 图标语法（如 ![](url)名称），
    // 先剥离图标再交给 translate，避免 translate 把 URL 中的冒号误判为 namespace 分隔符，
    // 也避免整段 markdown 被当作原文显示。
    const raw = props.tag
    let cleaned = ''
    if (/!\[.*?\]\(.*?\)/.test(raw)) {
      cleaned = raw
        .replace(/!\[.*?\]\(.*?\)/g, '')
        .replace(/!\[[^\]]*\]/g, '')
        .trim()
      if (!cleaned) {
        const alt = raw.match(/!\[(.*?)\]\(.*?\)/)
        if (alt && alt[1]) cleaned = alt[1].trim()
      }
    }
    base = tagStore.translate(cleaned || raw)
  } else if (props.tag && typeof props.tag === 'object') {
    base = props.tag as TagData
  } else {
    base = { namespace: 'other', key: '', name: '' }
  }
  return {
    namespace: typeof base.namespace === 'string' ? base.namespace : 'other',
    key: typeof base.key === 'string' ? base.key : '',
    name: typeof base.name === 'string' ? base.name : '',
  }
})

// 计算展示文案（含 Markdown 图标语法清洗）
const displayName = computed(() => {
  const { key, name } = tagData.value
  const cleanKey = key ? key.replace(/_/g, ' ') : ''

  if (props.showTranslation && name && name !== key) {
    let cleanName = name
      .replace(/!\[.*?\]\(.*?\)/g, '')
      .replace(/!\[[^\]]*\]/g, '')
      .trim()
    if (!cleanName) {
      const altMatch = name.match(/!\[(.*?)\]\(.*?\)/)
      if (altMatch && altMatch[1]) {
        cleanName = altMatch[1].trim()
      }
    }
    return cleanName || cleanKey
  }
  return cleanKey
})

// E 站分类调色盘
const getBgColor = (ns: string) => {
  switch ((ns || '').toLowerCase()) {
    case 'artist':
      return '#c62828'
    case 'group':
      return '#ad1457'
    case 'character':
      return '#2e7d32'
    case 'parody':
      return '#00838f'
    case 'female':
      return '#ad1457'
    case 'male':
      return '#1565c0'
    case 'reclass':
      return '#4527a0'
    case 'language':
      return '#424242'
    default:
      return '#37474f'
  }
}

// 🎯 点击 Tag 快捷搜索：根据所在域更新 Store 并回归列表页。
// 搜索联想下拉里（disableQuickSearch=true）需禁用：
// 原实现 @click.stop 会在点击联想项（即 TagChip）时拦截冒泡并触发快捷搜索（在线新开标签/离线跳转），
// 既阻止了 tag 被插入输入框，又导致"点击联想自动跳转搜索页"，无法连续输入多 tag。
const onClick = (e: Event) => {
  if (props.disableQuickSearch) return // 不 stopPropagation，冒泡交给父级（联想项）插入输入框
  e.stopPropagation()
  handleClick()
}

const handleClick = () => {
  const { namespace, key } = tagData.value
  const isOffline = modeStore.isOffline
  // 在线输出 E-Hentai f_search 标准语法（多词加引号与 $ 锚定），离线保持裸 namespace:key
  const queryTag = formatFSearchTag(namespace, key, isOffline)

  if (isOffline) {
    offlineSearchConfig.value.keyword = queryTag
    if (!route.path.startsWith('/offline/home')) {
      router.push('/offline/home')
    }
  } else {
    // 🆕 在线：用真实浏览器新标签打开搜索页（URL 携带关键词），
    // 原标签页的列表位点/详情面板/滚动位置完全不受影响（web 原生多标签优势）
    const url = router.resolve({ path: '/online/home', query: { kw: queryTag } }).href
    window.open(url, '_blank')
  }
}
</script>

<template>
  <span
    class="tag-chip"
    :style="{ backgroundColor: getBgColor(tagData.namespace) }"
    @click="onClick"
  >
    <span v-if="tagData.namespace && tagData.namespace !== 'other'" class="tag-namespace">
      {{ tagData.namespace }}:
    </span>
    <span class="tag-name">
      {{ displayName }}
    </span>
  </span>
</template>

<style scoped>
.tag-chip {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  padding: 2px 8px;
  border-radius: 4px;
  font-size: 0.75rem;
  color: #ffffff;
  font-weight: 500;
  cursor: pointer;
  user-select: none;
  transition:
    opacity 0.15s,
    transform 0.15s;
  line-height: 1.3;
}

.tag-chip:hover {
  opacity: 0.85;
  transform: translateY(-1px);
}

.tag-namespace {
  opacity: 0.75;
  font-size: 0.7rem;
}

.tag-name {
  font-weight: 600;
}
</style>
