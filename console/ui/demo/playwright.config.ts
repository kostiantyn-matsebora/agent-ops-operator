import { defineConfig, devices } from '@playwright/test'

// The recording has its OWN config, for the same reasons the screenshot capture
// does: it is not a test, it shares no baseURL or webServer with e2e/, and a
// flaky run must fail rather than quietly encode a different story on the
// second attempt.
export default defineConfig({
  testDir: '.',
  reporter: 'list',
  retries: 0,
  workers: 1,
  use: { ...devices['Desktop Chrome'] },
})
