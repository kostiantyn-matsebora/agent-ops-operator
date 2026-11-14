import { expect, test, type Page } from '@playwright/test'
import { createServer, type Server, type ServerResponse } from 'node:http'
import { readFile } from 'node:fs/promises'
import { extname, join, normalize } from 'node:path'
import type { AddressInfo } from 'node:net'
import { TESTID, testid } from '../src/model/testids'

// The workspace look in a real browser, against a mocked API — the checks
// jsdom cannot make: geometry (the panes fill the window and the page never
// scrolls), the clipboard, the fixed row height, layout surviving a reload,
// and the phone and tablet arrangements.
//
// It serves the built bundle itself, as stream-chip.spec.ts does, so the
// stream stays open and the connection settles on live.

const T0 = Date.parse('2025-06-11T09:00:00Z')
const at = (s: number) => new Date(T0 + s * 1000).toISOString()

const SESSION = {
  authenticated: true, configured: true, identity: 'e2e@example.com', identitySource: 'forward-auth',
  authMode: 'token', externalAuthenticator: '', writeEnabled: true, canWrite: true, canOriginate: true, metrics: false,
}

const base = { runCount: 1, queued: 0, joined: true, errored: false, unread: false, ageSeconds: 0, deleting: false }

const ROOT = {
  ...base, name: 'incident-root', title: 'Elevated 5xx on checkout', coordinator: 'incident', phase: 'Idle',
  created: at(-60), lastActivity: at(40), unread: true, unreadCount: 1, source: 'cluster-events',
  lastMessage: { kind: 'agent', text: 'Rolled back.' },
  runs: [{
    runId: 'run-1', status: 'succeeded', startedAt: at(0), finishedAt: at(25),
    toolCalls: [{ tool: 'mcp__k8s__get_pods', durationMs: 1500 }, { tool: 'mcp__aops__invoke', durationMs: 900 }],
  }],
}
const MEMBER = {
  ...base, name: 'incident-root-triage', title: 'triage: confirm the restarts', phase: 'Idle', joined: false,
  created: at(5), lastActivity: at(20), causedBy: { parent: 'incident-root', entry: 'triage' },
}
const ROWS = [
  ROOT,
  MEMBER,
  { ...base, name: 'working-1', title: 'payments rollout', pipeline: 'k8s-observe', phase: 'Working', presence: true, inflight: { runId: 'run-w' }, lastActivity: at(30), lastMessage: { kind: 'agent', text: 'Checking the rollout status for the payments deployment, which has a long name' } },
  { ...base, name: 'pending-1', title: 'HighMemoryPressure', pipeline: 'alert-triage', phase: 'Pending', runCount: 0, lastActivity: at(10) },
  { ...base, name: 'errored-1', title: 'nightly report', pipeline: 'nightly-report', phase: 'Idle', errored: true, lastActivity: at(-600) },
  { ...base, name: 'idle-1', title: 'namespaces without quotas', pipeline: 'k8s-observe', phase: 'Idle', lastActivity: at(-3600) },
]

const REPLY_TEXT = [
  '<title>', 'Rolled back checkout-api', '</title>', '',
  '<root-cause>', 'OOM-killed: `CACHE_ENTRIES` went 5000 → 50000.', '</root-cause>', '',
  '<fix>', '`kubectl -n storefront rollout undo deploy/checkout-api`', '</fix>',
].join('\n')
const OWN_TEXT = 'Does it affect **the other two** services?'
const SIGNAL_TEXT = 'checkout-api restarted 5 times in 10 minutes (BackOff).'

