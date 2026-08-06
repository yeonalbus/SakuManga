<script setup lang="ts">
import { computed } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useTagStore } from '@/stores/tagStore'
import { useModeStore } from '@/stores/modeStore'
// 🎯 引入全局 SearchConfig Store 状态
import { onlineSearchConfig, offlineSearchConfig } from '@/stores/searchStore'

export interface TagData {
  namespace: string
  key: string
  name: string
}

const props = withDefaults(
  defineProps<{
    tag: TagData | string
    showTranslation?: boolean
  }>(),
  {
    showTranslation: true,
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
    base = tagStore.translate(props.tag)
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
    let cleanName = name.replace(/!\[.*?\]\(.*?\)/g, '').trim()
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

// 🎯 点击 Tag 快捷搜索：根据所在域更新 Store 并回归列表页
const handleClick = () => {
  const { namespace, key } = tagData.value
  const queryTag = namespace && namespace !== 'other' ? `${namespace}:${key}` : key

  const isOffline = modeStore.isOffline

  if (isOffline) {
    offlineSearchConfig.value.keyword = queryTag
    if (!route.path.startsWith('/offline/home')) {
      router.push('/offline/home')
    }
  } else {
    onlineSearchConfig.value.keyword = queryTag
    if (!route.path.startsWith('/online/home')) {
      router.push('/online/home')
    }
  }
}
</script>

<template>
  <span
    class="tag-chip"
    :style="{ backgroundColor: getBgColor(tagData.namespace) }"
    @click.stop="handleClick"
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
