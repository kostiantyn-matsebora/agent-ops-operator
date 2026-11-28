import { expect, test } from './support/coverage'
import { testid } from '../src/model/testids'
import { at, base, ROOT, startHarness, type Harness } from './support/mock'

// console-workspace-feedback: "Nothing changes size while it animates" —
// "a layout-shift measurement over the header, the tab bar and every list
// row reports none beyond the single insertion." A real browser only: jsdom
// never lays anything out, so it cannot report a CLS entry at all.

test.use({ viewport: { width: 1600, height: 900 } })

let h: Harness
test.beforeEach(async () => { h = await startHarness() })
test.afterEach(async () => { await h.stop() })

test('one conversation opening and running to its reply shifts nothing but the rows below it', async ({ page }) => {
  await page.addInitScript(() => {
    const w = window as unknown as { __shifts: number[] }
    w.__shifts = []
    new PerformanceObserver((list) => {
      for (const e of list.getEntries() as unknown as { value: number }[]) w.__shifts.push(e.value)
    }).observe({ type: 'layout-shift', buffered: true })
  })
  await page.goto(`${h.url}/conversations/${ROOT.name}?look=workspace`)
  await expect(page.getByTestId(testid.row(ROOT.name))).toBeVisible()
  await expect(page.getByRole('tablist', { name: 'conversation views' })).toBeVisible()

  const header = page.locator('header.wk-top')
  const tabs = page.getByRole('tablist', { name: 'conversation views' })
  // Two rows that stay exactly where they are relative to each other, below
  // where the new row lands — their own gap must not change, whatever the
  // insertion above them does to their absolute position.
  const below = [page.getByTestId(testid.row('working-1')), page.getByTestId(testid.row('pending-1'))]

  const headerBefore = (await header.boundingBox())!
  const tabsBefore = (await tabs.boundingBox())!
  const belowBefore = await Promise.all(below.map((r) => r.boundingBox()))
  const rootBefore = (await page.getByTestId(testid.row(ROOT.name)).boundingBox())!

  // THE one legitimate insertion: a signal opens a new conversation. It is
  // not open in the thread, so nothing about the thread pane is touched.
  const opened = {
    ...base, name: 'arriving', title: 'a freshly opened conversation', source: 'cron', phase: 'Idle',
    created: at(90), lastActivity: at(90),
  }
  h.push('delta', { type: 'ADDED', kind: 'conversations', name: opened.name, conversationRow: opened })
  const newRow = page.getByTestId(testid.row('arriving'))
  await expect(newRow).toBeVisible()
  // `.wk-row.arrived`'s own entrance animation (workspace.css, 0.45s) moves
  // the row by a CSS transform while it plays — real motion, but not a
  // layout shift (the browser excludes transform/opacity from CLS, and
  // `getBoundingClientRect` reports it mid-flight). Measuring positions
  // before it settles would mistake the animation for a shift.
  await page.waitForTimeout(600)
  const newRowBox = (await newRow.boundingBox())!

  // The header and the tab bar never belong to the list — they must not
  // have moved by even one pixel for an insertion inside it.
  expect(await header.boundingBox()).toEqual(headerBefore)
  expect(await tabs.boundingBox()).toEqual(tabsBefore)
  // Every existing row below the insertion moved down by exactly the new
  // row's own height — a consequence of the insertion, not a stray shift —
  // and the two of them stayed the same distance apart.
  const belowAfterInsertion = await Promise.all(below.map((r) => r.boundingBox()))
  for (const [i, box] of belowAfterInsertion.entries()) {
    expect(box!.y - belowBefore[i]!.y).toBeCloseTo(newRowBox.height, 0)
    expect(box!.x).toBe(belowBefore[i]!.x)
  }
  expect(belowAfterInsertion[1]!.y - belowAfterInsertion[0]!.y).toBeCloseTo(belowBefore[1]!.y - belowBefore[0]!.y, 0)
  expect((await page.getByTestId(testid.row(ROOT.name)).boundingBox())!.y - rootBefore.y).toBeCloseTo(newRowBox.height, 0)

  // Mark the point after the one insertion has fully settled: everything
  // from here on is "beyond the single insertion" and must report nothing.
  await page.waitForTimeout(200)
  await page.evaluate(() => { (window as unknown as { __shifts: number[] }).__shifts.length = 0 })

  // THE run: the new conversation (not open in the thread) goes to work and
  // lands its reply, entirely through its own fixed-height row.
  const working = { ...opened, phase: 'Working', presence: true, inflight: { runId: 'run-arriving' }, lastActivity: at(95) }
  h.push('delta', { type: 'MODIFIED', kind: 'conversations', name: opened.name, conversationRow: working })
  await expect(newRow).toHaveAttribute('data-status', 'working')
  const idle = { ...opened, phase: 'Idle', presence: false, lastActivity: at(120), lastMessage: { kind: 'agent', text: 'Done.' } }
  h.push('delta', { type: 'MODIFIED', kind: 'conversations', name: opened.name, conversationRow: idle })
  await expect(newRow).toContainText('Done.')
  await page.waitForTimeout(200)

  // Nothing moved during the run: the header, the tab bar and every row —
  // the new one included — sit exactly where the one insertion left them.
  expect(await header.boundingBox()).toEqual(headerBefore)
  expect(await tabs.boundingBox()).toEqual(tabsBefore)
  for (const [i, r] of below.entries()) expect(await r.boundingBox()).toEqual(belowAfterInsertion[i])
  expect(await newRow.boundingBox()).toEqual(newRowBox)

  // A real browser still reports sub-pixel rounding noise as a vanishingly
  // small entry (observed: ~0.0001, from font/antialiasing settling, not a
  // visible move) — the bounding-box checks above are the precise assertion
  // that nothing actually moved. A real regression shifting even a few
  // pixels of real content reports several orders of magnitude more.
  const shiftsDuringTheRun: number[] = await page.evaluate(() => (window as unknown as { __shifts: number[] }).__shifts)
  const total = shiftsDuringTheRun.reduce((a, b) => a + b, 0)
  expect(total, `unexpected layout-shift beyond the one insertion: ${JSON.stringify(shiftsDuringTheRun)}`).toBeLessThan(0.01)
})
