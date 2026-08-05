<template>
  <div class="profile-settings">
    <!-- 页头 -->
    <div class="page-header">
      <button class="back-btn" @click="emit('back')">← 返回</button>
      <div class="page-title">Profile 设置（E 站）</div>
    </div>

    <!-- 加载 / 错误 / 空状态 -->
    <div v-if="loading" class="state-hint">加载中…</div>
    <div v-else-if="error" class="state-hint error-hint">{{ error }}</div>
    <div v-else-if="!uconfig" class="state-hint">暂无配置</div>

    <template v-else>
      <!-- Profile 操作区 -->
      <div class="profile-bar">
        <div class="profile-select-row">
          <span class="profile-label">Selected Profile:</span>
          <select
            class="profile-select"
            :value="profileValue"
            :disabled="!!submitting"
            @change="onProfileChange"
          >
            <option v-for="p in uconfig.profiles" :key="p.value" :value="p.value">
              {{ p.label }}
            </option>
          </select>
        </div>
        <div class="profile-actions">
          <button class="action-btn" :disabled="!!submitting" @click="renameProfile">重命名</button>
          <button class="action-btn" :disabled="!!submitting" @click="createProfile">新建</button>
          <button class="action-btn" :disabled="!!submitting" @click="setDefaultProfile">
            设为默认
          </button>
          <button class="action-btn danger" :disabled="!!submitting" @click="deleteProfile">
            删除
          </button>
        </div>
      </div>

      <!-- 配置分组 -->
      <div v-for="(section, si) in uconfig.sections" :key="si" class="section">
        <h3 class="section-title">{{ section.title }}</h3>

        <div v-for="(field, fi) in section.fields" :key="fi" class="field-block">
          <p v-if="field.description" class="field-desc">{{ field.description }}</p>

          <!-- radio -->
          <div v-if="field.type === 'radio'" class="radio-group">
            <span v-if="field.label" class="field-label">{{ field.label }}</span>
            <label
              v-for="(opt, oi) in field.options"
              :key="oi"
              class="radio-option"
              :class="{ disabled: opt.disabled }"
            >
              <input
                type="radio"
                :name="field.name"
                :value="opt.value"
                v-model="textValues[textKey(field)]"
                :disabled="!!opt.disabled"
              />
              <span>{{ opt.label }}</span>
            </label>
          </div>

          <!-- select -->
          <div v-else-if="field.type === 'select'" class="select-row">
            <span v-if="field.label" class="field-label">{{ field.label }}</span>
            <select v-model="textValues[textKey(field)]" class="uconfig-select">
              <option v-for="(opt, oi) in field.options" :key="oi" :value="opt.value">
                {{ opt.label }}
              </option>
            </select>
          </div>

          <!-- checkbox -->
          <label v-else-if="field.type === 'checkbox'" class="checkbox-option">
            <input type="checkbox" v-model="boolValues[boolKey(field)]" />
            <span>{{ field.label }}</span>
          </label>

          <!-- text -->
          <div v-else-if="field.type === 'text'" class="text-row">
            <div class="text-input-line">
              <span v-if="field.label" class="field-label">{{ field.label }}</span>
              <input
                type="text"
                class="uconfig-text"
                v-model="textValues[textKey(field)]"
                :maxlength="field.maxLength"
                :placeholder="field.placeholder"
              />
              <span v-if="field.suffix" class="field-suffix">{{ field.suffix }}</span>
            </div>
            <p v-if="field.hint" class="field-hint">{{ field.hint }}</p>
          </div>

          <!-- textarea -->
          <div v-else-if="field.type === 'textarea'" class="textarea-row">
            <textarea class="uconfig-textarea" v-model="textValues[textKey(field)]"></textarea>
          </div>

          <!-- category -->
          <div v-else-if="field.type === 'category'" class="category-grid">
            <div
              v-for="(cat, ci) in field.categories"
              :key="ci"
              class="category-chip"
              :class="{ active: boolValues[catKey(cat)] }"
              @click="toggleCategory(cat)"
            >
              {{ cat.label }}
            </div>
          </div>

          <!-- language-table -->
          <div v-else-if="field.type === 'language-table'" class="lang-table-wrap">
            <table class="lang-table">
              <thead>
                <tr>
                  <th></th>
                  <th v-for="(col, ci) in field.table?.columns" :key="ci">{{ col }}</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="(row, ri) in field.table?.rows" :key="ri">
                  <td class="lang-name">{{ row.label }}</td>
                  <td v-for="(cell, ci) in row.cells" :key="ci" class="lang-cell">
                    <input v-if="cell.name" type="checkbox" v-model="boolValues[cellKey(cell)]" />
                    <button
                      v-else
                      class="all-btn"
                      :title="'全选/取消 ' + row.label"
                      @click="toggleRowAll(row)"
                    >
                      全选
                    </button>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>

      <!-- Apply -->
      <div class="apply-bar">
        <button class="apply-btn" :disabled="!!submitting" @click="apply">
          {{ submitting ? '保存中…' : 'Apply 保存' }}
        </button>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useUI } from '@/composables/useUI'
