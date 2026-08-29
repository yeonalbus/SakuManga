import { type Ref } from 'vue'

/**
 * IME 回车提交处理（v2.0.1 修复）
 *
 * 背景：Vue 的 `.enter` 修饰符在输入法组合输入期间（isComposing=true / keyCode=229）
 * 忽略回车。中文/日文输入法「选词回车」恰好落在组合态：物理键是 Enter（code='Enter'），
 * 但 key 是 'Process' / keyCode 229——因此 `.enter` 修饰符匹配不上，用户必须按两次回车
 * （第一次选词、第二次才提交），体验割裂。
 *
 * 方案：keydown 时若处于组合态且物理键是 Enter（选词回车），记标记；compositionend
 * 时若标记存在则补一次提交——「选词回车一步到位」。鼠标点击候选词不会经过组合态回车，
 * 不误提交；PC 键盘回车走正常分支。
 *
 * 用法：
 *   const { onKeydown, onCompositionEnd } = useImeEnter(submit)
 *   <input @keydown="onKeydown" @compositionend="onCompositionEnd" />
 */
export const useImeEnter = (onEnter: () => void) => {
  // 组合态中是否出现过物理回车（选词确认）
  let imeEnterPending = false

  const onKeydown = (e: KeyboardEvent) => {
    if (e.isComposing || e.keyCode === 229) {
      // IME 组合中：物理 Enter = 选词确认，记录待提交
      if (e.code === 'Enter') imeEnterPending = true
      return
    }
    if (e.key === 'Enter') {
      e.preventDefault()
      onEnter()
    }
  }

  const onCompositionEnd = () => {
    if (imeEnterPending) {
      imeEnterPending = false
      onEnter()
    }
  }

  return { onKeydown, onCompositionEnd }
}
