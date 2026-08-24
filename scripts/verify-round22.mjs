// scripts/verify-round22.mjs
// Round22 回归验证：LexoRank 排序重构（拖拽 + 移动到/置顶 + 去掉↑↓）+ 快捷加入移除扩大化
// 静态断言：权值工具、后端新端点、store 新函数、↑↓ 移除、快捷加入/移除接入面

import { readFileSync } from "fs"
function log(mark, msg) { console.log(`[${mark}] ${msg}`) }
let pass = true
const fail = (msg) => { pass = false; log("FAIL", msg) }
const ok = (msg) => log("OK", msg)
const rd = (p) => readFileSync(p, "utf8")

// ── 前端：LexoRank 权值工具 ──
{
  const lr = rd("src/utils/lexoRank.ts")
  for (const fn of ["between", "needsReweight", "reweightAll", "orderByWeights", "weightForInsert"]) {
    if (lr.includes(`export function ${fn}`) || lr.includes(`export const ${fn}`)) ok(`lexoRank: ${fn} 存在`)
    else fail(`lexoRank: ${fn} 缺失`)
  }
  if (lr.includes("INITIAL_STEP") && lr.includes("MIN_GAP")) ok("lexoRank: INITIAL_STEP / MIN_GAP 常量存在")
  else fail("lexoRank: 常量缺失")
}

// ── 后端：模型新列 + 新端点 ──
{
  const mm = rd("backend/internal/models/models.go")
  if (mm.includes("SortKey") && mm.includes("SortKeys")) ok("models: Bookshelf 新增 SortKey / SortKeys")
  else fail("models: Bookshelf 缺少 SortKey / SortKeys")

  const lh = rd("backend/internal/handlers/library.go")
  for (const fn of ["MoveBookshelfPosition", "BatchRemoveComicsFromBookshelf", "sortedComicIDs"]) {
    if (lh.includes(`func (h *LibraryHandler) ${fn}`) || lh.includes(`func ${fn}`)) ok(`handlers: ${fn} 存在`)
    else fail(`handlers: ${fn} 缺失`)
  }
  if (lh.includes('sort_key asc, sort_order asc, name asc')) ok("handlers: GetBookshelves 按 sort_key 优先排序")
  else fail("handlers: GetBookshelves 未按 sort_key 排序")
  if (lh.includes('"sortKeys": sk')) ok("handlers: 响应携带 sortKeys 权值表")
  else fail("handlers: 响应缺少 sortKeys")

  const rt = rd("backend/internal/router/router.go")
  if (rt.includes('PUT("/bookshelves/:id/position"')) ok("router: PUT /bookshelves/:id/position 已注册")
  else fail("router: position 路由缺失")
  if (rt.includes('DELETE("/bookshelves/:id/comics/batch"')) ok("router: DELETE /bookshelves/:id/comics/batch 已注册")
  else fail("router: 批量移除路由缺失")
}

// ── 前端 store：新函数 + 移除 moveBookshelf ──
{
  const bs = rd("src/stores/bookshelfStore.ts")
  for (const fn of ["moveShelfToPosition", "moveShelfToTop", "moveComicToPosition", "moveComicToTop", "removeComicsFromShelf", "flushPendingSort", "orderedShelfComicIds", "ensureShelfComicWeights"]) {
    if (bs.includes(`export const ${fn}`)) ok(`store: ${fn} 存在`)
    else fail(`store: ${fn} 缺失`)
  }
  if (bs.includes("export const moveBookshelf")) fail("store: 旧 moveBookshelf 仍存在（应删除）")
  else ok("store: 旧 moveBookshelf 已移除")
  if (bs.includes("scheduleShelfPositionFlush") && bs.includes("scheduleComicWeightsFlush")) ok("store: 防抖合并持久化存在")
  else fail("store: 防抖持久化缺失")
}

// ── ↑↓ 按钮移除 ──
{
  const sb = rd("src/components/OfflineSidebar.vue")
  // 注意：removeBookshelf 含子串 moveBookshelf，须用词边界匹配独立标识符
  if (!/\bmoveBookshelf\b/.test(sb)) ok("OfflineSidebar: ↑↓/moveBookshelf 已移除")
  else fail("OfflineSidebar: 仍引用 moveBookshelf")
  if (sb.includes("drag-handle") && sb.includes("SortRowMenu")) ok("OfflineSidebar: 拖拽把手 + 操作菜单已接入")
  else fail("OfflineSidebar: 缺少拖拽把手/操作菜单")

  const pk = rd("src/components/BookshelfPickerOverlay.vue")
  if (!/\bmoveBookshelf\b/.test(pk)) ok("BookshelfPickerOverlay: ↑↓/moveBookshelf 已移除")
  else fail("BookshelfPickerOverlay: 仍引用 moveBookshelf")

  const ob = rd("src/views/offline/OfflineBookshelf.vue")
  if (!ob.includes("moveSortItem") && !ob.includes("class=\"move-btn\"")) ok("OfflineBookshelf: 排序视图 ↑↓ 已移除")
  else fail("OfflineBookshelf: 仍残留 moveSortItem/move-btn")
  if (ob.includes("useDragReorder") && ob.includes("SortRowMenu")) ok("OfflineBookshelf: 拖拽 + 排序菜单已接入")
  else fail("OfflineBookshelf: 拖拽/排序菜单缺失")
}

// ── 快捷加入/移除扩大化 ──
{
  const qa = rd("src/composables/useShelfQuickAdd.ts")
  if (qa.includes("export function useShelfQuickAdd")) ok("useShelfQuickAdd: 共享 composable 存在")
  else fail("useShelfQuickAdd 缺失")
  if (qa.includes("comic.source !== 'offline'")) ok("useShelfQuickAdd: 仅离线漫画可加入（在线结果不进入选择）")
  else fail("useShelfQuickAdd: 缺少离线过滤")

  const tb = rd("src/components/ShelfQuickAddToolbar.vue")
  if (tb.includes("showRemove") && tb.includes("移出书架")) ok("ShelfQuickAddToolbar: 支持「移出书架」")
  else fail("ShelfQuickAddToolbar: 缺少移出书架")

  const oh = rd("src/views/offline/OfflineHistory.vue")
  if (oh.includes("useShelfQuickAdd") && oh.includes("ShelfQuickAddToolbar")) ok("OfflineHistory: 快捷加入已接入")
  else fail("OfflineHistory: 快捷加入缺失")

  const rv = rd("src/views/RandomView.vue")
  if (rv.includes("useShelfQuickAdd") && rv.includes("ShelfQuickAddToolbar")) ok("RandomView: 快捷加入已接入（离线结果）")
  else fail("RandomView: 快捷加入缺失")

  const ob = rd("src/views/offline/OfflineBookshelf.vue")
  if (ob.includes("handleRemoveSelected") && ob.includes("removeComicsFromShelf")) ok("OfflineBookshelf: 多选快捷移除已接入")
  else fail("OfflineBookshelf: 快捷移除缺失")
  if (ob.includes("show-remove")) ok("OfflineBookshelf: 工具条 show-remove 透传")
  else fail("OfflineBookshelf: show-remove 缺失")

  const ohs = rd("src/views/offline/OfflineHome.vue")
  if (ohs.includes("useShelfQuickAdd") && !ohs.includes("addComicsToShelf")) ok("OfflineHome: 已改用共享 composable（重复逻辑消除）")
  else fail("OfflineHome: 未完全迁移共享逻辑")
}

console.log(pass ? "\n===== ROUND22 全部验证通过 =====" : "\n===== ROUND22 存在失败项 =====")
process.exit(pass ? 0 : 1)
