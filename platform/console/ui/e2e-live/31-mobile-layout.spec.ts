import { expect, test } from '@playwright/test'

// Item #31 QA — at a real phone width, the Inbox panel used to render as a
// fixed-width column beside the list regardless of how little room was
// left, overflowing the viewport and leaving the Inbox's own text bleeding
// behind the list. Separately, the app-level PatternFly nav sidebar forced
// itself back open on every resize, staying `pf-m-expanded` over the
// content and intercepting clicks meant for what was underneath. Both are
// measured live at 375px, the viewport this spec runs at.

test.use({ viewport: { width: 375, height: 812 } })

test('#31 the Inbox becomes a full-screen drawer, never a squeezed column, below the mobile breakpoint', async ({ page }) => {
  await page.goto('/conversations')
  // The app-level nav must start CLOSED here — `aria-hidden`, not merely
  // visually tucked away — or it sits on top of everything that follows.
  await expect(page.locator('.pf-v6-c-page__sidebar')).toHaveAttribute('aria-hidden', 'true')

  const filtersBtn = page.getByRole('button', { name: 'conversation filters' })
  await expect(filtersBtn).toBeVisible()
  // The Inbox panel itself is absent from the DOM inline — not merely
  // scrolled off — until the drawer is opened.
  await expect(page.getByTestId('inbox')).toHaveCount(0)

  const bodyScrollWidth = await page.evaluate(() => document.body.scrollWidth)
  expect(bodyScrollWidth, 'the page must not need horizontal scrolling at a phone width').toBeLessThanOrEqual(400)

  await filtersBtn.click()
  const inbox = page.getByTestId('inbox')
  await expect(inbox).toBeVisible()
  // The list is replaced, not merely covered: its own header is gone too.
  await expect(page.getByRole('button', { name: 'conversation filters' })).toHaveCount(0)

  await page.getByTestId('scope-Unread').click()
  // Picking a scope returns to the list, with the Inbox gone again.
  await expect(page.getByTestId('inbox')).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'conversation filters' })).toBeVisible()
  await page.screenshot({ path: 'e2e-live/proofs/31-mobile-inbox-drawer.png', fullPage: true })
})

test('#31 the app nav opens as a drawer on request, via the masthead toggle', async ({ page }) => {
  await page.goto('/conversations')
  await expect(page.getByTestId('inbox')).toHaveCount(0)
  await expect(page.getByRole('link', { name: /^Overview/ })).toHaveCount(0)

  await page.getByRole('button', { name: 'Conversations and other sections' }).click()
  await expect(page.getByRole('link', { name: /^Overview/ })).toBeVisible()
  await page.screenshot({ path: 'e2e-live/proofs/31-mobile-app-nav-drawer.png', fullPage: true })
})
