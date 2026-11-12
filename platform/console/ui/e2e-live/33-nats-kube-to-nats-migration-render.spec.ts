import { expect, test } from '@playwright/test'
import { listConversations } from './support/kube'

// nats-state-store-default task 5.10 — the migration lane
// (platform/manager/test/system/state_nats_upgrade_test.go's
// TestNatsImportFromKubeOnUpgrade, task 5.5) runs INSIDE THE SAME full-tier
// system.yml job, on the SAME cluster, immediately before this suite
// (SYSTEM_REUSE=1) — and deliberately leaves its own fixture conversation
// behind rather than tidying it away, exactly so this spec can find it by
// its stable title prefix and render it. Seeded the same way every other
// spec in this suite is: through `kubectl`/the manager's own HTTP surface,
// never a second mechanism — here, by another system test having already
// done so on this exact cluster.
//
// SKIPPED when that lane did not run (the smoke tier, task 5.11's own
// "default install" pass, or a bare run against an install nothing
// migrated) — a missing fixture is a precondition this suite cannot
// arrange a second way, never a console defect.

function findMigratedConversationName(): string | undefined {
  return listConversations().find((c) => (c.spec.title ?? '').startsWith('e2e-nats-import-conv-'))?.metadata.name
}

test('the pre-upgrade conversation, its run and its unread state render after the kube-to-nats migration', async ({ page }) => {
  const name = findMigratedConversationName()
  test.skip(!name, 'no e2e-nats-import-conv- fixture on this cluster — the kube-to-nats migration lane (task 5.5) did not run before this suite')

  await page.goto('/conversations')
  await expect(page.getByTestId('inbox')).toBeVisible()
  const row = page.getByTestId(`row-${name}`)
  await expect(row).toBeVisible({ timeout: 20_000 })

  // Never read by anything before this spec (task 5.5's lane posts only
  // through /signal/inbound, never through the console) — the migrated
  // record's genuinely unread state must still render correctly.
  await expect(page.getByTestId(`unread-${name}`)).toBeVisible()

  await page.getByTestId(`open-${name}`).click()
  await expect(page.getByTestId('timeline')).toBeVisible({ timeout: 20_000 })
  await expect(page.getByText('e2e-nats-import-conv-', { exact: false }).first()).toBeVisible()

  await page.screenshot({ path: 'e2e-live/proofs/33-nats-kube-to-nats-migration-render.png', fullPage: true })
})
