import { expect, test } from '@playwright/test'
import { loadFixtures } from './support/fixtures'
import { ManagerClient } from './support/manager'

// Table items #2 and #9 — the list's own tree controls, and what happens
// (and does NOT happen) when a new row arrives while the list is open.

const f = loadFixtures()
const SOURCE_TASKS = 'e2e-tasks'

test.beforeEach(async ({ page }) => {
  await page.goto('/conversations')
  await expect(page.getByTestId('inbox')).toBeVisible()
})

test('#2 Expand all / Collapse all actually expand and collapse a multi-member coordinator tree', async ({ page }) => {
  const [member1, member2] = f.conversations.coordOpenMembers
  const rootRow = page.getByTestId(`row-${f.conversations.coordOpenRoot}`)
  await expect(rootRow).toBeVisible({ timeout: 20_000 })
  const memberRow1 = page.getByTestId(`row-${member1}`)
  const memberRow2 = page.getByTestId(`row-${member2}`)

  // Starts expanded (ChatView's default `collapsedRoots` is empty).
  await expect(memberRow1).toBeVisible()
  await expect(memberRow2).toBeVisible()

  const collapseAll = page.getByRole('button', { name: 'Collapse all' })
  await expect(collapseAll).toBeVisible()
  await collapseAll.click()
  await expect(memberRow1).toBeHidden()
  await expect(memberRow2).toBeHidden()
  // The root itself stays — collapsing hides descendants, not the root.
  await expect(rootRow).toBeVisible()

  const expandAll = page.getByRole('button', { name: 'Expand all' })
  await expect(expandAll).toBeVisible()
  await expandAll.click()
  await expect(memberRow1).toBeVisible()
  await expect(memberRow2).toBeVisible()
  await page.screenshot({ path: 'e2e-live/proofs/02-expand-collapse-all.png', fullPage: true })
})

test('#9 a new row arriving shows no toast, only the row-level tint/tag', async ({ page }) => {
  // No toast-shaped element exists before the arrival either — the baseline
  // that makes "still none after" meaningful rather than assumed.
  await expect(page.locator('[class*="alert-group"]')).toHaveCount(0)

  const manager = new ManagerClient(f.managerUrl, f.adapterToken)
  const marker = `arrival-${Date.now()}`
  await manager.postTask(SOURCE_TASKS, marker, marker, `echo ${marker}`)

  // The live stream inserts the row with no page reload (apply.ts's
  // `writeConversation`, on the unfiltered first page) — this is the
  // SIGNAL that something arrived.
  const newRow = page.locator('li', { hasText: marker })
  await expect(newRow).toBeVisible({ timeout: 20_000 })
  await expect(newRow.getByText('new', { exact: true })).toBeVisible({ timeout: 5000 })
  await page.screenshot({ path: 'e2e-live/proofs/09-no-toast-just-tag.png', fullPage: true })

  // And still no toast — the row's own tint/tag is the WHOLE of the signal.
  await expect(page.locator('[class*="alert-group"]')).toHaveCount(0)
})
