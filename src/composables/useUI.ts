import { ref, reactive } from 'vue'

// ==========================================
// 1. Toast 消息通知 (非阻塞)
// ==========================================
export type ToastType = 'info' | 'success' | 'warning' | 'error'

export interface ToastItem {
  id: number
  message: string
  type: ToastType
}

const toasts = ref<ToastItem[]>([])
let toastId = 0

function showToast(message: string, type: ToastType = 'info', duration = 3000) {
  const id = ++toastId
  toasts.value.push({ id, message, type })

  setTimeout(() => {
    toasts.value = toasts.value.filter((t) => t.id !== id)
  }, duration)
}

// ==========================================
// 2. Modal 对话框 (阻塞式替代 alert / confirm / prompt)
// ==========================================
export interface ModalOptions {
  title?: string
  message: string
  defaultValue?: string // 供 prompt 模式下的输入默认值
  mode?: 'alert' | 'confirm' | 'prompt'
  confirmText?: string
  cancelText?: string
}

type ModalResolve = (value: unknown) => void

const modalState = reactive({
  isOpen: false,
  title: '',
  message: '',
  inputValue: '',
  mode: 'alert' as 'alert' | 'confirm' | 'prompt',
  confirmText: '确定',
  cancelText: '取消',
  resolve: null as ModalResolve | null,
})

/**
 * 打开模态框，返回 Promise。
 * @template T 通过调用方（alert/confirm/prompt）显式指定返回类型
 */
function openModal<T = string | boolean | null>(options: ModalOptions): Promise<T> {
  return new Promise((resolve) => {
    modalState.title = options.title || (options.mode === 'prompt' ? '请输入' : '提示')
    modalState.message = options.message
    modalState.inputValue = options.defaultValue || ''
    modalState.mode = options.mode || 'alert'
    modalState.confirmText = options.confirmText || '确定'
    modalState.cancelText = options.cancelText || '取消'
    modalState.resolve = resolve as ModalResolve
    modalState.isOpen = true
  })
}

function handleConfirm() {
  modalState.isOpen = false
  if (modalState.resolve) {
    if (modalState.mode === 'prompt') {
      modalState.resolve(modalState.inputValue)
    } else if (modalState.mode === 'confirm') {
      modalState.resolve(true)
    } else {
      modalState.resolve(true)
    }
  }
}

function handleCancel() {
  modalState.isOpen = false
  if (modalState.resolve) {
    if (modalState.mode === 'confirm') {
      modalState.resolve(false)
    } else {
      modalState.resolve(null)
    }
  }
}

// ==========================================
// 导出统一 hook
// ==========================================
export function useUI() {
  return {
    toasts,
    modalState,
    toast: {
      info: (msg: string) => showToast(msg, 'info'),
      success: (msg: string) => showToast(msg, 'success'),
      warning: (msg: string) => showToast(msg, 'warning'),
      error: (msg: string) => showToast(msg, 'error'),
    },
    modal: {
      alert: (msg: string, title?: string) =>
        openModal<boolean>({ message: msg, title, mode: 'alert' }),
      confirm: (msg: string, title?: string) =>
        openModal<boolean>({ message: msg, title, mode: 'confirm' }),
      prompt: (msg: string, defaultValue = '', title?: string) =>
        openModal<string>({ message: msg, defaultValue, title, mode: 'prompt' }),
    },
    handleConfirm,
    handleCancel,
  }
}
