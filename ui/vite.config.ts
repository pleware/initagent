import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// In dev, the Go hub runs on :4200 and Vite proxies API traffic to it.
export default defineConfig({
  plugins: [react(), tailwindcss()],
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
