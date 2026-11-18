import { chromium } from '@playwright/test'
const b = await chromium.launch()
const base = 'http://localhost:18091'
const shots = async (w, h, tag, scheme) => {
  const ctx = await b.newContext({ viewport: { width: w, height: h }, colorScheme: scheme })
  const page = await ctx.newPage()
  const errors = []
  page.on('pageerror', (e) => errors.push(String(e)))
  const visit = async (name, path) => {
    await page.goto(`${base}${path}${path.includes('?') ? '&' : '?'}look=workspace`)
    await page.waitForTimeout(2500)
    await page.screenshot({ path: `.pi/tmp/live/${tag}-${name}.png` })
  }
  await visit('overview', '/overview')
  await visit('queues', '/queues')
  await visit('config', '/config')
  await visit('topology', '/topology')
  await visit('conversations', '/conversations')
  // first conversation row
  const first = await page.locator('[data-testid^="open-"]').first().getAttribute('href')
  if (first) { await page.goto(`${base}${first}?look=workspace`); await page.waitForTimeout(2500); await page.screenshot({ path: `.pi/tmp/live/${tag}-thread.png` }) }
  console.log(tag, 'errors:', errors.length ? errors : 'none')
  await ctx.close()
}
await shots(1400, 900, 'lap', 'light')
await shots(390, 844, 'ph', 'light')
await shots(1400, 900, 'dark', 'dark')
await b.close()
