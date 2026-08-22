// scripts/verify-round20.mjs
// Round20 回归验证：四项任务（历史 gid 去重 / PWA 详情跳转 / 抽卡 tag 统一 / 书架 404 自愈）
// 静态断言 + 关键语义点检查（配合 go test 与 npm run type-check）

import { readFileSync } from "fs"
function log(mark, msg) { console.log(`[${mark}] ${msg}`) }
let pass = true
const fail = (msg) => { pass = false; log("FAIL", msg) }
const ok = (msg) => log("OK", msg)

const rd = (p) => readFileSync(p, "utf8")

// ─────────────────────────────────────────────
// Bug1：离线历史 gid 合并去重 + 引用清理迁移
// ─────────────────────────────────────────────
{
  const models = rd("backend/internal/models/models.go")
  if (/GID\s+string\s+`gorm:"index" json:"gid,omitempty"`/.test(models)) ok("Bug1: HistoryRecord 新增 GID 列")
  else fail("Bug1: HistoryRecord 缺少 GID 字段")

  const lib = rd("backend/internal/handlers/library.go")
  if (/Gid\s+string\s+`json:"gid"`/.test(lib)) ok("Bug1: AddHistory 入参支持 gid")
  else fail("Bug1: AddHistory 入参缺少 gid")
  if (lib.includes("g_id = ? AND comic_id != ?")) ok("Bug1: AddHistory 同 gid 旧行合并删除")
  else fail("Bug1: AddHistory 缺少同 gid 合并逻辑")
  if (lib.includes("func dedupeHistoryByGid")) ok("Bug1: GetHistory 按 gid 去重函数存在")
  else fail("Bug1: dedupeHistoryByGid 缺失")
  if (lib.includes("records = dedupeHistoryByGid(records)")) ok("Bug1: GetHistory 离线分支调用去重")
  else fail("Bug1: GetHistory 未调用去重")

  const refs = rd("backend/internal/services/comic_refs.go")
  if (refs.includes("func CleanupComicReferences")) ok("Bug1: 引用清理服务 CleanupComicReferences 存在")
  else fail("Bug1: CleanupComicReferences 缺失")
  if (refs.includes("func FindReplacementByGID")) ok("Bug1: FindReplacementByGID 存在")
  else fail("Bug1: FindReplacementByGID 缺失")

  const off = rd("backend/internal/services/offline.go")
  if (off.includes("CleanupComicReferences(db, comicID, replacement)")) ok("Bug1: DeleteOfflineComic 接线引用清理")
  else fail("Bug1: DeleteOfflineComic 未接线引用清理")
  const dl = rd("backend/internal/services/download.go")
  if (dl.includes("CleanupComicReferences(m.db, old.ID, \"\")")) ok("Bug1: 更新删除旧版后清理孤儿引用")
  else fail("Bug1: finalizeUpdate 未清理孤儿引用")
  const sc = rd("backend/internal/services/scanner.go")
  if (sc.includes("CleanupComicReferences(database.DB, existing.ID, comicID)")) ok("Bug1: 扫描归档升级迁移引用")
  else fail("Bug1: scanner 升级未迁移引用")

  const hs = rd("src/stores/historyStore.ts")
  if (hs.includes("gid?: string") && hs.includes("gid: r.gid")) ok("Bug1: 前端 HistoryRecordDTO 透传 gid")
  else fail("Bug1: 前端历史未透传 gid")
  if (hs.includes("function historyDedupeKey") || hs.includes("const historyDedupeKey")) ok("Bug1: 前端按 gid||id 去重键")
  else fail("Bug1: 前端去重键缺失")
  if (hs.includes("alive.has(h.comic.id)")) ok("Bug1/D2: loadHistory 剔除孤儿历史")
  else fail("Bug1/D2: loadHistory 孤儿剔除缺失")
  if (hs.includes("gid: (comic as OfflineComic).gid || ''")) ok("Bug1: syncHistory 透传 gid")
  else fail("Bug1: syncHistory 未透传 gid")

  const types = rd("src/types/comic.ts")
  if (types.includes("gid?: string")) ok("Bug1: OfflineComic 类型增加 gid")
  else fail("Bug1: OfflineComic 类型缺 gid")
}

// ─────────────────────────────────────────────
// Bug2：PWA 详情跳转 / 阅读器页列表错误可读化
// ─────────────────────────────────────────────
{
  const reader = rd("backend/internal/services/eh_reader.go")
  if (reader.includes("func classifySessionError")) ok("Bug2: 会话失效/限流识别函数存在")
  else fail("Bug2: classifySessionError 缺失")
  if (reader.includes("classifyUnavailableOrSession(body")) ok("Bug2: 抓取错误统一识别（gdata/预览页//s/页）")
  else fail("Bug2: 错误识别未接线")
  if (reader.includes("解析原图 URL 全部失败（画廊可能已删除")) ok("Bug2: /s/ 兜底失败错误可读化")
  else fail("Bug2: 兜底失败提示未更新")

  const onlineH = rd("backend/internal/handlers/online.go")
  if (onlineH.includes("ErrEHSession") && onlineH.includes("http.StatusUnauthorized")) ok("Bug2: 会话失效映射 401")
  else fail("Bug2: 会话失效未映射 401")
  if (onlineH.includes("ErrRateLimited") && onlineH.includes("http.StatusTooManyRequests")) ok("Bug2: 限流映射 429")
  else fail("Bug2: 限流未映射 429")

  const cr = rd("src/views/ComicReader.vue")
  if (cr.includes("resolveOnlineToken(realId)")) ok("Bug2: 离线+数字 id 无 token 也兜底解析")
  else fail("Bug2: 自动切在线未解析 token")
  if (cr.includes("loadError") && cr.includes("retryLoad")) ok("Bug2/Bug4: 阅读器错误重试层存在")
  else fail("Bug2/Bug4: 阅读器错误层缺失")
  if (cr.includes("reportError")) ok("Bug2: 阅读器错误上报已接线")
  else fail("Bug2: 阅读器未接错误上报")
}