import { http } from '@/utils/request'
import type {
  UConfigData,
  UConfigField,
  UConfigCategory,
  UConfigCell,
  UConfigTableRow,
} from '@/types/eh'

const emit = defineEmits<{ back: [] }>()

const { toast, modal } = useUI()

const uconfig = ref<UConfigData | null>(null)
const loading = ref(false)
const submitting = ref(false)
const error = ref('')
const profileValue = ref('')

// 字符串字段值（radio / select / text / textarea）
const textValues = ref<Record<string, string>>({})
// 布尔字段值（checkbox / category / language-table）
const boolValues = ref<Record<string, boolean>>({})

const textKey = (f: UConfigField) => `${f.type}:${f.name}`
const boolKey = (f: UConfigField) => `checkbox:${f.name}`
const catKey = (c: UConfigCategory) => `category:${c.name}`
const cellKey = (c: UConfigCell) => `lang:${c.name}`

const selectedLabel = () =>
  uconfig.value?.profiles.find((p) => p.value === profileValue.value)?.label || ''

// 依据后端解析结果初始化字段值
const initValues = (data: UConfigData) => {
  const tv: Record<string, string> = {}
  const bv: Record<string, boolean> = {}
  for (const section of data.sections) {
    for (const f of section.fields) {
      if (f.type === 'radio' || f.type === 'select') {
        tv[textKey(f)] = f.options?.find((o) => o.checked)?.value ?? ''
      } else if (f.type === 'text' || f.type === 'textarea') {
        tv[textKey(f)] = f.value ?? ''
      } else if (f.type === 'checkbox') {
        bv[boolKey(f)] = !!f.checked
      } else if (f.type === 'category') {
        for (const c of f.categories ?? []) bv[catKey(c)] = !!c.checked
      } else if (f.type === 'language-table') {
        for (const row of f.table?.rows ?? []) {
          for (const cell of row.cells) if (cell.name) bv[cellKey(cell)] = !!cell.checked
        }
      }
    }
  }
  textValues.value = tv
  boolValues.value = bv
}

// 应用后端返回的新数据（切换/操作后重载界面）
const applyUConfig = (data: UConfigData) => {
  uconfig.value = data
  profileValue.value = data.selectedProfile || data.profiles[0]?.value || ''
  initValues(data)
}

// 加载配置
const load = async () => {
  loading.value = true
  error.value = ''
  try {
    applyUConfig(await http<UConfigData>('/eh/uconfig'))
  } catch (err) {
    error.value = (err as Error)?.message || '读取配置失败'
    console.error(err)
  } finally {
    loading.value = false
  }
}

// 切换 profile（下拉变化即提交）
const onProfileChange = (e: Event) => {
  const value = (e.target as HTMLSelectElement).value
  if (!value || value === profileValue.value) return
  doProfileAction('', value)
}

// 执行 profile 操作：'' 切换 / rename / create / default / delete
const doProfileAction = async (action: string, value: string) => {
  submitting.value = true
  try {
    const data = await http<UConfigData>('/eh/uconfig', {
      method: 'POST',
      body: JSON.stringify({ profile: value || profileValue.value, action }),
    })
    applyUConfig(data)
    toast.success(action ? 'Profile 操作成功' : '已切换 Profile')
  } catch (err) {
    toast.error((err as Error)?.message || 'Profile 操作失败')
    console.error(err)
    await load()
  } finally {
    submitting.value = false
  }
}

const renameProfile = async () => {
  const name = await modal.prompt('输入新的 Profile 名称', selectedLabel())
  if (name == null || !name.trim()) return
  submitting.value = true
  try {
    const data = await http<UConfigData>('/eh/uconfig', {
      method: 'POST',
      body: JSON.stringify({
        profile: profileValue.value,
        action: 'rename',
        profileName: name.trim(),
      }),
    })
    applyUConfig(data)
    toast.success('已重命名')
  } catch (err) {
    toast.error((err as Error)?.message || '重命名失败')
    console.error(err)
  } finally {
    submitting.value = false
  }
}

