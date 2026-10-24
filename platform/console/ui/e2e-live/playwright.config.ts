import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { defineConfig, devices } from '@playwright/test'

// This package is ESM ("type": "module"), and Playwright loads its config as
// true ESM too (verified live: __dirname throws `ReferenceError` here) — so
// every absolute path below is derived from `import.meta.url`, never the
// CommonJS global.
const here = path.dirname(fileURLToPath(import.meta.url))

// Real backend, real browser — the actual e2e tier, distinct from both:
//   - vitest (jsdom, no browser)
//   - ../e2e (a real browser, but against a MOCKED API — a rendering smoke
//     test for the one thing jsdom cannot check, never a behaviour proof)
//
// `platform/manager/test/e2e` is a real backend too, but asserts only through
// HTTP/CRD reads — it proves the manager and console API, never what a person
// sees. This is the missing combination: a real cluster, driven through the
// actual rendered UI.
//
// Point it at any already-running install (the manager's own e2e pack's
// cluster, rancher-desktop, a live deploy) — this config starts nothing and
// owns no cluster lifecycle, on purpose: that is `test/e2e`'s job, this is
// read-only verification against whatever is already there.
//
//   E2E_LIVE_CONSOLE_URL=http://localhost:18099 \
//   npx playwright test --config e2e-live/playwright.config.ts
//
// `globalSetup` arranges every fixture these specs read (support/) — real
// signals, a real coordinate/invoke, real kubectl — and mints the one
// session cookie every spec reuses via `storageState`, so a spec never drives
// the login form itself. `globalSetup`'s own return value tears down the
// manager port-forward it opens for that arranging; it owns no cluster
// lifecycle beyond that, same as before.
export default defineConfig({
  testDir: '.',
  fullyParallel: false, // shares one console session's login state
  // ONE WORKER. `fullyParallel: false` only serialises tests WITHIN a file —
  // Playwright still spreads different spec FILES across its default worker
  // pool, which would mean several real Chromium instances hammering one
  // single-replica console pod on a one-node cluster other sessions are
  // ALSO using concurrently. Two genuine failures were chased down here
  // under a 9-worker run and root-caused to a test bug and a real backend
  // defect respectively (see runs-view.spec.ts) — NOT to concurrency — but
  // nothing about this suite needs parallel speed, and running serially is
  // what the comment above ("shares one console session's login state")
  // already assumed.
  workers: 1,
  outputDir: path.join(here, 'test-results'),
  reporter: [['list'], ['html', { outputFolder: path.join(here, 'playwright-report'), open: 'never' }]],
  timeout: 30_000,
  globalSetup: './global-setup.ts',
  use: {
    baseURL: process.env.E2E_LIVE_CONSOLE_URL ?? 'http://localhost:18099',
    storageState: path.join(here, '.auth', 'state.json'),
    screenshot: 'on',
    video: 'retain-on-failure',
    trace: 'retain-on-failure',
    ...devices['Desktop Chrome'],
  },
})
