import { expect, test } from '@playwright/test'
import { ManagerClient } from './support/manager'
import { loadFixtures } from './support/fixtures'

// Item #30 QA — the "seen" row set behind the arrivals tag was shared across
// every scope. Narrowing the view (e.g. to "Unread") established a small
// seen set for that scope alone; widening it back out (e.g. to "All")
// compared the wide list against that small set and tagged every row the
// narrow scope never had as "new" — even though nothing had just arrived.
// Fixed by resetting the seen set whenever the active scope changes.

const f = loadFixtures()
const SOURCE_TASKS = 'e2e-tasks'

test('#30 switching from a narrow scope to a wider one does not tag every row as new', async ({ page }) => {
  // Establish a real row that exists BEFORE this test's own narrowing step,
  // so "All" always has at least one row the narrow scope will exclude.
  const manager = new ManagerClient(f.managerUrl, f.adapterToken)
  const marker = `e2e-ui-scope-switch-${f.stamp}-${Date.now()}`
  await manager.postTask(SOURCE_TASKS, marker, marker, `echo ${marker}`)

  await page.goto('/conversations')
  await expect(page.getByTestId('inbox')).toBeVisible()
  const newRow = page.locator('li', { hasText: marker })
  await expect(newRow).toBeVisible({ timeout: 20_000 })
  // Let its own "new" tag (a real arrival, correctly tagged) clear, so it
  // cannot be confused with the tag this test is checking does NOT appear.
  await expect(newRow.getByText('new', { exact: true })).toBeHidden({ timeout: 10_000 })

  // Narrow to a scope this plain task never qualifies for.
  await page.getByTestId('scope-Errored').click()
  await expect(page.locator('li', { hasText: marker })).toHaveCount(0)

  // Widen back out. The task existed the whole time — it did not just arrive.
  await page.getByTestId('scope-All').click()
  await expect(page.locator('li', { hasText: marker })).toBeVisible()
  await expect(page.locator('li', { hasText: marker }).getByText('new', { exact: true })).toHaveCount(0)
  await page.screenshot({ path: 'e2e-live/proofs/30-no-spurious-new-on-widen.png', fullPage: true })
})
