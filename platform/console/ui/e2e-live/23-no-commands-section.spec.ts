import { expect, test } from '@playwright/test'

// Item #23 — there is no "COMMANDS" section in the sidebar, and `/help` /
// `/pipelines` never render as sidebar ROWS. Both remain real slash commands
// elsewhere (the in-thread composer's typeahead) — this only pins that the
// sidebar's own listing (`Inbox.tsx`) never grew one back.

test('#23 the sidebar has no COMMANDS section, and no /help or /pipelines row', async ({ page }) => {
  await page.goto('/conversations')
  const inbox = page.getByTestId('inbox')
  await expect(inbox).toBeVisible()

  await expect(inbox.getByText('COMMANDS', { exact: true })).toHaveCount(0)
  await expect(inbox.getByText('Commands', { exact: true })).toHaveCount(0)
  await expect(inbox.getByText('/help', { exact: false })).toHaveCount(0)
  await expect(inbox.getByText('/pipelines', { exact: false })).toHaveCount(0)
  await expect(inbox.getByTestId('scope-help')).toHaveCount(0)
  await expect(inbox.getByTestId('scope-pipelines')).toHaveCount(0)

  await page.screenshot({ path: 'e2e-live/proofs/23-no-commands-section.png', fullPage: true })
})
