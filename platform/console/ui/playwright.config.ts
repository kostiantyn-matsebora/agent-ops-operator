import { defineConfig, devices } from '@playwright/test'

// One smoke test, in a real browser.
//
// The Vitest suite already asserts the graph's semantics in jsdom, and that is
// what runs on every change. This exists for the one thing jsdom cannot check:
// that the SVG the app actually ships renders, animates and stays stable when
// the Display panel changes — layout and SMIL animation are exactly what a
// DOM-only renderer will happily report as working.
//
// It is NOT part of `npm test` on purpose: it needs a downloaded browser, and a
// suite that cannot run offline is a suite people skip. Run it deliberately:
//
//   npx playwright install chromium && npm run test:e2e
export default defineConfig({
  testDir: './e2e',
  fullyParallel: true,
  reporter: 'list',
  use: {
    baseURL: 'http://127.0.0.1:4173',
    ...devices['Desktop Chrome'],
  },
  webServer: {
    // Serves the production build against the mock API the spec installs, so
    // the smoke test exercises the SAME bundle the image embeds.
    command: 'npm run build && npm run preview -- --port 4173 --strictPort',
    url: 'http://127.0.0.1:4173',
    reuseExistingServer: true,
    timeout: 180_000,
  },
})
