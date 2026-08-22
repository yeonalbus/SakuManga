// scripts/verify-round19.mjs
// Round19 回归验证：阅读界面颜色跟随主题（深色→深色，浅色→浅色）
// 断言：ComicReader.vue 无硬编码深色（除语义白名单）；App.vue 提供 --reader-* 明暗两套

import { readFileSync } from "fs"
function log(mark, msg) { console.log(`[${mark}] ${msg}`) }
let pass = true
const fail = (msg) => { pass = false; log("FAIL", msg) }
const ok = (msg) => log("OK", msg)

const reader = readFileSync("src/views/ComicReader.vue", "utf8")
const app = readFileSync("src/App.vue", "utf8")

// ── 1) ComicReader.vue 深色硬编码清点（仅扫描 <style scoped> 段） ──
const styleStart = reader.indexOf("<style scoped>")
const styleEnd = reader.indexOf("</style>")
const styleBlock = reader.slice(styleStart, styleEnd)
// 语义白名单（两主题下都成立，刻意保留）：
//   #000           亮度滤镜 overlay（降亮度始终黑）
//   #4ade80        手柄连接绿
//   #7aa2f7        加载转圈顶部色
//   rgba(0,0,0,.5) 抽屉 / 缩略图条投影
//   rgba(0,0,0,.8) / #eee    缩略图数字角标（深底浅字，压在图片上）
//   rgba(0,0,0,.55) / #aaa   缩略图进度 chip（同上）
//   #fff           .control-btn.active 白字（accent 蓝底）
const whitelist = new Set([
  "background-color: #000;",
  "color: #4ade80;",
  "border-top-color: #7aa2f7;",
  "box-shadow: -5px 0 25px rgba(0, 0, 0, 0.5);",
  "box-shadow: 0 -6px 20px rgba(0, 0, 0, 0.5);",
  "background: rgba(0, 0, 0, 0.8);",
  "color: #eee;",
  "background: rgba(0, 0, 0, 0.55);",
  "color: #aaa; /* 角标压在缩略图上：深色半透明底恒定，文字保持浅色 */",
  "color: #fff;",
])
const colorRe = /#[0-9a-fA-F]{3,8}|rgba?\([0-9., ]+\)/g
const hits = []
for (const raw of styleBlock.split("\n")) {
  const line = raw.trim()
  if (whitelist.has(line)) continue
  const m = line.match(colorRe)
  if (m) hits.push(line)
}
if (hits.length === 0) ok("ComicReader.vue 样式段无遗留硬编码主题色（语义白名单除外）")
else fail("ComicReader.vue 样式段遗留硬编码颜色:\n" + hits.join("\n"))

// 关键语义点逐一断言（跟随主题而非残留硬编码）
const expectVars = {
  "阅读器画布底色": "background-color: var(--app-bg-deep)",
  "浮动顶/底栏背景": "background: var(--reader-bar-bg)",
  "图片容器底色": "background: var(--reader-page-bg)",
  "设置抽屉背景": "background: var(--app-surface-2)",
  "缩略图条背景": "background: var(--reader-thumb-bg)",
  "控件按钮背景": "background: var(--app-surface-3)",
  "强调色": "accent-color: var(--app-accent)",
  "页图投影": "box-shadow: var(--reader-img-shadow)",
  "悬浮呼出按钮": "background: var(--reader-reveal-bg)",
  "次要文字": "color: var(--app-text-2)",
  "弱化文字": "color: var(--app-text-3)",
  "更弱文字": "color: var(--app-text-muted)",
}
for (const [name, frag] of Object.entries(expectVars)) {
  if (styleBlock.includes(frag)) ok("ComicReader: " + name + " 已接主题变量")
  else fail("ComicReader: " + name + " 未接主题变量: " + frag)
}

// ── 2) App.vue 提供 --reader-* 明暗两套 ──
const darkBlock = app.slice(app.indexOf(":root {"), app.indexOf(":root[data-theme='light']"))
const lightBlock = app.slice(app.indexOf(":root[data-theme='light']"), app.indexOf("/* 响应式断点"))
const readerVars = [
  "--reader-page-bg",
  "--reader-bar-bg",
  "--reader-thumb-bg",
  "--reader-img-shadow",
  "--reader-reveal-bg",
  "--reader-reveal-color",
  "--reader-reveal-border",
  "--reader-reveal-hover",
]
for (const v of readerVars) {
  if (darkBlock.includes(v)) ok("App.vue :root 深色 " + v)
  else fail("App.vue :root 缺深色变量 " + v)
  if (lightBlock.includes(v)) ok("App.vue light 覆盖 " + v)
  else fail("App.vue light 缺覆盖 " + v)
}
if (darkBlock.includes("--reader-page-bg: #000")) ok("深色 page-bg=黑")
else fail("深色 page-bg 非黑")
if (lightBlock.includes("--reader-page-bg: #ffffff")) ok("浅色 page-bg=白")
else fail("浅色 page-bg 非白")

// ── 3) App.vue reader-fullscreen 背景接主题 ──
const fsBlock = app.slice(app.indexOf(".main-content.reader-fullscreen"), app.indexOf("/* ───", app.indexOf(".main-content.reader-fullscreen")))
if (fsBlock.includes("background-color: var(--app-bg)")) ok("reader-fullscreen 背景接 var(--app-bg)")
else fail("reader-fullscreen 仍硬编码 #000")

console.log(pass ? "\n===== ROUND19 全部验证通过 =====" : "\n===== ROUND19 存在失败项 =====")
process.exit(pass ? 0 : 1)
