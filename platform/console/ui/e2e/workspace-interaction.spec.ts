import { expect, test, type Page } from '@playwright/test'
import { TESTID, testid } from '../src/model/testids'
import { at, base, startHarness, type Harness } from './support/mock'

// The workspace look must STAY OPERABLE. A real browser, a realistic install
// (dozens of coordinator trees, each with members), the activity stream
// running hot, and a long run of the interactions a person actually makes:
// expand and collapse, open a conversation, switch its tabs, select, resize,
// fold the list. Every action must take effect, and between actions the main
// thread must answer a frame — a stuck page fails here, not in front of a user.

const TREES = 40
const MEMBERS = 3

function install() {
  const rows: object[] = []
  for (let i = 0; i < TREES; i++) {
    const root = `tree-${String(i).padStart(2, '0')}`
    rows.push({
      ...base, name: root, title: `Incident ${i}: the device is unreachable`, coordinator: 'incident', phase: 'Idle',
      created: at(-5000 - i * 60), lastActivity: at(-i * 60), unread: i % 4 === 0, unreadCount: i % 4 === 0 ? 2 : 0, runCount: 3,
      lastMessage: { kind: 'agent', text: `Escalated: ${i}` },
    })
    for (let m = 0; m < MEMBERS; m++) {
      rows.push({
        ...base, name: `${root}-m${m}`, title: `member ${m} of ${i}`, phase: 'Idle', joined: false, created: at(-4900 - i * 60),
        lastActivity: at(-i * 60 + m), causedBy: { parent: root, entry: ['triage', 'ops', 'observe'][m] },
      })
    }
  }
  return rows
}

let h: Harness
test.beforeEach(async () => {
  h = await startHarness({ rows: install() })
})
test.afterEach(async () => {
  await h.stop()
})

/** Resolves only if the page paints a frame soon — a frozen main thread never does. */
async function responsive(page: Page, what: string) {
  const alive = await page.evaluate(
    () => new Promise<boolean>((resolve) => {
      const timer = setTimeout(() => resolve(false), 4000)
      requestAnimationFrame(() => {
        clearTimeout(timer)
        resolve(true)
      })
    }),
  )
  expect(alive, `the page stopped answering after: ${what}`).toBe(true)
}

async function open(page: Page) {
  await page.goto(`${h.url}/conversations?look=workspace`)
  await expect(page.getByTestId(TESTID.lookSwitcher)).toHaveAttribute('data-active', 'workspace', { timeout: 15_000 })
  await expect(page.getByTestId(testid.row('tree-00'))).toBeVisible()
}

let cursor = 100
const hot = (n: number) => {
  for (let i = 0; i < n; i++) {
    cursor++
    h.push('activity', {
      cursor: String(cursor), ts: new Date().toISOString(), kind: i % 2 ? 'run.dispatched' : 'signal.received',
      runId: `run-${cursor}`, status: 'ok', conversation: `tree-${String(i % TREES).padStart(2, '0')}`,
    })
  }
}

