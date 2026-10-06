import { expect, test } from '@playwright/test'
import { loadFixtures } from './support/fixtures'
import { ManagerClient } from './support/manager'

// Item #28 QA — "Mine", "Unread", "Working" and "Errored" used to filter
// PER ROW with no tree awareness: a root this session started is "mine",
// but the MEMBER it invoked never is (a member is invoked, never
// originated) — so opening "Mine" on a root with an open member showed the
// root alone, with the member missing and nothing for the tree to attach
// to. The fix shows the WHOLE tree once anything in it qualifies, each row
// still rendering its own real status.
//
// This spec starts the root through the REAL "New conversation" UI flow
// (never `postChatCommand`, which carries no authenticated reader and so
// would never be stamped "mine" at all) and then invokes a real member on
// it directly, deterministically, rather than hoping a live agent decides
// to delegate.

const f = loadFixtures()

test('"Mine" shows a self-started root together with the member it invoked, even though the member itself is never "mine"', async ({ page }) => {
  await page.goto('/conversations')
  await expect(page.getByTestId('inbox')).toBeVisible()

  const marker = `e2e-ui-mine-tree-${f.stamp}-${Date.now()}`
  await page.getByTestId('new-conversation').click()
  await expect(page.getByTestId('destination-cards')).toBeVisible()
  await page.getByTestId(`destination-${f.coordinators.open}`).click()
  await page.getByLabel('task', { exact: true }).fill(`echo ${marker}`)
  const startBtn = page.getByTestId('start-conversation')
  await expect(startBtn).toBeEnabled()
  await startBtn.click()
  await expect(page.getByText(/is answering/)).toBeVisible({ timeout: 20_000 })

  await page.goto('/conversations')
  const rootRow = page.locator('li', { hasText: marker })
  await expect(rootRow).toBeVisible({ timeout: 20_000 })
  const rootTestId = await rootRow.getAttribute('data-testid')
  const rootName = rootTestId?.replace(/^row-/, '')
  if (!rootName) throw new Error(`could not read the new root's name from ${rootTestId}`)

  const manager = new ManagerClient(f.managerUrl, f.adapterToken)
  const memberName = await manager.invoke(f.coordinators.open, rootName, 'domain', `echo mine-tree-member-${f.stamp}`)

  await page.goto('/conversations')
  await page.getByTestId('scope-Mine').click()
  await expect(page.getByTestId(`row-${rootName}`)).toBeVisible({ timeout: 20_000 })
  await expect(page.getByTestId(`row-${memberName}`)).toBeVisible({ timeout: 20_000 })
  await page.screenshot({ path: 'e2e-live/proofs/28-mine-tree-inclusion.png', fullPage: true })
})
