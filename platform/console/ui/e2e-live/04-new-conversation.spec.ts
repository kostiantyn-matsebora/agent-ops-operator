import { expect, test } from '@playwright/test'
import { loadFixtures } from './support/fixtures'

// Table items #4 and #5 — a Coordinator is an addressable destination
// alongside Pipelines wherever a conversation is STARTED.
//
// The "New conversation" modal used to require typing "/" before it showed
// anything (a slash-typeahead menu, `matchEntries`). It now lists every
// Ready pipeline and coordinator as its own clickable CARD the instant the
// modal opens (`NewConversation.tsx`'s `destination-cards`) — discovering
// what can be addressed costs no typing at all. This file pins the NEW
// behaviour; the old typeahead assertions it replaced no longer apply
// (`matchEntries` is no longer called for this modal's 'general' position).

const f = loadFixtures()

test.beforeEach(async ({ page }) => {
  await page.goto('/conversations')
  await expect(page.getByTestId('inbox')).toBeVisible()
})

test('#4 every Ready pipeline/coordinator renders as its own card the instant the modal opens, with ZERO text typed', async ({ page }) => {
  await page.getByTestId('new-conversation').click()
  const cards = page.getByTestId('destination-cards')
  await expect(cards).toBeVisible()

  // The negative is the whole point of the fix: nothing was typed into
  // EITHER box, and the cards are already there.
  await expect(page.getByTestId('destination-filter')).toHaveValue('')
  await expect(page.getByLabel('task', { exact: true })).toHaveValue('')

  const coordinatorCard = page.getByTestId(`destination-${f.coordinators.open}`)
  const pipelineCard = page.getByTestId('destination-e2e-console')
  await expect(coordinatorCard).toBeVisible()
  await expect(pipelineCard).toBeVisible()
  // A real Pipeline is offered on exactly the same footing as a Coordinator
  // — ADDED to the set, never a replacement for it — and each card states
  // which kind it is.
  await expect(coordinatorCard.getByText('coordinator', { exact: true })).toBeVisible()
  await expect(pipelineCard.getByText('pipeline', { exact: true })).toBeVisible()

  await page.screenshot({ path: 'e2e-live/proofs/04-coordinator-typeahead.png', fullPage: true })
})

test('#4 the filter box narrows the visible cards without ever blanking the list', async ({ page }) => {
  await page.getByTestId('new-conversation').click()
  const cards = page.getByTestId('destination-cards')
  await expect(cards).toBeVisible()
  const before = await cards.getByRole('radio').count()
  expect(before).toBeGreaterThan(1)

  await page.getByTestId('destination-filter').fill('e2e-console')
  await expect(page.getByTestId('destination-e2e-console')).toBeVisible()
  await expect(page.getByTestId(`destination-${f.coordinators.open}`)).toHaveCount(0)
  const after = await cards.getByRole('radio').count()
  expect(after).toBeGreaterThan(0)
  expect(after).toBeLessThan(before)
})

test('#5 the empty "Select a conversation" state offers one-click start chips for a Pipeline AND a Coordinator', async ({ page }) => {
  // No conversation is open — the thread pane shows the empty state beside the list.
  await expect(page.getByText('Select a conversation')).toBeVisible()
  const chips = page.getByTestId('quick-chips')
  await expect(chips).toBeVisible()
  await expect(chips.getByRole('button', { name: f.coordinators.open, exact: true })).toBeVisible()
  await expect(chips.getByRole('button', { name: 'e2e-console', exact: true })).toBeVisible()
  await page.screenshot({ path: 'e2e-live/proofs/05-empty-state-chips.png', fullPage: true })
})

test('#4/#5 picking a card, typing a task, and Start really creates a conversation against the real backend', async ({ page }) => {
  await page.getByTestId('new-conversation').click()
  await expect(page.getByTestId('destination-cards')).toBeVisible()

  const startBtn = page.getByTestId('start-conversation')
  await expect(startBtn).toHaveText('Start')
  await expect(startBtn).toBeDisabled()

  await page.getByTestId('destination-e2e-console').click()
  await expect(startBtn).toHaveText('Start with e2e-console')
  // Selecting a destination alone never enables Start — a task is still required.
  await expect(startBtn).toBeDisabled()

  const marker = `e2e-ui-newconv-${f.stamp}-${Date.now()}`
  await page.getByLabel('task', { exact: true }).fill(`echo ${marker}`)
  await expect(startBtn).toBeEnabled()
  await startBtn.click()

  // A real `api.start` call, no mocked intercept: the success banner only
  // renders once that request actually resolved.
  await expect(page.getByText(/is answering/)).toBeVisible({ timeout: 20_000 })

  // The new conversation really exists server-side — found by reloading the
  // list and locating a row carrying this run's unique marker text.
  await page.goto('/conversations')
  await expect(page.getByText(marker, { exact: false })).toBeVisible({ timeout: 20_000 })
})
