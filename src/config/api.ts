// src/config/api.ts

// 优先读取 Vite 环境变量，若未定义则兜底使用 '/api/v1'
export const API_BASE = import.meta.env.VITE_API_BASE || '/api/v1'