const DETAIL = {
  conversation: ROOT,
  object: { kind: 'Conversation', metadata: { name: ROOT.name } },
  yaml: 'kind: Conversation',
  archived: false,
  transcript: [
    { id: 'm1', thread: 'console/incident-root', kind: 'signal', text: SIGNAL_TEXT, severity: 'warning', payload: '{"reason":"BackOff"}', at: at(-60) },
    { id: 'run:run-1', thread: 'console/incident-root', kind: 'agent', text: REPLY_TEXT, at: at(25) },
    { id: 'm3', thread: 'console/incident-root', kind: 'local', text: OWN_TEXT, at: at(40) },
  ],
  events: [
    { cursor: '1', ts: at(0), kind: 'run.dispatched', runId: 'run-1', status: 'ok', conversation: ROOT.name },
    { cursor: '2', ts: at(26), kind: 'context.checkpoint', runId: 'run-1', status: 'ok', conversation: ROOT.name },
    { cursor: '3', ts: at(26), kind: 'run.completed', runId: 'run-1', status: 'ok', conversation: ROOT.name },
  ],
}

const MIME: Record<string, string> = {
  '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css',
  '.svg': 'image/svg+xml', '.woff2': 'font/woff2', '.json': 'application/json',
}

function answer(path: string, query: URLSearchParams): unknown {
  if (path === '/api/session') return SESSION
  if (path === '/api/vocabulary') return { entries: [{ kind: 'coordinator', name: 'incident' }, { kind: 'pipeline', name: 'k8s-observe' }] }
  if (path === '/api/conversations') {
    const page = { total: ROWS.length, unreadTotal: 1, rootTotal: 5, offset: 0, limit: 100, scopes: { working: 1, mine: 0, errored: 1, closed: 0, incident: 1 } }
    return query.get('count') === '1' ? { ...page, items: [] } : { ...page, items: ROWS }
  }
  if (path === `/api/conversations/${ROOT.name}`) return DETAIL
  const m = /^\/api\/conversations\/([^/]+)$/.exec(path)
  const row = m && ROWS.find((r) => r.name === m[1])
  if (row) return { ...DETAIL, conversation: row, transcript: [], events: [] }
  return {}
}

interface Harness {
  url: string
  stop(): Promise<void>
}

async function startHarness(): Promise<Harness> {
  const live = new Set<ServerResponse>()
  const dist = join(process.cwd(), 'dist')
  const server: Server = createServer(async (req, res) => {
    const [path, rawQuery] = (req.url ?? '/').split('?')
    if (path === '/api/stream') {
      res.writeHead(200, { 'content-type': 'text/event-stream', 'cache-control': 'no-cache', connection: 'keep-alive' })
      res.write('event: resync\ndata: {"reason":"connected"}\n\n')
      live.add(res)
      req.on('close', () => live.delete(res))
      return
    }
    if (path.startsWith('/api/')) {
      res.writeHead(200, { 'content-type': 'application/json' })
      res.end(JSON.stringify(answer(path, new URLSearchParams(rawQuery ?? ''))))
      return
    }
    const rel = normalize(path).replace(/^([/\\])+/, '')
    for (const file of [rel, 'index.html']) {
      try {
        const body = await readFile(join(dist, file))
        res.writeHead(200, { 'content-type': MIME[extname(file)] ?? 'application/octet-stream' })
        res.end(body)
        return
      } catch {
        /* SPA fallback */
      }
    }
    res.writeHead(404)
    res.end()
  })
  await new Promise<void>((resolve) => server.listen(0, '127.0.0.1', resolve))
  const { port } = server.address() as AddressInfo
  return {
    url: `http://127.0.0.1:${port}`,
    async stop() {
      for (const res of live) res.end()
      live.clear()
      server.closeAllConnections()
      await new Promise<void>((resolve) => server.close(() => resolve()))
    },
  }
}

let h: Harness
test.beforeEach(async () => {
  h = await startHarness()
})
test.afterEach(async () => {
  await h.stop()
})

async function open(page: Page, path = `/conversations/${ROOT.name}`) {
  await page.goto(`${h.url}${path}?look=workspace`)
  await expect(page.getByTestId(TESTID.lookSwitcher)).toHaveAttribute('data-active', 'workspace', { timeout: 15_000 })
}

const reply = (page: Page) => page.locator(`[data-testid="${TESTID.message}"][data-kind="agent"]`)

