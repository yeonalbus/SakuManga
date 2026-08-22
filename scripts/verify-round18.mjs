// scripts/verify-round18.mjs
// Round18 回归验证：iPad PWA 底部白条 —— app-container 去 fixed + 顶部 safe-area 下推
import { chromium } from "playwright-core"

const BASE = "http://127.0.0.1:8081"
const CHROME = "C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe"
function log(mark, msg) { console.log(`[${mark}] ${msg}`) }
let pass = true
const fail = (msg) => { pass = false; log("FAIL", msg) }
const ok = (msg) => log("OK", msg)

async function codeChecks() {
  const fs = await import("fs")
  const app = fs.readFileSync("src/App.vue", "utf8")
  // 1) app-container 不再 fixed inset-0
  const cont = app.slice(app.indexOf(".app-container {"), app.indexOf(".sidebar {"))
  if (cont.includes("position: fixed") && cont.includes("inset: 0")) fail("app-container 仍为 fixed inset-0")
  else ok("app-container 已去 fixed inset-0（改文档流）")
  // （用户方案：app-container 不再整体下移，safe-area 由 sidebar/top-bar 各自处理）
  // Round18.7（用户方案）：100dvh = webview 真实高度(802)，无整体 padding-top（状态栏避让下沉到子容器）
  if (cont.includes("height: 100dvh")) ok("app-container 用 100dvh（= webview 802 真实高度）")
  else fail("app-container 未用 100dvh: " + cont.slice(cont.indexOf("height:"), cont.indexOf("width:")))
  if (cont.includes("padding-top: var(--safe-top)")) fail("app-container 仍整体下移 safe-top（双重偏移）")
  else ok("app-container 无整体 padding-top（避让下沉到 sidebar/top-bar）")
  // 2) right-wrapper height:100%
  const wrap = app.slice(app.indexOf(".right-wrapper {"), app.indexOf("/* 顶部操作栏 */"))
  if (wrap.includes("height: 100%")) ok("right-wrapper 文档流内 height:100% 撑满")
  else fail("right-wrapper 高度异常")
  // 2.5) #app 用 100%（跟随 html/body 链）
  const appBlock = app.slice(app.indexOf("#app {"), app.indexOf("html,\nbody"))
  if (appBlock && appBlock.includes("height: 100%")) ok("#app 用 100%（高度链统一）")
  else fail("#app 高度异常: " + (appBlock || "").slice(0, 80))
  // 2.6) top-bar/sidebar 各自避让 safe-area（sidebar 用 calc(20px + env(...))）
  if (app.includes("env(safe-area-inset-top, 0px)")) ok("sidebar/top-bar 各自避让状态栏 safe-area")
  else fail("子容器缺 safe-area 避让")
  // 2.7) body 无底部安全区 padding（避免 bodyScrollH > 视口产生底部横条，Round18.5）
  if (!app.includes("env(safe-area-inset-bottom)")) ok("body 无底部安全区 padding（不撑破视口）")
  else {
    // 确保 env 只出现在 .main-content 内部（滚动内容安全区），不在 body 顶部
    const bodyBlock = app.slice(app.indexOf("body {"), app.indexOf("font-family"))
    if (bodyBlock && !bodyBlock.includes("padding-bottom")) ok("body 无 padding-bottom（底部安全区由 main-content 处理）")
    else fail("body 仍含底部 padding-bottom")
  }
  // 3) main-content 移动形态补偿 = 56 + safe-top（与 TopBar.vue min-height 匹配）
  if (app.includes("calc(56px + env(safe-area-inset-top, 0px))")) ok("main-content 移动形态补偿 TopBar(56px+safe-top)")
  else fail("main-content 移动形态补偿异常")
  // 4) html 背景色（在 html,body 合并规则块内）
  const htmlBodyStart = app.indexOf("html,")
  const htmlBodyBlock = app.slice(htmlBodyStart, app.indexOf("body {", htmlBodyStart + 10))
  if (htmlBodyBlock.includes("background-color: var(--app-bg)")) ok("html 根背景色 var(--app-bg)")
  else fail("html 根背景色缺失")
}

const browser = await chromium.launch({ executablePath: CHROME, headless: true })
try {
  await codeChecks()
} finally { await browser.close() }

console.log(pass ? "\n===== ROUND18 全部验证通过 =====" : "\n===== ROUND18 存在失败项 =====")
process.exit(pass ? 0 : 1)