import { expect, test } from '@playwright/test'
import { loadFixtures } from './support/fixtures'

// Item #19 — the join hint for an UNJOINED Coordinator root names the
// COORDINATOR itself (`convapi.go`'s `joinHint`, the `s.Coordinator != ""`
// branch), never the generic "the Pipeline that originated it" wording a
// Coordinator never had in the first place.

const f = loadFixtures()

test('#19 an unjoined coordinator root is told to bind the COORDINATOR, never "the Pipeline that originated it"', async ({ page }) => {
  await page.goto(`/conversations/${f.conversations.coordUnjoinedRoot}`)
  await expect(page.getByTestId('timeline')).toBeVisible({ timeout: 20_000 })

  await expect(page.getByText('This conversation has no console thread')).toBeVisible()
  // The REASON paragraph names the Coordinator's own channelRefs explicitly.
  await expect(page.getByText(/this console channel is not in the Coordinator's channelRefs/)).toBeVisible()
  // The FIX is the exact kubectl patch, naming the Coordinator by name —
  // PatternFly's `ClipboardCopy` renders it as a readonly `<input>`'s VALUE,
  // which never becomes a text NODE `getByText` would match, so this reads
  // the input's value directly instead.
  await expect(page.locator('input[readonly]')).toHaveValue(new RegExp(`Coordinator ${f.coordinators.unjoined}\\b`))
  // The retired wording, which only ever applied to an unattributable
  // PIPELINE conversation, must never leak onto a Coordinator root.
  await expect(page.getByText('the Pipeline that originated it')).toHaveCount(0)

  // No composer either — this root never joined, so there is nothing to type into.
  await expect(page.getByLabel('message', { exact: true })).toHaveCount(0)

  await page.screenshot({ path: 'e2e-live/proofs/19-coordinator-joinhint.png', fullPage: true })
})
