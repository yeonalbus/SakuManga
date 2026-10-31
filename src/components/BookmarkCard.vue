<script setup lang="ts">
// 书签名片（Round49 v4）
//
// 结构（固定高度，内容变化不推挤卡片）：
//   a 标题行 24px：主标题（1 行省略）+ 右上角 ⚙ 配置菜单
//   b 副标题 18px：位置字段 + 非默认筛选（1 行）
//   c 徽章行 22px：追新/续扫/兴趣 · 🧾已囤货 · 🔔更新探测（仅状态展示）
//   d 备注框 76px：带边框，右上角 ✏️ 编辑
// 有更新 → 金色框；书签标记 → 左侧 3px 竖色条（追新蓝 / 续扫绿 / 兴趣灰）。
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import {
  bookmarkDisplayTitle,
  updateBookmarkFields,
  removeScrapeBookmark,
  renameScrapeBookmark,
  PINNED_BOOKMARK_LIMIT,
} from '@/stores/scrapeBookmarksStore'
import { bookmarkTitleWith } from '@/utils/bookmarkTitle'
import { useTagStore } from '@/stores/tagStore'
import { useUI } from '@/composables/useUI'
import { isStandalonePWA } from '@/utils/detailNav'
import type { ScrapeBookmark, ScrapeBookmarkRole } from '@/types/comic'

const props = defineProps<{
  bm: ScrapeBookmark
  /** 当前已置顶数量（用于置顶上限校验） */
  pinnedCount: number
}>()

const emit = defineEmits<{
  (e: 'edit-note', bm: ScrapeBookmark): void
}>()

const router = useRouter()
const tagStore = useTagStore()
const { toast, modal } = useUI()

const menuOpen = ref(false)
const rootEl = ref<HTMLElement | null>(null)

const roleLabels: Record<ScrapeBookmarkRole, string> = {
  newest: '追新',
  interest: '兴趣',
  oldest: '续扫',
}
/** 菜单里的展示顺序：追新 | 兴趣 | 续扫 */
const roleChoices: ScrapeBookmarkRole[] = ['newest', 'interest', 'oldest']

/** 主标题：自定义名称 > 翻译名 > (artist) tag key > 副标题内容 */
const title = computed(() => bookmarkTitleWith(props.bm, tagStore.translate))
/** 副标题：永远显示位置 + 非默认筛选（与主标题相同也保留） */
const subtitle = computed(() => bookmarkDisplayTitle(props.bm))

/** 点击卡片：跳转在线列表并定位锚点 */
const jump = () => {
  if (menuOpen.value) return
  const routeObj = { path: '/online/home', query: { bm: props.bm.id } }
  if (isStandalonePWA()) {
    router.push(routeObj).catch(() => {})
  } else {
    window.open(router.resolve(routeObj).href, '_blank')
  }
}

const closeMenu = () => {
  menuOpen.value = false
}

const togglePin = async () => {
  closeMenu()
  if (!props.bm.pinned && props.pinnedCount >= PINNED_BOOKMARK_LIMIT) {
    toast.warning(`置顶已达上限（${PINNED_BOOKMARK_LIMIT} 条），请先取消其他置顶`)
    return
  }
  const ok = await updateBookmarkFields(props.bm.id, { pinned: !props.bm.pinned })
  if (ok) toast.success(props.bm.pinned ? '已取消置顶' : '已置顶到侧栏')
}

const toggleWatch = async () => {
  closeMenu()
  const ok = await updateBookmarkFields(props.bm.id, { updateWatch: !props.bm.updateWatch })
  if (ok) toast.success(props.bm.updateWatch ? '已关闭更新探测' : '已开启更新探测')
}

const toggleBulk = async () => {
  closeMenu()
  const ok = await updateBookmarkFields(props.bm.id, { bulkDone: !props.bm.bulkDone })
  if (ok) toast.success(props.bm.bulkDone ? '已取消已囤货标记' : '已标记为已囤货')
}

/** 设置书签标记（三项直接切换，点完不关菜单） */
const setRole = async (role: ScrapeBookmarkRole) => {
  if (props.bm.role === role) return
  const ok = await updateBookmarkFields(props.bm.id, { role })
  if (ok) toast.success(`标记已改为「${roleLabels[role]}」`)
}

/** 修改名称（留空恢复默认显示） */
const rename = async () => {
  closeMenu()
  const val = await modal.prompt(
    '输入自定义名称（留空则恢复默认显示）',
    props.bm.name,
    '修改名称',
  )
  if (val === null || val === undefined) return
  const ok = await renameScrapeBookmark(props.bm.id, val.trim())
  if (ok) toast.success(val.trim() ? '名称已保存' : '已恢复默认名称')
}

const remove = async () => {
  closeMenu()
  const name = title.value
  const ok = await modal.confirm(`确定删除书签「${name}」吗？`, '删除书签')
  if (!ok) return
  if (await removeScrapeBookmark(props.bm.id)) toast.success('书签已删除')
}

