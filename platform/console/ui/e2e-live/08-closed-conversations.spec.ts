import { expect, test } from './support/test'
import { TESTID, testid } from '../src/model/testids'
import { loadFixtures } from './support/fixtures'

// Table item #8 — Open excludes a REAL closed conversation always, with no
// "show closed" control anywhere to reveal it in place, and the dedicated
// Closed scope is the only place it shows.

const f = loadFixtures()

test('#8 Open always excludes a closed row, with no toggle to reveal it, and Closed always shows it', async ({ page }) => {
  const { closedTask } = f.conversations
  await page.goto('/conversations')
  await expect(page.getByTestId(TESTID.inbox)).toBeVisible()

  await expect(page.getByTestId(testid.row(closedTask))).toBeHidden()
  await expect(page.getByLabelText(/show closed/i)).toHaveCount(0)

  await page.getByTestId(testid.scope('Closed')).click()
  await expect(page.getByTestId(testid.row(closedTask))).toBeVisible({ timeout: 10_000 })
  await page.screenshot({ path: 'e2e-live/proofs/08-closed-scope.png', fullPage: true })

  await page.getByTestId(testid.scope('Open')).click()
  await expect(page.getByTestId(testid.row(closedTask))).toBeHidden()
})
