import { expect, test } from '@playwright/test'
import { loadFixtures } from './support/fixtures'
import { kubectl, listConversations, waitFor } from './support/kube'

// nats-state-store-default — the console's streams must survive the manager
// restarting under them. A page sits on the Archive scope and is NEVER
// reloaded. The manager is rolled. A conversation is then closed from a second
// page, and the first page must list it without a reload: if its stream had
// dropped and not recovered, it would show a frozen view.

const f = loadFixtures()

function rowNameByTitle(substring: string): string | undefined {
  return listConversations().find((c) => (c.spec.title ?? '').includes(substring))?.metadata.name
}

test('a conversation closed after a manager restart appears in an open Archive view without a reload', async ({ browser }) => {
  test.setTimeout(240_000)
  const ctx = await browser.newContext()
  const watcher = await ctx.newPage()
  const actor = await ctx.newPage()

  // A conversation of this spec's own, to close later.
  await actor.goto('/conversations')
  await actor.getByTestId('new-conversation').click()
  await actor.getByTestId('destination-e2e-console').click()
  const marker = `e2e-stream-${f.stamp}-${Date.now()}`
  await actor.getByLabel('task', { exact: true }).fill(`echo ${marker}`)
  await actor.getByTestId('start-conversation').click()
  await expect(actor.getByText(/is answering/)).toBeVisible({ timeout: 20_000 })
  const name = await waitFor('the started conversation to exist', () => rowNameByTitle(marker), 30_000)

  // The watcher goes to the Archive scope and stays there.
  await watcher.goto('/conversations')
  await expect(watcher.getByTestId('inbox')).toBeVisible()
  await watcher.getByTestId('scope-Archive').click()
  await expect(watcher.getByTestId(`row-${name}`)).toBeHidden()

  // Roll the manager under both pages.
  kubectl(['rollout', 'restart', 'deployment/agentops-manager'])
  kubectl(['rollout', 'status', 'deployment/agentops-manager', '--timeout=300s'])

  // Close it from the other page.
  await actor.goto(`/conversations/${name}`)
  await expect(actor.getByTestId('timeline')).toBeVisible({ timeout: 30_000 })
  const composer = actor.getByLabel('message', { exact: true })
  await composer.fill('/close')
  await actor.getByRole('button', { name: 'Send', exact: true }).click()
  const modal = actor.getByTestId('close-confirm-modal')
  if (await modal.isVisible({ timeout: 3_000 }).catch(() => false)) {
    await modal.getByTestId('close-confirm-ok').click()
  }
  await expect(actor.getByText('This thread was archived')).toBeVisible({ timeout: 30_000 })

  // The never-reloaded page sees it.
  await expect(watcher.getByTestId(`row-${name}`)).toBeVisible({ timeout: 60_000 })
  await watcher.screenshot({ path: 'e2e-live/proofs/34-stream-survives-manager-restart.png', fullPage: true })
  await ctx.close()
})
