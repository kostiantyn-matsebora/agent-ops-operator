import { expect, test } from '@playwright/test'
import { loadFixtures } from './support/fixtures'

// Item #29 QA — a member conversation carries the SAME `turns[]`/`toolCalls[]`
// the backend records for any other run, but no "Runs" tab ever offered
// them: `ThreadPane.tsx` hid it unconditionally for `isMember`. Fixed by
// showing Runs for a member too (Graph and Sequence stay root-only — a
// member is one node its parent's own Graph/Sequence already shows).
//
// Reuses `coordOpenMembers[0]`, the SAME member `16-runs-view.spec.ts`
// exercises for a root, so this is a true parallel proof: the identical
// kind of data, read from the identical kind of view, on the OTHER side of
// the root/member line.

const f = loadFixtures()

test('#29 a member has its own Runs tab, and Graph/Sequence stay root-only', async ({ page }) => {
  const [member1] = f.conversations.coordOpenMembers
  await page.goto(`/conversations/${member1}`)
  await expect(page.getByTestId('timeline')).toBeVisible({ timeout: 20_000 })

  await expect(page.getByRole('button', { name: 'Graph', exact: true })).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Sequence', exact: true })).toHaveCount(0)

  const runsBtn = page.getByRole('button', { name: 'Runs', exact: true })
  await expect(runsBtn).toBeVisible()
  await runsBtn.click()

  const runsView = page.getByTestId('runs-view')
  await expect(runsView).toBeVisible()
  // The member's own task and the agent's own answer, not an empty shell.
  await expect(runsView.getByText('Finished')).toBeVisible()
  await expect(runsView.getByText('member-domain', { exact: false })).toBeVisible()
  await page.screenshot({ path: 'e2e-live/proofs/29-member-runs-tab.png', fullPage: true })
})
