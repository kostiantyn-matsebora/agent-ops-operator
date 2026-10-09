import { expect, test } from '@playwright/test'
import { loadFixtures } from './support/fixtures'

// Table item #16 — the Runs view keeps a run's raw result text UNCHANGED,
// shows turns/tool-calls tables for a run that reported them (the stub
// runtime's `calls` directive, test/stubruntime/main.go — real runtime
// contract, not a hand-crafted /work/done payload), and shows NEITHER table
// for a run that reported neither.

const f = loadFixtures()

test('#16 Runs view: raw text unchanged, turns/tool-calls tables only where the run actually reported them', async ({ page }) => {
  await page.goto(`/conversations/${f.conversations.runsTask}`)
  await expect(page.getByTestId('timeline')).toBeVisible({ timeout: 20_000 })
  await page.getByRole('button', { name: 'Runs', exact: true }).click()

  // Scoped to the Runs panel, never the bare page: the sidebar's own row for
  // this same conversation shows the identical result text as its snippet,
  // so an unscoped getByText matches both and fails strict mode.
  const runsView = page.getByTestId('runs-view')
  await expect(runsView).toBeVisible()

  // Run 1 (`calls`): raw result text verbatim, plus both tables with real values.
  await expect(runsView.getByText('[stub] made two model calls and one tool call', { exact: true })).toBeVisible()
  // PatternFly's <Table> renders role="grid", not the implicit HTML "table"
  // role — matched on the `aria-label` attribute directly instead.
  const turnsTable = runsView.locator('table[aria-label="turns"]')
  await expect(turnsTable).toBeVisible()
  await expect(turnsTable.getByText('stub-model').first()).toBeVisible()
  await expect(turnsTable.getByText('tool_use')).toBeVisible()
  await expect(turnsTable.getByText('end_turn')).toBeVisible()
  const toolCallsTable = runsView.locator('table[aria-label="tool calls"]')
  await expect(toolCallsTable).toBeVisible()
  await expect(toolCallsTable.getByText('mcp__stub__lookup')).toBeVisible()
  await expect(toolCallsTable.getByText('stub', { exact: true })).toBeVisible()

  // Run 2 (plain echo): raw result text verbatim, and NEITHER table renders
  // for it — `turns — N model call(s)` / `tool calls — N` only ever appear
  // for run 1's own section above, never duplicated for run 2.
  await expect(runsView.getByText(`[stub] plainrun-${f.stamp}`, { exact: true })).toBeVisible()
  await expect(runsView.getByText(/^turns —/)).toHaveCount(1)
  await expect(runsView.getByText(/^tool calls —/)).toHaveCount(1)
  await page.screenshot({ path: 'e2e-live/proofs/16-runs-view-turns-tool-calls.png', fullPage: true })
})