const createProfile = async () => {
  const name = await modal.prompt('输入新 Profile 名称', 'New Profile')
  if (name == null || !name.trim()) return
  submitting.value = true
  try {
    const data = await http<UConfigData>('/eh/uconfig', {
      method: 'POST',
      body: JSON.stringify({
        profile: profileValue.value,
        action: 'create',
        profileName: name.trim(),
      }),
    })
    applyUConfig(data)
    toast.success('已创建新 Profile')
  } catch (err) {
    toast.error((err as Error)?.message || '创建失败')
    console.error(err)
  } finally {
    submitting.value = false
  }
}

const setDefaultProfile = async () => {
  doProfileAction('default', profileValue.value)
}

const deleteProfile = async () => {
  const ok = await modal.confirm(`确认删除 Profile「${selectedLabel()}」吗？`)
  if (!ok) return
  doProfileAction('delete', profileValue.value)
}

// 分类开关切换
const toggleCategory = (cat: UConfigCategory) => {
  const key = catKey(cat)
  boolValues.value[key] = !boolValues.value[key]
}

// 语言表格整行全选 / 取消
const toggleRowAll = (row: UConfigTableRow) => {
  const named = row.cells.filter((c) => c.name)
  if (!named.length) return
  const allChecked = named.every((c) => boolValues.value[cellKey(c)])
  for (const c of named) boolValues.value[cellKey(c)] = !allChecked
}

// 收集字段值 → 表单提交参数
const collect = (): Record<string, string> => {
  const fields: Record<string, string> = {}
  for (const section of uconfig.value?.sections ?? []) {
    for (const f of section.fields) {
      if (f.type === 'radio' || f.type === 'select' || f.type === 'text' || f.type === 'textarea') {
        fields[f.name] = textValues.value[textKey(f)] ?? ''
      } else if (f.type === 'checkbox') {
        if (boolValues.value[boolKey(f)]) fields[f.name] = 'on'
      } else if (f.type === 'category') {
        for (const c of f.categories ?? []) {
          fields[`ct_${c.name}`] = boolValues.value[catKey(c)] ? '1' : '0'
        }
      } else if (f.type === 'language-table') {
        for (const row of f.table?.rows ?? []) {
          for (const cell of row.cells) {
            if (cell.name && boolValues.value[cellKey(cell)]) fields[cell.name] = 'on'
          }
        }
      }
    }
  }
  return fields
}

// Apply 保存全部配置
const apply = async () => {
  if (!uconfig.value) return
  submitting.value = true
  try {
    const data = await http<UConfigData>('/eh/uconfig', {
      method: 'POST',
      body: JSON.stringify({ profile: profileValue.value, fields: collect() }),
    })
    applyUConfig(data)
    toast.success('配置已保存')
  } catch (err) {
    toast.error((err as Error)?.message || '保存失败')
    console.error(err)
  } finally {
    submitting.value = false
  }
}

load()
</script>

<style scoped>
.profile-settings {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

/* 页头 */
.page-header {
  display: flex;
  align-items: center;
  gap: 12px;
}

.back-btn {
  background: #26262a;
  border: 1px solid #36363a;
  color: #c0c0c4;
  padding: 6px 12px;
  border-radius: 6px;
  font-size: 13px;
  cursor: pointer;
  transition: all 0.2s ease;
}

.back-btn:hover {
  background: #38383e;
  color: #ffffff;
}

.page-title {
  font-size: 16px;
  font-weight: 600;
  color: #ffffff;
}

.state-hint {
  padding: 24px;
  text-align: center;
  color: #88888c;
  font-size: 14px;
}

.error-hint {
  color: #ff7588;
}

/* Profile 操作区 */
.profile-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 12px;
  padding: 14px 16px;
  background-color: #1a1a1e;
  border-radius: 8px;
  border: 1px solid #26262a;
}

.profile-select-row {
  display: flex;
  align-items: center;
  gap: 8px;
}

.profile-label {
  font-size: 13px;
  color: #88888c;
  white-space: nowrap;
}

.profile-select {
  background: #26262a;
  border: 1px solid #36363a;
  border-radius: 6px;
  padding: 7px 10px;
  color: #ffffff;
  font-size: 14px;
  font-weight: 600;
  outline: none;
  cursor: pointer;
  min-width: 220px;
}

.profile-select:focus {
  border-color: #ff7588;
}

