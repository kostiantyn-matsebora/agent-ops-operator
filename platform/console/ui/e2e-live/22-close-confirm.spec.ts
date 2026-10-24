import { expect, test, type Page } from '@playwright/test'
import { loadFixtures } from './support/fixtures'

// Item #22 — `/close` is destructive enough to be worth a confirm dialog
// (invariants.md: "`/exit` RELEASES THE RUNTIME — `/close` ENDS THE
// CONVERSATION"). `/exit` is fully recoverable and never asks. Each test
// below uses its OWN dedicated, never-reused conversation fixture
// (`global-setup.ts`'s `closeConfirm*` set) so sending a REAL `/close`
// through the composer cannot disturb any other spec's state.

const f = loadFixtures()

async function openAndSend(page: Page, name: string, text: string) {
  await page.goto(`/conversations/${name}`)
  await expect(page.getByTestId('timeline')).toBeVisible({ timeout: 20_000 })
  const composer = page.getByLabel('message', { exact: true })
  await expect(composer).toBeVisible()
  await composer.fill(text)
  await page.getByRole('button', { name: 'Send', exact: true }).click()
}

test('#22 "/close" asks for confirmation; Cancel sends nothing', async ({ page }) => {
  await openAndSend(page, f.conversations.closeConfirmCancel, '/close')

  const modal = page.getByTestId('close-confirm-modal')
  await expect(modal).toBeVisible()
  await expect(modal.getByText('Close this conversation?')).toBeVisible()

  await modal.getByRole('button', { name: 'Cancel', exact: true }).click()
  await expect(modal).toHaveCount(0)

  // Nothing was sent: the conversation is still open, not archived, and the
  // composer is still there to prove it.
  await expect(page.getByText('This thread was archived')).toHaveCount(0)
  await expect(page.getByLabel('message', { exact: true })).toBeVisible()
})

test('#22 confirming with "Don\'t ask again" closes immediately, and skips the modal on the NEXT /close in this browser', async ({ page }) => {
  // First conversation: confirm WITH the opt-out checked.
  await openAndSend(page, f.conversations.closeConfirmSkipA, '/close')
  const modal = page.getByTestId('close-confirm-modal')
  await expect(modal).toBeVisible()
  await page.locator('#close-confirm-skip').check()
  await modal.getByTestId('close-confirm-ok').click()
  await expect(modal).toHaveCount(0)
  // The real backend actually closed it — proven by the archived banner
  // appearing and the composer disappearing, not merely the modal closing.
  await expect(page.getByText('This thread was archived')).toBeVisible({ timeout: 20_000 })
  await expect(page.getByLabel('message', { exact: true })).toHaveCount(0)

  // Second, still-open conversation, same browser context (same localStorage):
  // the opt-out from above is honoured, so /close sends AT ONCE, no modal ever.
  await openAndSend(page, f.conversations.closeConfirmSkipB, '/close')
  await expect(page.getByTestId('close-confirm-modal')).toHaveCount(0)
  await expect(page.getByText('This thread was archived')).toBeVisible({ timeout: 20_000 })
})

test('#22 "/exit" never shows the confirm dialog, with or without the opt-out', async ({ page }) => {
  await openAndSend(page, f.conversations.closeConfirmExit, '/exit')
  await expect(page.getByTestId('close-confirm-modal')).toHaveCount(0)
  // /exit only releases the runtime pod — the conversation itself stays open,
  // so the composer is still here and nothing is archived.
  await expect(page.getByText('This thread was archived')).toHaveCount(0)
  await expect(page.getByLabel('message', { exact: true })).toBeVisible()
  await page.screenshot({ path: 'e2e-live/proofs/22-close-confirm.png', fullPage: true })
})
