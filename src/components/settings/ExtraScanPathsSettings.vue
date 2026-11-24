<template>
  <div class="extra-scan-settings">
    <!-- 顶部 Header -->
    <div class="header">
      <button class="back-btn" @click="$emit('back')" title="返回下载设置">
        <AppIcon name="arrowLeft" :size="20" />
      </button>

      <div class="header-text">
        <div class="header-title">额外的画廊扫描路径</div>
        <div class="header-subtext">扫描额外的漫画路径，包括子文件夹中的漫画/压缩包</div>
      </div>
    </div>

    <!-- 添加新路径卡片 -->
    <div class="card add-card">
      <div class="input-label">
        添加新路径（可填显示名作为来源标签，如「汉化合集」「自购存档」）
      </div>
      <div class="input-group">
        <input
          v-model="inputName"
          type="text"
          placeholder="显示名（可选），留空则使用路径名"
          class="setting-input name-input"
          @keyup.enter="handleAdd"
        />
        <input
          v-model="inputPath"
          type="text"
          placeholder="输入或粘贴绝对路径，如 Z:\Comics\"
          class="setting-input"
          @keyup.enter="handleAdd"
        />
        <button @click="handleAdd" :disabled="!inputPath.trim()" class="btn btn-primary">
          添加
        </button>
      </div>
    </div>

    <!-- 动态路径列表 -->
    <div class="path-list">
      <div v-for="item in scanPaths" :key="item.id" class="card path-item">
        <div class="path-row">
          <div class="path-info">
            <!-- 路径文字与图标 -->
            <div class="path-title">
              <span class="folder-icon"><AppIcon name="folder" :size="14" /></span>
              <span class="path-text" :title="item.path">{{ item.path }}</span>

              <!-- 显示名（来源标签）：可设置/重命名（问题3） -->
              <input
                v-if="editingNameId === item.id"
                v-model="editNameValue"
                class="name-edit-input"
                placeholder="来源标签，留空则清除"
                @keyup.enter="saveName(item)"
                @keyup.esc="cancelEditName"
                @blur="saveName(item)"
              />
              <span v-else-if="item.name" class="path-name-badge" :title="'来源标签：' + item.name">
                {{ item.name }}
                <button
                  class="name-edit-btn"
                  @mousedown.prevent
                  @click="startEditName(item)"
                  title="重命名显示名"
                >
                  <AppIcon name="edit" :size="11" />
                </button>
              </span>
              <button
                v-else
                class="name-set-btn"
                @mousedown.prevent
                @click="startEditName(item)"
                title="设置显示名（来源标签），卡片与详情页来源栏目将显示它"
              >
                <AppIcon name="tag" :size="12" /> 起名
              </button>
            </div>

            <!-- 扫描状态与元信息 -->
            <div class="path-meta">
              <span v-if="item.lastScanned">上次扫描: {{ formatTime(item.lastScanned) }}</span>
              <span v-else class="status-unscanned">未扫描</span>
              <span v-if="item.comicCount !== undefined" class="count-text">
                • 发现 {{ item.comicCount }} 本漫画
              </span>
              <span v-if="isScanning(item)" class="scan-badge">{{ scanModeText(item) }}</span>
              <!-- Round54：路径不存在（整条目录被删）→ 提示恢复，不提示移除路径 -->
              <span v-if="item.pathMissing" class="path-missing">⚠ 路径不存在，建议进行恢复</span>
            </div>
          </div>

          <div class="path-actions">
            <!-- 包含子文件夹 Switch 开关 -->
            <label class="toggle-container" title="包含子文件夹">
              <span class="toggle-label">包含子文件夹</span>
              <span class="toggle-switch">
                <input
                  type="checkbox"
                  :checked="item.includeSubfolders"
                  @change="
                    handleToggleSubfolders(item.id, ($event.target as HTMLInputElement).checked)
                  "
                />
                <span class="slider"></span>
              </span>
            </label>

            <!-- 离线维护开关（问题4）：控制该路径是否参与「离线更新检测」与「本地维护查重」 -->
            <label
              class="toggle-container"
              title="参与离线更新检测与本地维护查重；关闭后该路径下的漫画不会出现在「离线更新」与「维护查重」中（下载导入的漫画始终参与）"
            >
              <span class="toggle-label">离线维护</span>
              <span class="toggle-switch">
                <input
                  type="checkbox"
                  :checked="item.enableOfflineUpdate !== false"
                  @change="
                    handleToggleOfflineUpdate(item, ($event.target as HTMLInputElement).checked)
                  "
                />
                <span class="slider"></span>
              </span>
            </label>

            <!-- 扫描模式按钮（空闲或已完成时显示） -->
            <template v-if="!isScanning(item)">
              <button
                class="btn btn-mode"
                @click="handleSingleScan(item, 'full')"
                :disabled="isPathLocked(item)"
                title="全文件夹扫描：重新扫描整个目录"
              >
                全量
              </button>
              <button
                class="btn btn-mode incremental"
                @click="handleSingleScan(item, 'incremental')"
                :disabled="isPathLocked(item)"
                title="增量式更新：跳过已收录的漫画，仅入库新内容"
              >
                增量
              </button>
              <!-- Round54：恢复（增量右侧），可恢复队列非空且未在恢复中时可点击 -->
              <button
                class="btn btn-mode recovery"
                :disabled="!canRecover || recoveryBusy"
                :title="recoverTitle"
                @click="openRecovery"
              >
                恢复
              </button>
              <!-- Round54：阵亡（移除路径左侧），全局阵亡名单非空时可点击 -->
              <button
                class="btn btn-mode fallen"
                :disabled="fallenTotal === 0"
                :title="fallenTotal > 0 ? `阵亡名单（${fallenTotal}）` : '暂无阵亡记录'"
                @click="openFallen"
              >
                阵亡
              </button>
            </template>

            <!-- 扫描中显示旋转图标 -->
            <span v-else class="btn btn-icon" title="扫描中…">
              <span class="spinning"><AppIcon name="refresh" :size="15" /></span>
            </span>

            <!-- 删除按钮（二次确认；移除路径会连带清理该路径下已入库的漫画记录） -->
            <button
              @click="handleRemove(item)"
              class="btn btn-icon danger"
              :disabled="removingId === item.id"
              title="移除路径"
            >
              <AppIcon name="close" :size="15" />
            </button>
          </div>
        </div>

        <!-- 扫描进度区域（扫描中） -->
        <div v-if="isScanning(item)" class="scan-progress-area">
          <!-- 计数阶段 -->
          <div v-if="progressOf(item)!.phase === 'counting'" class="progress-counting">
            <span class="spinning"><AppIcon name="refresh" :size="14" /></span> 正在扫描文件夹结构…
          </div>
          <!-- 扫描阶段 -->
          <template v-else>
            <div class="progress-bar">
              <div
                class="progress-fill"
                :style="{ width: progressPercent(progressOf(item)!) + '%' }"
              ></div>
            </div>
            <div class="progress-meta">
              <span
                v-if="progressOf(item)!.currentTitle"
                class="progress-title"
                :title="progressOf(item)!.currentTitle"
              >
                正在处理: {{ progressOf(item)!.currentTitle }}
              </span>
              <span v-else class="progress-title">正在扫描…</span>
              <span class="progress-num">
                {{ progressOf(item)!.processed }}/{{ progressOf(item)!.total || '?' }}
                <template v-if="progressOf(item)!.found">
                  • 发现 {{ progressOf(item)!.found }}
                </template>
                <template v-if="progressOf(item)!.skipped">
                  • 跳过 {{ progressOf(item)!.skipped }}
                </template>
              </span>
            </div>
          </template>
        </div>

        <!-- 完成提示区域 -->
        <div
          v-else-if="progressOf(item) && progressOf(item)!.done"
          class="scan-done-area"
          :class="{ error: progressOf(item)!.error }"
        >
          <span v-if="progressOf(item)!.error">
            <AppIcon name="error" weight="fill" :size="13" /> 扫描失败: {{ progressOf(item)!.error }}
          </span>
          <span v-else>
            <AppIcon name="success" weight="fill" :size="13" /> 扫描完成，共
            {{ progressOf(item)!.comicCount ?? progressOf(item)!.found }} 本
            <template v-if="skippedDetail(progressOf(item)!)">
              （跳过 {{ skippedDetail(progressOf(item)!) }}）
            </template>
            <template v-else-if="progressOf(item)!.skipped"
              >（跳过 {{ progressOf(item)!.skipped }}）</template
            >
            <!-- Round54：失效对账结果（失效 / 可恢复 / 阵亡 / 自愈） -->
            <template v-if="reconDetail(progressOf(item)!)">
              （对账：{{ reconDetail(progressOf(item)!) }}）
            </template>
          </span>
        </div>
      </div>

      <!-- 无路径时的空状态卡片 -->
      <div v-if="scanPaths.length === 0" class="empty-state">
        暂无额外的扫描路径，请在上方输入框添加
      </div>
    </div>

    <!-- Round54：恢复中状态条 -->
    <div v-if="recoveryStatus.phase === 'recovering'" class="recovery-banner card">
      <span class="recovery-text">
        恢复中：进行中 {{ recoveryStatus.activeTasks }} 个任务（候选剩余
        {{ recoveryStatus.candidateCount }} 本）
      </span>
      <button class="btn btn-mode danger" @click="handleCancelRecovery">全部取消</button>
    </div>

    <!-- Round54：恢复档位弹窗 / 阵亡名单面板 -->
    <RecoveryDialog
      :open="recoveryDialogOpen"
      :candidate-count="recoveryStatus.candidateCount"
      :busy="recoveryBusy"
      @confirm="handleConfirmRecovery"
      @cancel="recoveryDialogOpen = false"
    />
    <FallenMenu
      :open="fallenOpen"
      :items="fallenItems"
      :loading="fallenLoading"
      @dismiss="handleDismissFallen"
      @reclassify="handleReclassifyFallen"
      @close="fallenOpen = false"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, watch, computed } from 'vue'
