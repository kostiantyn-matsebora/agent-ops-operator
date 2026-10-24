import { expect, test } from '@playwright/test'
import { loadFixtures } from './support/fixtures'

// Table items #1, #7, #13 — what a conversation ROW in the list actually
// shows, against the real backend's real rows (never a hand-written
// fixture): the pipeline/coordinator chip, the title-prefix-stripping rule
// (`ConversationRow.stripNamePrefix`), and the single "observed" marker.

const f = loadFixtures()

test.beforeEach(async ({ page }) => {
  await page.goto('/conversations')
  await expect(page.getByTestId('inbox')).toBeVisible()
})

test('#1 a coordinator root shows its coordinator name as a chip', async ({ page }) => {
  const row = page.getByTestId(`row-${f.conversations.coordOpenRoot}`)
  await expect(row).toBeVisible({ timeout: 20_000 })
  await expect(row.getByText(f.coordinators.open, { exact: true })).toBeVisible()
  await page.screenshot({ path: 'e2e-live/proofs/01-coordinator-chip.png', fullPage: true })
})

test('#1 a redundant "<pipeline>: " title prefix is stripped, with the pipeline shown only as the chip', async ({ page }) => {
  const { name, title, pipeline } = f.conversations.prefixTask
  const row = page.getByTestId(`row-${name}`)
  await expect(row).toBeVisible({ timeout: 20_000 })
  // The chip names the pipeline...
  await expect(row.getByText(pipeline!, { exact: true })).toBeVisible()
  // ...and the title text shown is the SUFFIX only — the full, un-stripped
  // title (chip + colon + rest, concatenated) must not appear anywhere in
  // the row, which is exactly what stripping failing would produce.
  const strippedSuffix = title.slice(`${pipeline}: `.length)
  await expect(row.getByText(strippedSuffix, { exact: true })).toBeVisible()
  const rowText = await row.innerText()
  expect(rowText).not.toContain(title)
})

test('#1 a title with an UNRELATED colon is left untouched', async ({ page }) => {
  const { name, title, pipeline } = f.conversations.colonTask
  const row = page.getByTestId(`row-${name}`)
  await expect(row).toBeVisible({ timeout: 20_000 })
  // The chip still names the pipeline...
  await expect(row.getByText(pipeline!, { exact: true })).toBeVisible()
  // ...but the title itself is shown WHOLE, colon and all — "alert" is not
  // this pipeline's name, so nothing may strip it.
  await expect(row.getByText(title, { exact: true })).toBeVisible()
})

test('#13 "observed" appears exactly once per row — the chip, never also the snippet', async ({ page }) => {
  const { name } = f.conversations.observedChat
  const row = page.getByTestId(`row-${name}`)
  await expect(row).toBeVisible({ timeout: 20_000 })
  await expect(row.getByText('observed', { exact: true })).toBeVisible()
  const rowText = await row.innerText()
  const occurrences = rowText.match(/observed/gi) ?? []
  expect(occurrences).toHaveLength(1)
  await page.screenshot({ path: 'e2e-live/proofs/13-observed-once.png', fullPage: true })
})

test('#7 the sidebar section holding Pipelines and Coordinators is not labelled as Pipelines alone', async ({ page }) => {
  await expect(page.getByText('PIPELINES & COORDINATORS')).toBeVisible()
  await expect(page.getByText('PIPELINES', { exact: true })).toHaveCount(0)
  await page.screenshot({ path: 'e2e-live/proofs/07-sidebar-label.png', fullPage: true })
})
