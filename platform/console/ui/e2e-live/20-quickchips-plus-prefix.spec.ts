import { expect, test } from '@playwright/test'
import { loadFixtures } from './support/fixtures'

// Item #20 — RESHAPED by direct, repeated instruction after this spec was
// first written: a thread command (`/exit`, `/close`) is an ICON the reader
// RUNS directly, never raw mono "/name" text to insert and send yourself —
// and the starter chip ("+<name>", beginning a brand-new conversation from
// beside an already-open one) is GONE entirely, not merely relabelled. Both
// changes are pinned here together since they are the same component
// (`QuickChips.tsx`) and the same user-facing area. Read-only, like every
// other spec touching this SHARED fixture conversation: it asserts what
// renders, never clicks a command that would alter `markTask`'s state for
// the other specs reading it.

const f = loadFixtures()

test('#20 a thread command renders as an ICON with no visible "/name" text, and no starter chip exists any more', async ({ page }) => {
  await page.goto(`/conversations/${f.conversations.markTask}`)
  await expect(page.getByTestId('timeline')).toBeVisible({ timeout: 20_000 })
  const chips = page.getByTestId('quick-chips')
  await expect(chips).toBeVisible()

  // No starter chip of any shape exists any more — nothing here begins a
  // brand-new conversation. The "New conversation" modal is the only way.
  await expect(chips.getByRole('button', { name: f.coordinators.open, exact: true })).toHaveCount(0)
  await expect(chips.getByText('+', { exact: false })).toHaveCount(0)

  const exitCmd = chips.getByRole('button', { name: '/exit', exact: true })
  await expect(exitCmd).toBeVisible()
  // The accessible NAME is still "/exit" (the aria-label), but the rendered
  // TEXT CONTENT carries no "/exit" string — an icon stands for it instead.
  await expect(exitCmd).not.toHaveText('/exit')
  await expect(exitCmd.locator('svg')).toBeVisible()

  const closeCmd = chips.getByRole('button', { name: '/close', exact: true })
  await expect(closeCmd).toBeVisible()
  await expect(closeCmd).not.toHaveText('/close')
  await expect(closeCmd.locator('svg')).toBeVisible()

  await page.screenshot({ path: 'e2e-live/proofs/20-icon-chips-no-starter.png', fullPage: true })
})
