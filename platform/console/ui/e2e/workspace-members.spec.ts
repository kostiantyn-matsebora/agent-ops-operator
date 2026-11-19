import { expect, test, type Page } from '@playwright/test'
import { TESTID, testid } from '../src/model/testids'
import { at, base, startHarness, type Harness } from './support/mock'

// A coordinator that has invoked members and is waiting for their answers must
// SAY so — on its list row and in its thread — expand to show them, and stop
// saying it the moment they finish. In a real browser, with the live stream.

const ROOT = {
  ...base, name: 'wait-root', title: 'Disk pressure on the node', coordinator: 'incident', phase: 'Idle',
  created: at(-300), lastActivity: at(-5), lastMessage: { kind: 'agent', text: 'Asking triage and ops.' },
}
const member = (name: string, entry: string, over: object = {}) => ({
  ...base, name, title: `${entry}: look at it`, phase: 'Working', presence: true, joined: false, inflight: { runId: `run-${name}` },
  created: at(-20), lastActivity: at(-2), causedBy: { parent: ROOT.name, entry }, ...over,
})

let h: Harness
let rows: object[]
test.beforeEach(async () => {
  rows = [ROOT, member('wait-root-triage', 'triage'), member('wait-root-ops', 'ops')]
  h = await startHarness({ rows })
})
test.afterEach(async () => {
  await h.stop()
})

async function open(page: Page, path = '/conversations') {
  await page.goto(`${h.url}${path}?look=workspace`)
  await expect(page.getByTestId(TESTID.lookSwitcher)).toHaveAttribute('data-active', 'workspace', { timeout: 15_000 })
}

/** The server's view of a conversation changed: the mock holds it, the stream announces it. */
function change(row: object, type: 'ADDED' | 'MODIFIED' = 'MODIFIED') {
  const name = (row as { name: string }).name
  const at_ = rows.findIndex((r) => (r as { name: string }).name === name)
  if (at_ >= 0) rows[at_] = row
  else rows.push(row)
  h.push('delta', { type, kind: 'conversations', name, conversationRow: row })
}

test.use({ viewport: { width: 1400, height: 900 } })

test('the list says a coordinator is waiting, names who, and shows the members at work', async ({ page }) => {
  await open(page)
  const row = page.getByTestId(testid.row(ROOT.name))
  await expect(row).toHaveAttribute('data-status', 'waiting')
  await expect(page.getByTestId(`waiting-${ROOT.name}`)).toContainText('Waiting')
  await expect(row).toContainText('waiting on triage, ops')
  // The members are in view, not folded away behind the arrow.
  await expect(page.getByTestId(testid.row('wait-root-triage'))).toBeVisible()
  await expect(page.getByTestId(testid.row('wait-root-ops'))).toBeVisible()
  await expect(page.getByTestId(testid.row('wait-root-ops'))).toHaveAttribute('data-status', 'working')
})

test('a fold the viewer chose does not hide the work while it is under way', async ({ page }) => {
  await open(page)
  // Folded by default; unfold everything, then fold everything again: the wait still shows.
  await page.getByRole('button', { name: 'Expand all' }).click()
  await page.getByRole('button', { name: 'Collapse all' }).click()
  await expect(page.getByTestId(testid.row('wait-root-triage'))).toBeVisible()
})

test('the thread says it is waiting, with the members as links, and clears when they finish', async ({ page }) => {
  await open(page, `/conversations/${ROOT.name}`)
  await expect(page.getByTestId('waiting-pill')).toContainText('Waiting on 2')
  const card = page.getByTestId('waiting-on-members')
  await expect(card).toContainText('Waiting on 2 members')
  await card.getByRole('link', { name: 'triage' }).click()
  await expect(page).toHaveURL(/wait-root-triage/)
  await page.goBack()
  // One finishes: the count drops. Both finish: the wait is over everywhere.
  change(member('wait-root-triage', 'triage', { phase: 'Idle', presence: false, inflight: undefined }))
  await expect(page.getByTestId('waiting-pill')).toContainText('Waiting on 1')
  change(member('wait-root-ops', 'ops', { phase: 'Idle', presence: false, inflight: undefined }))
  await expect(page.getByTestId('waiting-on-members')).toHaveCount(0)
  await expect(page.getByTestId('waiting-pill')).toHaveCount(0)
  await expect(page.getByTestId(testid.row(ROOT.name))).toHaveAttribute('data-status', 'idle')
})

test('every member waited on is listed, none clipped, when the thread overflows its pane', async ({ page }) => {
  // The mock's incident root has a real transcript; three more members are at work under it.
  for (const [n, e] of [['a', 'k8s-ops'], ['b', 'k8s-ops'], ['c', 'k8s-ops']]) {
    rows.push({ ...member(`incident-root-${n}`, e), causedBy: { parent: 'incident-root', entry: e } })
  }
  await page.setViewportSize({ width: 1400, height: 460 })
  await open(page, '/conversations/incident-root')
  const card = page.getByTestId('waiting-on-members')
  await card.scrollIntoViewIfNeeded()
  await expect(card).toContainText('Waiting on 3 members')
  const items = card.locator('li')
  await expect(items).toHaveCount(3)
  const box = (await card.boundingBox())!
  for (let i = 0; i < 3; i++) {
    const b = (await items.nth(i).boundingBox())!
    expect(b.height, `member ${i} has no room`).toBeGreaterThan(12)
    expect(b.y + b.height, `member ${i} spills out of the card`).toBeLessThanOrEqual(box.y + box.height + 1)
  }
})

test('a member invoked while the coordinator is open appears nested and starts the wait', async ({ page }) => {
  rows.splice(1, 2) // nothing invoked yet
  await open(page, `/conversations/${ROOT.name}`)
  await expect(page.getByTestId(testid.row(ROOT.name))).toHaveAttribute('data-status', 'idle')
  await expect(page.getByTestId('waiting-on-members')).toHaveCount(0)
  change(member('wait-root-late', 'observe'), 'ADDED')
  await expect(page.getByTestId(testid.row('wait-root-late'))).toBeVisible()
  await expect(page.getByTestId(testid.row(ROOT.name))).toHaveAttribute('data-status', 'waiting')
  await expect(page.getByTestId('waiting-on-members')).toContainText('observe')
  // The row has an arrow now, and it works.
  const toggle = page.getByRole('button', { name: /^(Collapse|Expand) Disk pressure/ })
  await expect(toggle).toBeVisible()
})
