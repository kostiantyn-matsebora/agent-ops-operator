import { expect, test } from './support/test'
import { TESTID, testid } from '../src/model/testids'
import { loadFixtures } from './support/fixtures'

// Table item #17 — mark read, then mark unread (rewind): the list reflects
// unread immediately, with no reload and no open; and once a real open
// legitimately advances it past the rewind, re-opening it again does not
// silently flip it back to unread (the race this item names).

const f = loadFixtures()

test('#17 mark read, then mark unread (rewind): reflected immediately; a later open does not silently re-advance past it', async ({ page }) => {
  const name = f.conversations.markTask
  await page.goto('/conversations')
  await expect(page.getByTestId(TESTID.inbox)).toBeVisible()

  const row = page.getByTestId(testid.row(name))
  await expect(row).toBeVisible({ timeout: 20_000 })

  await page.getByRole('button', { name: 'Select', exact: true }).click()
  await page.getByTestId(testid.select(name)).check()
  const bar = page.getByTestId(TESTID.selectionBar)
  await expect(bar).toBeVisible()

  await bar.getByTestId(TESTID.markRead).click()
  await expect(page.getByTestId(testid.unread(name))).toHaveCount(0)

  // Re-select after the bar's own onDone cleared the selection.
  await page.getByTestId(testid.select(name)).check()
  await bar.getByTestId(TESTID.markUnread).click()
  // Reflected IMMEDIATELY — no reload, no open.
  await expect(page.getByTestId(testid.unread(name))).toBeVisible({ timeout: 10_000 })
  await page.screenshot({ path: 'e2e-live/proofs/17-mark-unread-rewind.png', fullPage: true })

  await page.getByRole('button', { name: 'Done selecting', exact: true }).click()

  // A real open legitimately marks it read (the intentional effect in
  // ThreadPane — opening a thread you can read IS reading it).
  await page.getByTestId(testid.open(name)).click()
  await expect(page.getByTestId(TESTID.timeline)).toBeVisible({ timeout: 20_000 })
  await page.goto('/conversations')
  await expect(page.getByTestId(testid.unread(name))).toHaveCount(0)

  // Opening it AGAIN must not silently flip it back to unread — the
  // dedup-by-activity-stamp guard against a double-fired mark-read race.
  await page.getByTestId(testid.open(name)).click()
  await expect(page.getByTestId(TESTID.timeline)).toBeVisible({ timeout: 20_000 })
  await page.goto('/conversations')
  await expect(page.getByTestId(testid.unread(name))).toHaveCount(0)
})
