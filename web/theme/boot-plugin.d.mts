export function themeBootPlugin(): {
  name: string
  transformIndexHtml(html: string): string
}
