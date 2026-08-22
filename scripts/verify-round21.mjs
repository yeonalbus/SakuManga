// scripts/verify-round21.mjs
// Round21 回归验证：PC 端跳转策略平台分流 + 返回误关修复
// 静态断言：detailNav 新 API、各入口分流、handleBack 统一决策、PWA 同标签分支保留

import { readFileSync } from "fs"
function log(mark, msg) { console.log(`[${mark}] ${msg}`) }
let pass = true
const fail = (msg) => { pass = false; log("FAIL", msg) }
const ok = (msg) => log("OK", msg)
const rd = (p) => readFileSync(p, "utf8")

// ── detailNav.ts：平台分流新 API + 清理旧标记 ──
{
  const dn = rd("src/utils/detailNav.ts")
  for (const fn of ["contentOpensNewTab", "openContentTab", "shouldCloseTab"]) {
    if (dn.includes(`export function ${fn}`) || dn.includes(`export const ${fn}`)) ok(`detailNav: ${fn} 存在`)
    else fail(`detailNav: ${fn} 缺失`)
  }
  if (dn.includes("saku_tab_entry_")) ok("detailNav: 标签入口路由记录存在")
  else fail("detailNav: 标签入口路由记录缺失")
  if (!dn.includes("saku_newtab_")) ok("detailNav: 旧 saku_newtab_ 标记已移除")
  else fail("detailNav: 旧 saku_newtab_ 标记残留")
  if (!dn.includes("isDetailNewTab")) ok("detailNav: isDetailNewTab 已移除")
  else fail("detailNav: isDetailNewTab 残留")
  if (!dn.includes("window.location.href = href")) ok("detailNav: 无整页导航（Round16 不回归）")
  else fail("detailNav: 出现 location.href 整页导航")
  if (dn.includes("!window.opener || window.opener.closed")) ok("detailNav: 仅 opener 存活才允许 close（单标签不关）")
  else fail("detailNav: 缺少 opener 存活判定")
}

// ── 入口分流：ItemCard / OnlineDetailPanel / ReadingListView ──
{
  const ic = rd("src/components/ItemCard.vue")
  if (ic.includes("contentOpensNewTab()")) ok("ItemCard: 默认点击按平台分流")
  else fail("ItemCard: 未接入 contentOpensNewTab")
  const odp = rd("src/components/OnlineDetailPanel.vue")
  if (odp.includes("openContentTab({ href, id: props.gid })")) ok("OnlineDetailPanel: 画廊详情↗ PC 新标签")
  else fail("OnlineDetailPanel: 画廊详情↗ 未接入 openContentTab")
  const rlv = rd("src/views/ReadingListView.vue")
  if (rlv.includes("openContentTab({ href, id: comic.id })")) ok("ReadingListView: 阅读 PC 新标签")
  else fail("ReadingListView: 未接入 openContentTab")
}

// ── handleBack 统一决策（防误关） ──
{
  for (const [label, file] of [
    ["OnlineDetail", "src/views/online/OnlineDetail.vue"],
    ["OfflineDetail", "src/views/offline/OfflineDetail.vue"],
    ["ComicReader", "src/views/ComicReader.vue"],
  ]) {
    const src = rd(file)
    if (src.includes("shouldCloseTab(")) ok(`${label}: 返回走 shouldCloseTab`)
    else fail(`${label}: 未接入 shouldCloseTab`)
    // 不再存在「window.opener 存在即 close」的裸判定
    if (src.includes("if (window.opener)")) fail(`${label}: 残留裸 window.opener 判定`)
  }
  const cr = rd("src/views/ComicReader.vue")
  if (cr.includes("handleReaderBack")) ok("ComicReader: 统一返回处理存在")
  else fail("ComicReader: handleReaderBack 缺失")
  if (cr.includes('@click="handleReaderBack"')) ok("ComicReader: 返回按钮接入 handleReaderBack")
  else fail("ComicReader: 返回按钮未接入")
  const od = rd("src/views/online/OnlineDetail.vue")
  if (od.includes("openContentTab({ href, id })")) ok("OnlineDetail: 立即阅读 PC 新标签")
  else fail("OnlineDetail: 立即阅读未接入 openContentTab")
  const ofd = rd("src/views/offline/OfflineDetail.vue")
  if (ofd.includes("openContentTab({ href, id: comic.value.id })")) ok("OfflineDetail: 立即阅读/预览 PC 新标签")
  else fail("OfflineDetail: 立即阅读未接入 openContentTab")
}

// ── PWA 同标签分支保留（Round15/16 不回归） ──
{
  const dn = rd("src/utils/detailNav.ts")
  if (dn.includes("isStandalonePWA()")) ok("PWA: isStandalonePWA 判定保留")
  else fail("PWA: isStandalonePWA 缺失")
  if (dn.includes("router.push(href)")) ok("分流: 非 PC 场景降级同标签 SPA")
  else fail("分流: 缺少同标签降级")
}

console.log(pass ? "\n===== ROUND21 全部验证通过 =====" : "\n===== ROUND21 存在失败项 =====")
process.exit(pass ? 0 : 1)
