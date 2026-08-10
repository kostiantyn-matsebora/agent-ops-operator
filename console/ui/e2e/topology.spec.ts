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
