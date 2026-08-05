<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useUI } from '@/composables/useUI'
import { bookshelves, addComicToShelf, removeComicFromShelf } from '@/stores/bookshelfStore'
import { offlineReadingList, toggleReadingList } from '@/stores/readingStore'
import { getMyRating, setMyRating } from '@/stores/ratingStore'
import { fetchOfflineComics, recordComicClick, deleteOfflineComics } from '@/stores/comicStore'
import type { OfflineComic } from '@/types/comic'
// 🎯 核心引入：直接复用 TagChip 组件以支持全局字典翻译与配色
import TagChip from '@/components/TagChip.vue'
import { http } from '@/utils/request'

// 后端 GetOfflineComicDetail 返回的离线漫画 DTO
// tags 字段可能是 JSON 字符串，也可能是字符串数组，需在运行时归一化
interface OfflineDetailDTO {
  id: string
  title: string
  coverUrl: string
  category?: string
  tags?: string[] | string
  tagRaws?: string[]
  tagSources?: ('online' | 'local')[]
  onlineTagsList?: string[]
  offlineAddTagsList?: string[]
  offlineRemoveTagsList?: string[]
  rating?: number
  pageCount?: number
  updatedAt: string
  localPath: string
  fileSize?: number
  readCount?: number
}

const route = useRoute()
const router = useRouter()
const { toast, modal } = useUI()

const comic = ref<OfflineComic>({
  id: (route.query.id as string) || '',
  title: '加载中...',
  coverUrl: '',
  source: 'offline',
  category: 'Local',
  tags: [],
  rating: 0,
  pageCount: 0,
  updatedAt: '',
  localPath: '',
  fileSize: 0,
  readCount: 0,
})

// 双轨三态展示辅助数据（与后端 GetOfflineComicDetail 返回对应）
const tagRaws = ref<string[]>([])
const tagSources = ref<('online' | 'local')[]>([])
const onlineTagsList = ref<string[]>([])
const offlineAddTagsList = ref<string[]>([])

// 兜底：由 TagItem/原始 tag 反查原始字符串（优先使用后端返回的 tagRaws）
const rawTagOf = (t: string | { namespace?: string; key?: string }): string => {
  if (typeof t === 'string') return t
  const ns = (t.namespace || '').toLowerCase()
  return ns && ns !== 'other' ? `${ns}:${t.key}` : t.key || ''
}

// 该 tag 属于官方(online) 还是 本地新增(local)
const tagSource = (idx: number): 'online' | 'local' => tagSources.value[idx] || 'online'
const tagClass = (idx: number) => (tagSource(idx) === 'local' ? 'tag-local' : 'tag-official')

// 向 Go 后端拉取单本漫画真实数据
const fetchComicDetail = async () => {
  const comicId = route.query.id as string
  if (!comicId) return

  try {
    // 🟢 1. 使用 http 请求，省去 /api/v1 前缀和 res.json()
    const data = await http<OfflineDetailDTO>(`/comics/${comicId}`)

    // 🟢 2. 解析 tags 数组逻辑保持不变
    let parsedTags: string[] = []
    if (typeof data.tags === 'string') {
      try {
        parsedTags = JSON.parse(data.tags)
      } catch {
        parsedTags = []
      }
    } else if (Array.isArray(data.tags)) {
      parsedTags = data.tags
    }

    comic.value = {
      ...data,
      source: 'offline',
      tags: parsedTags,
    }
    myLocalRating.value = getMyRating(comicId)
    // 三态展示辅助数据
    tagRaws.value = data.tagRaws || []
    tagSources.value = data.tagSources || []
    onlineTagsList.value = data.onlineTagsList || []
    offlineAddTagsList.value = data.offlineAddTagsList || []
  } catch (err) {
    console.error('获取漫画详情失败:', err)
    const msg = err instanceof Error ? err.message : ''
    if (/找不到该漫画|not found|404/i.test(msg)) {
      // 漫画 id 不在本地库：可能是扫描数据被重建 / 列表缓存过期，刷新离线列表
      toast.error('找不到该漫画，可能已从本地库移除，已刷新离线列表')
      fetchOfflineComics()
    } else {
      toast.error('连接后端失败')
    }
  }
}

onMounted(() => {
  fetchComicDetail()
})

const getCategoryColor = (cat?: string) => {
  switch (cat) {
    case 'Doujinshi':
      return '#ff7588'
    case 'Manga':
      return '#ff9800'
    case 'Artist CG':
      return '#e91e63'
    case 'Game CG':
      return '#4caf50'
    case 'Non-H':
      return '#2196f3'
    default:
      return '#9e9e9e'
  }
}

const isInReadingList = computed(() =>
  offlineReadingList.value.some((item) => item.id === comic.value.id),
)

