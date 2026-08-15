import { defineConfig, devices } from '@playwright/test'

// The screenshot capture has its OWN config, because it is not a test.
//
// It shares no settings with e2e/: no baseURL (the spec serves the bundle on a
// port it picks itself), no webServer (a `vite preview` would be a second
// server doing the same job), and no retries — a flaky capture must fail rather
// than quietly write a different image on the second attempt.
export default defineConfig({
  testDir: '.',
  reporter: 'list',
  retries: 0,
  workers: 1,
  use: { ...devices['Desktop Chrome'] },
})
