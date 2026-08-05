// src/types/user.ts
// 当前登录用户信息（与后端 handlers/auth.go userPublic 返回结构对应）

export interface UserInfo {
  id: number
  username: string
  role: 'admin' | 'member'
  allowDownload: boolean
  isEx: boolean
  ipb_member_id: string
  createdAt: string
}

export interface LoginResult {
  token: string
  user: UserInfo
}