import AppIcon from '@/components/icons/AppIcon.vue'
import RecoveryDialog from '@/components/settings/RecoveryDialog.vue'
import FallenMenu from '@/components/settings/FallenMenu.vue'
import {
  scanPaths,
  scanProgress,
  fetchScanPaths,
  addScanPath,
  toggleSubfolders,
  updateScanPathName,
  updateEnableOfflineUpdate,
  removeScanPath,
  getScanPathImpact,
  startScanPath,
  refreshScanProgress,
  ensurePolling,
  clearScanProgress,
  getScanProgress,
  hasActiveScan,
  recoveryStatus,
  fallenItems,
  fallenTotal,
  fetchRecoveryStatus,
  fetchFallen,
  startRecovery,
  cancelRecovery,
  dismissFallen,
  reclassifyRecovery,
  type ExtraScanPath,
  type ScanProgress,
  type ScanMode,
  type RecoveryScheme,
} from '@/stores/scanPathStore'
import { fetchOfflineComics } from '@/stores/comicStore'
import { useUI } from '@/composables/useUI'

const { toast, modal } = useUI()

defineEmits<{
  (e: 'back'): void
}>()

// ─────────────────────────────────────────────────────────────
// Round54：恢复与阵亡
// ─────────────────────────────────────────────────────────────

