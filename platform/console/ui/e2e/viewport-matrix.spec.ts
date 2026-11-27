import { expect, test, type Page } from './support/coverage'
import type { Locator } from '@playwright/test'
import { TESTID, testid } from '../src/model/testids'
import { ROOT, startHarness, type Harness } from './support/mock'
import { VIEWPORTS, viewportUse, type NamedViewport } from './support/viewports'

// console-looks: "Every look is responsive and tested at set viewports" — the
// six named viewports, each look, every page. At every one: no sideways
// scroll, every control reachable, and on a touch viewport every control at
// least 44 pixels in its smaller dimension.

test('the six viewports are exactly the ones console-looks/spec.md names, in its order', () => {
  // Pinned once against the openspec table itself, so a hand edit to either
  // side shows up here rather than only as a silent drift between the two.
  expect(VIEWPORTS.map((v) => ({ name: v.name, width: v.width, height: v.height, touch: v.touch }))).toEqual([
    { name: 'phone', width: 390, height: 844, touch: true },
    { name: 'tablet portrait', width: 768, height: 1024, touch: true },
    { name: 'tablet landscape', width: 1024, height: 768, touch: true },
    { name: 'laptop', width: 1280, height: 800, touch: false },
    { name: 'desktop', width: 1440, height: 900, touch: false },
    { name: 'wide', width: 2560, height: 1300, touch: false },
  ])
})

const LOOKS = ['classic', 'workspace'] as const

// One representative path per page the console has, plus a locator both
// looks name that proves the page actually rendered before the controls on
// it are scanned — a look that fails to reach this point fails loudly here,
// not as a silent zero-control page passing every check vacuously.
const PAGES: readonly [string, string, (page: Page) => Locator][] = [
  ['/overview', 'Overview', (page) => page.getByRole('heading', { level: 1, name: 'Installation' })],
  ['/queues', 'Queues', (page) => page.getByRole('heading', { level: 1, name: 'Queues and capacity' })],
  ['/config', 'Config', (page) => page.getByRole('heading', { level: 1, name: 'Configuration' })],
  ['/topology', 'Topology', (page) => page.getByRole('navigation', { name: 'Breadcrumb' }).filter({ hasText: 'Topology' })],
  ['/conversations', 'Conversations list', (page) => page.getByTestId(testid.row(ROOT.name))],
  [`/conversations/${ROOT.name}`, 'Conversation thread', (page) => page.getByTestId(testid.view('runs'))],
]

// Known, pre-existing, structural gaps in the CLASSIC look alone — measured
// live (never asserted from a guess): a 2px sideways overflow at phone width
// on every page, and PatternFly's default control sizing falling short of
// 44px on every page at every touch viewport. Both predate #510's remaining
// items and are out of this change's "never in classic" scope — pinned as
// `test.fail()` so the real assertion stays unweakened and the suite still
// names them, rather than silently going green over a real gap.
const CLASSIC_PHONE_OVERFLOWS = (look: string, vp: string) => look === 'classic' && vp === 'phone'
const CLASSIC_TOUCH_TARGETS_ARE_TOO_SMALL = (look: string, vp: NamedViewport) => look === 'classic' && vp.touch
// Measured live: at phone width classic's Topology toolbar pushes its zoom
// controls to a negative x — off the left edge entirely, not merely small.
// Not reproduced at tablet portrait/landscape (wide enough there). Same
// "never in classic" scope boundary as the two gaps above.
const CLASSIC_PHONE_TOPOLOGY_ZOOM_IS_OFFSCREEN = (look: string, vp: string, pageName: string) =>
  look === 'classic' && vp === 'phone' && pageName === 'Topology'

async function open(page: Page, look: string, path: string) {
  await page.goto(`${h.url}${path}?look=${look}`)
  await expect(page.getByTestId(TESTID.lookSwitcher)).toHaveAttribute('data-active', look, { timeout: 15_000 })
}

/** Every control a person can act on, excluding a closed/off-canvas nav drawer's own links (its own reachability is a toggle-and-see check elsewhere) and a disabled control (not a target at all while disabled). */
function controls(page: Page) {
  const nav = `[data-testid="${TESTID.appNav}"]`
  const skip = `:not(${nav} *):not([disabled]):not([aria-disabled="true"])`
  return page.locator(
    [`button:visible${skip}`, `a:visible${skip}`, `[role="button"]:visible${skip}`,
      `[role="tab"]:visible${skip}`, `input:visible${skip}`, `select:visible${skip}`].join(', '),
  )
}

