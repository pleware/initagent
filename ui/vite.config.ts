import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { defineConfig, type Plugin } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import { authSourceDir, prepareAuthBg } from './scripts/prepare-auth-bg.mjs'
import { themeBootPlugin } from '../web/theme/boot-plugin.mjs'

const here = dirname(fileURLToPath(import.meta.url))

function authBgPlugin(): Plugin {
  return {
    name: 'auth-bg',
    async buildStart() {
      await prepareAuthBg()
    },
    configureServer(server) {
      server.watcher.add(authSourceDir)
      server.watcher.on('change', async (file) => {
        if (!file.replaceAll('\\', '/').includes('/assets/auth/')) return
        await prepareAuthBg({ force: true })
        server.ws.send({ type: 'full-reload' })
      })
    },
  }
}

// In dev, the Go hub runs on :4200 and Vite proxies API traffic to it.
export default defineConfig({
  plugins: [themeBootPlugin(), authBgPlugin(), react(), tailwindcss()],
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
    port: 5174,
    strictPort: true,
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
