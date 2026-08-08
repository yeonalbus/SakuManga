<script setup lang="ts">
// src/components/BatchDownloadBar.vue
// 在线列表页的批量下载工具条：固定悬浮于底部，显示已选数量，一键批量入队（零弹窗、默认方案）
import { ref, computed } from 'vue'
import { batchCreateDownloads, type DownloadTarget } from '@/api/download'
import { useUI } from '@/composables/useUI'
import { isGidDownloading, markGidActive } from '@/stores/downloadTasksStore'
import { useUserStore } from '@/stores/userStore'

const props = withDefaults(
  defineProps<{
    /** 已选中的待下载画廊（gid + token 即可入队） */
    selected: DownloadTarget[]
    /** 可选：当前列表可用总数（如收藏页「一键全部下载」场景展示） */
    total?: number
  }>(),
  {
    total: 0,
  },
)

const emit = defineEmits<{
  (e: 'close'): void
  /** 全选 / 取消全选本页（由父页面根据当前可见列表实现） */
  (e: 'select-all'): void
}>()

const { toast } = useUI()
const userStore = useUserStore()
// 下载权限：管理员或有 allowDownload 许可才展示批量下载入口（中心制：无许可用户不展示下载能力）
const canDownload = computed(() => userStore.isAdmin || !!userStore.user?.allowDownload)
const isBatchDownloading = ref(false)

const count = computed(() => props.selected.length)

const handleBatchDownload = async () => {
  if (isBatchDownloading.value) return
  if (props.selected.length === 0) {
    toast.warning('请先选择要下载的作品')
    return
  }
  // 预过滤去重：已下载到本地 / 已在下载队列中的条目不再提交后端
  const downloadedCount = props.selected.filter((t) => t.isDownloaded).length
  const downloadingCount = props.selected.filter(
    (t) => !t.isDownloaded && isGidDownloading(t.gid),
  ).length
  const toSubmit = props.selected.filter((t) => !t.isDownloaded && !isGidDownloading(t.gid))

  if (toSubmit.length === 0) {
    const parts: string[] = []
    if (downloadedCount > 0) parts.push(`已下载 ${downloadedCount} 部`)
    if (downloadingCount > 0) parts.push(`下载中 ${downloadingCount} 部`)
    toast.info(`所选作品无需下载：${parts.join('、')}`)
    emit('close')
    return
  }

  isBatchDownloading.value = true
  try {
    const res = await batchCreateDownloads(toSubmit)
    if (res.created > 0) {
      toSubmit.forEach((t) => markGidActive(t.gid))
    }
    // 汇总提示：新增 + 本轮预过滤跳过 + 后端兜底跳过
    const skipParts: string[] = []
    if (downloadedCount > 0) skipParts.push(`已下载 ${downloadedCount} 部`)
    if (downloadingCount > 0) skipParts.push(`下载中 ${downloadingCount} 部`)
    if (res.skipped > 0) skipParts.push(`任务已存在 ${res.skipped} 部`)
    if (res.failed > 0) {
      toast.error(`批量加入失败 ${res.failed} 部：${(res.errors || []).join('；')}`)
    } else if (skipParts.length > 0) {
      toast.success(`成功加入 ${res.created} 部，跳过：${skipParts.join('、')}`)
    } else {
      toast.success(`已加入 ${res.created} 部到下载队列`)
    }
    emit('close')
  } catch (err) {
    toast.error(`批量加入下载队列失败：${(err as Error)?.message || '未知错误'}`)
  } finally {
    isBatchDownloading.value = false
  }
}
</script>

<template>
  <div v-if="canDownload" class="batch-download-bar">
    <span class="bar-count">已选 {{ count }} 部</span>
    <button class="bar-btn" :disabled="isBatchDownloading" @click="emit('select-all')">
      全选本页
    </button>
    <button
      class="bar-btn primary"
      :disabled="isBatchDownloading || count === 0"
      @click="handleBatchDownload"
    >
      {{ isBatchDownloading ? '加入中…' : '⬇ 批量下载' }}
    </button>
    <button class="bar-btn" :disabled="isBatchDownloading" @click="emit('close')">取消</button>
  </div>
</template>

<style scoped>
.batch-download-bar {
  position: fixed;
  left: 50%;
  bottom: 24px;
  transform: translateX(-50%);
  z-index: 999;
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 16px;
  border-radius: 12px;
  background-color: var(--app-surface-2);
  border: 1px solid var(--app-border-3);
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.45);
}

.bar-count {
  font-size: 13px;
  font-weight: 600;
  color: var(--app-text-strong);
  white-space: nowrap;
}

.bar-btn {
  padding: 7px 16px;
  border: 1px solid var(--app-border-3);
  border-radius: 8px;
  background-color: transparent;
  color: var(--app-text-2);
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.15s ease;
}

.bar-btn:hover:not(:disabled) {
  border-color: var(--app-border-strong);
  color: var(--app-text-strong);
}

.bar-btn.primary {
  background-color: #ff7588;
  border-color: #ff7588;
  color: #ffffff;
}

.bar-btn.primary:hover:not(:disabled) {
  background-color: #ff5f74;
  border-color: #ff5f74;
}

.bar-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
</style>
