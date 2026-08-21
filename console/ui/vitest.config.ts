import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'

// Test config is separate from vite.config.ts so the build config stays a pure
// UserConfig � mixing `test` into it makes the type check fail on a field Vite
// itself does not declare. This file is not in tsconfig's `include` for the same
// reason: vitest ships its own copy of Vite, and type-checking both against each
// other produces a wall of structurally-identical-but-nominally-different errors.
export default defineConfig({
  plugins: [react()],
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./src/test-setup.ts'],
    // e2e/ and screenshots/ are Playwright's — they need a real browser and are
    // run deliberately, not as part of `npm test`. screenshots/ was missing,
    // which made a clean tree report a failing suite: vitest collected a
    // Playwright spec and choked on test.describe.configure().
    exclude: ['node_modules/**', 'dist/**', 'e2e/**', 'screenshots/**'],
  },
})
