/**
 * 验证 dist 产物：确认没有「直接作用 <html> 元素」的 data-layout 规则。
 *
 * 背景：Vue scoped CSS 中 `:global(html[...]) .class` 会被 @vue/compiler-sfc 编译丢弃
 * 子类名，产物变成 `html[data-layout=...]{...}`，整条规则直接作用在 <html> 上，
 * 曾导致移动模式白屏（整页 display:none / transform 平移出视口）。
 *
 * 判定规则：选择器以 html 开头，且 `{` 前不含空格（空格表示有后代子类，是安全的）——
 * 若不含空格则直接作用于 html 元素本身，判定为危险。
 *
 * 用法：node scripts/verify-layout-css.mjs
 */
import { readdirSync, readFileSync } from 'node:fs'
import { join } from 'node:path'

const dir = 'dist/assets'
let bad = []

for (const f of readdirSync(dir)) {
  if (!f.endsWith('.css')) continue
  const css = readFileSync(join(dir, f), 'utf8')
  // 仅匹配含 data-layout 的 html 规则（scoped :global 编译 bug 的目标）；选择器内不含空格 ⇒ 直接作用 html
  const re = /html\[data-layout[^{}]*\{/g
  let m
  while ((m = re.exec(css)) !== null) {
    const sel = m[0].replace(/\{\s*$/, '').trim()
    if (!sel.includes(' ')) {
      bad.push({ file: f, sel })
    }
  }
}

console.log('直接作用 html 的危险规则数量:', bad.length)
for (const b of bad) console.log('  ', b.file, '=>', b.sel)

if (bad.length > 0) {
  console.error('❌ 检测到危险规则，可能存在白屏风险！')
  process.exit(1)
}
console.log('✅ 全部通过：无直接作用 html 的 data-layout 规则')