const handleAddToReadingList = () => {
  toggleReadingList(comic.value)
  if (isInReadingList.value) {
    toast.success(`已将《${comic.value.title}》加入本地阅读清单 📑`)
  } else {
    toast.info(`已从本地阅读清单中移除《${comic.value.title}》`)
  }
}

const handleBack = () => {
  if (window.history.length > 1) {
    router.back()
  } else {
    router.push('/offline/home')
  }
}

const formattedSize = computed(() => {
  if (!comic.value.fileSize) return '未知'
  const mb = comic.value.fileSize / (1024 * 1024)
  return `${mb.toFixed(1)} MB`
})

const myLocalRating = ref(0)
const setRating = (star: number) => {
  myLocalRating.value = star
  comic.value.rating = star
  setMyRating(comic.value.id, star)
  toast.success(`个人打分更新为 ${star} 星`)
}

const copyPath = () => {
  navigator.clipboard.writeText(comic.value.localPath)
  toast.info('本地文件路径已复制到剪贴板')
}

const handleAddTag = async () => {
  const newTag = await modal.prompt(
    '请输入要新增的 Tag 名称（如 artist:xxx）：',
    '',
    '新增本地标签',
  )
  if (!newTag || !newTag.trim()) return
  const trimmed = newTag.trim()
  // 本地新增 tag 走双轨模型：加入 OfflineAddTags（仅本地客制化，不写回）
  try {
    await http(`/comics/${comic.value.id}/tags`, {
      method: 'PUT',
      body: JSON.stringify({ addTags: [trimmed], removeTags: [] }),
    })
    toast.success(`标签「${trimmed}」添加成功！`)
    await fetchComicDetail()
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '添加标签失败')
  }
}

const handleRemoveTag = async (index: number) => {
  // 优先使用后端返回的原始 tag 字符串精确匹配（官方/本地都能正确剔除）
  const raw = tagRaws.value[index] || rawTagOf(comic.value.tags[index] as never)
  if (!raw) return
  try {
    await http(`/comics/${comic.value.id}/tags`, {
      method: 'PUT',
      body: JSON.stringify({ addTags: [], removeTags: [raw] }),
    })
    toast.info(`已移除标签「${raw}」`)
    await fetchComicDetail()
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '移除标签失败')
  }
}

const toggleShelfCheck = async (shelfId: string) => {
  const shelf = bookshelves.value.find((s) => s.id === shelfId)
  if (!shelf) return

  const inShelf = (shelf.comicIds || []).includes(comic.value.id)

  if (inShelf) {
    await removeComicFromShelf(shelfId, comic.value.id)
    toast.info(`从书架「${shelf.name}」中移出`)
  } else {
    await addComicToShelf(shelfId, comic.value.id)
    toast.success(`已加入书架「${shelf.name}」`)
  }
}

const isComicInShelf = (shelfId: string) => {
  const shelf = bookshelves.value.find((s) => s.id === shelfId)
  return shelf?.comicIds?.includes(comic.value.id) ?? false
}

const handleStartReading = () => {
  if (!comic.value || !comic.value.id) return
  recordComicClick(comic.value.id)
  comic.value.readCount = (comic.value.readCount || 0) + 1
  router.push(`/reader?id=${comic.value.id}&source=offline`)
}

const deleting = ref(false)

// 删除本地画廊：确认后询问是否同时删除本地文件
const handleDelete = async () => {
  if (!comic.value.id || deleting.value) return

  const confirmed = await modal.confirm(
    `确定要删除《${comic.value.title}》吗？\n将同时移除书架与历史记录中的引用。`,
    '删除本地画廊',
  )
  if (!confirmed) return

  // 询问是否同时物理删除本地文件（取消 = 仅删除记录）
  const alsoDeleteFile = await modal.confirm(
    '是否同时删除本地文件？\n选择「确定」将永久删除磁盘上的漫画文件，无法恢复。',
    '删除本地文件',
  )

  deleting.value = true
  try {
    const okCount = await deleteOfflineComics([comic.value.id], alsoDeleteFile)
    if (okCount > 0) {
      toast.success(alsoDeleteFile ? '已删除漫画记录及本地文件' : '已删除漫画记录')
      handleBack()
    } else {
      toast.error('删除失败，请重试')
    }
  } finally {
    deleting.value = false
  }
}
</script>

