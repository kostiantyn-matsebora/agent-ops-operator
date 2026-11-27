// Every e2e-live spec navigates through THIS `test`/`page`, never
// `@playwright/test`'s own — so the look under test is pinned without
// touching ~20 call sites' `page.goto('/some/path')`. The convention mirrors
// `../e2e/look-conformance.spec.ts`: `?look=<id>` on the URL, merged with
// whatever query string (none today, see the grep in #510's task 6.3) a
// spec's own path already carries, never clobbering it.
//
// `look` is a per-PROJECT option (`playwright.config.ts`'s `projects[].use.look`),
// never a per-spec choice — a spec that cared which look it ran under would
// defeat the point of running the whole suite twice.
//
// Most specs navigate via the ordinary `page` fixture. One
// (`34-stream-survives-manager-restart.spec.ts`) opens a SECOND context and
// pages of its own through `browser.newContext()` — so `browser` and
// `context` are wrapped too, not just `page`, or that spec's own `goto`
// calls would silently run look-less.
import { test as base, expect, type Browser, type BrowserContext, type Page } from '@playwright/test'

export type Look = 'classic' | 'workspace'

function withLook(url: string, look: Look): string {
  const [path, query = ''] = url.split('?')
  const params = new URLSearchParams(query)
  if (!params.has('look')) params.set('look', look)
  const qs = params.toString()
  return qs ? `${path}?${qs}` : path
}

function pinLook(page: Page, look: Look): void {
  const originalGoto = page.goto.bind(page)
  page.goto = ((url: string, options?: Parameters<Page['goto']>[1]) =>
    originalGoto(withLook(url, look), options)) as Page['goto']
}

function pinContext(context: BrowserContext, look: Look): void {
  const originalNewPage = context.newPage.bind(context)
  context.newPage = (async (...args: Parameters<BrowserContext['newPage']>) => {
    const page = await originalNewPage(...args)
    pinLook(page, look)
    return page
  }) as BrowserContext['newPage']
}

export const test = base.extend<{}, { look: Look }>({
  // WORKER-scoped, not test-scoped: `browser` (below) is itself a worker
  // fixture, and a worker fixture may only depend on another worker fixture.
  // The default only matters outside a configured project (e.g. `--list`);
  // every real run sets this per-project in `playwright.config.ts`.
  look: ['classic', { scope: 'worker', option: true }],

  page: async ({ page, look }, use) => {
    pinLook(page, look)
    await use(page)
  },

  context: async ({ context, look }, use) => {
    pinContext(context, look)
    await use(context)
  },

  browser: async ({ browser, look }, use) => {
    const originalNewContext = browser.newContext.bind(browser)
    browser.newContext = (async (...args: Parameters<Browser['newContext']>) => {
      const context = await originalNewContext(...args)
      pinContext(context, look)
      return context
    }) as Browser['newContext']
    await use(browser)
  },
})

export { expect }
export type { Page }
