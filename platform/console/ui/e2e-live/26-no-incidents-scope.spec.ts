import { expect, test } from '@playwright/test'
import { loadFixtures } from './support/fixtures'

// Item #26 — there is no "Incidents" scope anywhere, ever: not the expanded
// sidebar, not the collapsed icon rail. `Inbox.tsx`'s `fixed` scope list
// (All/Unread/Working/Mine/Errored) is the single source both renders read
// from, so this is one regression with two render paths to check.
//
// A member's breadcrumb never saying "Incident" is covered in
// `06-coordinator-transcript.spec.ts` (item #14/#26), alongside the fixture
// that actually HAS a member to open.
//
// NOT COVERED HERE: "no Open incident row-menu item". `RowMenu.tsx` exists
// and is unit-tested in isolation, but it is not wired into any page in this
// tree yet (no page imports it) — there is no live "…" row action menu to
// open against the real UI, so this cannot be exercised end-to-end until it
// is actually mounted somewhere.

const f = loadFixtures()

test('#26 no "Incidents" scope in the expanded sidebar or the collapsed icon rail', async ({ page }) => {
  await page.goto('/conversations')
  const inbox = page.getByTestId('inbox')
  await expect(inbox).toBeVisible()
  await expect(page.getByText('Incidents', { exact: true })).toHaveCount(0)
  await expect(inbox.getByTestId('scope-Incidents')).toHaveCount(0)

  // The collapsed icon rail is a SEPARATE render branch (`Inbox.tsx`'s
  // `collapsed` early return) — the expanded check above proves nothing
  // about it.
  await page.getByRole('button', { name: 'collapse the inbox' }).click()
  const collapsedRail = page.getByTestId('inbox-collapsed')
  await expect(collapsedRail).toBeVisible()
  await expect(collapsedRail.getByText('Incidents', { exact: true })).toHaveCount(0)
  await expect(page.getByRole('button', { name: /incidents/i })).toHaveCount(0)
  await page.screenshot({ path: 'e2e-live/proofs/26-no-incidents-sidebar.png', fullPage: true })

  await page.getByRole('button', { name: 'expand the inbox' }).click()
  await expect(inbox).toBeVisible()
})

test('#26 the inbox lists only All / Unread / Working / Mine / Errored as fixed scopes', async ({ page }) => {
  await page.goto('/conversations')
  await expect(page.getByTestId('inbox')).toBeVisible()
  for (const label of ['All', 'Unread', 'Working', 'Mine', 'Errored']) {
    await expect(page.getByTestId(`scope-${label}`)).toBeVisible()
  }
  await expect(page.getByTestId('scope-Incidents')).toHaveCount(0)
  // Sanity: the fixture set really is present, so an empty sidebar could
  // never pass this test by accident.
  await expect(page.getByTestId(`row-${f.conversations.coordOpenRoot}`)).toBeVisible({ timeout: 20_000 })
})
