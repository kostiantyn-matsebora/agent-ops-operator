import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// The build emits into `dist`, which the Go binary embeds with
// `go:embed all:ui/dist`. npm exists at BUILD time only, inside this module and
// its image — no other module and not the manager gains a build step.
export default defineConfig({
  plugins: [react()],
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    // Deterministic chunking keeps the embedded asset set small and stable:
    // PatternFly and the topology renderer are large and change rarely, so
    // splitting them keeps an ordinary UI edit from re-emitting everything.
    rollupOptions: {
      output: {
        manualChunks: {
          patternfly: ['@patternfly/react-core', '@patternfly/react-table', '@patternfly/react-icons'],
          topology: ['@patternfly/react-topology'],
          // PF Charts v8 exports only subpaths — the bare specifier has no
          // entry point, and naming it here fails the build rather than the
          // import.
          charts: ['@patternfly/react-charts/victory'],
          // Syntax grammars change only when a language is added, which is
          // roughly never — while app code changes every release. In the main
          // chunk they would be re-downloaded on every deploy despite being
          // byte-identical, because the hash covers the whole chunk.
          //
          // Split out, they keep their hash across app changes and stay in the
          // browser cache, where `immutable` already promised they would.
          highlight: ['rehype-highlight', 'highlight.js'],
        },
      },
    },
  },
  server: {
    // `npm run dev` proxies to a console running locally, or one you have
    // port-forwarded to 8080. Hardcoded rather than read from the environment so
    // this file needs no Node types.
    proxy: {
      '/api': { target: 'http://localhost:8080', changeOrigin: true },
      '/healthz': { target: 'http://localhost:8080' },
    },
  },
})