<template>
  <div class="offline-detail-page">
    <div class="top-bar">
      <button class="back-btn" @click="handleBack">‹ 返回</button>
      <div class="right-actions">
        <button
          class="add-reading-btn"
          :class="{ active: isInReadingList }"
          @click="handleAddToReadingList"
        >
          {{ isInReadingList ? '✓ 已在清单' : '📑 加入清单' }}
        </button>

        <button class="read-btn" @click="handleStartReading">
          📖 继续阅读 (已读 {{ comic.readCount || 0 }} 次)
        </button>

        <button class="delete-btn" :disabled="deleting" @click="handleDelete">
          {{ deleting ? '删除中…' : '🗑️ 删除' }}
        </button>
      </div>
    </div>

    <div class="main-layout">
      <div class="left-cover">
        <img :src="comic.coverUrl" :alt="comic.title" />
      </div>

      <div class="right-panel">
        <div v-if="comic.category" class="category-wrapper">
          <span
            class="category-badge"
            :style="{ backgroundColor: getCategoryColor(comic.category) }"
          >
            {{ comic.category }}
          </span>
        </div>

        <h1 class="title">{{ comic.title }}</h1>

        <div class="rating-box">
          <span class="label">我的个人评分：</span>
          <div class="stars">
            <span
              v-for="star in 5"
              :key="star"
              class="star-icon"
              :class="{ active: star <= myLocalRating }"
              @click="setRating(star)"
            >
              ★
            </span>
          </div>
          <span class="rating-num">({{ myLocalRating }} / 5.0)</span>
        </div>

        <div class="info-card">
          <div class="card-header">
            <h3 class="card-title">🏷️ 本地作品标签 (Tags)</h3>
            <button class="add-tag-btn" @click="handleAddTag">➕ 添加 Tag</button>
          </div>

          <!-- 🎯 替换为 TagChip 组件 + 独立删除按钮组合 -->
          <div class="tags-cloud">
            <div
              v-for="(tag, idx) in comic.tags"
              :key="`${tagRaws[idx] || idx}-${idx}`"
              class="detail-tag-item"
              :class="tagClass(idx)"
            >
              <TagChip :tag="tag" />
              <span v-if="tagSource(idx) === 'local'" class="local-badge" title="本地新增标签">
                本地
              </span>
              <span class="remove-tag" title="删除此标签" @click.stop="handleRemoveTag(idx)">
                ✕
              </span>
            </div>

            <span v-if="!comic.tags || comic.tags.length === 0" class="empty-tag-tip">
              暂无标签，点击右上方按钮添加...
            </span>
          </div>
        </div>

        <div class="info-card">
          <h3 class="card-title">💾 本地文件属性</h3>
          <div class="info-grid">
            <div class="info-item">
              <span class="k">文件大小:</span>
              <span class="v">{{ formattedSize }}</span>
            </div>
            <div class="info-item">
              <span class="k">总页数:</span>
              <span class="v">{{ comic.pageCount }} 页</span>
            </div>
            <div class="info-item">
              <span class="k">最后扫描:</span>
              <span class="v">{{ comic.updatedAt }}</span>
            </div>
            <div class="info-item full">
              <span class="k">存储路径:</span>
              <span class="v path-text">{{ comic.localPath }}</span>
              <button class="copy-btn" @click="copyPath">复制</button>
            </div>
          </div>
        </div>

        <div class="info-card">
          <h3 class="card-title">📚 归属本地书架 (可多选)</h3>
          <div class="bookshelves-list">
            <div
              v-for="shelf in bookshelves"
              :key="shelf.id"
              class="shelf-checkbox-item"
              :class="{ checked: isComicInShelf(shelf.id) }"
              @click="toggleShelfCheck(shelf.id)"
            >
              <input type="checkbox" :checked="isComicInShelf(shelf.id)" readonly />
              <span class="shelf-name">{{ shelf.name }}</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.offline-detail-page {
  padding: 20px;
  max-width: 1000px;
  margin: 0 auto;
  color: #e0e0e0;
}

.top-bar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 20px;
}

.back-btn {
  background: transparent;
  border: 1px solid #333;
  color: #aaa;
  padding: 6px 14px;
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.2s;
}
.back-btn:hover {
  color: #fff;
  border-color: #555;
}

.read-btn {
  background: #00a896;
  color: #fff;
  border: none;
  padding: 8px 20px;
  border-radius: 6px;
  font-weight: bold;
  cursor: pointer;
}

.main-layout {
  display: flex;
  gap: 28px;
}

.left-cover img {
  width: 240px;
  border-radius: 8px;
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.6);
  border: 1px solid #333;
}