const recoveryDialogOpen = ref(false)
const recoveryBusy = ref(false)
const fallenOpen = ref(false)
const fallenLoading = ref(false)

/** 恢复按钮可点：有候选且未在恢复中 */
const canRecover = computed(
  () => recoveryStatus.value.phase === 'ready' && recoveryStatus.value.candidateCount > 0,
)

const recoverTitle = computed(() => {
  if (recoveryStatus.value.phase === 'recovering') return '恢复任务进行中，请等待完成或取消'
  if (recoveryStatus.value.candidateCount === 0) return '没有可恢复的本子（请先对路径执行全量扫描）'
  return `恢复 ${recoveryStatus.value.candidateCount} 个本子`
})

/** 该路径是否被恢复任务锁定（恢复期间禁止扫描该路径） */
const isPathLocked = (item: ExtraScanPath): boolean =>
  recoveryStatus.value.lockedPathIds.includes(item.id)

// 恢复进度轮询：恢复中每 1s 刷新一次，结束自动停止
let recoveryPollTimer: ReturnType<typeof setInterval> | null = null

function stopRecoveryPolling() {
  if (recoveryPollTimer) {
    clearInterval(recoveryPollTimer)
    recoveryPollTimer = null
  }
}

function startRecoveryPolling() {
  if (recoveryPollTimer) return
  recoveryPollTimer = setInterval(async () => {
    await fetchRecoveryStatus()
    if (recoveryStatus.value.phase !== 'recovering') {
      stopRecoveryPolling()
      // 恢复结束可能已剔除阵亡条目 → 刷新阵亡名单
      void fetchFallen()
    }
  }, 1000)
}

