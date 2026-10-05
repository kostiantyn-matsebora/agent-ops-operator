import { expect, test } from '@playwright/test'
import { loadFixtures } from './support/fixtures'

// Item #20 — a STARTER chip (one that begins a brand-new conversation)
// carries a leading "+", distinct from a thread command chip (the raw
// "/name", monospace) and an offered choice (no marker at all) —
// `QuickChips.tsx`'s dedicated `aria-hidden` span.
//
// The accessible NAME of a starter button excludes that `aria-hidden` span
// (ARIA's own text-alternative computation), so `getByRole` still finds it
// by its plain name — the "+" shows up only in the rendered TEXT CONTENT,
// which is what this test actually asserts on.

const f = loadFixtures()

test('#20 a starter chip reads "+<name>"; a thread command chip never does', async ({ page }) => {
  await page.goto(`/conversations/${f.conversations.markTask}`)
  await expect(page.getByTestId('timeline')).toBeVisible({ timeout: 20_000 })
  const chips = page.getByTestId('quick-chips')
  await expect(chips).toBeVisible()

  // A starter: begins a brand-new conversation with this Coordinator.
  const starter = chips.getByRole('button', { name: f.coordinators.open, exact: true })
  await expect(starter).toBeVisible()
  await expect(starter).toHaveText(`+${f.coordinators.open}`)

  // A thread command: acts on the conversation already open here. No "+" —
  // it addresses nothing new.
  const closeCmd = chips.getByRole('button', { name: '/close', exact: true })
  await expect(closeCmd).toBeVisible()
  await expect(closeCmd).toHaveText('/close')

  const exitCmd = chips.getByRole('button', { name: '/exit', exact: true })
  await expect(exitCmd).toBeVisible()
  await expect(exitCmd).toHaveText('/exit')

  await page.screenshot({ path: 'e2e-live/proofs/20-starter-chip-plus-prefix.png', fullPage: true })
})
