import { expect, test } from '@playwright/test'
import { loadFixtures } from './support/fixtures'

// Table items #6, #14, #15 — a coordinator root's own transcript, the
// composer's REAL gate (a bound channel, never `escalatedAt`), the incident
// breadcrumb, and a member's own read-only view.

const f = loadFixtures()

test('#6 an OPEN coordinator root that never escalated still shows a live composer', async ({ page }) => {
  await page.goto(`/conversations/${f.conversations.coordOpenRoot}`)
  await expect(page.getByTestId('timeline')).toBeVisible({ timeout: 20_000 })
  await expect(page.getByLabel('message', { exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Send' })).toBeVisible()
  await page.screenshot({ path: 'e2e-live/proofs/06-open-coordinator-composer.png', fullPage: true })
})

test('#6 a CLOSED coordinator root shows no composer', async ({ page }) => {
  await page.goto(`/conversations/${f.conversations.coordClosedRoot}`)
  await expect(page.getByTestId('timeline')).toBeVisible({ timeout: 20_000 })
  await expect(page.getByLabel('message', { exact: true })).toHaveCount(0)
  await expect(page.getByText('This thread was archived')).toBeVisible()
})

test('#14/#26 the breadcrumb is absent on the root, present on a member naming the COORDINATOR (never an invented "Incident" noun)', async ({ page }) => {
  const [member1] = f.conversations.coordOpenMembers

  await page.goto(`/conversations/${f.conversations.coordOpenRoot}`)
  await expect(page.getByTestId('timeline')).toBeVisible({ timeout: 20_000 })
  await expect(page.getByText('Incident', { exact: true })).toHaveCount(0)

  await page.goto(`/conversations/${member1}`)
  await expect(page.getByText('A member holds no channel of its own')).toBeVisible({ timeout: 20_000 })
  // Item 26: there is no "incident" entity in this domain. The breadcrumb's
  // leading label names the COORDINATOR the chain belongs to instead — this
  // used to read "Incident" here, and that wording is now banned everywhere,
  // not merely renamed on the sidebar scope.
  await expect(page.getByText('Incident', { exact: true })).toHaveCount(0)
  await expect(page.getByText(f.coordinators.open, { exact: true }).first()).toBeVisible()
  // The crumb step for the root has no `causedBy` of its own, so it is
  // labelled by the ROOT's own title (its task text) — asserting THAT text,
  // positively, is what tells "names the parent" apart from "names nothing
  // in particular". It must never carry the member's OWN task text instead.
  const parentLink = page.getByRole('link', { name: new RegExp(`coordopen-${f.stamp}`) })
  await expect(parentLink).toBeVisible()
  await expect(parentLink).not.toContainText(`member-domain-${f.stamp}`)
  await page.screenshot({ path: 'e2e-live/proofs/14-member-breadcrumb.png', fullPage: true })
})

test('#15 a coordinator root renders the coordinator\'s own exchange plus a collapsed, expandable member invocation', async ({ page }) => {
  const { coordOpenRoot, coordOpenMembers } = f.conversations
  const [member1] = coordOpenMembers

  await page.goto(`/conversations/${coordOpenRoot}`)
  const timeline = page.getByTestId('timeline')
  await expect(timeline).toBeVisible({ timeout: 20_000 })

  // The coordinator's own task + reply are real content, not placeholders.
  await expect(timeline.getByText(`coordopen-${f.stamp}`, { exact: false }).first()).toBeVisible()

  // The member invocation is a <details> disclosure, collapsed by default.
  const disclosure = page.locator('details', { hasText: `as ${member1}` })
  await expect(disclosure).toBeVisible()
  await expect(disclosure).not.toHaveAttribute('open', '')
  await expect(disclosure.getByText('Task sent')).toHaveCount(0)

  await disclosure.locator('summary').click()
  await expect(disclosure).toHaveAttribute('open', '')
  await expect(disclosure.getByText('Task sent')).toBeVisible()
  await expect(disclosure.getByText('domain responded')).toBeVisible()
  // Real text, not a placeholder — the actual task sent AND the actual
  // result, each present once (the stub echoes the task back inside its
  // own "[stub] " result, so both lines carry the same marker).
  await expect(disclosure.getByText(`echo member-domain-${f.stamp}`, { exact: true })).toBeVisible()
  await expect(disclosure.getByText(`[stub] member-domain-${f.stamp}`, { exact: true })).toBeVisible()
  await expect(disclosure.getByText('No recorded input')).toHaveCount(0)
  await expect(disclosure.getByText('No result yet')).toHaveCount(0)
  await page.screenshot({ path: 'e2e-live/proofs/15-real-transcript-member-expanded.png', fullPage: true })
})

test('#15 a member conversation opened directly shows its own task+result as read-only, with no composer', async ({ page }) => {
  const [member1] = f.conversations.coordOpenMembers
  await page.goto(`/conversations/${member1}`)
  const timeline = page.getByTestId('timeline')
  await expect(timeline).toBeVisible({ timeout: 20_000 })
  await expect(timeline.getByText(`echo member-domain-${f.stamp}`, { exact: true })).toBeVisible()
  await expect(timeline.getByText(`[stub] member-domain-${f.stamp}`, { exact: true })).toBeVisible()
  await expect(page.getByText('A member holds no channel of its own')).toBeVisible()
  await expect(page.getByLabel('message', { exact: true })).toHaveCount(0)
})

// #15 (HIGHEST PRIORITY) — the regression this fixes: `handleConversation`
// used to call `mergeTranscript` only when a console thread already existed,
// so a root whose Coordinator never bound this console's channel rendered
// NOTHING but the member-invocation cards — the triggering message and the
// coordinator's own reasoning turn were simply absent, not merely out of
// order. `mergeTranscript` is now called unconditionally, deriving both from
// `summary.Runs` with no live buffer needed (`platform/console/convapi.go`,
// `rehydrate.go`). This test pins the ACTUAL CONTENT and its ACTUAL ORDER —
// never "an element exists" — against the real backend.
test('#15 a coordinator root renders its OWN exchange as ordinary message bubbles, interleaved by time with the member invocations', async ({ page }) => {
  const { coordOpenRoot, coordOpenMembers } = f.conversations
  const [member1, member2] = coordOpenMembers

  await page.goto(`/conversations/${coordOpenRoot}`)
  const timeline = page.getByTestId('timeline')
  await expect(timeline).toBeVisible({ timeout: 20_000 })

  // The coordinator's own reply is a genuine run result — wait for it to
  // actually land; the stub's own run may still be in flight the instant
  // this page opens; the member invocations were started directly over
  // `/coordinate/invoke` by global-setup and do not wait for it.
  await expect(timeline.getByText(`[stub] coordopen-${f.stamp}`, { exact: true })).toBeVisible({ timeout: 30_000 })

  const domainCard = page.locator('details', { hasText: `as ${member1}` })
  const supportCard = page.locator('details', { hasText: `as ${member2}` })
  await expect(domainCard).toBeVisible()
  await expect(supportCard).toBeVisible()

  // DOM order, read directly off the rendered tree — every item Timeline
  // interleaves by time is a DIRECT child of the `timeline` container.
  const children = await page.locator('[data-testid="timeline"] > *').evaluateAll((els) =>
    els.map((el) => ({ tag: el.tagName, text: el.textContent ?? '' })),
  )
  const triggerIdx = children.findIndex((c) => c.tag === 'ARTICLE' && c.text.includes(`echo coordopen-${f.stamp}`))
  const ownReplyIdx = children.findIndex((c) => c.tag === 'ARTICLE' && c.text.includes(`[stub] coordopen-${f.stamp}`))
  const domainIdx = children.findIndex((c) => c.text.includes(`as ${member1}`))
  const supportIdx = children.findIndex((c) => c.text.includes(`as ${member2}`))

  // All four are REAL, present content. Pre-fix, triggerIdx and ownReplyIdx
  // were both -1: the transcript held only the two invocation cards.
  const dump = () => JSON.stringify(children, null, 2)
  expect(triggerIdx, dump()).toBeGreaterThanOrEqual(0)
  expect(ownReplyIdx, dump()).toBeGreaterThanOrEqual(0)
  expect(domainIdx, dump()).toBeGreaterThanOrEqual(0)
  expect(supportIdx, dump()).toBeGreaterThanOrEqual(0)

  // The triggering message precedes the coordinator's OWN reply to it — a
  // run's `finishedAt` is necessarily later than the input it answered by
  // real wall-clock seconds (pod scheduling, not a tie), so this one is safe
  // to pin exactly.
  //
  // NOT pinned here: triggerIdx vs. domainIdx/supportIdx. Measured live: a
  // member's `created` is a Kubernetes `creationTimestamp`, truncated to
  // whole SECONDS, while the trigger's `receivedAt` carries sub-second
  // precision — global-setup invokes both members within the same
  // wall-clock second the root is created, so a truncated member timestamp
  // can parse as *earlier* than the trigger's own sub-second one purely from
  // rounding, not from any real ordering. Asserting that relative order
  // would pin a timestamp-granularity coincidence, not the contract.
  expect(triggerIdx, dump()).toBeLessThan(ownReplyIdx)

  // The coordinator's own reasoning turn is an ORDINARY message bubble (an
  // <article>, Timeline's own message element) — never folded into a
  // <details> disclosure and never rendered as a muted status/lifecycle line.
  expect(children[ownReplyIdx].tag, dump()).toBe('ARTICLE')

  // A lifecycle/run-event line, if the activity feed produced one for this
  // conversation, is visibly SUBORDINATE — a plain, smaller-scale line, never
  // an <article> standing in for real content — and it is never the only
  // thing shown: the four indices above already prove real content renders
  // alongside it.
  const lifecycleLine = page.locator('[data-testid="timeline"] > div', {
    hasText: /run dispatched|runtime starting|context restored|context checkpoint|run completed/,
  })
  if (await lifecycleLine.count() > 0) {
    const [lifecycleFontSize, bubbleFontSize] = await Promise.all([
      lifecycleLine.first().evaluate((el) => getComputedStyle(el).fontSize),
      timeline.locator('article').first().evaluate((el) => getComputedStyle(el).fontSize),
    ])
    // A cheap tripwire only, kept deliberately loose — the font sizes are in
    // different EM contexts, so this only guards against a lifecycle line
    // somehow rendering AT LEAST as large as an ordinary message bubble,
    // which would mean it had stopped being visually subordinate. The real
    // proof is the screenshot below, read by a human.
    expect(Number.parseFloat(lifecycleFontSize), `lifecycle line font-size ${lifecycleFontSize} vs bubble ${bubbleFontSize}`)
      .toBeLessThanOrEqual(Number.parseFloat(bubbleFontSize))
  }

  // The proof screenshot, at the point the strongest assertions above just
  // passed — scrolled to the TOP of the (internally-scrolling) timeline so
  // the earliest real content (the trigger bubble, the coordinator's own
  // reply) is actually in frame, rather than whatever the pinned-to-bottom
  // default happens to show.
  await timeline.evaluate((el) => {
    el.scrollTop = 0
  })
  await page.screenshot({ path: 'e2e-live/proofs/15-coordinator-transcript-real-content.png', fullPage: true })

  // Expand both invocations — the collapsed cards are real exchanges too,
  // not empty shells, each distinct from the other. (`15-real-transcript-
  // member-expanded.png`, above, already carries the expanded-card proof.)
  await domainCard.locator('summary').click()
  await expect(domainCard.getByText(`[stub] member-domain-${f.stamp}`, { exact: true })).toBeVisible()
  await supportCard.locator('summary').click()
  await expect(supportCard.getByText(`[stub] member-support-${f.stamp}`, { exact: true })).toBeVisible()
})