/** Every control, for the touch-target-size check — a disabled control still has to clear 44px for when it isn't. */
function everyControlIncludingDisabled(page: Page) {
  const nav = `[data-testid="${TESTID.appNav}"]`
  const skip = `:not(${nav} *)`
  return page.locator(
    [`button:visible${skip}`, `a:visible${skip}`, `[role="button"]:visible${skip}`,
      `[role="tab"]:visible${skip}`, `input:visible${skip}`, `select:visible${skip}`].join(', '),
  )
}

let h: Harness
test.beforeEach(async () => { h = await startHarness() })
test.afterEach(async () => { await h.stop() })

for (const look of LOOKS) {
  for (const vp of VIEWPORTS) {
    test.describe(`${look} at ${vp.name}`, () => {
      test.use(viewportUse(vp))

      for (const [path, pageName, ready] of PAGES) {
        test(`${pageName} does not scroll sideways`, async ({ page }) => {
          test.fail(CLASSIC_PHONE_OVERFLOWS(look, vp.name), 'classic overflows by 2px at phone width on every page — measured, pre-existing, out of #510\'s remaining scope')
          await open(page, look, path)
          await expect(ready(page)).toBeVisible()
          const { scrollWidth, clientWidth } = await page.evaluate(() => ({
            scrollWidth: document.documentElement.scrollWidth, clientWidth: document.documentElement.clientWidth,
          }))
          expect(scrollWidth, `${look} ${pageName} at ${vp.name}: scrolls sideways (${scrollWidth} > ${clientWidth})`).toBeLessThanOrEqual(clientWidth)
        })

        test(`${pageName} has every control reachable`, async ({ page }) => {
          test.fail(CLASSIC_PHONE_TOPOLOGY_ZOOM_IS_OFFSCREEN(look, vp.name, pageName), 'classic\'s Topology toolbar pushes its zoom controls off the left edge at phone width — measured, pre-existing, out of #510\'s remaining scope')
          await open(page, look, path)
          await expect(ready(page)).toBeVisible()
          const els = controls(page)
          const n = await els.count()
          expect(n, `${look} ${pageName} at ${vp.name}: no controls found at all`).toBeGreaterThan(0)
          const unreachable: string[] = []
          for (let i = 0; i < n; i++) {
            const el = els.nth(i)
            await el.scrollIntoViewIfNeeded()
            const box = await el.boundingBox()
            if (!box || box.width <= 0 || box.height <= 0) {
              unreachable.push(await el.evaluate((node) => node.outerHTML.slice(0, 80)))
              continue
            }
            const hitsItself = await el.evaluate(
              (node, [x, y]) => {
                const hit = document.elementFromPoint(x, y)
                return hit === node || (hit != null && node.contains(hit))
              },
              [box.x + box.width / 2, box.y + box.height / 2] as [number, number],
            )
            if (!hitsItself) unreachable.push(await el.evaluate((node) => node.outerHTML.slice(0, 80)))
          }
          expect(unreachable, `${look} ${pageName} at ${vp.name}: unreachable controls`).toEqual([])
        })

        if (vp.touch) {
          test(`${pageName} keeps every control at least 44px`, async ({ page }) => {
            test.fail(CLASSIC_TOUCH_TARGETS_ARE_TOO_SMALL(look, vp), 'classic falls short of the 44px touch target on every page at every touch viewport — PatternFly\'s default control sizing, measured, pre-existing, out of #510\'s remaining scope')
            await open(page, look, path)
            await expect(ready(page)).toBeVisible()
            const els = everyControlIncludingDisabled(page)
            const n = await els.count()
            const small: string[] = []
            for (let i = 0; i < n; i++) {
              const box = await els.nth(i).boundingBox()
              if (box && box.width > 0 && box.height > 0 && Math.min(box.width, box.height) < 44) {
                small.push(await els.nth(i).evaluate((node) => node.outerHTML.slice(0, 80)))
              }
            }
            expect(small, `${look} ${pageName} at ${vp.name}: controls under 44px`).toEqual([])
          })
        }
      }
    })
  }
}
