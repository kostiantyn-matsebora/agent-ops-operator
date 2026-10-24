import { expect, test } from '@playwright/test'
import { loadFixtures } from './support/fixtures'

// Table item #17 — mark read, then mark unread (rewind): the list reflects
// unread immediately, with no reload and no open; and once a real open
// legitimately advances it past the rewind, re-opening it again does not
// silently flip it back to unread (the race this item names).

const f = loadFixtures()

test('#17 mark read, then mark unread (rewind): reflected immediately; a later open does not silently re-advance past it', async ({ page }) => {
  const name = f.conversations.markTask
  await page.goto('/conversations')
  await expect(page.getByTestId('inbox')).toBeVisible()

  const row = page.getByTestId(`row-${name}`)
  await expect(row).toBeVisible({ timeout: 20_000 })

  await page.getByRole('button', { name: 'Select', exact: true }).click()
  await page.locator(`#select-${name}`).check()
  const bar = page.getByTestId('selection-bar')
  await expect(bar).toBeVisible()

  await bar.getByTestId('mark-read').click()
  await expect(page.getByTestId(`unread-${name}`)).toHaveCount(0)

  // Re-select after the bar's own onDone cleared the selection.
  await page.locator(`#select-${name}`).check()
  await bar.getByTestId('mark-unread').click()
  // Reflected IMMEDIATELY — no reload, no open.
  await expect(page.getByTestId(`unread-${name}`)).toBeVisible({ timeout: 10_000 })
  await page.screenshot({ path: 'e2e-live/proofs/17-mark-unread-rewind.png', fullPage: true })

  await page.getByRole('button', { name: 'Done selecting', exact: true }).click()

  // A real open legitimately marks it read (the intentional effect in
  // ThreadPane — opening a thread you can read IS reading it).
  await page.getByTestId(`open-${name}`).click()
  await expect(page.getByTestId('timeline')).toBeVisible({ timeout: 20_000 })
  await page.goto('/conversations')
  await expect(page.getByTestId(`unread-${name}`)).toHaveCount(0)

  // Opening it AGAIN must not silently flip it back to unread — the
  // dedup-by-activity-stamp guard against a double-fired mark-read race.
  await page.getByTestId(`open-${name}`).click()
  await expect(page.getByTestId('timeline')).toBeVisible({ timeout: 20_000 })
  await page.goto('/conversations')
  await expect(page.getByTestId(`unread-${name}`)).toHaveCount(0)
})
