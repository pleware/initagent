import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

const here = dirname(fileURLToPath(import.meta.url))

// In dev, the Go hub runs on :4200 and Vite proxies API traffic to it.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    dedupe: ['react', 'react-dom'],
    alias: {
      react: resolve(here, 'node_modules/react'),
      'react/jsx-runtime': resolve(here, 'node_modules/react/jsx-runtime.js'),
      '@ia/web': resolve(here, '../web'),
      '@base-ui/react': resolve(here, 'node_modules/@base-ui/react'),
      cn: resolve(here, 'node_modules/cn'),
      'class-variance-authority': resolve(here, 'node_modules/class-variance-authority'),
      '@phosphor-icons/react': resolve(here, 'node_modules/@phosphor-icons/react'),
    },
  },
  server: {
    // index.css imports the theme tokens from ../internal/brand/themes, which
    // is outside this project root. Dev needs it readable to serve and watch.
    fs: { allow: ['..'] },
    proxy: {
      '/api': {
        target: 'http://localhost:4200',
        ws: true,
      },
      '/install': 'http://localhost:4200',
    },
  },
})
