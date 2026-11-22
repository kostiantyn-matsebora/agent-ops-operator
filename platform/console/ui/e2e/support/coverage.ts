import { mkdirSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import { randomUUID } from 'node:crypto'
import { expect, test as base } from '@playwright/test'

// BROWSER COVERAGE FOR THE TESTING STATION. With E2E_COVERAGE=1 the production
// build served to these specs is instrumented (vite.config.ts), and every test
// hands the page's counters to .nyc_output/ when it ends. `lcov.mjs` then folds
// them into one lcov file, which `.github/scripts/coverage-diff.py` reads.
//
// WITHOUT THE VARIABLE THIS IS THE PLAIN `test`: no instrumentation, no file
// written, nothing to run differently. Every spec imports `test` and `expect`
// from here instead of from Playwright, which is the one place a page's
// lifetime can be hooked without editing each spec.
export const collecting = process.env.E2E_COVERAGE === '1'
const outDir = join(process.cwd(), '.nyc_output')

export const test = base.extend<{ coverage: void }>({
  coverage: [
    async ({ page }, use) => {
      await use()
      if (!collecting) return
      // The counters are a global the plugin's instrumentation owns. A page the
      // test already closed, or one that never loaded the app, has none.
      const data = await page.evaluate(() => (window as unknown as { __coverage__?: unknown }).__coverage__).catch(() => undefined)
      if (!data) return
      mkdirSync(outDir, { recursive: true })
      writeFileSync(join(outDir, `${randomUUID()}.json`), JSON.stringify(data))
    },
    { auto: true },
  ],
})

export { expect }
export type { Page } from '@playwright/test'
