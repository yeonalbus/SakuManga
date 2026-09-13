<script setup lang="ts">
/**
 * Round38-R5：书架封面选择浮层
 *
 * 展示架内全部本子的缩略图，点选即设为该书架封面；支持「恢复自动封面」
 * （自动封面 = 架内展示顺序第一本，由后端解析 coverUrl 返回）。
 *
 * 缩略图直接用本子自身的 coverUrl（`/api/v1/comics/<id>/cover`），无需额外接口。
 */
import { computed } from 'vue'
import { useUI } from '@/composables/useUI'
import { bookshelves, setBookshelfCover } from '@/stores/bookshelfStore'
import { shelfComicsOf } from '@/composables/useShelfStats'

const props = defineProps<{
  open: boolean
  shelfId: string
}>()

const emit = defineEmits<{ (e: 'close'): void }>()

const { toast } = useUI()

const shelf = computed(() => bookshelves.value.find((b) => b.id === props.shelfId))
/** 架内仍存在的本子（按书架展示顺序） */
const comics = computed(() => shelfComicsOf(shelf.value))
const currentCoverId = computed(() => shelf.value?.coverComicId || '')

const close = () => emit('close')

const pick = async (comicId: string) => {
  const target = shelf.value
  if (!target) return
  if (comicId === currentCoverId.value) {
    close()
    return
  }
  await setBookshelfCover(target.id, comicId)
  toast.success(`「${target.name}」封面已更新`)
  close()
}

const resetAuto = async () => {
  const target = shelf.value
  if (!target || !currentCoverId.value) return
  await setBookshelfCover(target.id, '')
  toast.success(`「${target.name}」已恢复自动封面`)
  close()
}

/** 缩略图加载失败（封面缓存未生成）时收起图片，露出中性底块 */
const onImgError = (e: Event) => {
  const img = e.target as HTMLImageElement | null
  if (img) img.style.visibility = 'hidden'
}
</script>

<template>
  <Teleport to="body">
    <Transition name="fade">
      <div v-if="open" class="cover-mask" @click.self="close">
        <div class="cover-panel">
          <div class="cover-header">
            <h3 class="cover-title">🖼 设置封面 · {{ shelf?.name || '' }}</h3>
            <button class="cover-close" @click="close">✕</button>
          </div>

          <p class="cover-hint">
            点选架内任一本作品作为书架封面；未手动设置时自动使用架内第一本。
          </p>

          <div class="cover-grid">
            <div v-if="comics.length === 0" class="cover-empty">该书架暂无可选作品</div>
            <button
              v-for="c in comics"
              :key="c.id"
              class="cover-item"
              :class="{ current: c.id === currentCoverId }"
              :title="c.title"
              @click="pick(c.id)"
            >
              <img :src="c.coverUrl || ''" :alt="c.title" loading="lazy" @error="onImgError" />
              <span v-if="c.id === currentCoverId" class="current-badge">✓ 当前封面</span>
            </button>
          </div>

          <div class="cover-footer">
            <button class="reset-btn" :disabled="!currentCoverId" @click="resetAuto">
              ↺ 恢复自动封面
            </button>
            <button class="close-btn" @click="close">关闭</button>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.cover-mask {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.55);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
  backdrop-filter: blur(2px);
}
.cover-panel {
  width: 560px;
  max-width: 94vw;
  max-height: 84vh;
  display: flex;
  flex-direction: column;
  background: var(--app-surface-2);
  border: 1px solid var(--app-border-2);
  border-radius: 12px;
  box-shadow: 0 12px 40px rgba(0, 0, 0, 0.6);
  overflow: hidden;
}
.cover-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 16px 8px;
}
.cover-title {
  font-size: 0.98rem;
  font-weight: 600;
  color: var(--app-text-strong);
  margin: 0;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.cover-close {
  background: transparent;
  border: none;
  color: var(--app-text-3);
  font-size: 1rem;
  cursor: pointer;
  padding: 2px 6px;
}
.cover-close:hover {
  color: var(--app-text-strong);
}
.cover-hint {
  margin: 0;
  padding: 0 16px 10px;
  font-size: 0.78rem;
  color: var(--app-text-3);
  line-height: 1.5;
  border-bottom: 1px solid var(--app-border-2);
}
.cover-grid {
  flex: 1;
  overflow-y: auto;
  padding: 12px 16px;
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(84px, 1fr));
  gap: 10px;
}
.cover-empty {
  grid-column: 1 / -1;
  text-align: center;
  color: var(--app-text-3);
  font-size: 0.85rem;
  padding: 24px 0;
}
.cover-item {
  position: relative;
  padding: 0;
  border: 2px solid transparent;
  border-radius: 6px;
  overflow: hidden;
  cursor: pointer;
  background: var(--app-surface-3);
  aspect-ratio: 3 / 4;
  transition:
    border-color 0.15s,
    transform 0.15s;
}
.cover-item:hover {
  border-color: #007acc;
  transform: translateY(-2px);
}
.cover-item.current {
  border-color: #4cc38a;
}
.cover-item img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
}
.current-badge {
  position: absolute;
  left: 0;
  right: 0;
  bottom: 0;
  font-size: 0.68rem;
  color: #fff;
  background: rgba(76, 195, 138, 0.85);
  padding: 2px 0;
  text-align: center;
}
.cover-footer {
  display: flex;
  justify-content: space-between;
  gap: 10px;
  padding: 12px 16px;
  border-top: 1px solid var(--app-border-2);
}
.reset-btn {
  background: transparent;
  border: 1px dashed var(--app-border-3);
  color: var(--app-text-2);
  padding: 7px 14px;
  border-radius: 6px;
  cursor: pointer;
  font-size: 0.82rem;
  transition: all 0.15s;
}
.reset-btn:hover:not(:disabled) {
  border-color: #007acc;
  color: #007acc;
}
.reset-btn:disabled {
  opacity: 0.4;
  cursor: default;
}
.close-btn {
  background: var(--app-surface-3);
  border: 1px solid var(--app-border-3);
  color: var(--app-text-2);
  padding: 7px 18px;
  border-radius: 6px;
  cursor: pointer;
  font-size: 0.82rem;
}
.close-btn:hover {
  color: var(--app-text-strong);
}
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.18s;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
