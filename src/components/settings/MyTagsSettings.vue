<template>
  <div class="my-tags-settings">
    <div class="sub-header">
      <button class="back-btn" title="返回" @click="$emit('back')">←</button>
      <h3 class="sub-title">我的标签</h3>
      <button class="refresh-btn" title="从 E 站重新读取" :disabled="loading" @click="load">
        {{ loading ? '读取中…' : '↻ 刷新' }}
      </button>
    </div>

    <p class="desc">
      以下标签直接读取自 E 站「我的标签」页面。关注标签（Watch）用于标记兴趣标签，
      隐藏标签（Hide）用于过滤不感兴趣的内容。新增/移除操作会实时上传至 E 站。
    </p>

    <!-- Tagset 管理：切换当前 Tagset + 新建 Tagset -->
    <section class="tag-section">
      <div class="section-header">
        <span class="section-title">Tagset 管理</span>
      </div>

      <div class="tagset-row">
        <label class="tagset-label" for="tagset-select">当前 Tagset：</label>
        <select
          id="tagset-select"
          v-model.number="currentTagset"
          class="tagset-select"
          :disabled="loading || tagsetList.length === 0"
          @change="switchTagset"
        >
          <option v-for="ts in tagsetList" :key="ts.id" :value="ts.id">
            {{ ts.name }}（{{ ts.count }}）
          </option>
        </select>
        <span class="tagset-hint">切换后重新读取该 Tagset 下的标签</span>
      </div>

      <div class="add-row">
        <input
          v-model="tagsetInput"
          class="tag-input"
          placeholder="输入新 Tagset 名称，如 Tagset #2"
          @keyup.enter="createTagset"
        />
        <button
          class="action-btn primary"
          :disabled="!!submitting || creatingTagset || !tagsetInput.trim()"
          @click="createTagset"
        >
          {{ creatingTagset ? '创建中…' : '新建 Tagset' }}
        </button>
      </div>
    </section>

    <div v-if="!isLoggedIn" class="login-hint">
      请先在「账户设置」中绑定并保存 E 站账户凭证后再管理标签。
    </div>

    <!-- 关注的标签 -->
    <section class="tag-section">
      <div class="section-header">
        <span class="section-title">关注的标签</span>
        <span class="section-count">{{ watched.length }}</span>
      </div>

      <div v-if="watched.length" class="tag-list">
        <span
          v-for="tag in watched"
          :key="tag"
          class="tag-chip watch-chip"
          :class="{ removing: removingTag === `${actionWatch}:${tag}` }"
        >
          {{ tag }}
          <button
            class="chip-remove"
            title="取消关注"
            :disabled="!!submitting"
            @click="removeTag(actionWatch, tag)"
          >
            ✕
          </button>
        </span>
      </div>
      <div v-else-if="!loading" class="empty-hint">暂无关注的标签</div>

      <div class="add-row">
        <div class="suggest-wrap">
          <input
            v-model="watchInput"
            class="tag-input"
            placeholder="输入标签名，如 artist:xxx（支持联想）"
            @keyup.enter="addTag(actionWatch)"
            @focus="refreshWatchSuggestions"
            @blur="closeWatchSuggestions"
          />
          <div v-if="watchSuggestions.length" class="tag-suggest-dropdown">
            <div
              v-for="s in watchSuggestions"
              :key="`${s.namespace}:${s.key}`"
              class="suggest-item"
              @mousedown.prevent="pickSuggestion(actionWatch, s)"
            >
              <span class="suggest-text">{{ s.namespace }}:{{ s.key }}</span>
              <span v-if="s.count" class="suggest-count">🔥 {{ s.count.toLocaleString() }}</span>
            </div>
          </div>
        </div>
        <button
          class="action-btn primary"
          :disabled="!!submitting || !watchInput.trim()"
          @click="addTag(actionWatch)"
        >
          {{ submitting === actionWatch ? '添加中…' : '关注' }}
        </button>
      </div>
    </section>

    <!-- 隐藏的标签 -->
    <section class="tag-section">
      <div class="section-header">
        <span class="section-title">隐藏的标签</span>
        <span class="section-count">{{ hidden.length }}</span>
      </div>

      <div v-if="hidden.length" class="tag-list">
        <span
          v-for="tag in hidden"
          :key="tag"
          class="tag-chip hide-chip"
          :class="{ removing: removingTag === `${actionHide}:${tag}` }"
        >
          {{ tag }}
          <button
            class="chip-remove"
            title="取消隐藏"
            :disabled="!!submitting"
            @click="removeTag(actionHide, tag)"
          >
            ✕
          </button>
        </span>
      </div>
      <div v-else-if="!loading" class="empty-hint">暂无隐藏的标签</div>

      <div class="add-row">
        <div class="suggest-wrap">
          <input
            v-model="hideInput"
            class="tag-input"
            placeholder="输入标签名，如 language:chinese（支持联想）"
            @keyup.enter="addTag(actionHide)"
            @focus="refreshHideSuggestions"
            @blur="closeHideSuggestions"
          />
          <div v-if="hideSuggestions.length" class="tag-suggest-dropdown">
            <div
              v-for="s in hideSuggestions"
              :key="`${s.namespace}:${s.key}`"
              class="suggest-item"
              @mousedown.prevent="pickSuggestion(actionHide, s)"
            >
              <span class="suggest-text">{{ s.namespace }}:{{ s.key }}</span>
              <span v-if="s.count" class="suggest-count">🔥 {{ s.count.toLocaleString() }}</span>
            </div>
          </div>
        </div>
        <button
          class="action-btn primary"
          :disabled="!!submitting || !hideInput.trim()"
          @click="addTag(actionHide)"
        >
          {{ submitting === actionHide ? '添加中…' : '隐藏' }}
        </button>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, watch } from 'vue'
