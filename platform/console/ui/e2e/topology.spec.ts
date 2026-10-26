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

  // the two wiring hops stream traffic; the third (into the manager) names no
  // wiring edge. `data-stream` is `EdgeLine`'s own attribute for this — the
  // `data-flashing`/`data-animated` attributes and the SMIL `<animate>` count
  // this block asserted before were never rendered by `Canvas.tsx`, which
  // drives traffic through `TrafficLayer`'s own moving marks instead. Fixed
  // here rather than left red for `console-e2e`'s first run; a stronger
  // assertion on the marks themselves is a `traffic.ts`-owning follow-up, not
  // this wiring change's.
  await expect(fed).toHaveAttribute('data-stream', 'on')
  await expect(dispatched).toHaveAttribute('data-stream', 'on')
  await expect(posted).toHaveAttribute('data-stream', 'off')

  // an edge with traffic animates; one without is visibly idle rather than absent
  await expect(posted).toHaveAttribute('data-tone', 'idle')

  // Hide the capability layer first. Nothing is hidden by default
  // (`VIEW_DEFAULTS`), so the spec folds the failing toolset away itself. That
  // is exactly the case worth pinning: a filter that could conceal a broken
  // component without saying so is the one way this view could mislead, so the
  // counts are computed BEFORE filtering and the panel names the concealment.
  // The class checkboxes live inside the Display panel's own collapsible
  // content, folded away by default, so it has to be opened first.
  await page.getByLabel('Toggle display panel').click()
  await page.getByLabel('Toolsets').click()
  await expect(page.getByTestId('node-mcptoolsets/admin')).toHaveCount(0)
  await expect(page.getByTestId('health-summary')).toContainText('1 failing')
  await expect(page.getByText(/hidden element\(s\) are failing/)).toBeVisible()

  const nodeCountBefore = await page.locator('[data-testid^="node-"]').count()

  // bringing the class back adds the node and moves no count
  await page.getByLabel('Toolsets').click()
  await expect(page.getByTestId('node-mcptoolsets/admin')).toHaveCount(1)
  await expect(page.locator('[data-testid^="node-"]')).toHaveCount(nodeCountBefore + 1)
  await expect(page.getByTestId('health-summary')).toContainText('1 failing')

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

  // The capability layer starts shown (`VIEW_DEFAULTS` hides nothing), so the
  // failing toolset is in view before scoping. An element the display panel is
  // hiding is not one the SCOPE put out of view — those are two different
  // statements, and this test is about the second.
  await expect(page.getByTestId('node-mcptoolsets/admin')).toHaveCount(1)

  // Scaled to the area and centred in it. The fit centres `pictureBounds` — the
  // nodes' positions plus its own fixed margins, deeper below a node than above
  // it for the label — so that is what is measured, not the painted box, whose
  // labels run as wide as their names do.
  const fit = await page.evaluate(() => {
    const host = document.querySelector('[data-testid="graph-viewport"]') as HTMLElement
    const h = host.getBoundingClientRect()
    let k = 1
    const at = [...document.querySelectorAll('[data-testid="graph-canvas"] > [data-testid^="node-"]')].map((n) => {
      const m = (n as SVGGElement).getScreenCTM()!
      k = m.a
      return { x: m.e, y: m.f }
    })
    const xs = at.map((p) => p.x)
    const ys = at.map((p) => p.y)
    const [x0, x1] = [Math.min(...xs) - 96 * k, Math.max(...xs) + 96 * k]
    const [y0, y1] = [Math.min(...ys) - 54 * k, Math.max(...ys) + 80 * k]
    return {
      fillsWidth: x1 - x0 > h.width * 0.4,
      dx: Math.abs((x0 - h.left) - (h.right - x1)),
      dy: Math.abs((y0 - h.top) - (h.bottom - y1)),
    }
  })
  expect(fit.fillsWidth).toBe(true)
  expect(fit.dx).toBeLessThanOrEqual(4)
  expect(fit.dy).toBeLessThanOrEqual(4)

  // Inside the canvas only: the double-click also selects, opening `node-panel`.
  const nodes = () => page.getByTestId('graph-canvas').locator('[data-testid^="node-"]').count()
  const whole = await nodes()

  // The toolset hangs off the pipeline, so scoping the CHANNEL at one hop must
  // leave it out — and must still bring in the pipeline that posts to it, which
  // is upstream. Following edges forward only would scope a channel to nothing.
  // Scoping is a double-click; a single click only selects.
  await page.getByTestId('node-channels/console').dblclick()
  await page.getByTestId('scope-bar').waitFor()
  await expect(page.getByTestId('scope-bar')).toContainText('Scoped to console')

  await page.getByLabel('1 hop').click()
  await expect(page.getByTestId('node-pipelines/ops')).toBeVisible()
  await expect(page.getByTestId('node-mcptoolsets/admin')).toHaveCount(0)

  // A failing element put out of view is named, not silently dropped.
  await expect(page.getByText(/failing element\(s\) are outside this scope/)).toBeVisible()
  await expect(page.getByTestId('failing-hidden')).toContainText('Toolset')

  await page.getByLabel('reset scope').click()
  expect(await nodes()).toBe(whole)
  await expect(page.getByTestId('scope-bar')).toHaveCount(0)

})