test.describe('on a wide screen', () => {
  test.use({ viewport: { width: 2560, height: 1300 } })

  test('fills the viewport: the panes reach both edges less the gutter, and the page does not scroll', async ({ page }) => {
    await open(page)
    await expect(reply(page)).toBeVisible()
    const gutter = 16
    const nav = await page.getByTestId(TESTID.appNav).boundingBox()
    const thread = await page.locator('section.tp').boundingBox()
    const header = await page.getByRole('banner').boundingBox()
    expect(nav!.x).toBeCloseTo(gutter, 0)
    expect(thread!.x + thread!.width).toBeCloseTo(2560 - gutter, 0)
    expect(header!.x).toBeCloseTo(gutter, 0)
    expect(header!.x + header!.width).toBeCloseTo(2560 - gutter, 0)
    // The panes stretch to the window's height, less the bottom padding.
    expect(thread!.y + thread!.height).toBeCloseTo(1300 - 12, 0)
    expect(nav!.y + nav!.height).toBeCloseTo(1300 - 12, 0)
    const page_ = await page.evaluate(() => ({
      scrollW: document.documentElement.scrollWidth,
      scrollH: document.documentElement.scrollHeight,
      w: window.innerWidth,
      h: window.innerHeight,
    }))
    expect(page_.scrollW).toBeLessThanOrEqual(page_.w)
    expect(page_.scrollH).toBeLessThanOrEqual(page_.h)
    await page.mouse.wheel(0, 2000)
    expect(await page.evaluate(() => window.scrollY)).toBe(0)
  })
})

test.describe('on a laptop', () => {
  test.use({ viewport: { width: 1280, height: 800 } })

  test('a finished reply shows only the reply, its Invoked row and the collapsed Run details', async ({ page }) => {
    await open(page)
    const r = reply(page)
    await expect(r.getByText('Rolled back checkout-api')).toBeVisible()
    const invoked = r.getByTestId(`invoked-${MEMBER.name}`)
    await expect(invoked).toBeVisible()
    await expect(invoked).toContainText('Invoked triage')
    const details = r.getByTestId('run-details')
    await expect(details).toBeVisible()
    await expect(details).toHaveAttribute('data-expanded', 'false')
    await expect(details).toContainText('Run details · 25s')
    // No per-event line, no tool row, no working card anywhere in the thread.
    await expect(page.locator('[data-event]')).toHaveCount(0)
    await expect(page.getByText(/^Ran \d+ tools?$/)).toHaveCount(0)
    await expect(page.getByTestId(TESTID.runEvents)).toHaveCount(0)
    await expect(page.getByTestId(TESTID.typing)).toHaveCount(0)
    // The member opens from its row without opening details.
    await invoked.getByRole('link', { name: 'open conversation →' }).click()
    await expect(page).toHaveURL(new RegExp(`/conversations/${MEMBER.name}`))
  })

  test('copies a reply, a person\u2019s message and a signal as markdown, confirming in place', async ({ page, context }) => {
    await context.grantPermissions(['clipboard-read', 'clipboard-write'])
    await open(page)
    const kinds: [string, (text: string) => void][] = [
      ['agent', (t) => expect(t).toBe(REPLY_TEXT)],
      ['local', (t) => expect(t).toBe(OWN_TEXT)],
      ['signal', (t) => {
        expect(t.startsWith(SIGNAL_TEXT)).toBe(true)
        expect(t).toContain('severity: warning')
        expect(t).toContain('source: cluster-events')
        expect(t).toContain('"reason":"BackOff"')
        expect(t).not.toMatch(/<[a-z]+[ >]/)
      }],
    ]
    for (const [kind, check] of kinds) {
      const message = page.locator(`[data-testid="${TESTID.message}"][data-kind="${kind}"]`)
      await message.scrollIntoViewIfNeeded()
      await message.hover()
      await message.getByRole('button', { name: 'Copy message' }).click()
      await expect(message.getByRole('button', { name: 'Copied' })).toBeVisible()
      check(await page.evaluate(() => navigator.clipboard.readText()))
    }
    await expect(page.getByTestId(TESTID.toast)).toHaveCount(0)
  })

  test('every row has one height, whatever its status', async ({ page }) => {
    await open(page, '/conversations')
    await page.getByRole('button', { name: /Expand Elevated/ }).click()
    const rows = page.locator('[data-testid^="row-"]')
    await expect(rows).toHaveCount(ROWS.length)
    const heights = await rows.evaluateAll((els) => els.map((e) => e.getBoundingClientRect().height))
    expect(new Set(heights).size).toBe(1)
    const statuses = await rows.evaluateAll((els) => els.map((e) => e.getAttribute('data-status')))
    expect(new Set(statuses)).toEqual(new Set(['idle', 'working', 'pending', 'failed']))
  })

  test('keeps pane sizes and collapsed states across a reload', async ({ page }) => {
    await open(page)
    await page.getByRole('button', { name: 'Collapse the navigation' }).click()
    const splitter = page.getByRole('separator', { name: 'Resize the conversation list' })
    await splitter.focus()
    await page.keyboard.press('ArrowRight')
    await page.keyboard.press('ArrowRight')
    const before = (await page.locator('section.lp').boundingBox())!.width
    expect(before).toBeCloseTo(340 + 32, 0)
    await page.reload()
    await expect(page.getByTestId(TESTID.appNav)).toHaveClass(/collapsed/)
    await expect.poll(async () => (await page.locator('section.lp').boundingBox())!.width).toBeCloseTo(before, 0)
    const stored = await page.evaluate(() => localStorage.getItem('agentops-console-workspace-layout'))
    expect(JSON.parse(stored!).state).toMatchObject({ navCollapsed: true, listWidth: 372 })
  })

  test('is chosen from the header switcher, with no reload', async ({ page }) => {
    await page.goto(`${h.url}/conversations`)
    const switcher = page.getByRole('combobox', { name: 'Look' })
    await expect(switcher).toBeVisible({ timeout: 15_000 })
    await switcher.selectOption('workspace')
    await expect(page.getByTestId(TESTID.lookSwitcher)).toHaveAttribute('data-active', 'workspace')
    await expect(page.locator('html')).toHaveAttribute('data-look', 'workspace')
    expect(await page.evaluate(() => performance.getEntriesByType('navigation').length)).toBe(1)
  })
})

