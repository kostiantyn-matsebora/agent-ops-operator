import { expect, test } from '@playwright/test'
import { loadFixtures } from './support/fixtures'
import { kubectl, listConversations, portForward, waitFor } from './support/kube'

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
  // This spec rolls the manager, which kills any port-forward bound to a
  // manager pod, including the one global-setup opened. Use its own, so the
  // spec neither depends on a forward that an earlier run of it already broke
  // nor leaves a dead one for the next.
  const forward = await portForward('agentops-manager', 8080)
  process.env.E2E_LIVE_MANAGER_URL = `http://127.0.0.1:${forward.localPort}`
  try {
    await run(browser)
  } finally {
    forward.stop()
  }
})

async function run(browser: import('@playwright/test').Browser) {
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

  // The conversation this spec closes later must be a settled one. A thread
  // still being created when the manager rolls cannot be written to, and what
  // this spec proves is the stream, not topic creation across a rollout.
  await waitFor('the conversation to have an established console thread', () => {
    const c = listConversations().find((x) => x.metadata.name === name)
    const threads = (c?.status as { threads?: { channel?: string; threadId?: string }[] } | undefined)?.threads ?? []
    return threads.some((t) => t.channel === 'console' && t.threadId) ? true : undefined
  }, 60_000)

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
  try {
    await expect(actor.getByText('This thread was archived')).toBeVisible({ timeout: 30_000 })
  } catch (e) {
    // The close was accepted and did nothing. Say what each manager saw, while
    // the pods that served it are still the live ones.
    const view = await actor.evaluate(async (n) => {
      const r = await fetch(`/api/conversations/${n}`)
      const j = await r.json()
      return JSON.stringify({ status: r.status, thread: j.conversation?.consoleThread, joined: j.conversation?.joined, phase: j.conversation?.phase })
    }, name)
    // The forward opened at the top died with the manager it was bound to.
    let storedThreads = 'unreadable'
    try {
      const fresh = await portForward('agentops-manager', 8080)
      const before = process.env.E2E_LIVE_MANAGER_URL
      process.env.E2E_LIVE_MANAGER_URL = `http://127.0.0.1:${fresh.localPort}`
      try {
        const stored = listConversations().find((c) => c.metadata.name === name)
        const st = stored?.status as { threads?: unknown; phase?: string; closedAt?: string } | undefined
        storedThreads = `${JSON.stringify(st?.threads ?? null)} and phase ${st?.phase} closedAt ${st?.closedAt}`
      } finally {
        process.env.E2E_LIVE_MANAGER_URL = before
        fresh.stop()
      }
    } catch (probe) {
      storedThreads = `unreadable (${(probe as Error).message.slice(0, 80)})`
    }
    throw new Error(
      `/close of ${name} was not applied. The console sees ${view}. The store holds threads ${storedThreads}.\nManager log lines:\n${managerLogLines(name)}\n${(e as Error).message}`,
    )
  }

  // The never-reloaded page sees it.
  try {
    await expect(watcher.getByTestId(`row-${name}`)).toBeVisible({ timeout: 60_000 })
  } catch (e) {
    const ids = await watcher.locator('[data-testid^="row-"]').evaluateAll((els) => els.map((el) => el.getAttribute('data-testid')))
    // Tell a browser that missed the update from a console that never had it:
    // the console's own listing, and what a reload of this page shows.
    const consolePhase = await watcher.evaluate(async (n) => {
      const r = await fetch('/api/conversations')
      const j = await r.json()
      const items = Array.isArray(j) ? j : (j.items ?? [])
      const hit = items.find((i: { name?: string; metadata?: { name?: string } }) => (i.name ?? i.metadata?.name) === n)
      return hit ? JSON.stringify(hit).slice(0, 200) : 'absent'
    }, name)
    await watcher.reload()
    await watcher.getByTestId('scope-Archive').click()
    const afterReload = await watcher.getByTestId(`row-${name}`).isVisible({ timeout: 10_000 }).catch(() => false)
    throw new Error(`row-${name} never appeared in the open Archive view. Rows present: ${JSON.stringify(ids)}. Console says: ${consolePhase}. Visible after a reload: ${afterReload}\n${(e as Error).message}`)
  }
  await watcher.screenshot({ path: 'e2e-live/proofs/34-stream-survives-manager-restart.png', fullPage: true })
  await ctx.close()
}

function managerLogLines(name: string): string {
  const pods = kubectl(['get', 'pods', '-l', 'app.kubernetes.io/name=agentops-manager', '-o', 'name']).split('\n').filter(Boolean)
  return pods
    .map((p) => {
      const log = kubectl(['logs', p, '--tail=400'])
      const hit = log
        .split('\n')
        .filter((l) => /dropped|conversation closed|leader proxy|inbound|leaderelection|signal stored/.test(l) || l.includes(name))
        .slice(-12)
        .map((l) => l.slice(0, 260))
      return `-- ${p}\n${hit.join('\n')}`
    })
    .join('\n')
}
