import { expect, test } from '@playwright/test'

// Item #24 — the "All" scope's count badge explains the gap between its
// number (every conversation, server-side, before any filter) and the rows a
// reader actually sees below (closed ones hidden by default, roots that may
// be collapsed). `Inbox.tsx`'s own `ALL_COUNT_TOOLTIP` string, matched here
// verbatim so this test breaks the moment that wording drifts rather than
// silently accepting a paraphrase.

test('#24 hovering the "All" badge explains why it does not match the visible rows', async ({ page }) => {
  await page.goto('/conversations')
  const allRow = page.getByTestId('scope-All')
  await expect(allRow).toBeVisible()

  const badge = allRow.getByText(/^\d+$/)
  await expect(badge).toBeVisible()
  await badge.hover()

  const tooltip = page.getByRole('tooltip')
  await expect(tooltip).toBeVisible()
  await expect(tooltip).toHaveText(
    'Every conversation, including closed ones and invoked members — not all are visible in the list below by default.',
  )

  await page.screenshot({ path: 'e2e-live/proofs/24-all-badge-tooltip.png', fullPage: true })
})
