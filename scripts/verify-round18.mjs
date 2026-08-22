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
  if (cont.includes("padding-top: var(--safe-top)")) ok("app-container 保留顶部 safe-area（防误触下拉通知栏）")
  else fail("app-container 缺 padding-top: var(--safe-top)")
  // Round18.4：真机诊断证实 100dvh = 真实可视高度（innerH），100vh = 物理屏偏大
  if (cont.includes("height: 100dvh")) ok("app-container 用 100dvh（= innerH 真实可视高度，真机诊断修正）")
  else fail("app-container 未用 100dvh: " + cont.slice(cont.indexOf("height:"), cont.indexOf("width:")))
  if (cont.includes("height: 100vh")) fail("app-container 仍含 100vh（物理屏偏大导致底部条）")
  else ok("app-container 无 100vh（避免超出可视区）")
  // 2) right-wrapper height:100%
  const wrap = app.slice(app.indexOf(".right-wrapper {"), app.indexOf("/* 顶部操作栏 */"))
  if (wrap.includes("height: 100%")) ok("right-wrapper 文档流内 height:100% 撑满")
  else fail("right-wrapper 高度异常")
  // 2.5) #app 用 100dvh（与 app-container 一致，避免父级钳制）
  const appBlock = app.slice(app.indexOf("#app {"), app.indexOf("html,\nbody"))
  if (appBlock && appBlock.includes("height: 100dvh")) ok("#app 用 100dvh（与容器一致）")
  else fail("#app 高度异常: " + (appBlock || "").slice(0, 80))
  // 2.6) body 底部安全区
  if (app.includes("padding-bottom: env(safe-area-inset-bottom)")) ok("body 底部安全区（sun-panel 同款）")
  else fail("body 缺底部安全区 padding")
  // 3) main-content 不再双重 safe-top
  if (app.includes("padding-top: 56px;")) ok("main-content 已去重复 safe-top（仅补偿 TopBar 56px）")
  else fail("main-content 仍双重 safe-top")
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