test.describe('on a tablet in landscape', () => {
  test.use({ viewport: { width: 1024, height: 768 }, hasTouch: true })

  test('keeps the list beside the thread, and the navigation is a drawer', async ({ page }) => {
    await open(page)
    await expect(page.locator('section.lp')).toBeVisible()
    await expect(page.locator('section.tp')).toBeVisible()
    await expect(page.getByTestId(TESTID.appNav)).toBeHidden()
    await page.getByRole('button', { name: 'Toggle navigation' }).click()
    await expect(page.getByTestId(TESTID.appNav)).toBeVisible()
    await page.getByRole('button', { name: 'Close the navigation' }).click({ position: { x: 900, y: 400 } })
    await expect(page.getByTestId(TESTID.appNav)).toBeHidden()
  })
})

test.describe('on a phone', () => {
  test.use({ viewport: { width: 390, height: 844 }, hasTouch: true, isMobile: true })

  test('shows one pane at a time, with a way back, and every message\u2019s copy control at touch size', async ({ page }) => {
    await open(page)
    await expect(page.locator('section.tp')).toBeVisible()
    await expect(page.locator('section.lp')).toBeHidden()
    for (const button of await page.getByRole('button', { name: 'Copy message' }).all()) {
      await expect(button).toBeVisible()
      const box = (await button.boundingBox())!
      expect(Math.min(box.width, box.height)).toBeGreaterThanOrEqual(44)
    }
    await page.getByRole('button', { name: 'Back to the list' }).click()
    await expect(page.locator('section.lp')).toBeVisible()
    await expect(page.locator('section.tp')).toBeHidden()
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390)
  })
})

test('opens on a row the thread a link names', async ({ page }) => {
  await open(page, `/conversations/${ROOT.name}`)
  await expect(page.getByTestId(testid.row(ROOT.name))).toHaveAttribute('aria-current', 'true')
})