onUnmounted(stopRecoveryPolling)

const openRecovery = () => {
  if (!canRecover.value || recoveryBusy.value) return
  recoveryDialogOpen.value = true
}

const handleConfirmRecovery = async (scheme: RecoveryScheme) => {
  if (recoveryBusy.value) return
  recoveryBusy.value = true
  try {
    const res = await startRecovery(scheme)
    const extra = [res.skipped ? `跳过 ${res.skipped}` : '', res.failed ? `失败 ${res.failed}` : '']
      .filter(Boolean)
      .join('，')
    toast.success(`已启动恢复：创建 ${res.created} 个任务${extra ? `（${extra}）` : ''}`)
    recoveryDialogOpen.value = false
    await fetchRecoveryStatus()
    startRecoveryPolling()
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '启动恢复失败')
  } finally {
    recoveryBusy.value = false
  }
}

const handleCancelRecovery = async () => {
  const confirmed = await modal.confirm(
    '取消全部恢复任务？\n候选清单会保留，之后可再次点击「恢复」重新入队。',
    '取消恢复',
    { tone: 'warn', confirmText: '取消恢复' },
  )
  if (!confirmed) return
  try {
    const res = await cancelRecovery()
    toast.success(`已取消 ${res.cancelled} 个恢复任务`)
    await fetchRecoveryStatus()
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '取消恢复失败')
  }
}

const openFallen = async () => {
  fallenOpen.value = true
  fallenLoading.value = true
  try {
    await fetchFallen()
  } finally {
    fallenLoading.value = false
  }
}

const handleDismissFallen = async (id: number, dismissed: boolean) => {
  try {
    await dismissFallen(id, dismissed)
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '操作失败')
  }
}

// 存量异常任务一键转入阵亡：已取消/出错的恢复任务中 404 等确定性失败直接进阵亡名单（不重新下载）
const handleReclassifyFallen = async () => {
  const confirmed = await modal.confirm(
    '把已取消/出错的恢复任务中「404/下架/解压失败」的条目直接转入阵亡名单？\n这些任务不会重新下载，会直接标记为阵亡；网络类错误（可重试）保持原样。',
    '异常任务转入阵亡',
    { tone: 'warn', confirmText: '确认转入' },
  )
  if (!confirmed) return
  try {
    const res = await reclassifyRecovery()
    toast.success(
      `已转入阵亡 ${res.fallen} 条${res.left ? `，保留 ${res.left} 条可重试任务` : ''}`,
    )
    await Promise.all([fetchFallen(), fetchRecoveryStatus()])
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '操作失败')
  }
}

