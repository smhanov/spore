import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import path from 'path'

export default defineConfig({
  plugins: [vue()],
  root: path.resolve(__dirname),
  base: '/blog/admin/',
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
})
