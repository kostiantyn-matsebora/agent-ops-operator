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
    // e2e/, e2e-live/, screenshots/ and demo/ are Playwright's — they need a
    // real browser and are run deliberately, not as part of `npm test`. Each
    // was missing here in turn, and each made a clean tree report a failing
    // suite: vitest collected a Playwright spec and choked on
    // test.describe.configure().
    exclude: ['node_modules/**', 'dist/**', 'e2e/**', 'e2e-live/**', 'screenshots/**', 'demo/**'],
    // Coverage is the APPLICATION's, so only src/. Without `include` the lcov
    // reports the Playwright configs and every harness as uncovered source,
    // which is what the analysis dashboard would then count.
    coverage: { include: ['src/**'] },
    // vitest's default (`css: false`) STUBS every `.css` import to an empty
    // string, `?raw` query included — the one way a theme/contrast test can
    // read the actual stylesheet it is asserting on (`theme.test.tsx`, item
    // 27). Safe to flip on for the whole suite: `main.tsx` is the only
    // ordinary (non-`?raw`) `.css` import anywhere under `src/`, and no test
    // renders it, so nothing else here currently depends on the stub.
    css: true,
  },
})