import { useUI } from '@/composables/useUI'
import { http } from '@/utils/request'
import type { EHMyTags, EHTagset, TagSuggestion } from '@/types/eh'

defineEmits<{ (e: 'back'): void }>()

const { toast } = useUI()

const actionWatch = 'watch'
const actionHide = 'hide'

const watched = ref<string[]>([])
const hidden = ref<string[]>([])
const loading = ref(false)
const submitting = ref<'' | 'watch' | 'hide'>('')
const removingTag = ref('')
const isLoggedIn = ref(true)

const watchInput = ref('')
const hideInput = ref('')

const tagsetInput = ref('')
const creatingTagset = ref(false)

// Tagset 列表与当前选中的 Tagset ID
const tagsetList = ref<EHTagset[]>([])
const currentTagset = ref(0)

// 标签联想（复用后端 /tags/suggest 的 tagname_count.csv 热度库）
const watchSuggestions = ref<TagSuggestion[]>([])
const hideSuggestions = ref<TagSuggestion[]>([])
let watchTimer: number | null = null
let hideTimer: number | null = null

const errorMessage = (err: unknown) => (err as Error)?.message || '操作失败'

const load = async () => {
  loading.value = true
  try {
    const params: Record<string, string | number> = {}
    if (currentTagset.value > 0) params.tagset = currentTagset.value
    const res = await http<EHMyTags>('/eh/mytags', { params })
    watched.value = res.watched || []
    hidden.value = res.hidden || []
    tagsetList.value = res.tagsets || []
    if (res.currentTagset !== undefined) currentTagset.value = res.currentTagset
    isLoggedIn.value = true
  } catch (err) {
    isLoggedIn.value = false
    toast.error(errorMessage(err))
    console.error(err)
  } finally {
    loading.value = false
  }
}

// 切换 Tagset 后重新读取该集下的标签
const switchTagset = () => {
  load()
}

const addTag = async (action: 'watch' | 'hide') => {
  const input = action === actionWatch ? watchInput.value : hideInput.value
  const tag = input.trim()
  if (!tag) return

  submitting.value = action
  try {
    await http('/eh/mytags', {
      method: 'POST',
      body: JSON.stringify({ action, tag, tagset: currentTagset.value }),
    })
    toast.success(action === actionWatch ? `已关注标签: ${tag}` : `已隐藏标签: ${tag}`)
    if (action === actionWatch) {
      watched.value.push(tag)
      watchInput.value = ''
      watchSuggestions.value = []
    } else {
      hidden.value.push(tag)
      hideInput.value = ''
      hideSuggestions.value = []
    }
  } catch (err) {
    toast.error(errorMessage(err))
    console.error(err)
  } finally {
    submitting.value = ''
  }
}