// 组件挂载时向 Go 后端拉取最新路径列表，并恢复可能仍在进行的扫描进度
// （解决切页后“看起来被截断”的问题：后端扫描在 goroutine 中持续运行，回来时重新拉取）
onMounted(async () => {
  await fetchScanPaths()
  await refreshScanProgress()
  if (hasActiveScan()) ensurePolling()
  // Round54：恢复状态 / 阵亡名单（恢复中则启动进度轮询）
  await Promise.all([fetchRecoveryStatus(), fetchFallen()])
  if (recoveryStatus.value.phase === 'recovering') startRecoveryPolling()
})

// 记录上一轮各路径是否已完成，用于在扫描完成瞬间弹出提示
const prevDone = ref<Record<string, boolean>>({})
watch(
  scanProgress,
  (map) => {
    for (const [id, p] of Object.entries(map)) {
      if (p.done && !prevDone.value[id]) {
        if (p.error) {
          toast.error(`扫描失败：${p.error}`)
        } else {
          toast.success(`扫描完成：${p.comicCount ?? p.found} 本漫画`)
          // Round54：全量扫描含失效对账 → 刷新恢复状态（候选/锁路径）+ 阵亡名单（对账会写入阵亡条目）
          void fetchRecoveryStatus()
          void fetchFallen()
          // 完成后展示几秒，再收起进度提示区
          setTimeout(() => {
            if (getScanProgress(id)?.done) clearScanProgress(id)
          }, 4000)
        }
      }
    }
    prevDone.value = Object.fromEntries(Object.entries(map).map(([id, p]) => [id, !!p.done]))
  },
  { deep: true, immediate: true },
)

const inputPath = ref('')
const inputName = ref('')

// 1. 添加路径（addScanPath 返回 Promise<boolean>，必须 await 才能正确判断）
//    可选填写「显示名」作为来源标签（问题3：卡片/详情页来源栏目展示该名称）
const handleAdd = async () => {
  const path = inputPath.value.trim()
  if (!path) return

  try {
    const ok = await addScanPath(path, inputName.value)
    if (ok) {
      inputPath.value = ''
      inputName.value = ''
      toast.success('路径已添加')
    }
  } catch (err) {
    // addScanPath 现在会抛出后端真实错误（如“该路径已存在”/“未登录”/“会话已失效”），
    // 这里直接透传给用户，避免把一切失败都误报成“该路径已存在”。
    const msg = err instanceof Error ? err.message : '添加失败'
    toast.warning(msg)
  }
}

// 1.1 显示名编辑（问题3）：铅笔进入编辑态，Enter/失去焦点保存，Esc 取消
const editingNameId = ref('')
const editNameValue = ref('')

const startEditName = (item: ExtraScanPath) => {
  editingNameId.value = item.id
  editNameValue.value = item.name || ''
}

const saveName = (item: ExtraScanPath) => {
  if (editingNameId.value !== item.id) return
  updateScanPathName(item.id, editNameValue.value)
  editingNameId.value = ''
  editNameValue.value = ''
}

const cancelEditName = () => {
  editingNameId.value = ''
  editNameValue.value = ''
}

// 2. 切换子文件夹开关
const handleToggleSubfolders = (id: string, value: boolean) => {
  toggleSubfolders(id, value)
}

// 2.1 切换「离线维护」开关（问题4）：该路径是否参与离线更新检测/本地维护查重
const handleToggleOfflineUpdate = (item: ExtraScanPath, value: boolean) => {
  updateEnableOfflineUpdate(item.id, value)
  toast.success(value ? '该路径已开启离线维护' : '该路径已关闭离线维护')
}

// 3. 移除路径：二次确认（显示将连带清理的本数）→ 后端级联清理该路径下已入库的漫画记录。
//    磁盘文件保留：撤出来源 ≠ 销毁文件；需要物理删除请到离线库中单独操作。
const removingId = ref('')

