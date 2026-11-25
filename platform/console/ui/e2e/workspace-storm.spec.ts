import { expect, test } from './support/coverage'
import { testid } from '../src/model/testids'
import { at, base, startHarness } from './support/mock'

// A busy install streams hundreds of changes in a burst — runs starting, members
// finishing, messages landing. The page must keep answering the person using it:
// a burst that blocks the main thread for seconds is a frozen console, however
// correct each change is.

const TREES = 60

test.use({ viewport: { width: 1600, height: 900 } })

test('a burst of stream events never blocks the page, and clicks still land', async ({ page }) => {
  test.setTimeout(120_000)
  const rows: Record<string, unknown>[] = []
  for (let i = 0; i < TREES; i++) {
    const root = `tree-${i}`
    rows.push({ ...base, name: root, title: `Incident ${i}`, coordinator: 'incident', phase: 'Idle', created: at(-9000 - i), lastActivity: at(-i), runCount: 3, lastMessage: { kind: 'agent', text: 'x'.repeat(400) } })
    for (let m = 0; m < 3; m++) {
      rows.push({ ...base, name: `${root}-m${m}`, title: `m${m}`, phase: 'Working', presence: true, joined: false, causedBy: { parent: root, entry: 'triage' }, created: at(-8000), lastActivity: at(-i) })
    }
  }
  const h = await startHarness({ rows })
  try {
    await page.addInitScript(() => {
      const w = window as unknown as { __long: number[] }
      w.__long = []
      new PerformanceObserver((list) => list.getEntries().forEach((e) => w.__long.push(Math.round(e.duration)))).observe({ entryTypes: ['longtask'] })
    })
    await page.goto(`${h.url}/conversations?look=workspace`)
    await page.getByTestId(testid.row('tree-0')).waitFor()

    let cursor = 1000
    const burst = (count: number) => {
      for (let i = 0; i < count; i++) {
        const name = `tree-${i % TREES}-m${i % 3}`
        const row = { ...rows.find((r) => r.name === name), lastActivity: new Date().toISOString(), presence: i % 2 === 0, phase: i % 2 ? 'Idle' : 'Working' }
        h.push('delta', { type: 'MODIFIED', kind: 'conversations', name, conversationRow: row })
        h.push('activity', { cursor: String(++cursor), ts: new Date().toISOString(), kind: 'run.dispatched', runId: `r${cursor}`, status: 'ok', conversation: name })
      }
    }
    burst(1500)
    // A person clicks in the middle of it. The click must land promptly.
    const started = Date.now()
    await page.getByTestId(testid.open('tree-3')).click({ timeout: 10_000 })
    await expect(page).toHaveURL(/tree-3/)
    expect(Date.now() - started, 'a click during a burst took too long to land').toBeLessThan(3000)

    await page.waitForTimeout(1500)
    const long = await page.evaluate(() => (window as unknown as { __long: number[] }).__long)
    const worst = Math.max(0, ...long)
    expect(worst, `the main thread was blocked for ${worst}ms in one task (${JSON.stringify(long)})`).toBeLessThan(400)
  } finally {
    await h.stop()
  }
})