const removeTag = async (action: 'watch' | 'hide', tag: string) => {
  removingTag.value = `${action}:${tag}`
  try {
    await http('/eh/mytags/remove', {
      method: 'POST',
      body: JSON.stringify({ action, tag, tagset: currentTagset.value }),
    })
    toast.success(action === actionWatch ? `已取消关注: ${tag}` : `已取消隐藏: ${tag}`)
    if (action === actionWatch) {
      watched.value = watched.value.filter((t) => t !== tag)
    } else {
      hidden.value = hidden.value.filter((t) => t !== tag)
    }
  } catch (err) {
    toast.error(errorMessage(err))
    console.error(err)
  } finally {
    removingTag.value = ''
  }
}

const createTagset = async () => {
  const name = tagsetInput.value.trim()
  if (!name) return

  creatingTagset.value = true
  try {
    await http('/eh/mytags/tagset', {
      method: 'POST',
      body: JSON.stringify({ name }),
    })
    toast.success(`Tagset 已创建: ${name}`)
    tagsetInput.value = ''
    // 刷新 Tagset 列表（新建的 Tagset 默认未选中，标签仍归属当前集）
    await load()
  } catch (err) {
    toast.error(errorMessage(err))
    console.error(err)
  } finally {
    creatingTagset.value = false
  }
}

// ===================== 标签联想补全（/tags/suggest 热度库） =====================
const fetchSuggestions = async (q: string): Promise<TagSuggestion[]> => {
  const kw = q.trim()
  if (!kw) return []
  try {
    return await http<TagSuggestion[]>('/tags/suggest', { params: { q: kw, limit: 8 } })
  } catch (e) {
    console.error('标签联想失败:', e)
    return []
  }
}

// 输入内容变化时防抖请求联想
watch(watchInput, (val) => {
  if (watchTimer) clearTimeout(watchTimer)
  watchTimer = window.setTimeout(async () => {
    watchSuggestions.value = await fetchSuggestions(val)
  }, 200)
})

watch(hideInput, (val) => {
  if (hideTimer) clearTimeout(hideTimer)
  hideTimer = window.setTimeout(async () => {
    hideSuggestions.value = await fetchSuggestions(val)
  }, 200)
})

// 输入框获得焦点时立即刷新联想
const refreshWatchSuggestions = async () => {
  if (watchTimer) clearTimeout(watchTimer)
  watchSuggestions.value = await fetchSuggestions(watchInput.value)
}

const refreshHideSuggestions = async () => {
  if (hideTimer) clearTimeout(hideTimer)
  hideSuggestions.value = await fetchSuggestions(hideInput.value)
}

// 选中联想项：填入完整 "namespace:key"（含 other: 前缀，与 E 站页面 title 一致）
const pickSuggestion = (action: 'watch' | 'hide', tag: TagSuggestion) => {
  const text = `${tag.namespace}:${tag.key}`
  if (action === actionWatch) {
    watchInput.value = text
    watchSuggestions.value = []
  } else {
    hideInput.value = text
    hideSuggestions.value = []
  }
}

// 失焦时延迟关闭下拉（留出点击联想项的时间）
const closeWatchSuggestions = () => {
  window.setTimeout(() => {
    watchSuggestions.value = []
  }, 150)
}

const closeHideSuggestions = () => {
  window.setTimeout(() => {
    hideSuggestions.value = []
  }, 150)
}

onMounted(load)
</script>