const handleRemove = async (item: ExtraScanPath) => {
  if (removingId.value) return
  removingId.value = item.id
  try {
    // 计数失败不阻断删除，退化为不显示本数
    let impact = 0
    try {
      impact = await getScanPathImpact(item.id)
    } catch (err) {
      console.error('统计路径影响数量失败:', err)
    }

    const confirmed = await modal.confirm(
      impact > 0
        ? `移除扫描路径「${item.path}」吗？\n该路径下已入库的 ${impact} 本漫画将一并从离线库移除。`
        : `移除扫描路径「${item.path}」吗？该路径下暂无已入库漫画。`,
      '移除扫描路径',
      {
        tone: 'danger',
        confirmText: '移除',
        details: '路径下的磁盘文件不会被删除；如需物理删除，请到离线库中单独操作。',
      },
    )
    if (!confirmed) return

    const removed = await removeScanPath(item.id)
    toast.success(removed > 0 ? `已移除路径，并清理 ${removed} 本离线记录` : '路径已移除')
    // 同步刷新离线库内存列表，避免已清理的条目在页面间残留
    void fetchOfflineComics()
  } catch (err) {
    const msg = err instanceof Error ? err.message : '移除路径失败'
    toast.error(msg)
  } finally {
    removingId.value = ''
  }
}

// 4. 单路径扫描（异步启动，进度由轮询实时更新）
const handleSingleScan = async (item: ExtraScanPath, mode: ScanMode) => {
  try {
    await startScanPath(item.id, mode)
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '触发扫描失败')
  }
}

// 5. 进度辅助函数
const progressOf = (item: ExtraScanPath): ScanProgress | undefined => getScanProgress(item.id)

// D2/D4/D17：跳过项摘要（空目录 / 不可读归档 / 已存在 / 同 gid / 写库失败）
const skippedDetail = (p: ScanProgress): string => {
  const parts: string[] = []
  if (p.skippedEmpty) parts.push(`空目录 ${p.skippedEmpty}`)
  if (p.skippedUnsupported) parts.push(`不可读归档 ${p.skippedUnsupported}`)
  if (p.skippedExisting) parts.push(`已存在 ${p.skippedExisting}`)
  if (p.skippedGid) parts.push(`同 gid ${p.skippedGid}`)
  if (p.failed) parts.push(`失败 ${p.failed}`)
  return parts.join(' / ')
}

// Round54：失效对账摘要（失效 / 可恢复 / 阵亡 / 自愈）
const reconDetail = (p: ScanProgress): string => {
  const parts: string[] = []
  if (p.missingCount) parts.push(`失效 ${p.missingCount}`)
  if (p.candidateCount) parts.push(`可恢复 ${p.candidateCount}`)
  if (p.fallenCount) parts.push(`阵亡 ${p.fallenCount}`)
  if (p.healedCount) parts.push(`自愈 ${p.healedCount}`)
  return parts.join(' / ')
}

const isScanning = (item: ExtraScanPath): boolean => {
  const p = progressOf(item)
  return !!p && !p.done
}

const scanModeText = (item: ExtraScanPath): string => {
  const p = progressOf(item)
  return p?.mode === 'incremental' ? '增量更新中' : '全量扫描中'
}

const progressPercent = (p: ScanProgress): number => {
  if (p.phase !== 'scanning' || !p.total) return 0
  return Math.min(100, Math.round((p.processed / p.total) * 100))
}

const formatTime = (ts: number) => {
  const d = new Date(ts)
  return `${d.getMonth() + 1}/${d.getDate()} ${d.getHours().toString().padStart(2, '0')}:${d.getMinutes().toString().padStart(2, '0')}`
}
</script>

