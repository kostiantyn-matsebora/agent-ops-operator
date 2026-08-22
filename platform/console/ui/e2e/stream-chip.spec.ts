import { expect, test } from '@playwright/test'
import { createServer, type Server } from 'node:http'
import { readFile } from 'node:fs/promises'
import { extname, join, normalize } from 'node:path'
import type { AddressInfo } from 'node:net'

// The masthead chip, in a real browser.
//
// This is the one thing jsdom cannot check about it: how an actual EventSource
// behaves when the server answers with an HTTP ERROR rather than dropping the
// connection. The browser retries a drop by itself, but on a 502 (console pod
// rolling) or a 401 (session expired) it sets readyState to CLOSED and gives up
// PERMANENTLY. Nothing reopened it, so the chip sat on "stream disconnected"
// and the graph stayed frozen until someone reloaded the page.
//
// A viewer that cannot tell "the link died" from "the system is idle" is the
// failure this surface exists to prevent, and a chip that only tells the truth
// after F5 is that failure with extra steps.
//
// It serves the built bundle itself rather than using page.route, because the
// behaviour under test is the LIFETIME of an SSE connection: route.fulfill
// delivers headers and body together, so the stream would open and close in the
// same tick and the chip would never paint "live" at all.

const SESSION = {
  authenticated: true, configured: true, identity: 'e2e',
  writeEnabled: false, canOriginate: false, metrics: false,
}

const OVERVIEW = {
  namespace: 'agent-ops',
  manager: { version: 'e2e', leader: 'e2e', runtimeSlots: { inUse: 0, max: 5, waiting: 0 } },
  stream: { connected: true, events: 0, resyncs: 0 },
  runtimes: [], workloads: [], adapters: [], problems: [],
}

const MIME: Record<string, string> = {
  '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css',
  '.svg': 'image/svg+xml', '.woff2': 'font/woff2', '.json': 'application/json',
}

/** How the stream endpoint is currently answering. */
type StreamMode = 'ok' | 'http-error'

interface Harness {
  server: Server
  url: string
  /** Flip what /api/stream does; open connections are cut so the client re-asks. */
  setMode(m: StreamMode): void
  /** How many times the client has opened the stream. */
  opens(): number
  /** Shut down, including the streams deliberately left open. */
  stop(): Promise<void>
}

async function startHarness(): Promise<Harness> {
  let mode: StreamMode = 'ok'
  let opens = 0
  const live = new Set<import('node:http').ServerResponse>()
  const dist = join(process.cwd(), 'dist')

  const server = createServer(async (req, res) => {
    const path = (req.url ?? '/').split('?')[0]

    if (path === '/api/stream') {
      opens++
      if (mode === 'http-error') {
        // The case the browser will NOT retry on its own.
        res.writeHead(502, { 'content-type': 'text/plain' })
        res.end('bad gateway')
        return
      }
      res.writeHead(200, {
        'content-type': 'text/event-stream',
        'cache-control': 'no-cache',
        connection: 'keep-alive',
      })
      res.write('event: resync\ndata: {"reason":"connected"}\n\n')
      // Deliberately NOT ended: a real stream stays open, which is what lets
      // the chip settle on "live" instead of flapping.
      live.add(res)
      req.on('close', () => live.delete(res))
      return
    }
    if (path === '/api/session') {
      res.writeHead(200, { 'content-type': 'application/json' })
      res.end(JSON.stringify(SESSION))
      return
    }
    if (path === '/api/overview') {
      res.writeHead(200, { 'content-type': 'application/json' })
      res.end(JSON.stringify(OVERVIEW))
      return
    }
    if (path.startsWith('/api/')) {
      res.writeHead(200, { 'content-type': 'application/json' })
      res.end('{}')
      return
    }

    // static: the SAME bundle the image embeds, with SPA fallback
    const rel = normalize(path).replace(/^([/\\])+/, '')
    for (const file of [rel, 'index.html']) {
      try {
        const body = await readFile(join(dist, file))
        res.writeHead(200, { 'content-type': MIME[extname(file)] ?? 'application/octet-stream' })
        res.end(body)
        return
      } catch {
        /* fall through to index.html */
      }
    }
    res.writeHead(404)
    res.end()
  })

  await new Promise<void>((resolve) => server.listen(0, '127.0.0.1', resolve))
  const { port } = server.address() as AddressInfo

  return {
    server,
    url: `http://127.0.0.1:${port}`,
    setMode(m) {
      mode = m
      // Cut whatever is open so the client notices immediately.
      for (const res of live) res.end()
      live.clear()
    },
    opens: () => opens,
    async stop() {
      for (const res of live) res.end()
      live.clear()
      // An open SSE would otherwise keep server.close() waiting forever.
      server.closeAllConnections()
      await new Promise<void>((resolve) => server.close(() => resolve()))
    },
  }
}

test.describe('the masthead stream chip', () => {
  let h: Harness

  test.beforeEach(async () => {
    h = await startHarness()
  })

  test.afterEach(async () => {
    await h.stop()
  })

  test('shows live while the stream is healthy', async ({ page }) => {
    await page.goto(h.url)

    await expect(page.getByText('live', { exact: true })).toBeVisible({ timeout: 15_000 })
    await expect(page.getByText('stream disconnected')).toHaveCount(0)
  })

  test('recovers on its own after the browser gives up on the stream', async ({ page }) => {
    await page.goto(h.url)
    await expect(page.getByText('live', { exact: true })).toBeVisible({ timeout: 15_000 })

    // The console starts refusing with an HTTP error — EventSource closes for
    // good and will never retry by itself.
    const opensBefore = h.opens()
    h.setMode('http-error')
    await expect(page.getByText('stream disconnected')).toBeVisible({ timeout: 20_000 })

    // The client must actually ASK again and be REFUSED, or the
    // permanent-failure path is never exercised and this test proves nothing —
    // cutting the connection alone is the case the browser retries by itself.
    await expect.poll(() => h.opens(), { timeout: 20_000 }).toBeGreaterThan(opensBefore)
    const opensWhileDown = h.opens()

    // The console comes back. Before this fix nothing reopened the stream, so
    // the chip stayed wrong until the page was reloaded.
    h.setMode('ok')
    await expect(page.getByText('live', { exact: true })).toBeVisible({ timeout: 30_000 })

    // It recovered by RECONNECTING, not because the page reloaded.
    expect(h.opens()).toBeGreaterThan(opensWhileDown)
    const navigations = await page.evaluate(() => performance.getEntriesByType('navigation').length)
    expect(navigations).toBe(1)
  })
})