.profile-actions {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.action-btn {
  background: #26262a;
  border: 1px solid #36363a;
  color: #c0c0c4;
  padding: 6px 12px;
  border-radius: 6px;
  font-size: 13px;
  cursor: pointer;
  transition: all 0.2s ease;
}

.action-btn:hover:not(:disabled) {
  background: #38383e;
  color: #ffffff;
}

.action-btn.danger {
  border-color: #5a2a30;
  color: #ff7588;
}

.action-btn.danger:hover:not(:disabled) {
  background: #3a1e24;
}

.action-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

/* 分组 */
.section {
  background-color: #1a1a1e;
  border-radius: 8px;
  border: 1px solid #26262a;
  padding: 14px 16px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.section-title {
  font-size: 14px;
  font-weight: 600;
  color: #ff7588;
  margin: 0;
}

.field-block {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.field-desc {
  font-size: 13px;
  color: #88888c;
  line-height: 1.5;
  margin: 0;
}

.field-label {
  font-size: 13px;
  color: #c0c0c4;
  font-weight: 500;
}

.field-hint {
  font-size: 12px;
  color: #6a6a70;
  line-height: 1.5;
  margin: 0;
}

/* radio */
.radio-group {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.radio-option {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  font-size: 13px;
  color: #d0d0d4;
  cursor: pointer;
}

.radio-option input {
  margin-top: 2px;
  accent-color: #ff7588;
}

.radio-option.disabled {
  opacity: 0.45;
  cursor: not-allowed;
}

/* select */
.select-row {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.uconfig-select {
  background: #26262a;
  border: 1px solid #36363a;
  border-radius: 6px;
  padding: 6px 10px;
  color: #e0e0e0;
  font-size: 13px;
  outline: none;
  cursor: pointer;
}

.uconfig-select:focus {
  border-color: #ff7588;
}

/* checkbox */
.checkbox-option {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  font-size: 13px;
  color: #d0d0d4;
  cursor: pointer;
}

.checkbox-option input {
  margin-top: 2px;
  accent-color: #ff7588;
}

/* text */
.text-row {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.text-input-line {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.uconfig-text {
  background: #26262a;
  border: 1px solid #36363a;
  border-radius: 6px;
  padding: 6px 10px;
  color: #e0e0e0;
  font-size: 13px;
  outline: none;
  width: 140px;
}

.uconfig-text:focus {
  border-color: #ff7588;
}

.field-suffix {
  font-size: 13px;
  color: #88888c;
}

/* textarea */
.textarea-row {
  display: flex;
}

.uconfig-textarea {
  background: #26262a;
  border: 1px solid #36363a;
  border-radius: 6px;
  padding: 8px 10px;
  color: #e0e0e0;
  font-size: 13px;
  outline: none;
  width: 100%;
  height: 120px;
  resize: vertical;
  font-family: inherit;
}

.uconfig-textarea:focus {
  border-color: #ff7588;
}

/* category */
.category-grid {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.category-chip {
  padding: 6px 14px;
  border-radius: 16px;
  font-size: 13px;
  background: #26262a;
  border: 1px solid #36363a;
  color: #88888c;
  cursor: pointer;
  user-select: none;
  transition: all 0.2s ease;
}

.category-chip:hover {
  background: #303036;
}

.category-chip.active {
  background: #ff7588;
  border-color: #ff7588;
  color: #1a1a1e;
  font-weight: 600;
}

/* language table */
.lang-table-wrap {
  overflow-x: auto;
}

.lang-table {
  border-collapse: collapse;
  font-size: 13px;
  color: #d0d0d4;
}

.lang-table th {
  padding: 6px 10px;
  color: #88888c;
  font-weight: 500;
  text-align: center;
  border-bottom: 1px solid #2a2a2e;
}

.lang-table td {
  padding: 5px 10px;
  text-align: center;
  border-bottom: 1px solid #222226;
}

.lang-table .lang-name {
  text-align: left;
  font-weight: 500;
  color: #c0c0c4;
}

.lang-cell input {
  accent-color: #ff7588;
  cursor: pointer;
}

.all-btn {
  background: #26262a;
  border: 1px solid #36363a;
  color: #88888c;
  font-size: 11px;
  padding: 2px 8px;
  border-radius: 4px;
  cursor: pointer;
}

.all-btn:hover {
  background: #38383e;
  color: #ffffff;
}

/* Apply */
.apply-bar {
  display: flex;
  justify-content: center;
  padding: 4px 0 8px;
}

.apply-btn {
  background: #ff7588;
  border: none;
  color: #1a1a1e;
  font-size: 14px;
  font-weight: 600;
  padding: 10px 36px;
  border-radius: 8px;
  cursor: pointer;
  transition: all 0.2s ease;
}

.apply-btn:hover:not(:disabled) {
  background: #ff8a9a;
}

.apply-btn:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}
</style>