<style scoped>
.extra-scan-settings {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

/* 顶部 Header */
.header {
  display: flex;
  align-items: center;
  gap: 12px;
  padding-bottom: 12px;
  border-bottom: 1px solid var(--app-border-2);
}

.back-btn {
  background: var(--app-surface-2);
  border: 1px solid var(--app-border-2);
  color: var(--app-text-2);
  font-size: 24px;
  line-height: 1;
  width: 36px;
  height: 36px;
  border-radius: 8px;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: all 0.2s;
}

.back-btn:hover {
  background: var(--app-surface-2-hover);
  color: var(--app-text-strong);
}

.header-title {
  font-size: 16px;
  font-weight: 600;
  color: var(--app-text-strong);
}

.header-subtext {
  font-size: 12px;
  color: var(--app-text-3);
  margin-top: 2px;
}

/* 卡片容器 */
.card {
  background-color: var(--app-surface-2);
  border: 1px solid var(--app-border-2);
  border-radius: 8px;
  padding: 14px 16px;
}

/* 添加路径卡片 */
.add-card {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.input-label {
  font-size: 13px;
  color: var(--app-text-2);
}

.input-group {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
}

.setting-input {
  flex: 1;
  background-color: var(--app-input-bg);
  border: 1px solid var(--app-border-3);
  border-radius: 6px;
  padding: 8px 12px;
  font-size: 13px;
  color: var(--app-text-strong);
  outline: none;
  transition: border-color 0.2s;
}

.setting-input:focus {
  border-color: #ff7588;
}

/* 路径列表卡片 */
.path-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.path-item {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.path-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.path-info {
  min-width: 0;
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.path-title {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.folder-icon {
  font-size: 14px;
  opacity: 0.8;
}

.path-text {
  font-size: 14px;
  font-weight: 500;
  font-family: monospace;
  color: var(--app-text-strong);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* 显示名（来源标签）相关样式（问题3） */
.name-input {
  flex: 1 1 100%;
}

.name-edit-input {
  flex: 1;
  min-width: 120px;
  background-color: var(--app-input-bg);
  border: 1px solid #3d5afe;
  border-radius: 6px;
  padding: 4px 8px;
  font-size: 12px;
  color: var(--app-text-strong);
  outline: none;
}

.path-name-badge {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  background-color: rgba(61, 90, 254, 0.2);
  border: 1px solid rgba(61, 90, 254, 0.5);
  color: #9db0ff;
  font-size: 12px;
  padding: 1px 8px;
  border-radius: 10px;
  white-space: nowrap;
}

.name-edit-btn {
  background: transparent;
  border: none;
  color: #9db0ff;
  cursor: pointer;
  font-size: 11px;
  padding: 0;
  line-height: 1;
}

.name-set-btn {
  background: transparent;
  border: 1px dashed #4a4a52;
  color: #7a7a82;
  border-radius: 10px;
  font-size: 12px;
  padding: 1px 8px;
  cursor: pointer;
  white-space: nowrap;
}

.name-set-btn:hover {
  border-color: #3d5afe;
  color: #9db0ff;
}

.path-meta {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  color: var(--app-text-3);
}

.status-unscanned {
  color: #e6a23c;
}

.count-text {
  color: var(--app-text-3);
}

.scan-badge {
  font-size: 11px;
  color: #ff7588;
  border: 1px solid rgba(255, 117, 136, 0.4);
  border-radius: 4px;
  padding: 1px 6px;
  white-space: nowrap;
}

.path-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  row-gap: 6px;
  flex-shrink: 0;
}

/* Toggle 开关 */
.toggle-container {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
  user-select: none;
}

.toggle-label {
  font-size: 12px;
  color: var(--app-text-2);
}

.toggle-switch {
  position: relative;
  display: inline-block;
  width: 38px;
  height: 20px;
}

.toggle-switch input {
  opacity: 0;
  width: 0;
  height: 0;
}

.slider {
  position: absolute;
  cursor: pointer;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background-color: var(--app-border-3);
  transition: 0.3s;
  border-radius: 20px;
}

.slider:before {
  position: absolute;
  content: '';
  height: 14px;
  width: 14px;
  left: 3px;
  bottom: 3px;
  background-color: var(--app-text-2);
  transition: 0.3s;
  border-radius: 50%;
}

input:checked + .slider {
  background-color: #ff7588;
}

input:checked + .slider:before {
  transform: translateX(18px);
  background-color: #ffffff;
}

/* 按钮基础及变体样式 */
.btn {
  border: none;
  border-radius: 6px;
  padding: 6px 14px;
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
  transition:
    background-color 0.2s,
    opacity 0.2s;
}

.btn-primary {
  background-color: #ff7588;
  color: #ffffff;
}

.btn-primary:hover {
  background-color: #ff5e74;
}

.btn-primary:disabled {
  background-color: var(--app-surface-3);
  color: var(--app-text-muted);
  cursor: not-allowed;
}

.btn-icon {
  background: transparent;
  color: var(--app-text-2);
  padding: 6px 8px;
  font-size: 15px;
}

.btn-icon:hover {
  background-color: var(--app-surface-3);
  color: var(--app-text-strong);
}

.btn-icon.danger:hover {
  background-color: rgba(255, 77, 79, 0.15);
  color: #ff4d4f;
}

/* 扫描模式按钮 */
.btn-mode {
  background: var(--app-input-bg);
  border: 1px solid var(--app-border-3);
  color: var(--app-text-2);
  padding: 5px 10px;
  font-size: 12px;
}

.btn-mode:hover {
  border-color: #ff7588;
  color: #ff7588;
}

.btn-mode.incremental:hover {
  border-color: #4fc3f7;
  color: #4fc3f7;
}

/* Round54：恢复 / 阵亡按钮 */
.btn-mode:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.btn-mode.recovery {
  border-color: rgba(255, 117, 136, 0.5);
  color: #ff7588;
}

.btn-mode.recovery:hover:not(:disabled) {
  border-color: #ff7588;
  background: rgba(255, 117, 136, 0.12);
}

.btn-mode.fallen:hover:not(:disabled) {
  border-color: #f0b429;
  color: #f0b429;
}

.btn-mode.danger {
  border-color: rgba(255, 117, 136, 0.4);
  color: #ff7588;
}

/* Round54：路径不存在提示（建议恢复，不提示移除路径） */
.path-missing {
  font-size: 12px;
  color: #f0b429;
}

/* Round54：恢复中状态条 */
.recovery-banner {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  border-color: rgba(255, 117, 136, 0.3);
}

.recovery-text {
  font-size: 13px;
  color: var(--app-text-2);
}

/* 旋转动画 */
.spinning {
  display: inline-block;
  animation: spin 1s linear infinite;
}

@keyframes spin {
  from {
    transform: rotate(0deg);
  }
  to {
    transform: rotate(360deg);
  }
}

/* 扫描进度区域 */
.scan-progress-area {
  border: 1px solid rgba(255, 117, 136, 0.25);
  background-color: rgba(255, 117, 136, 0.05);
  border-radius: 6px;
  padding: 10px 12px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.progress-counting {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  color: #ffb6c1;
}

.progress-bar {
  height: 6px;
  background-color: var(--app-surface-3);
  border-radius: 4px;
  overflow: hidden;
}

.progress-fill {
  height: 100%;
  background: linear-gradient(90deg, #ff7588, #ff5e74);
  border-radius: 4px;
  transition: width 0.3s ease;
}

.progress-meta {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  font-size: 12px;
  color: var(--app-text-2);
}

.progress-title {
  min-width: 0;
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.progress-num {
  flex-shrink: 0;
  color: #ffb6c1;
  font-variant-numeric: tabular-nums;
}

/* 完成提示区域 */
.scan-done-area {
  border: 1px solid rgba(82, 196, 26, 0.3);
  background-color: rgba(82, 196, 26, 0.06);
  border-radius: 6px;
  padding: 8px 12px;
  font-size: 13px;
  color: #95de64;
}

.scan-done-area.error {
  border-color: rgba(255, 77, 79, 0.3);
  background-color: rgba(255, 77, 79, 0.06);
  color: #ff7875;
}

/* 空状态 */
.empty-state {
  text-align: center;
  padding: 32px 16px;
  border: 1px dashed var(--app-border-2);
  border-radius: 8px;
  color: var(--app-text-muted);
  font-size: 13px;
}
</style>
