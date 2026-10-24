import { expect, test } from '@playwright/test'
import { loadFixtures } from './support/fixtures'

// Table item #18 — `AttributeCoordinator` regression, through the real
// rendered UI, not its JSON. A conversation's `coordinatorRef` is
// provenance, snapshotted once at creation, and SHALL survive its
// Coordinator being edited or deleted (the same guarantee `escalate()` and
// the budget snapshot already carry). The bug: the console silently dropped
// a coordinator-rooted row back to looking like an ordinary pipeline
// conversation — wrong chip, wrong icon ring, wrong section in the sidebar
// — the moment its Coordinator CR was gone.
//
// Properly fixtured now: `global-setup.ts` opens this conversation by
// addressing a dedicated Coordinator, waits for it, and deletes the
// Coordinator CR — the exact regression state — BEFORE any spec runs, so
// this is never dependent on a hand-run reproduction or an env var naming
// one.

const f = loadFixtures()

test.beforeEach(async ({ page }) => {
  await page.goto('/conversations')
  await expect(page.getByTestId('inbox')).toBeVisible()
})

test('#18 the row names its coordinator, not a guessed pipeline, after the Coordinator CR is deleted', async ({ page }) => {
  const row = page.getByTestId(`row-${f.conversations.coordDeletedRoot}`)
  await expect(row).toBeVisible({ timeout: 20_000 })

  // Positive: the chip names the REAL coordinator this conversation
  // actually belongs to, read from `spec.coordinatorRef` alone.
  await expect(row.getByText(f.coordinators.deleted, { exact: true })).toBeVisible()
  // Regression's exact wrong output: falling back to an unrelated pipeline's
  // name once the Coordinator list no longer contains it.
  await expect(row.getByText('e2e-console', { exact: true })).toHaveCount(0)
  await page.screenshot({ path: 'e2e-live/proofs/18-coordinator-survives-deletion.png', fullPage: true })
})

test('#18 the thread pane attributes it the same way when opened directly', async ({ page }) => {
  await page.goto(`/conversations/${f.conversations.coordDeletedRoot}`)
  await expect(page.getByTestId('timeline')).toBeVisible({ timeout: 20_000 })
  // A root, even one whose Coordinator is gone, still shows no breadcrumb —
  // it was never caused by anything, which has nothing to do with whether
  // its own Coordinator still exists.
  await expect(page.getByText('Incident', { exact: true })).toHaveCount(0)
})

test('#18 the sidebar still groups it with Pipelines and Coordinators', async ({ page }) => {
  await expect(page.getByText('PIPELINES & COORDINATORS')).toBeVisible()
})
