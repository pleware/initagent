import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const bootFile = resolve(dirname(fileURLToPath(import.meta.url)), 'boot.js')

/** Inline `boot.js` just after the charset meta so `data-theme` is set before CSS. */
export function themeBootPlugin() {
  const script = `<script>${readFileSync(bootFile, 'utf8')}</script>`
  return {
    name: 'initagent-theme-boot',
    transformIndexHtml(html) {
      const needle = '<meta charset="UTF-8" />'
      if (html.includes(needle)) {
        return html.replace(needle, `${needle}\n    ${script}`)
      }
      return html.replace('<head>', `<head>\n    ${script}`)
    },
  }
}
