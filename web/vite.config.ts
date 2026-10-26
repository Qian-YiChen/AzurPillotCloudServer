import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// 管理后台由 Go 服务端挂载在 /admin 前缀下，故 base 必须一致；
// 开发模式下 API 请求代理到本机 Go 服务。
export default defineConfig({
  plugins: [react()],
  base: '/admin/',
  build: {
    outDir: '../internal/adminui/dist',
    emptyOutDir: true,
  },
  server: {
    port: 5173,
    proxy: {
      '/api': 'http://127.0.0.1:8080',
    },
  },
})
