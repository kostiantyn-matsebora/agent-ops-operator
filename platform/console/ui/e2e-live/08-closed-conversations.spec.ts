import { expect, test } from '@playwright/test'
import { loadFixtures } from './support/fixtures'

// Table item #8 — "show closed" defaults off, hides a REAL closed
// conversation from the ordinary list, and the dedicated Closed/Archive
// scope always shows it regardless.

const f = loadFixtures()

test('#8 "show closed" is off by default, reveals closed rows when toggled, and the Closed scope always shows them', async ({ page }) => {
  const { closedTask } = f.conversations
  await page.goto('/conversations')
  await expect(page.getByTestId('inbox')).toBeVisible()

  const checkbox = page.locator('#show-closed')
  await expect(checkbox).not.toBeChecked()
  await expect(page.getByTestId(`row-${closedTask}`)).toBeHidden()

  await checkbox.check()
  await expect(page.getByTestId(`row-${closedTask}`)).toBeVisible({ timeout: 10_000 })
  await page.screenshot({ path: 'e2e-live/proofs/08-show-closed-toggle.png', fullPage: true })

  await checkbox.uncheck()
  await expect(page.getByTestId(`row-${closedTask}`)).toBeHidden()

  // The dedicated Closed/Archive scope shows it regardless of the toggle.
  await page.getByTestId('scope-Archive').click()
  await expect(page.getByTestId(`row-${closedTask}`)).toBeVisible({ timeout: 10_000 })
})