const onDocClick = (e: MouseEvent) => {
  if (!menuOpen.value) return
  const el = rootEl.value
  if (el && !el.contains(e.target as Node)) closeMenu()
}

onMounted(() => document.addEventListener('click', onDocClick))
onBeforeUnmount(() => document.removeEventListener('click', onDocClick))
</script>

<template>
  <article
    ref="rootEl"
    class="bm-card"
    :class="[`role-${bm.role}`, { updated: bm.hasUpdate }]"
    @click="jump"
  >
    <div class="bm-cover">
      <img v-if="bm.anchor?.coverUrl" :src="bm.anchor.coverUrl" alt="" loading="lazy" />
      <span v-else class="bm-cover-placeholder">🖼</span>
    </div>

    <div class="bm-body">
      <!-- a：标题行 -->
      <div class="bm-title-row">
        <div class="bm-title" :title="title">{{ title }}</div>
        <button class="bm-menu-btn" title="配置菜单" @click.stop="menuOpen = !menuOpen">
          ⚙
        </button>
      </div>

      <!-- b：副标题（永远显示位置 + 非默认筛选） -->
      <div class="bm-subtitle" :title="subtitle">{{ subtitle }}</div>

      <!-- c：徽章行（状态展示） -->
      <div class="bm-badges">
        <span class="bm-badge" :class="`role-${bm.role}`">{{ roleLabels[bm.role] }}</span>
        <span v-if="bm.bulkDone" class="bm-badge bulk">🧾 已囤货</span>
        <span v-if="bm.updateWatch" class="bm-badge watch">🔔</span>
      </div>

      <!-- d：备注框（边框 + 右上角编辑按钮） -->
      <div class="bm-note-box" :class="{ empty: !bm.note }" @click.stop>
        {{ bm.note || '（暂无备注）' }}
        <button class="bm-note-edit" title="编辑备注" @click.stop="emit('edit-note', bm)">
          ✏️
        </button>
      </div>

      <!-- 配置菜单 -->
      <div v-if="menuOpen" class="bm-menu" @click.stop>
        <div class="bm-menu-item" @click="rename">✏️ 修改名称</div>
        <div class="bm-menu-item" @click="togglePin">
          📌 {{ bm.pinned ? '取消置顶' : '置顶到侧栏' }}
        </div>
        <div class="bm-menu-item" @click="toggleWatch">
          🔔 {{ bm.updateWatch ? '关闭更新探测' : '开启更新探测' }}
        </div>
        <div class="bm-menu-item" @click="toggleBulk">
          🧾 {{ bm.bulkDone ? '取消已囤货' : '标记已囤货' }}
        </div>
        <div class="bm-menu-sep" />
        <div class="bm-menu-role-label">书签标记</div>
        <div class="bm-menu-role">
          <button
            v-for="r in roleChoices"
            :key="r"
            class="bm-role-chip"
            :class="[r, { active: bm.role === r }]"
            @click.stop="setRole(r)"
          >
            {{ roleLabels[r] }}
          </button>
        </div>
        <div class="bm-menu-sep" />
        <div class="bm-menu-item danger" @click="remove">🗑 删除书签</div>
      </div>
    </div>
  </article>
</template>

<style scoped>
/* 固定高度：封面 160 + 上下 padding 12×2 = 184 */
.bm-card {
  position: relative;
  display: flex;
  gap: 10px;
  height: 184px;
  padding: 12px;
  background-color: var(--app-surface-2);
  border: 1px solid var(--app-border-3);
  border-radius: 10px;
  cursor: pointer;
  transition:
    border-color 0.15s ease,
    box-shadow 0.15s ease;
}
.bm-card:hover {
  border-color: var(--app-text-muted);
}

/* 有更新：金色框（不再用红点） */
.bm-card.updated {
  border-color: #e6b800;
  box-shadow: 0 0 0 1px rgba(230, 184, 0, 0.18);
}

/* 书签标记：左侧 3px 竖色条（与金框并存，不抢更新信号） */
.bm-card::before {
  content: '';
  position: absolute;
  left: 0;
  top: 0;
  bottom: 0;
  width: 3px;
  background: transparent;
}
.bm-card.role-newest::before {
  background: #4fa3e3;
}
.bm-card.role-oldest::before {
  background: #4cc38a;
}
.bm-card.role-interest::before {
  background: #5a5a5a;
}

/* 封面：3:4，固定 120×160 */
.bm-cover {
  position: relative;
  flex-shrink: 0;
  width: 120px;
  height: 160px;
  border-radius: 6px;
  overflow: hidden;
  background: linear-gradient(160deg, #2a2a2a, #222);
  display: flex;
  align-items: center;
  justify-content: center;
}
.bm-cover img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
}
.bm-cover-placeholder {
  font-size: 30px;
  color: #555;
}

.bm-body {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
}