.right-panel {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.title {
  font-size: 1.3rem;
  margin: 0;
  color: #fff;
}

.rating-box {
  display: flex;
  align-items: center;
  gap: 10px;
  background: #1a1a1d;
  padding: 10px 14px;
  border-radius: 6px;
  border: 1px solid #2a2a2d;
}

.stars {
  display: flex;
  gap: 4px;
  cursor: pointer;
}
.star-icon {
  font-size: 1.2rem;
  color: #444;
  transition: color 0.15s;
}
.star-icon.active {
  color: #ffc107;
}
.rating-num {
  color: #888;
  font-size: 0.85rem;
}

.info-card {
  background: #1a1a1d;
  border: 1px solid #2a2a2d;
  border-radius: 8px;
  padding: 16px;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
}

.card-title {
  font-size: 0.95rem;
  color: #aaa;
  margin: 0;
}

.add-tag-btn {
  background: transparent;
  border: 1px dashed #007acc;
  color: #007acc;
  padding: 2px 8px;
  border-radius: 4px;
  font-size: 0.75rem;
  cursor: pointer;
  transition: all 0.2s;
}
.add-tag-btn:hover {
  background: rgba(0, 122, 204, 0.2);
}

/* 🎯 标签云与单项卡片包裹层 */
.tags-cloud {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.detail-tag-item {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  background-color: #242428;
  padding-right: 6px;
  border-radius: 4px;
}

/* 本地新增 tag：橙色虚线圈出，区别于官方 online tag */
.detail-tag-item.tag-local {
  outline: 1px dashed #ffb74d;
  outline-offset: 1px;
  background-color: #2a231a;
}

.local-badge {
  font-size: 0.62rem;
  color: #ffb74d;
  background-color: rgba(255, 183, 77, 0.15);
  border: 1px solid rgba(255, 183, 77, 0.4);
  border-radius: 3px;
  padding: 0 4px;
  line-height: 1.4;
  margin-right: 2px;
  white-space: nowrap;
}

.remove-tag {
  font-size: 0.75rem;
  color: #777;
  cursor: pointer;
  padding: 0 2px;
  transition: color 0.15s;
}
.remove-tag:hover {
  color: #ef4444;
}

.empty-tag-tip {
  font-size: 0.8rem;
  color: #555;
  font-style: italic;
}

.info-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10px;
  font-size: 0.88rem;
  margin-top: 12px;
}

.info-item {
  display: flex;
  gap: 8px;
}
.info-item.full {
  grid-column: span 2;
  align-items: center;
}
.info-item .k {
  color: #888;
}
.info-item .v {
  color: #ddd;
}

.path-text {
  font-family: monospace;
  background: #242428;
  padding: 2px 8px;
  border-radius: 4px;
  color: #007acc;
  font-size: 0.8rem;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 320px;
}

.copy-btn {
  background: #2a2a2e;
  border: 1px solid #444;
  color: #ccc;
  padding: 2px 8px;
  border-radius: 4px;
  font-size: 0.75rem;
  cursor: pointer;
  margin-left: 6px;
}

.bookshelves-list {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  margin-top: 12px;
}

.shelf-checkbox-item {
  display: flex;
  align-items: center;
  gap: 8px;
  background: #242428;
  border: 1px solid #333;
  padding: 6px 12px;
  border-radius: 6px;
  cursor: pointer;
  user-select: none;
  transition: all 0.15s;
}

.shelf-checkbox-item:hover {
  border-color: #007acc;
}

.shelf-checkbox-item.checked {
  background: rgba(0, 122, 204, 0.15);
  border-color: #007acc;
  color: #007acc;
}

.right-actions {
  display: flex;
  align-items: center;
  gap: 12px;
}

.add-reading-btn {
  background: #242428;
  border: 1px solid #3a3a3d;
  color: #ccc;
  padding: 8px 16px;
  border-radius: 6px;
  font-size: 0.88rem;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s ease;
}

.add-reading-btn:hover {
  background-color: #2e2e33;
  border-color: #007acc;
  color: #007acc;
}

.add-reading-btn.active {
  background-color: rgba(0, 122, 204, 0.2);
  border-color: #007acc;
  color: #007acc;
  font-weight: bold;
}

.read-btn {
  flex-shrink: 0;
}

.delete-btn {
  background: rgba(255, 77, 79, 0.12);
  border: 1px solid rgba(255, 77, 79, 0.5);
  color: #ff4d4f;
  padding: 8px 16px;
  border-radius: 6px;
  font-size: 0.88rem;
  font-weight: 500;
  cursor: pointer;
  flex-shrink: 0;
  transition: all 0.2s ease;
}

.delete-btn:hover {
  background: rgba(255, 77, 79, 0.25);
  border-color: #ff4d4f;
}

.delete-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.category-wrapper {
  margin-bottom: 8px;
}

.category-badge {
  display: inline-block;
  padding: 4px 12px;
  border-radius: 4px;
  color: #ffffff;
  font-size: 0.8rem;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  box-shadow: 0 2px 6px rgba(0, 0, 0, 0.4);
}
</style>
