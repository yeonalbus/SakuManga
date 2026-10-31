// demo/api/auth.mjs
// 认证：演示环境固定返回一个已登录的管理员，密码任意。
import { json } from './util.mjs'

/** 演示用户（对应界面右上角/设置页显示的身份信息） */
export const DEMO_USER = {
  id: 1,
  username: 'demo',
  role: 'admin',
  allowDownload: true,
  isEx: false,
  ipb_member_id: '',
  createdAt: '2026-01-01T00:00:00Z',
}

export function register(add) {
  add('GET', '/api/v1/auth/me', () => json({ user: DEMO_USER }))
  add('POST', '/api/v1/auth/login', () => json({ token: 'demo-token', user: DEMO_USER }))
  add('POST', '/api/v1/auth/logout', () => json({ message: '已退出登录' }))
}
