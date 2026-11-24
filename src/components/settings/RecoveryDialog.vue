<script setup lang="ts">
/**
 * Round54：恢复下载弹窗（档位三选一）。
 * 用户点「恢复」后弹出，选择 H@H 原图 / H@H 压缩 / 画廊逐图 后开始批量入队。
 */
import { ref } from 'vue'
import type { RecoveryScheme } from '@/stores/scanPathStore'

defineProps<{
  open: boolean
  candidateCount: number
  busy: boolean
}>()

const emit = defineEmits<{
  (e: 'confirm', scheme: RecoveryScheme): void
  (e: 'cancel'): void
}>()

const scheme = ref<RecoveryScheme>('archive-original')

const schemes: Array<{ value: RecoveryScheme; label: string; hint: string }> = [
  {
    value: 'archive-original',
    label: 'H@H 原图',
    hint: '归档原图（画质最高，走 H@H 归档通道，会消耗 GP 配额）',
  },
  {
    value: 'archive-resample',
    label: 'H@H 压缩',
    hint: '归档压缩图（体积更小，走 H@H 归档通道，消耗较少 GP）',
  },
  {
    value: 'gallery',
    label: '画廊逐图',
    hint: '画廊逐图下载（不消耗 GP，但速度较慢、请求次数多）',
  },
]
</script>

<template>
  <Teleport to="body">
    <div v-if="open" class="mask" @click.self="emit('cancel')">
      <div class="dialog">
        <div class="title">恢复下载</div>
        <div class="desc">
          将对 <b>{{ candidateCount }}</b> 个可恢复的本子创建下载任务，恢复到 SakuManga 下载路径。
          下载期间该路径禁止扫描。
        </div>

        <div class="scheme-list">
          <label
            v-for="s in schemes"
            :key="s.value"
            class="scheme"
            :class="{ active: scheme === s.value }"
          >
            <input v-model="scheme" type="radio" :value="s.value" :disabled="busy" />
            <div class="scheme-text">
              <div class="scheme-name">{{ s.label }}</div>
              <div class="scheme-hint">{{ s.hint }}</div>
            </div>
          </label>
        </div>

        <div class="actions">
          <button class="btn" :disabled="busy" @click="emit('cancel')">取消</button>
          <button class="btn primary" :disabled="busy" @click="emit('confirm', scheme)">
            {{ busy ? '启动中…' : '开始恢复' }}
          </button>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
.mask {
  position: fixed;
  inset: 0;
  z-index: 1000;
  background: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
}

.dialog {
  width: min(520px, calc(100vw - 32px));
  background: var(--app-surface-2);
  border: 1px solid var(--app-border-2);
  border-radius: 10px;
  padding: 20px;
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.title {
  font-size: 16px;
  font-weight: 600;
  color: var(--app-text-strong);
}

.desc {
  font-size: 13px;
  color: var(--app-text-2);
  line-height: 1.6;
}

.desc b {
  color: #ff7588;
}

.scheme-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.scheme {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 10px 12px;
  border: 1px solid var(--app-border-2);
  border-radius: 8px;
  cursor: pointer;
  transition: border-color 0.2s;
}

.scheme.active {
  border-color: #ff7588;
}

.scheme input {
  margin-top: 3px;
  accent-color: #ff7588;
}

.scheme-text {
  display: flex;
  flex-direction: column;
  gap: 3px;
}

.scheme-name {
  font-size: 13px;
  font-weight: 600;
  color: var(--app-text-strong);
}

.scheme-hint {
  font-size: 12px;
  color: var(--app-text-2);
}

.actions {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
}

.btn {
  padding: 8px 16px;
  border-radius: 6px;
  border: 1px solid var(--app-border-3);
  background: var(--app-input-bg);
  color: var(--app-text-strong);
  font-size: 13px;
  cursor: pointer;
}

.btn.primary {
  border-color: #ff7588;
  background: #ff7588;
  color: #fff;
}

.btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
</style>
