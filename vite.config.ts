import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  server: {
    port: 5173,
    host: '0.0.0.0',
    proxy: {
      // 🎯 转发后端接口代理
      '/api': {
        target: 'http://127.0.0.1:8081', // 👈 关键点：将 localhost 改为 127.0.0.1，解决 ECONNREFUSED
        changeOrigin: true,
      },
    },
  },
})