/* a：标题行 24px */
.bm-title-row {
  height: 24px;
  display: flex;
  align-items: flex-start;
  gap: 6px;
}
.bm-title {
  flex: 1;
  min-width: 0;
  font-size: 1.02rem;
  font-weight: 700;
  line-height: 20px;
  color: var(--app-text-strong);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.bm-menu-btn {
  flex-shrink: 0;
  width: 24px;
  height: 24px;
  display: flex;
  align-items: center;
  justify-content: center;
  border: none;
  border-radius: 5px;
  background: transparent;
  color: var(--app-text-3);
  font-size: 0.9rem;
  cursor: pointer;
}
.bm-menu-btn:hover {
  background-color: var(--app-surface-3);
  color: var(--app-text-strong);
}

/* b：副标题 18px */
.bm-subtitle {
  height: 18px;
  margin-top: 3px;
  font-size: 0.74rem;
  line-height: 18px;
  color: var(--app-text-muted);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* c：徽章行 22px */
.bm-badges {
  height: 22px;
  margin-top: 7px;
  display: flex;
  align-items: center;
  gap: 5px;
  overflow: hidden;
}
.bm-badge {
  font-size: 0.68rem;
  line-height: 1.5;
  padding: 0 5px;
  border-radius: 4px;
  border: 1px solid var(--app-border-3);
  color: var(--app-text-2);
  white-space: nowrap;
  flex-shrink: 0;
}
.bm-badge.role-newest {
  border-color: rgba(79, 163, 227, 0.45);
  color: #4fa3e3;
}
.bm-badge.role-oldest {
  border-color: rgba(76, 195, 138, 0.45);
  color: #4cc38a;
}
.bm-badge.bulk {
  border-color: rgba(230, 184, 0, 0.4);
  color: #e6b800;
}

/* d：备注框 76px（3 行） */
.bm-note-box {
  position: relative;
  height: 76px;
  margin-top: 8px;
  padding: 7px 30px 7px 9px;
  border: 1px solid var(--app-border-2);
  border-radius: 8px;
  background-color: rgba(255, 255, 255, 0.02);
  font-size: 0.75rem;
  line-height: 1.55;
  color: var(--app-text-2);
  word-break: break-all;
  display: -webkit-box;
  -webkit-line-clamp: 3;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.bm-note-box.empty {
  color: var(--app-text-muted);
}
.bm-note-edit {
  position: absolute;
  top: 3px;
  right: 3px;
  width: 22px;
  height: 22px;
  display: flex;
  align-items: center;
  justify-content: center;
  border: none;
  border-radius: 5px;
  background: transparent;
  color: var(--app-text-muted);
  font-size: 0.75rem;
  cursor: pointer;
}
.bm-note-edit:hover {
  background-color: var(--app-surface-3);
  color: var(--app-text-strong);
}

/* 配置菜单 */
.bm-menu {
  position: absolute;
  top: 42px;
  right: 12px;
  width: 210px;
  background-color: var(--app-surface-2);
  border: 1px solid var(--app-border-3);
  border-radius: 8px;
  box-shadow: 0 10px 28px rgba(0, 0, 0, 0.55);
  padding: 4px;
  z-index: 5;
}
.bm-menu-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 7px 10px;
  border-radius: 5px;
  font-size: 0.8rem;
  color: var(--app-text-2);
  cursor: pointer;
  min-width: 0;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.bm-menu-item:hover {
  background-color: var(--app-surface-3);
  color: var(--app-text-strong);
}
.bm-menu-item.danger {
  color: #ef4444;
}
.bm-menu-item.danger:hover {
  background-color: rgba(239, 68, 68, 0.14);
  color: #ef4444;
}
.bm-menu-sep {
  height: 1px;
  background-color: var(--app-border-2);
  margin: 4px 6px;
}

/* 菜单里的书签标记三项 chips */
.bm-menu-role-label {
  padding: 4px 10px 2px;
  font-size: 0.72rem;
  color: var(--app-text-3);
}
.bm-menu-role {
  display: flex;
  gap: 4px;
  padding: 2px 10px 6px;
}
.bm-role-chip {
  flex: 1;
  padding: 4px 0;
  border-radius: 4px;
  border: 1px solid var(--app-border-3);
  background: transparent;
  color: var(--app-text-3);
  font-size: 0.74rem;
  font-family: inherit;
  cursor: pointer;
  transition: all 0.15s ease;
}
.bm-role-chip:hover {
  color: var(--app-text-strong);
}
.bm-role-chip.newest.active {
  border-color: #4fa3e3;
  color: #4fa3e3;
  background: rgba(79, 163, 227, 0.12);
}
.bm-role-chip.oldest.active {
  border-color: #4cc38a;
  color: #4cc38a;
  background: rgba(76, 195, 138, 0.12);
}
.bm-role-chip.interest.active {
  border-color: var(--app-text-muted);
  color: var(--app-text-2);
  background: rgba(255, 255, 255, 0.06);
}
</style>
