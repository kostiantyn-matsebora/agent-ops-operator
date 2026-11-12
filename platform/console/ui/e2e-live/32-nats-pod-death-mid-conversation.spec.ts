import { expect, test } from '@playwright/test'
import { loadFixtures } from './support/fixtures'
import { kubectl, listConversations, waitFor } from './support/kube'

// nats-state-store-default task 5.9 — a state server pod dies while a
// conversation this spec itself opened through the console is live: the
// page must keep rendering, the thread and its unread state must survive,
// and a new message must still arrive once the StatefulSet replaces the
// pod. One of three down (design D-8) is comfortably inside quorum — this
// never scales anything, only deletes.

const f = loadFixtures()

function findRowNameByTitleSubstring(substring: string): string | undefined {
  return listConversations().find((c) => (c.spec.title ?? '').includes(substring))?.metadata.name
}

test('a state server pod dying mid-conversation never breaks the console view, and delivery resumes once it returns', async ({ page }) => {
  test.setTimeout(150_000)

  await page.goto('/conversations')
  await expect(page.getByTestId('new-conversation')).toBeVisible()
  await page.getByTestId('new-conversation').click()
  await expect(page.getByTestId('destination-cards')).toBeVisible()
  await page.getByTestId('destination-e2e-console').click()

  const marker = `e2e-nats-pod-${f.stamp}-${Date.now()}`
  await page.getByLabel('task', { exact: true }).fill(`echo ${marker}`)
  await page.getByTestId('start-conversation').click()
  await expect(page.getByText(/is answering/)).toBeVisible({ timeout: 20_000 })

  const name = await waitFor('the started conversation to exist, by its marker', () => findRowNameByTitleSubstring(marker), 30_000)

  await page.goto('/conversations')
  await expect(page.getByTestId(`row-${name}`)).toBeVisible({ timeout: 20_000 })
  await page.getByTestId(`open-${name}`).click()
  await expect(page.getByTestId('timeline')).toBeVisible({ timeout: 20_000 })
  await expect(page.getByText(marker, { exact: false }).first()).toBeVisible()

  const victim = kubectl(['get', 'pods', '-l', 'app.kubernetes.io/name=agentops-nats',
    '-o', 'jsonpath={.items[0].metadata.name}']).trim()
  if (!victim) throw new Error('no nats pod found to delete')
  kubectl(['delete', 'pod', victim, '--grace-period=0', '--force', '--wait=false'])

  // The page keeps rendering through the pod's death — reloaded, not read
  // from a cached view, so this actually exercises a fresh read.
  await page.reload()
  await expect(page.getByTestId('timeline')).toBeVisible({ timeout: 30_000 })
  await expect(page.getByText(marker, { exact: false }).first()).toBeVisible()

  // Thread + unread state survive: back on the list, the row this spec just
  // opened (and therefore already read) is still there, rendering cleanly.
  await page.goto('/conversations')
  await expect(page.getByTestId(`row-${name}`)).toBeVisible({ timeout: 30_000 })
  await expect(page.getByTestId(`unread-${name}`)).toHaveCount(0)

  // A new message still arrives once the StatefulSet replaces the pod and
  // quorum holds again — no scaling here, only the controller's own
  // replacement, which is the point.
  await page.getByTestId(`open-${name}`).click()
  const composer = page.getByLabel('message', { exact: true })
  await expect(composer).toBeVisible({ timeout: 30_000 })
  const reply = `echo nats-pod-reply-${f.stamp}-${Date.now()}`
  await composer.fill(reply)
  await page.getByRole('button', { name: 'Send', exact: true }).click()
  await expect(page.getByText(reply, { exact: false })).toBeVisible({ timeout: 90_000 })

  await page.screenshot({ path: 'e2e-live/proofs/32-nats-pod-death-mid-conversation.png', fullPage: true })
})
