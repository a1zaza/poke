import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import path from 'path'

// https://vite.dev/config/
export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src')
    }
  },
  server: {
    port: 5173,
    proxy: {
      '/logo.png': {
        target: 'http://localhost:3001',
        changeOrigin: true
      },
      '/api': {
        target: 'http://localhost:3001',
        // 保留浏览器访问的 Host，供后端校验请求来源。
        changeOrigin: false,
        ws: true
      },
      '/ws': {
        target: 'ws://localhost:3001',
        ws: true
      }
    }
  }
})