test.describe('a busy install', () => {
  test.use({ viewport: { width: 1400, height: 900 } })

  test('expands and collapses a coordinator, again and again, while events stream in', async ({ page }) => {
    await open(page)
    const toggle = page.getByRole('button', { name: /^(Expand|Collapse) Incident 0/ })
    for (let i = 0; i < 12; i++) {
      hot(25)
      const before = await toggle.getAttribute('aria-expanded')
      await toggle.click()
      await expect(toggle).toHaveAttribute('aria-expanded', before === 'true' ? 'false' : 'true')
      await responsive(page, `toggle ${i}`)
    }
    // Every member row shows when its root is expanded.
    if ((await toggle.getAttribute('aria-expanded')) !== 'true') await toggle.click()
    for (let m = 0; m < MEMBERS; m++) await expect(page.getByTestId(testid.row(`tree-00-m${m}`))).toBeVisible()
  })

  test('opens conversation after conversation, each one showing its own thread', async ({ page }) => {
    await open(page)
    for (let i = 0; i < 10; i++) {
      hot(25)
      const name = `tree-${String(i).padStart(2, '0')}`
      await page.getByTestId(testid.open(name)).click()
      await expect(page).toHaveURL(new RegExp(`/conversations/${name}`))
      await expect(page.locator('section.tp .wk-th h2')).toContainText(`Incident ${i}`)
      await responsive(page, `open ${name}`)
    }
  })

  test('a long, mixed run of interactions never leaves the page stuck', async ({ page }) => {
    test.setTimeout(120_000)
    await open(page)
    const steps: [string, () => Promise<void>][] = [
      ['expand all', async () => {
        const expand = page.getByRole('button', { name: 'Expand all', exact: true })
        if (await expand.count()) await expand.click()
      }],
      ['flatten', async () => { await page.getByRole('button', { name: 'flatten', exact: true }).click() }],
      ['nest again', async () => { await page.getByRole('button', { name: 'nest', exact: true }).click() }],
      ['select mode on', async () => { await page.getByRole('button', { name: 'Select', exact: true }).click() }],
      ['select all', async () => { await page.getByRole('button', { name: 'Select all' }).click() }],
      ['clear all', async () => { await page.getByRole('button', { name: 'Clear all' }).click() }],
      ['select mode off', async () => { await page.getByRole('button', { name: 'Done selecting' }).click() }],
      ['open a root', async () => { await page.getByTestId(testid.open('tree-01')).click() }],
      ['runs tab', async () => { await page.getByTestId(testid.view('runs')).click() }],
      ['yaml tab', async () => { await page.getByTestId(testid.view('yaml')).click() }],
      ['conversation tab', async () => { await page.getByRole('tab', { name: 'Conversation' }).click() }],
      ['row menu', async () => { await page.getByRole('button', { name: 'Actions for Incident 3: the device is unreachable' }).click() }],
      ['menu closes on Escape', async () => { await page.keyboard.press('Escape') }],
      ['collapse the list', async () => { await page.getByRole('button', { name: 'Collapse the conversation list' }).click() }],
      ['expand the list', async () => { await page.getByRole('button', { name: 'Expand the conversation list' }).click() }],
      ['open a member', async () => {
        await page.getByTestId(testid.open('tree-02-m1')).click()
        await expect(page).toHaveURL(/tree-02-m1/)
        await expect(page.locator('section.tp .wk-th h2')).toContainText('member 1 of 2')
      }],
      ['show closed off', async () => { await page.getByTestId(TESTID.showClosed).uncheck() }],
      ['show closed on', async () => { await page.getByTestId(TESTID.showClosed).check() }],
      ['search', async () => { await page.getByRole('searchbox', { name: 'Search conversations' }).fill('Incident 1') }],
      ['clear search', async () => { await page.getByRole('searchbox', { name: 'Search conversations' }).fill('') }],
      ['new conversation', async () => { await page.getByTestId(TESTID.newConversation).click() }],
      ['back to a conversation', async () => { await page.getByTestId(testid.open('tree-04')).click() }],
    ]
    const timings: [string, number][] = []
    for (let round = 0; round < 4; round++) {
      for (const [what, run] of steps) {
        hot(15)
        const t = Date.now()
        await run()
        await responsive(page, `${what} (round ${round})`)
        timings.push([`${what} (round ${round})`, Date.now() - t])
      }
    }
    // No single action takes seconds: a slow one is a freeze in the making.
    const slow = timings.filter(([, ms]) => ms > 1500)
    expect(slow, `slow actions: ${JSON.stringify(slow)}`).toEqual([])
    // After all of it, the page still does what it is asked.
    await page.getByTestId(testid.open('tree-05')).click()
    await expect(page.locator('section.tp .wk-th h2')).toContainText('Incident 5')
  })

  test('the list pane resizes by dragging and by keyboard without sticking', async ({ page }) => {
    await open(page)
    const gutter = page.getByRole('separator', { name: 'Resize the conversation list' })
    const before = Number(await gutter.getAttribute('aria-valuenow'))
    const box = (await gutter.boundingBox())!
    await page.mouse.move(box.x + box.width / 2, box.y + 100)
    await page.mouse.down()
    await page.mouse.move(box.x + 120, box.y + 100, { steps: 5 })
    await page.mouse.up()
    expect(Number(await gutter.getAttribute('aria-valuenow'))).toBeGreaterThan(before)
    // Released: moving the pointer no longer resizes, and rows still open.
    const settled = await gutter.getAttribute('aria-valuenow')
    await page.mouse.move(box.x + 300, box.y + 200, { steps: 3 })
    expect(await gutter.getAttribute('aria-valuenow')).toBe(settled)
    await page.getByTestId(testid.open('tree-03')).click()
    await expect(page).toHaveURL(/tree-03/)
  })
})

// Nothing a person opens may be left covering the page: a drawer's scrim and a
// dialog's scrim each swallow every click until dismissed, which looks exactly
// like a frozen UI.
for (const [label, size] of [['a tablet', { width: 1000, height: 800 }], ['a phone', { width: 390, height: 844 }]] as const) {
  test.describe(label, () => {
    test.use({ viewport: size })

    test('opening and closing the navigation drawer never leaves a scrim in the way', async ({ page }) => {
      await open(page)
      for (let i = 0; i < 4; i++) {
        await page.getByRole('button', { name: /menu|navigation/i }).first().click()
        await expect(page.locator('.wk-scrim')).toBeVisible()
        await page.keyboard.press('Escape')
        await expect(page.locator('.wk-scrim')).toHaveCount(0)
        await responsive(page, `drawer ${i}`)
      }
      // The list still answers, and so does a navigation link inside the drawer.
      await page.getByTestId(testid.open('tree-01')).click()
      await expect(page).toHaveURL(/tree-01/)
      await page.getByRole('button', { name: /menu|navigation/i }).first().click()
      await page.getByRole('link', { name: 'Queues' }).click()
      await expect(page).toHaveURL(/queues/)
      await expect(page.locator('.wk-scrim')).toHaveCount(0)
    })
  })
}

test.describe('a writer', () => {
  test.use({ viewport: { width: 1400, height: 900 } })

  test('closing a selection through its dialog, cancelling it, and carrying on', async ({ page }) => {
    await open(page)
    await page.getByRole('button', { name: 'Select', exact: true }).click()
    await page.getByTestId(testid.open('tree-01')).click()
    await page.getByTestId(TESTID.closeSelected).click()
    await expect(page.locator('.wk-modal-scrim')).toBeVisible()
    await page.keyboard.press('Escape')
    await expect(page.locator('.wk-modal-scrim')).toHaveCount(0)
    await page.getByRole('button', { name: 'Done selecting' }).click()
    // The dialog is gone and the page takes clicks again.
    await page.getByTestId(testid.open('tree-02')).click()
    await expect(page).toHaveURL(/tree-02/)
    await responsive(page, 'after the dialog')
  })
})
