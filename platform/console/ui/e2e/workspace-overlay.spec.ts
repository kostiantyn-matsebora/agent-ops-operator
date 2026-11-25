import { expect, test, type Page } from './support/coverage'
import { TESTID, testid } from '../src/model/testids'
import { at, base, startHarness, type Harness } from './support/mock'

// Nothing in a thread may sit over the list. A notice with a copy control (a
// conversation that has no console thread, with the command that fixes it) once
// left an invisible, viewport-sized hit area on top of the whole page: every
// click on the list, the nav and the header went to it, and the console looked
// frozen. A real browser, a real mouse: what lies under the pointer must be what
// is drawn there.

const HINTED = {
  ...base, name: 'hinted-member', title: 'ha-ops: report', phase: 'Closed', joined: false, created: at(-200), lastActivity: at(-150),
  causedBy: { parent: 'incident-root', entry: 'ha-ops' },
  joinHint: { reason: 'this conversation is not attributable to a pipeline, so there is no single wiring to patch', fix: 'add console to the channelRefs of the Pipeline that originated it' },
}

let h: Harness
test.beforeEach(async () => {
  h = await startHarness({ rows: [HINTED] })
})
test.afterEach(async () => {
  await h.stop()
})

test.use({ viewport: { width: 1800, height: 900 } })

/** The element a real pointer would hit at the centre of a locator. */
async function hitOf(page: Page, target: ReturnType<Page['locator']>) {
  return target.evaluate((el) => {
    const r = el.getBoundingClientRect()
    const top = document.elementFromPoint(r.x + r.width / 2, r.y + r.height / 2)
    return { isTarget: top === el || el.contains(top), top: top ? `${top.tagName}.${String(top.className)}` : null }
  })
}

test('a thread with a copy control in its notice leaves the list, nav and header clickable', async ({ page }) => {
  await page.goto(`${h.url}/conversations/${HINTED.name}?look=workspace`)
  await expect(page.getByTestId(TESTID.joinHintFix)).toBeVisible()
  const targets = [
    page.getByTestId(testid.open('working-1')),
    page.getByRole('link', { name: 'Queues' }),
    page.getByRole('button', { name: 'Select', exact: true }),
    page.getByTestId(TESTID.newConversation),
  ]
  for (const t of targets) {
    const hit = await hitOf(page, t)
    expect(hit.isTarget, `something covers the page: the pointer would hit ${hit.top}`).toBe(true)
  }
  // And a real click still lands.
  await page.getByTestId(testid.open('working-1')).click({ timeout: 5000 })
  await expect(page).toHaveURL(/working-1/)
})

test('no copy control in a thread leaves anything positioned against the page', async ({ page }) => {
  await page.goto(`${h.url}/conversations/${HINTED.name}?look=workspace`)
  await expect(page.getByTestId(TESTID.joinHintFix)).toBeVisible()
  const escaped = await page.evaluate(() => {
    const out: string[] = []
    for (const cp of document.querySelectorAll('.wk-cp')) {
      for (const pseudo of ['::before', '::after']) {
        const s = getComputedStyle(cp, pseudo)
        if (s.content === 'none' || s.position !== 'absolute') continue
        // Positioned against something inside the thread, never the page.
        let anchor: Element | null = cp
        while (anchor && getComputedStyle(anchor).position === 'static') anchor = anchor.parentElement
        if (!anchor || anchor === document.documentElement || anchor === document.body) out.push(`${cp.className}${pseudo}`)
      }
    }
    return out
  })
  expect(escaped).toEqual([])
})
