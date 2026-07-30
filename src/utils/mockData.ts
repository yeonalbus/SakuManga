import type { OnlineComic, OfflineComic, Chapter, ComicPage, HistoryRecord } from '@/types/comic'

// ==========================================
// 1. 零网络依赖的纯前端原生 SVG Data URI 生成器
// ==========================================

// 高颜值暗色调背景调色盘
const PALETTE = [
  { bg: '#1e293b', text: '#94a3b8' }, // 蓝灰
  { bg: '#311b92', text: '#b388ff' }, // 深紫
  { bg: '#1b5e20', text: '#b9f6ca' }, // 深绿
  { bg: '#4a148c', text: '#ea80fc' }, // 紫红
  { bg: '#3e2723', text: '#ffcc80' }, // 棕褐
  { bg: '#881337', text: '#fda4af' }, // 玫瑰红
  { bg: '#064e3b', text: '#6ee7b7' }, // 青绿
  { bg: '#7c2d12', text: '#ffedd5' }, // 橙红
]

/**
 * 核心：生成 100% 本地内嵌的 SVG 封面/页码图片 Data URI
 */
export function generateSvgImage(
  title: string,
  subtext = '',
  width = 300,
  height = 400,
  colorIndex?: number,
): string {
  const color = PALETTE[(colorIndex ?? Math.floor(Math.random() * PALETTE.length)) % PALETTE.length]
  const cleanTitle = title.replace(/[<>&"]/g, '') // 转义特殊字符
  const cleanSubtext = subtext.replace(/[<>&"]/g, '')

  const svgStr = `<svg xmlns="http://www.w3.org/2000/svg" width="${width}" height="${height}" viewBox="0 0 ${width} ${height}">
    <rect width="100%" height="100%" fill="${color.bg}"/>
    <rect x="12" y="12" width="${width - 24}" height="${height - 24}" rx="8" fill="none" stroke="${color.text}" stroke-width="2" stroke-dasharray="4 4" opacity="0.3"/>
    <text x="50%" y="42%" dominant-baseline="middle" text-anchor="middle" fill="${color.text}" font-size="16" font-weight="bold" font-family="sans-serif">
      ${cleanTitle.length > 16 ? cleanTitle.slice(0, 16) + '...' : cleanTitle}
    </text>
    ${
      cleanSubtext
        ? `<text x="50%" y="58%" dominant-baseline="middle" text-anchor="middle" fill="${color.text}" font-size="13" opacity="0.8" font-family="sans-serif">
      ${cleanSubtext}
    </text>`
        : ''
    }
  </svg>`

  return `data:image/svg+xml;charset=UTF-8,${encodeURIComponent(svgStr)}`
}

// ==========================================
// 2. 预设常用标签与分类数据
// ==========================================

export const mockTags = [
  'female:big breasts',
  'female:sole female',
  'female:nun',
  'female:glasses',
  'male:sole male',
  'male:shotacon',
  'language:chinese',
  'language:translated',
  'artist:hiten',
  'artist:tsukino',
  'group:circle_a',
  'parody:original',
  'character:reimu',
  'full color',
  'uncensored',
]

export const mockCategories = ['Doujinshi', 'Manga', 'Artist CG', 'Game CG', 'Non-H']

// ==========================================
// 3. 在线 / 离线 漫画数据生成器
// ==========================================

/** 生成在线漫画列表 */
export function generateOnlineComics(count = 30): OnlineComic[] {
  return Array.from({ length: count }, (_, i) => {
    const idNum = i + 1
    const title = `[Hanazono] 极品本子作品名称 #${idNum}`
    const shuffledTags = [...mockTags].sort(() => 0.5 - Math.random())

    return {
      id: `online-${idNum}`,
      source: 'online',
      title,
      coverUrl: generateSvgImage(title, `Online Gallery #${idNum}`, 300, 400, i),
      tags: shuffledTags.slice(0, Math.floor(Math.random() * 3) + 2),
      rating: +(3.8 + Math.random() * 1.2).toFixed(1),
      pageCount: Math.floor(Math.random() * 30) + 12,
      updatedAt: '2026-07-29',
      category: mockCategories[i % mockCategories.length],
      uploader: `User_${idNum}`,
      token: `token_${idNum}`,
      isFavorite: i < 5,
      favIndex: i < 5 ? i : undefined,
      isDownloaded: i % 3 === 0,
    }
  })
}

/** 生成离线（本地）漫画列表 */
export function generateOfflineComics(count = 30): OfflineComic[] {
  return Array.from({ length: count }, (_, i) => {
    const idNum = i + 1
    const title = `📖 [本地] 深度学习资料包作品 #${idNum}`
    const shuffledTags = [...mockTags].sort(() => 0.5 - Math.random())

    return {
      id: `offline-${idNum}`,
      category: mockCategories[i % mockCategories.length],
      source: 'offline',
      title,
      coverUrl: generateSvgImage(title, `Local Directory #${idNum}`, 300, 400, i + 2),
      tags: shuffledTags.slice(0, Math.floor(Math.random() * 3) + 2),
      rating: 5.0,
      pageCount: Math.floor(Math.random() * 40) + 10,
      updatedAt: '2026-07-28',
      localPath: `Z:\\Comics\\Local_Comic_${idNum}`,
      fileSize: `${(Math.random() * 200 + 50).toFixed(1)} MB`,
      format: i % 2 === 0 ? 'folder' : 'zip',
      bookshelfId: i < 5 ? 'shelf-1' : 'shelf-3',
      isDownloaded: true,
    }
  })
}

// ==========================================
// 4. 阅读器（章节与图片页）Mock 逻辑
// ==========================================

/** 生成某一话/本子的多页 Mock 图像 */
export function generateMockPages(comicTitle: string, totalPages = 20): ComicPage[] {
  return Array.from({ length: totalPages }, (_, i) => {
    const pageIndex = i + 1
    return {
      pageIndex,
      imageUrl: generateSvgImage(comicTitle, `Page ${pageIndex} / ${totalPages}`, 800, 1100, i),
    }
  })
}

/** 生成离线/在线详情页需要的章节数据 */
export function generateMockChapters(comicId: string, comicTitle: string): Chapter[] {
  return [
    {
      id: `chap-${comicId}-1`,
      comicId,
      title: '第 01 话（单行本全彩）',
      chapterIndex: 1,
      pages: generateMockPages(`${comicTitle} - Ch.1`, 18),
    },
    {
      id: `chap-${comicId}-2`,
      comicId,
      title: '第 02 话（番外篇）',
      chapterIndex: 2,
      pages: generateMockPages(`${comicTitle} - Ch.2`, 12),
    },
  ]
}

// ==========================================
// 5. 历史记录 Mock 数据
// ==========================================

export function generateMockHistory(): HistoryRecord[] {
  return [
    {
      comicId: 'online-1',
      source: 'online',
      comicTitle: '[Hanazono] 极品本子作品名称 #1',
      coverUrl: generateSvgImage('[Hanazono] 极品本子作品名称 #1', 'Online #1', 300, 400, 0),
      lastChapterTitle: '第 01 话',
      lastPageIndex: 5,
      totalPageCount: 32,
      lastReadAt: '2026-07-29 14:30',
    },
    {
      comicId: 'offline-1',
      source: 'offline',
      comicTitle: '📖 [本地] 深度学习资料包作品 #1',
      coverUrl: generateSvgImage('📖 [本地] 深度学习资料包作品 #1', 'Local #1', 300, 400, 2),
      lastChapterTitle: '第 01 话',
      lastPageIndex: 12,
      totalPageCount: 28,
      lastReadAt: '2026-07-29 10:15',
    },
  ]
}
