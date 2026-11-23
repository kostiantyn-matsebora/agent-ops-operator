import { expect, test, type Page } from './support/coverage'
import { TESTID, testid } from '../src/model/testids'
import { at, MEMBER, ROOT, startHarness, type Harness } from './support/mock'

// EVERY LOOK OFFERS THE SAME CONVERSATION ACTIONS (console-looks). A look may
// draw them however it likes; none may omit one another has. Each look is
// opened over the same mocked API and probed through the model's stable test
// identifiers and the labels the actions carry, and the two sets are compared,
// so a failure names the action and the look that lacks it.

const CLOSED = {
  name: 'closed-1', title: 'archived triage', phase: 'Closed', runCount: 1, queued: 0, joined: true,
  errored: false, unread: false, ageSeconds: 0, deleting: false, created: at(-7200), lastActivity: at(-7000),
}
const VOCABULARY = [
  { kind: 'builtin', name: 'exit', position: 'thread', description: 'release the runtime' },
  { kind: 'builtin', name: 'close', position: 'thread', description: 'end the conversation' },
]

const LOOKS = ['classic', 'workspace'] as const

let h: Harness
test.beforeEach(async () => {
  h = await startHarness({ rows: [CLOSED], vocabulary: VOCABULARY })
})
test.afterEach(async () => {
  await h.stop()
})

async function open(page: Page, look: string, path: string) {
  await page.goto(`${h.url}${path}?look=${look}`)
  await expect(page.getByTestId(TESTID.lookSwitcher)).toHaveAttribute('data-active', look, { timeout: 15_000 })
}

const visible = (page: Page, id: string) => page.getByTestId(id).first().isVisible()

/** The actions a look exposes, each as a yes or no. */
async function exposed(page: Page, look: string): Promise<Record<string, boolean>> {
  const found: Record<string, boolean> = {}

  // A root's thread.
  await open(page, look, `/conversations/${ROOT.name}`)
  await expect(page.getByTestId(testid.view('runs'))).toBeVisible({ timeout: 15_000 })
  for (const view of ['runs', 'graph', 'sequence', 'yaml']) found[`thread view: ${view}`] = await visible(page, testid.view(view))
  found['thread commands as chips'] = await visible(page, TESTID.quickChips)
  await page.getByRole('textbox', { name: 'message' }).fill('/')
  found['slash typeahead'] = await visible(page, TESTID.commandTypeahead)
  await page.getByRole('textbox', { name: 'message' }).fill('')

  // A member's thread offers no graph or sequence.
  await open(page, look, `/conversations/${MEMBER.name}`)
  await expect(page.getByTestId(testid.view('runs'))).toBeVisible()
  found['member has no graph view'] = !(await visible(page, testid.view('graph')))

  // The list: a modifier-click selects, the bar carries the batch actions.
  await open(page, look, '/conversations')
  await expect(page.getByTestId(testid.row('working-1'))).toBeVisible({ timeout: 15_000 })
  await page.getByTestId(testid.open('working-1')).click({ modifiers: ['Control'] })
  found['select with a modifier-click'] = await visible(page, TESTID.selectionBar)
  for (const [name, id] of [
    ['mark read', TESTID.markRead], ['close selected', TESTID.closeSelected], ['delete selected', TESTID.deleteSelected],
  ] as const) found[`bulk: ${name}`] = await visible(page, id)

  // A closed row's menu offers reopen and delete — reached through the
  // Closed scope, the only place a closed row shows.
  await open(page, look, '/conversations')
  await page.getByTestId(testid.scope('Closed')).click()
  const row = page.getByTestId(testid.row(CLOSED.name))
  await expect(row).toBeVisible()
  await row.getByRole('button', { name: /actions for/i }).click()
  for (const item of ['Reopen', 'Delete']) {
    found[`closed row: ${item}`] = await page.getByRole('menuitem', { name: item, exact: true }).isVisible()
  }
  return found
}

test('every look exposes the same conversation actions', async ({ page }) => {
  test.setTimeout(120_000)
  await page.setViewportSize({ width: 1400, height: 900 })
  const per: Record<string, Record<string, boolean>> = {}
  for (const look of LOOKS) per[look] = await exposed(page, look)
  const actions = Object.keys(per[LOOKS[0]])
  // Each look exposes everything the contract names…
  const missing = LOOKS.flatMap((look) => actions.filter((a) => !per[look][a]).map((a) => `${look} omits "${a}"`))
  expect(missing).toEqual([])
})
