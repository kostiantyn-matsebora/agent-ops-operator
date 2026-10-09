import { afterEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import {
  ConversationRow, phaseDot, rowSnippet, rowTag, stripNamePrefix,
} from './ConversationRow'
import type { ConversationSummary } from '../../api/types'

function conv(name: string, over: Partial<ConversationSummary> = {}): ConversationSummary {
  return {
    name, runCount: 0, queued: 0, joined: true, errored: false, unread: false,
    ageSeconds: 4, deleting: false, phase: 'Idle', ...over,
  }
}

function renderRow(row: ConversationSummary, over: Partial<React.ComponentProps<typeof ConversationRow>> = {}) {
  return render(
    <ul>
      <ConversationRow
        row={row}
        depth={0}
        memberCount={0}
        parentMissing={false}
        isNew={false}
        selectionMode={false}
        selected={false}
        highlighted={false}
        onSelect={vi.fn()}
        onOpen={vi.fn()}
        canWrite
        hasReader
        onMarkRead={vi.fn()}
        onMarkUnread={vi.fn()}
        onReopen={vi.fn()}
        onExitRuntime={vi.fn()}
        onClose={vi.fn()}
        onDelete={vi.fn()}
        {...over}
      />
    </ul>,
  )
}

describe('row states (prototype/States.html)', () => {
  it('NEW — tagged new, pending dot', () => {
    const row = conv('crashloop', { phase: 'Pending' })
    renderRow(row, { isNew: true })
    expect(screen.getByText('new')).toBeInTheDocument()
    expect(phaseDot(row)).toBe('pending')
  })

  it('UNREAD + WORKING — bold title, count badge, snippet carries the last message', () => {
    const row = conv('checkout-api', {
      presence: true, unreadCount: 2, unread: true,
      lastMessage: { kind: 'relay', sender: 'oncall', text: 'Does the same setting affect…' },
    })
    renderRow(row)
    expect(screen.getByTestId('unread-checkout-api')).toHaveTextContent('2')
    expect(rowSnippet(row)).toBe('working · oncall: Does the same setting affect…')
  })

  it('WORKING, READ — no badge, the ack is presence rather than a message', () => {
    const row = conv('payments', { presence: true, pipeline: 'k8s-engineer', unreadCount: 0 })
    renderRow(row)
    expect(screen.queryByTestId(`unread-${row.name}`)).toBeNull()
    expect(rowSnippet(row)).toBe('k8s-engineer is working…')
  })

  it('PENDING, never joined — no console thread, never unread, the snippet says why it waits', () => {
    const row = conv('high-mem', { joined: false, phase: 'Pending' })
    renderRow(row)
    expect(rowSnippet(row)).toContain('pending')
  })

  it('never joined with nothing else to say falls through to the empty snippet — there is no "observed" tag any more', () => {
    // `observed` (`!row.joined`) was dropped: it showed on effectively every
    // unattended alert-investigator row, distinguishing nothing.
    const row = conv('watched', { joined: false, phase: 'Idle' })
    expect(rowSnippet(row)).toBe('')
    expect(rowTag(row, false)).toBeUndefined()
  })

  it('ERRORED — failed dot and the run-failed tag, still unread', () => {
    const row = conv('tls-cert', { errored: true, unreadCount: 1, unread: true })
    renderRow(row)
    expect(phaseDot(row)).toBe('failed')
    expect(screen.getByText('run failed')).toBeInTheDocument()
  })

  it('CLOSED — muted, reopen lives in the row menu rather than here', () => {
    const row = conv('webhook-timeout', { phase: 'Closed', runCount: 4 })
    renderRow(row)
    expect(screen.getByText('closed')).toBeInTheDocument()
  })

  it('an autosolved incident shows its reason and that nobody was notified', () => {
    const row = conv('node-3', {
      phase: 'Closed', coordinator: 'node-3', closeReason: 'evicted 2 pods, pressure cleared',
    })
    renderRow(row)
    expect(screen.getByText('nobody notified')).toBeInTheDocument()
    expect(rowSnippet(row)).toContain('evicted 2 pods')
  })
})

describe('selection and navigation', () => {
  it('shows a checkbox only in selection mode, disabled while the finalizer holds it', () => {
    const row = conv('closing', { deleting: true })
    renderRow(row, { selectionMode: true })
    expect(screen.getByLabelText(`select ${row.title || row.name}`)).toBeDisabled()
  })

  it('opens the conversation when the row body is activated', async () => {
    const onOpen = vi.fn()
    const row = conv('a')
    renderRow(row, { onOpen })
    screen.getByTestId('open-a').click()
    expect(onOpen).toHaveBeenCalled()
  })
})

describe('arrival (console-thread-live-cues)', () => {
  function stubReducedMotion(matches: boolean) {
    window.matchMedia = vi.fn().mockReturnValue({ matches }) as unknown as typeof window.matchMedia
  }

  it('carries the arrival class and the new tag ordinarily', () => {
    stubReducedMotion(false)
    const row = conv('a', { phase: 'Pending' })
    renderRow(row, { isNew: true })
    expect(screen.getByTestId('row-a')).toHaveClass('ao-row-arrive')
    expect(screen.getByText('new')).toBeInTheDocument()
  })

  it('drops the animation class under reduced motion, but keeps the marker', () => {
    stubReducedMotion(true)
    const row = conv('a', { phase: 'Pending' })
    renderRow(row, { isNew: true })
    expect(screen.getByTestId('row-a')).not.toHaveClass('ao-row-arrive')
    expect(screen.getByText('new')).toBeInTheDocument()
  })
})

describe('coordination extras', () => {
  it('a root with members shows a caret — no turn/member summary, no snippet line any more', () => {
    // Every row is exactly two lines now: title + time, then chips + menu.
    // The old "turn 2 of 6 · 3 members" line was internal budget-tracking
    // detail with no scanning value, and the snippet line that replaced it
    // was dropped too — a third line fighting the chips for space, when the
    // chips (coordinator + source) already say what a reader needs.
    const row = conv('root-1', {
      coordinator: 'root-1', budget: { maxTurns: 6, turns: 2 },
      lastMessage: { kind: 'relay', sender: 'oncall', text: 'any update?' },
    })
    renderRow(row, { memberCount: 3 })
    expect(screen.getByLabelText(`collapse ${row.title || row.name}`)).toBeInTheDocument()
    expect(screen.queryByText(/turn 2 of 6/)).not.toBeInTheDocument()
    expect(screen.queryByText(/3 members/)).not.toBeInTheDocument()
    expect(screen.queryByText(/any update\?/)).not.toBeInTheDocument()
  })

  it('a member names the entry it was invoked as', () => {
    const row = conv('member-1', { causedBy: { parent: 'root-1', entry: 'diagnose' } })
    renderRow(row, { depth: 1 })
    expect(screen.getByText('via diagnose')).toBeInTheDocument()
  })
})

describe('stripNamePrefix — the title must not repeat the chip', () => {
  it('strips "<name>: " when it is exactly the chip name', () => {
    expect(stripNamePrefix('agentops-coordinator: Check status of ha restart', 'agentops-coordinator'))
      .toBe('Check status of ha restart')
    expect(stripNamePrefix('ha-control: Check the current status of Home Assistant', 'ha-control'))
      .toBe('Check the current status of Home Assistant')
  })

  it('never strips a colon-prefix unrelated to the chip name', () => {
    expect(stripNamePrefix('esphome: tion-4s-bedroom lost its connection', 'ha-control'))
      .toBe('esphome: tion-4s-bedroom lost its connection')
    expect(stripNamePrefix('homeassistant: config_entry_reauth', 'ha-control'))
      .toBe('homeassistant: config_entry_reauth')
  })

  it('is a no-op with no name to compare against', () => {
    expect(stripNamePrefix('esphome: tion-4s-bedroom lost its connection', undefined))
      .toBe('esphome: tion-4s-bedroom lost its connection')
  })

  it('leaves a title with no matching prefix exactly as it is', () => {
    expect(stripNamePrefix('Check status of ha restart', 'agentops-coordinator'))
      .toBe('Check status of ha restart')
  })

  // Item 1's acceptance criteria, verbatim.
  it('(a) strips "esphome: " for the esphome pipeline, exact match', () => {
    expect(stripNamePrefix('esphome: light is on', 'esphome')).toBe('light is on')
  })

  it('(b) leaves "esphome:light is on" untouched — no space is not the stripped form', () => {
    expect(stripNamePrefix('esphome:light is on', 'esphome')).toBe('esphome:light is on')
  })

  it('(c) leaves an unrelated "something: else" untouched when the chip names a different pipeline', () => {
    expect(stripNamePrefix('something: else', 'other-pipeline')).toBe('something: else')
  })
})

describe('the row title no longer repeats its own chip', () => {
  it('strips the coordinator prefix for a root', () => {
    const row = conv('root-1', { coordinator: 'agentops-coordinator', title: 'agentops-coordinator: Check status of ha restart' })
    renderRow(row)
    expect(screen.getByText('Check status of ha restart')).toBeInTheDocument()
    expect(screen.queryByText('agentops-coordinator: Check status of ha restart')).not.toBeInTheDocument()
  })

  it('strips the pipeline prefix for an ordinary row', () => {
    const row = conv('job-1', { pipeline: 'ha-control', title: 'ha-control: Check the current status of Home Assistant' })
    renderRow(row)
    expect(screen.getByText('Check the current status of Home Assistant')).toBeInTheDocument()
  })

  it('leaves an unrelated colon-prefixed title untouched', () => {
    const row = conv('job-2', { pipeline: 'ha-control', title: 'esphome: tion-4s-bedroom lost its connection' })
    renderRow(row)
    expect(screen.getByText('esphome: tion-4s-bedroom lost its connection')).toBeInTheDocument()
  })

  // Bug: a member's `row.pipeline` is unset, so comparing against it (as a
  // root does against `coordinator`) left the prefix in place. The
  // comparison name for a member is `causedBy.entry` instead.
  it('strips the entry prefix for a member, exact match against causedBy.entry', () => {
    const row = conv('member-1', {
      causedBy: { parent: 'root-1', entry: 'ha-control' },
      title: 'ha-control: Read-only triage of a Home Assistant alert',
    })
    renderRow(row, { depth: 1 })
    expect(screen.getByText('Read-only triage of a Home Assistant alert')).toBeInTheDocument()
    expect(screen.queryByText('ha-control: Read-only triage of a Home Assistant alert')).not.toBeInTheDocument()
  })

  // The manager writes a member's title as `"🤝 " + entry + ": " + ...`
  // (coordinate.go) — the live-data shape, as opposed to the plain one above.
  it('strips both the 🤝 lane icon and the entry prefix for a real member title', () => {
    const row = conv('member-2a', {
      causedBy: { parent: 'root-1', entry: 'ha-control' },
      title: '🤝 ha-control: Read-only triage of a Home Assistant/ESPHome alert',
    })
    renderRow(row, { depth: 1 })
    expect(screen.getByText('Read-only triage of a Home Assistant/ESPHome alert')).toBeInTheDocument()
  })

  it('leaves a member title untouched when it does not match causedBy.entry', () => {
    const row = conv('member-2', {
      causedBy: { parent: 'root-1', entry: 'ha-control' },
      title: 'esphome: tion-4s-bedroom lost its connection',
    })
    renderRow(row, { depth: 1 })
    expect(screen.getByText('esphome: tion-4s-bedroom lost its connection')).toBeInTheDocument()
  })
})

describe('every row names what answers it', () => {
  it('a root chip names the coordinator, not just "coordinator"', () => {
    const row = conv('root-1', { coordinator: 'agentops-coordinator' })
    renderRow(row)
    expect(screen.getByText('agentops-coordinator')).toBeInTheDocument()
  })

  it('an ordinary row chips its pipeline', () => {
    const row = conv('job-1', { pipeline: 'k8s-observe' })
    renderRow(row)
    expect(screen.getByText('k8s-observe')).toBeInTheDocument()
  })

  it('a member row chips neither its parent coordinator nor a pipeline', () => {
    const row = conv('member-1', { causedBy: { parent: 'root-1', entry: 'diagnose' }, pipeline: 'k8s-observe' })
    renderRow(row, { depth: 1 })
    expect(screen.queryByText('k8s-observe')).not.toBeInTheDocument()
  })

  it('a PLAIN conversation (no coordinator, no pipeline) still chips its source', () => {
    // Measured live: `job-cb5vg` ("Self-heal sweep") had `source:
    // "reaper-sweep"` but no `coordinator` field at all — a cron-triggered
    // conversation reached by a claimed source, never a Coordinator root.
    // Gating the source chip on `isRoot` hid a source that was genuinely
    // there.
    const row = conv('job-cb5vg', { source: 'reaper-sweep' })
    renderRow(row)
    expect(screen.getByText('reaper-sweep')).toBeInTheDocument()
  })

  it('a member still never chips a source, even if one happened to be set', () => {
    const row = conv('member-1', { causedBy: { parent: 'root-1', entry: 'diagnose' }, source: 'reaper-sweep' })
    renderRow(row, { depth: 1 })
    expect(screen.queryByText('reaper-sweep')).not.toBeInTheDocument()
  })
})

describe('the row menu (regression: Reopen was built but never rendered)', () => {
  it('a closed root offers a working Reopen action, through the real row menu', async () => {
    const onReopen = vi.fn()
    const row = conv('webhook-timeout', { phase: 'Closed', runCount: 4 })
    renderRow(row, { onReopen })
    await userEvent.click(screen.getByLabelText(`actions for ${row.title || row.name}`))
    await userEvent.click(screen.getByText('Reopen'))
    expect(onReopen).toHaveBeenCalledTimes(1)
  })

  it('a member row never offers Reopen, even once closed', async () => {
    const row = conv('member-1', { phase: 'Closed', causedBy: { parent: 'root-1', entry: 'diagnose' } })
    renderRow(row, { depth: 1 })
    await userEvent.click(screen.getByLabelText(`actions for ${row.title || row.name}`))
    expect(screen.queryByText('Reopen')).toBeNull()
  })
})

describe('tree guide lines (distinguishing nesting levels from one another, not just from a root)', () => {
  it('a root has no guide columns and a FULL-WIDTH divider', () => {
    const row = conv('root-1', { coordinator: 'agentops-coordinator' })
    renderRow(row, { depth: 0 })
    const li = screen.getByTestId('row-root-1')
    // The row itself carries the divider at depth 0 — nothing narrower.
    expect(li).toHaveStyle({ borderBottom: '1px solid var(--ao-canvas)' })
    // No guide column before the row's own content div.
    expect(li.children).toHaveLength(1)
  })

  it('a depth-1 member gets exactly one guide column, and its divider sits on the INNER content, not the row', () => {
    const row = conv('member-1', { causedBy: { parent: 'root-1', entry: 'diagnose' } })
    renderRow(row, { depth: 1 })
    const li = screen.getByTestId('row-member-1')
    // One guide column + one content div.
    expect(li.children).toHaveLength(2)
    // The outer <li> carries no divider of its own at a nested depth — a
    // nested row must never look like it closes out a whole group.
    expect(li).not.toHaveStyle({ borderBottom: '1px solid var(--ao-canvas)' })
    const content = li.children[1] as HTMLElement
    expect(content).toHaveStyle({ borderBottom: '1px solid var(--ao-canvas)' })
  })

  it('a depth-2 row (a member that is itself a Coordinator\'s own member) gets TWO guide columns — visibly distinct from depth 1', () => {
    const row = conv('sub-member-1', { causedBy: { parent: 'member-1', entry: 'k8s-observe' } })
    renderRow(row, { depth: 2 })
    const li = screen.getByTestId('row-sub-member-1')
    // Two guide columns + one content div — one more than a depth-1 row,
    // which is the whole point: today's indent alone could not tell a
    // depth-1 and a depth-2 row apart at a glance past two levels.
    expect(li.children).toHaveLength(3)
  })
})

describe('a parent missing from the current view', () => {
  it('shows a "parent missing" tag naming the parent, for a member whose parent is absent', () => {
    const row = conv('orphan-1', { causedBy: { parent: 'root-gone', entry: 'diagnose' } })
    renderRow(row, { depth: 1, parentMissing: true })
    expect(screen.getByText('parent missing')).toBeInTheDocument()
    expect(screen.getByTitle('parent root-gone is not in view')).toBeInTheDocument()
  })

  it('never shows it for a root, even if parentMissing were somehow set', () => {
    const row = conv('root-1', { coordinator: 'root-1' })
    renderRow(row, { parentMissing: true })
    expect(screen.queryByText('parent missing')).not.toBeInTheDocument()
  })
})

describe('the row menu\'s navigation actions, through the real row (regression: wired to a stub in isolation only)', () => {
  const realLocation = window.location
  const realClipboard = navigator.clipboard

  afterEach(() => {
    Object.defineProperty(window, 'location', { value: realLocation, configurable: true })
    Object.defineProperty(navigator, 'clipboard', { value: realClipboard, configurable: true })
  })

  it('"Open in new tab" opens this row\'s own conversation URL', async () => {
    Object.defineProperty(window, 'location', { value: { origin: 'https://console.example' }, configurable: true })
    const openSpy = vi.fn()
    window.open = openSpy
    const row = conv('checkout-api')
    renderRow(row)
    await userEvent.click(screen.getByLabelText(`actions for ${row.title || row.name}`))
    await userEvent.click(screen.getByText('Open in new tab'))
    expect(openSpy).toHaveBeenCalledWith('https://console.example/conversations/checkout-api', '_blank', 'noopener')
  })

  it('"Copy link" writes the same URL to the clipboard', async () => {
    Object.defineProperty(window, 'location', { value: { origin: 'https://console.example' }, configurable: true })
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
    const row = conv('checkout-api')
    renderRow(row)
    await userEvent.click(screen.getByLabelText(`actions for ${row.title || row.name}`))
    await userEvent.click(screen.getByText('Copy link'))
    expect(writeText).toHaveBeenCalledWith('https://console.example/conversations/checkout-api')
  })
})
