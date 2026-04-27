import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import path from 'node:path'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: { '@': path.resolve(__dirname, './src') },
  },
  server: {
    port: 5173,
    strictPort: true,
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
        secure: false,
      },
      '/sse': {
        target: 'http://localhost:8080',
        changeOrigin: true,
        secure: false,
        proxyTimeout: 60 * 60 * 1000,
        timeout: 60 * 60 * 1000,
      },
    },
  },
  build: { outDir: 'dist', sourcemap: true },
})
