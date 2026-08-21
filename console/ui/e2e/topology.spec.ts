import { expect, test, type Page } from '@playwright/test'

// The topology smoke test: load the graph, drive three synthetic events, and
// assert the animated edges are the ones the events name — then toggle an
// element class and assert layout and health stay stable.

const SESSION = {
  authenticated: true, configured: true, identity: 'e2e',
  writeEnabled: false, canOriginate: false, metrics: false,
}

const TOPOLOGY = {
  consoleChannel: 'console',
  unjoinedPipelines: [],
  synced: {},
  stream: { connected: true, events: 3, resyncs: 0 },
  oldestEvent: new Date(Date.now() - 60_000).toISOString(),
  metricsAvailable: false,
  topology: {
    windowSeconds: 300,
    eventNodeKinds: {
      'signal-source': 'signalsources',
      pipeline: 'pipelines',
      channel: 'channels',
      runtime: 'agentruntimes',
      toolset: 'mcptoolsets',
    },
    nodes: [
      { id: 'signalsources/events', kind: 'signalsources', name: 'events', health: 'ok', active: 0, recent: 0 },
      { id: 'pipelines/ops', kind: 'pipelines', name: 'ops', health: 'ok', active: 1, recent: 4 },
      { id: 'agentruntimes/default', kind: 'agentruntimes', name: 'default', health: 'none', active: 0, recent: 0 },
      { id: 'channels/console', kind: 'channels', name: 'console', health: 'ok', active: 0, recent: 0 },
      { id: 'mcptoolsets/admin', kind: 'mcptoolsets', name: 'admin', health: 'bad', reason: 'Missing', active: 0, recent: 0 },
    ],
    edges: [
      {
        from: 'signalsources/events', to: 'pipelines/ops', kind: 'feeds',
        traffic: { events: 9, errors: 0, ratePerMin: 3 },
      },
      {
        from: 'pipelines/ops', to: 'agentruntimes/default', kind: 'uses',
        traffic: { events: 4, errors: 0, ratePerMin: 1.5 },
      },
      { from: 'pipelines/ops', to: 'channels/console', kind: 'posts' },
      { from: 'pipelines/ops', to: 'mcptoolsets/admin', kind: 'uses' },
    ],
  },
}

/** The three synthetic hops, as the manager would stream them. */
const EVENTS = [
  {
    cursor: '0000000000000001', ts: new Date().toISOString(), kind: 'signal.claimed', status: 'ok',
    from: { kind: 'signal-source', name: 'events' }, to: { kind: 'pipeline', name: 'ops' },
  },
  {
    cursor: '0000000000000002', ts: new Date().toISOString(), kind: 'run.dispatched', status: 'ok',
    from: { kind: 'pipeline', name: 'ops' }, to: { kind: 'runtime', name: 'default' },
  },
  {
    // A hop into the manager: NOT a wiring edge, so it must animate nothing.
    cursor: '0000000000000003', ts: new Date().toISOString(), kind: 'channel.op.completed', status: 'ok',
    from: { kind: 'channel-adapter', name: 'console' }, to: { kind: 'manager', name: 'manager' },
  },
]

async function mockApi(page: Page) {
  // The catch-all goes FIRST: Playwright checks handlers in reverse
  // registration order, so a fallback registered last swallows every specific
  // route above it — the app then renders the login page and none of the
  // assertions below can pass.
  await page.route('**/api/**', (r) => r.fulfill({ json: {} }))
  await page.route('**/api/session', (r) => r.fulfill({ json: SESSION }))
  await page.route('**/api/topology*', (r) => r.fulfill({ json: TOPOLOGY }))
  await page.route('**/api/sources', (r) =>
    r.fulfill({ json: { sources: [], canOriginate: false, writeEnabled: false } }))
  await page.route('**/api/stream', (r) =>
    r.fulfill({
      status: 200,
      headers: { 'content-type': 'text/event-stream' },
      body:
        'event: resync\ndata: {"reason":"connected"}\n\n' +
        EVENTS.map((e) => `event: activity\ndata: ${JSON.stringify(e)}\n\n`).join(''),
    }))
}