<style scoped>
.my-tags-settings {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.sub-header {
  display: flex;
  align-items: center;
  gap: 12px;
}

.back-btn {
  background: transparent;
  border: none;
  color: #a0a0a0;
  font-size: 18px;
  cursor: pointer;
  padding: 4px 8px;
  border-radius: 4px;
  transition:
    color 0.2s,
    background-color 0.2s;
}

.back-btn:hover {
  color: #ffffff;
  background-color: #26262a;
}

.sub-title {
  margin: 0;
  font-size: 16px;
  font-weight: 600;
  color: #ffffff;
  flex: 1;
}

.refresh-btn {
  background: #26262a;
  border: 1px solid #36363a;
  color: #c0c0c5;
  border-radius: 6px;
  padding: 5px 12px;
  font-size: 12px;
  cursor: pointer;
  transition: all 0.2s;
}

.refresh-btn:hover:not(:disabled) {
  color: #ffffff;
  border-color: #ff7588;
}

.refresh-btn:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.desc {
  margin: 0;
  font-size: 13px;
  color: #88888c;
  line-height: 1.5;
}

.login-hint {
  padding: 12px 16px;
  background-color: rgba(255, 193, 7, 0.1);
  border: 1px solid rgba(255, 193, 7, 0.35);
  color: #e8c14c;
  border-radius: 8px;
  font-size: 13px;
}

.tag-section {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 16px;
  background-color: #18181c;
  border: 1px solid #26262a;
  border-radius: 10px;
}

.section-header {
  display: flex;
  align-items: center;
  gap: 8px;
  padding-bottom: 10px;
  border-bottom: 1px solid #26262a;
}

.section-title {
  font-size: 14px;
  font-weight: 600;
  color: #ffffff;
}

.section-count {
  font-size: 11px;
  background-color: #2b2b30;
  color: #a0a0a5;
  padding: 2px 8px;
  border-radius: 10px;
}

.tagset-row {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.tagset-label {
  font-size: 13px;
  color: #a0a0a5;
  white-space: nowrap;
}

.tagset-select {
  background: #1f1f24;
  border: 1px solid #36363a;
  border-radius: 6px;
  color: #e0e0e0;
  font-size: 13px;
  padding: 6px 10px;
  outline: none;
  cursor: pointer;
  max-width: 260px;
}

.tagset-select:focus {
  border-color: #ff7588;
}

.tagset-select:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.tagset-hint {
  font-size: 12px;
  color: #6a6a70;
}

.tag-list {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.tag-chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 5px 10px;
  border-radius: 6px;
  font-size: 13px;
  transition: opacity 0.2s;
}

.tag-chip.removing {
  opacity: 0.4;
  pointer-events: none;
}

.watch-chip {
  background-color: rgba(80, 200, 120, 0.12);
  border: 1px solid rgba(80, 200, 120, 0.4);
  color: #7fe0a4;
}

.hide-chip {
  background-color: rgba(255, 117, 136, 0.12);
  border: 1px solid rgba(255, 117, 136, 0.4);
  color: #ff9aa8;
}

.chip-remove {
  background: transparent;
  border: none;
  color: inherit;
  font-size: 12px;
  cursor: pointer;
  opacity: 0.6;
  padding: 0;
  line-height: 1;
  transition: opacity 0.2s;
}

.chip-remove:hover:not(:disabled) {
  opacity: 1;
}

.chip-remove:disabled {
  cursor: not-allowed;
}

.empty-hint {
  font-size: 13px;
  color: #6a6a70;
}

.add-row {
  display: flex;
  gap: 10px;
}

.tag-input {
  flex: 1;
  background: #1f1f24;
  border: 1px solid #36363a;
  border-radius: 6px;
  padding: 8px 12px;
  color: #e0e0e0;
  font-size: 13px;
  outline: none;
  transition: border-color 0.2s;
}

.tag-input:focus {
  border-color: #ff7588;
}

.action-btn {
  padding: 8px 18px;
  border-radius: 6px;
  border: 1px solid #36363a;
  background: #26262a;
  color: #eee;
  cursor: pointer;
  font-size: 13px;
  transition: all 0.2s;
  white-space: nowrap;
}

.action-btn:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.action-btn:hover:not(:disabled) {
  background: #323236;
}

.action-btn.primary {
  background: #ff7588;
  border-color: #ff7588;
  color: #fff;
}

.action-btn.primary:hover:not(:disabled) {
  background: #f06477;
}

/* ============ 标签联想下拉 ============ */
.suggest-wrap {
  position: relative;
  flex: 1;
}

.suggest-wrap .tag-input {
  width: 100%;
  flex: none;
}

.tag-suggest-dropdown {
  position: absolute;
  top: calc(100% + 4px);
  left: 0;
  right: 0;
  z-index: 30;
  max-height: 280px;
  overflow-y: auto;
  background: #222226;
  border: 1px solid #3a3a40;
  border-radius: 8px;
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.45);
  padding: 4px;
}

.suggest-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 7px 10px;
  border-radius: 6px;
  cursor: pointer;
  font-size: 13px;
  color: #e0e0e0;
  transition: background-color 0.15s;
}

.suggest-item:hover {
  background-color: #2e2e34;
}

.suggest-text {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.suggest-count {
  flex-shrink: 0;
  font-size: 12px;
  color: #ffb86c;
}
</style>