// ─────────────────────────────────────────────
// Bug3：离线/在线 tag 筛选统一（本地遵循线上格式）
// ─────────────────────────────────────────────
{
  const tf = rd("backend/internal/services/tagfilter.go")
  for (const fn of ["func ParseFSearchTag", "func (t FSearchTag) MatchTag", "func (t FSearchTag) TagJSONMatchPatterns", "func EscapeLike"]) {
    if (tf.includes(fn)) ok("Bug3: Go 解析器 " + fn)
    else fail("Bug3: Go 解析器缺 " + fn)
  }
  const rnd = rd("backend/internal/handlers/random.go")
  if (rnd.includes("services.ParseFSearchTag(kw)") && rnd.includes("tag.TagJSONMatchPatterns()")) ok("Bug3: random.go 离线正向 tag 解析匹配")
  else fail("Bug3: random.go 离线正向未按 f_search 解析")
  if (rnd.includes("pt.TagJSONMatchPatterns()")) ok("Bug3: random.go 离线负向 tag 解析匹配")
  else fail("Bug3: random.go 离线负向未按 f_search 解析")
  if (rnd.includes("services.EscapeLike(kw)")) ok("Bug3: random.go LIKE 通配符转义")
  else fail("Bug3: random.go 未转义 LIKE 通配符")

  const tfts = rd("src/utils/tagFilter.ts")
  if (tfts.includes("export const parseFSearchTag") && tfts.includes("export const matchFSearchKeyword")) ok("Bug3: 前端解析器/匹配器存在")
  else fail("Bug3: 前端解析器缺失")
  if (tfts.includes("const t = parseFSearchTag(et)")) ok("Bug3: matchExcludes 负向 tag 走 f_search 解析")
  else fail("Bug3: matchExcludes 未升级")

  const home = rd("src/views/offline/OfflineHome.vue")
  if (home.includes("matchFSearchKeyword(searchBarKw, comic)") && home.includes("matchFSearchKeyword(filterKw, comic)")) ok("Bug3: OfflineHome 关卡1/2 支持 f_search tag")
  else fail("Bug3: OfflineHome 关卡未接入 f_search 匹配")

  // 联想/点击插入格式统一为线上格式（不再按离线裸格式特例）
  for (const [label, file, frag] of [
    ["RandomView", "src/views/RandomView.vue", "formatFSearchTag(namespace, key, false)"],
    ["FilterDrawer", "src/components/FilterDrawer.vue", "formatFSearchTag(namespace, key, false)"],
    ["SearchBar", "src/components/SearchBar.vue", "formatFSearchTag(tag.namespace, tag.key, false)"],
    ["TagChip", "src/components/TagChip.vue", "formatFSearchTag(namespace, key, false)"],
  ]) {
    const src = rd(file)
    if (src.includes(frag)) ok(`Bug3: ${label} 插入格式统一为 f_search`)
    else fail(`Bug3: ${label} 插入格式未统一`)
  }
}

// ─────────────────────────────────────────────
// Bug4：书架 404 非阻塞自愈 + 弹窗不粘滞
// ─────────────────────────────────────────────
{
  const router = rd("src/router/index.ts")
  if (router.includes("modalState.isOpen) handleCancel()")) ok("Bug4: 路由切换自动取消未决 modal")
  else fail("Bug4: 路由切换未取消 modal")

  const cr = rd("src/views/ComicReader.vue")
  if (!cr.includes("是否刷新离线列表后返回")) ok("Bug4: 原阻塞确认框已移除")
  else fail("Bug4: 仍存在阻塞确认框")
  if (cr.includes("purgeOrphanOfflineRefs(realId)")) ok("Bug4: 阅读器 404 自愈（刷新+清理孤儿引用）")
  else fail("Bug4: 阅读器未自愈")

  const od = rd("src/views/offline/OfflineDetail.vue")
  if (od.includes("purgeOrphanOfflineRefs(comicId)")) ok("Bug4: 详情页 404 自愈")
  else fail("Bug4: 详情页未自愈")

  const cs = rd("src/stores/comicStore.ts")
  if (cs.includes("export const purgeOrphanOfflineRefs")) ok("Bug4: purgeOrphanOfflineRefs 导出")
  else fail("Bug4: purgeOrphanOfflineRefs 未导出")
  if (cs.includes("拉取离线漫画列表失败")) ok("Bug4: fetchOfflineComics 失败可见")
  else fail("Bug4: fetchOfflineComics 失败仍静默")

  const li = rd("src/stores/libraryInit.ts")
  if (li.includes("await fetchOfflineComics()")) ok("Bug4/D2: 登录初始化先拉离线列表再载历史")
  else fail("Bug4/D2: libraryInit 未先拉离线列表")

  const oh = rd("src/views/offline/OfflineHistory.vue")
  if (oh.includes("await fetchOfflineComics()") && oh.includes("await loadHistory('offline')")) ok("Bug1/D2: 历史页挂载刷新去重+剔孤儿")
  else fail("Bug1/D2: 历史页未刷新")
}

console.log(pass ? "\n===== ROUND20 全部验证通过 =====" : "\n===== ROUND20 存在失败项 =====")
process.exit(pass ? 0 : 1)