test('the graph animates exactly the edges the events name, and hiding a class keeps health honest', async ({
  page,
}) => {
  await mockApi(page)
  await page.goto('/topology')

  const fed = page.getByTestId('edge-signalsources/events->pipelines/ops')
  const dispatched = page.getByTestId('edge-pipelines/ops->agentruntimes/default')
  const posted = page.getByTestId('edge-pipelines/ops->channels/console')

  await expect(fed).toBeVisible()

  // the two wiring hops flash; the third (into the manager) names no wiring edge
  await expect(fed).toHaveAttribute('data-flashing', 'true')
  await expect(dispatched).toHaveAttribute('data-flashing', 'true')
  await expect(posted).toHaveAttribute('data-flashing', 'false')

  // an edge with traffic animates; one without is visibly idle rather than absent
  await expect(fed).toHaveAttribute('data-animated', 'true')
  await expect(posted).toHaveAttribute('data-tone', 'idle')

  // SMIL is actually attached — the thing jsdom cannot tell us
  await expect(fed.locator('animate')).toHaveCount(1)

  const nodeCountBefore = await page.locator('[data-testid^="node-"]').count()

  // The capability layer (toolsets, MCP configs, runtime pods) starts FOLDED
  // AWAY, so the failing toolset is hidden before anything is clicked. That is
  // exactly the case worth pinning: a filter that could conceal a broken
  // component without saying so is the one way this view could mislead, so the
  // counts are computed BEFORE filtering and the panel names the concealment.
  await expect(page.getByTestId('node-mcptoolsets/admin')).toHaveCount(0)
  await expect(page.getByText('1 failing')).toBeVisible()
  await expect(page.getByText(/hidden element\(s\) are failing/)).toBeVisible()

  // bringing the class back adds the node and moves no count
  await page.getByLabel('Toolsets').click()
  await expect(page.getByTestId('node-mcptoolsets/admin')).toHaveCount(1)
  await expect(page.locator('[data-testid^="node-"]')).toHaveCount(nodeCountBefore + 1)
  await expect(page.getByText('1 failing')).toBeVisible()

  // the selection persists across a reload
  await page.reload()
  await expect(page.getByTestId('node-mcptoolsets/admin')).toHaveCount(1)
})

// Scoping, and the fit — both about geometry, which is what this file is for.
//
// The unit suite pins the MECHANISM of the fit (a host with no size defers its
// fit until it has one). This pins the OUTCOME: a real graph, laid out by a real
// browser, ends up scaled to its viewport and centred in it. jsdom reports every
// element as 0x0, so it can never tell those apart.
test('scoping narrows the graph to what an element is connected to, and the graph fits its viewport', async ({
  page,
}) => {
  await mockApi(page)
  await page.goto('/topology')
  await page.getByTestId('node-pipelines/ops').waitFor()

  // Bring the capability layer back first. The failing toolset starts folded
  // away, and an element the display panel is already hiding is not one the
  // SCOPE put out of view — those are two different statements, and this test
  // is about the second.
  await page.getByLabel('Toolsets').click()
  await expect(page.getByTestId('node-mcptoolsets/admin')).toHaveCount(1)

  const nodes = () => page.locator('[data-testid^="node-"]').count()
  const whole = await nodes()

  // The toolset hangs off the pipeline, so scoping the CHANNEL at one hop must
  // leave it out — and must still bring in the pipeline that posts to it, which
  // is upstream. Following edges forward only would scope a channel to nothing.
  await page.getByTestId('node-channels/console').click()
  await page.getByTestId('scope-bar').waitFor()
  await expect(page.getByTestId('scope-bar')).toContainText('Scoped to console')

  await page.getByLabel('1 hop').click()
  await expect(page.getByTestId('node-pipelines/ops')).toBeVisible()
  await expect(page.getByTestId('node-mcptoolsets/admin')).toHaveCount(0)

  // A failing element put out of view is named, not silently dropped.
  await expect(page.getByTestId('scope-failing')).toContainText('mcptoolsets')

  await page.getByLabel('reset scope').click()
  expect(await nodes()).toBe(whole)
  await expect(page.getByTestId('scope-bar')).toHaveCount(0)

  // Scaled to the area and centred in it.
  const fit = await page.evaluate(() => {
    const host = document.querySelector('[data-testid="graph-viewport"]') as HTMLElement
    const canvas = document.querySelector('[data-testid="graph-canvas"]') as SVGGElement
    const h = host.getBoundingClientRect()
    const c = canvas.getBoundingClientRect()
    return {
      fillsWidth: c.width > h.width * 0.4,
      dx: Math.abs((c.left - h.left) - (h.right - c.right)),
      dy: Math.abs((c.top - h.top) - (h.bottom - c.bottom)),
    }
  })
  expect(fit.fillsWidth).toBe(true)
  expect(fit.dx).toBeLessThanOrEqual(4)
  expect(fit.dy).toBeLessThanOrEqual(4)
})
