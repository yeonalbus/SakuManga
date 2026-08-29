<script setup lang="ts">
// 搜刮书签创建弹窗（Round27）
// - 展示书签类型（首页/搜索内容）+ 搜索词，输入自定义名称
// - 「选择锚定卡片」→ emit('pick', draft)：由页面进入拾取模式，点击列表卡片完成锚定并创建
// - 「保存」→ emit('create', { name, anchor: null })：跳过锚定，纯位置快照
// 样式对齐 DateJumpModal 的 overlay/panel 暗色主题结构
import { ref, watch } from 'vue'
import type { ScrapeBookmark } from '@/types/comic'

const props = defineProps<{
  show: boolean
  type: 'home' | 'search'
  keyword: string
  defaultName: string
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'pick', draft: { name: string }): void
  (e: 'create', payload: { name: string; anchor: ScrapeBookmark['anchor'] }): void
}>()

const name = ref('')
// 用户手动改过名称后不再被 defaultName 覆盖（拾取取消回表单时保留输入）
let nameDirty: boolean = false

watch(
  () => props.show,
  (v) => {
    if (v) {
      if (!nameDirty) name.value = props.defaultName
    } else {
      nameDirty = false // 下次打开重新按 defaultName 初始化
    }
  },
)

const effectiveName = () => name.value.trim() || props.defaultName

// 用户手动输入标记（模板 @input 调用；避免模板内联赋值触发 vue-tsc 字面量类型误判）
const markNameDirty = () => {
  nameDirty = true
}

const handlePick = () => {
  emit('pick', { name: effectiveName() })
}

const handleCreateWithoutAnchor = () => {
  emit('create', { name: effectiveName(), anchor: null })
}
</script>

<template>
  <Teleport to="body">
    <Transition name="bm-fade">
      <div v-if="show" class="bm-overlay" @click.self="emit('close')">
        <div class="bm-panel">
          <div class="bm-header">
            <span class="bm-title">🔖 存为书签</span>
            <button class="bm-close" title="关闭" @click="emit('close')">✕</button>
          </div>

          <div class="bm-body">
            <!-- 类型与搜索词信息 -->
            <div class="bm-info">
              <span class="bm-type-badge" :class="type">
                {{ type === 'search' ? '🔍 搜索内容' : '🏠 首页' }}
              </span>
              <span v-if="type === 'search' && keyword" class="bm-keyword" :title="keyword">
                {{ keyword }}
              </span>
              <span v-else class="bm-keyword muted">无搜索词（首页流）</span>
            </div>

            <!-- 自定义名称 -->
            <label class="bm-field">
              <span class="bm-label">书签名称</span>
              <input
                v-model="name"
                type="text"
                class="bm-input"
                :placeholder="defaultName"
                maxlength="40"
                @input="markNameDirty"
                @keyup.enter="handlePick"
              />
            </label>
          </div>

          <div class="bm-footer">
            <button class="bm-btn primary" @click="handlePick">🎯 选择锚定卡片</button>
            <button class="bm-btn" title="不锚定卡片，仅保存当前浏览位置" @click="handleCreateWithoutAnchor">
              仅保存位置
            </button>
            <button class="bm-btn ghost" @click="emit('close')">取消</button>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
/* 与 DateJumpModal 一致的暗色主题弹窗骨架 */
.bm-overlay {
  position: fixed;
  inset: 0;
  z-index: 2000;
  background-color: rgba(0, 0, 0, 0.55);
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 16px;
}

.bm-panel {
  width: 100%;
  max-width: 380px;
  background-color: var(--app-surface-2);
  border: 1px solid var(--app-border-3);
  border-radius: 12px;
  box-shadow: 0 12px 32px rgba(0, 0, 0, 0.55);
  overflow: hidden;
}

.bm-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 16px;
  border-bottom: 1px solid var(--app-border-2);
}

.bm-title {
  font-size: 0.95rem;
  font-weight: 600;
  color: var(--app-text-strong);
}

.bm-close {
  background: transparent;
  border: none;
  color: var(--app-text-muted);
  cursor: pointer;
  font-size: 0.85rem;
  padding: 4px;
  border-radius: 6px;
}
.bm-close:hover {
  color: var(--app-text-strong);
  background-color: var(--app-surface-3);
}

.bm-body {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 16px;
}

.bm-info {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.bm-type-badge {
  padding: 2px 8px;
  border-radius: 4px;
  font-size: 0.78rem;
  font-weight: 600;
  color: #ffffff;
  white-space: nowrap;
}
.bm-type-badge.search {
  background-color: #007acc;
}
.bm-type-badge.home {
  background-color: #00a896;
}

.bm-keyword {
  font-size: 0.8rem;
  color: var(--app-text-2);
  max-width: 200px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.bm-keyword.muted {
  color: var(--app-text-muted);
}

.bm-field {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.bm-label {
  font-size: 0.75rem;
  color: var(--app-text-3);
}

.bm-input {
  background-color: var(--app-input-bg);
  border: 1px solid var(--app-border-3);
  border-radius: 8px;
  color: var(--app-text-strong);
  font-size: 0.88rem;
  padding: 8px 10px;
  outline: none;
  transition: border-color 0.15s ease;
}
.bm-input:focus {
  border-color: #00a896;
}

.bm-footer {
  display: flex;
  gap: 8px;
  padding: 12px 16px;
  border-top: 1px solid var(--app-border-2);
}

.bm-btn {
  flex: 1;
  background-color: var(--app-surface-3);
  border: 1px solid var(--app-border-3);
  color: var(--app-text-2);
  border-radius: 8px;
  padding: 8px 10px;
  font-size: 0.8rem;
  cursor: pointer;
  transition: all 0.15s ease;
  white-space: nowrap;
}
.bm-btn:hover {
  background-color: var(--app-surface-3-hover);
  color: var(--app-text-strong);
}
.bm-btn.primary {
  background-color: #00a896;
  border-color: #00a896;
  color: #ffffff;
}
.bm-btn.primary:hover {
  background-color: #00c4af;
  color: #ffffff;
}
.bm-btn.ghost {
  background: transparent;
  border-color: transparent;
}
.bm-btn.ghost:hover {
  background-color: var(--app-surface-3);
}

/* 淡入动画 */
.bm-fade-enter-active,
.bm-fade-leave-active {
  transition: opacity 0.18s ease;
}
.bm-fade-enter-from,
.bm-fade-leave-to {
  opacity: 0;
}
</style>
