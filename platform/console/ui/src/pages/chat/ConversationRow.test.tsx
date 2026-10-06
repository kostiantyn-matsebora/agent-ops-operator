import { describe, expect, it, vi } from 'vitest'
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

  it('OBSERVED — no console thread, never unread, the snippet says why it waits', () => {
    const row = conv('high-mem', { joined: false, phase: 'Pending' })
    renderRow(row)
    expect(rowTag(row, false)).toBe('observed')
    expect(rowSnippet(row)).toContain('pending')
  })

  it('OBSERVED with nothing else to say falls through to the empty snippet — "observed" lives only in the chip', () => {
    const row = conv('watched', { joined: false, phase: 'Idle' })
    expect(rowSnippet(row)).toBe('')
    expect(rowTag(row, false)).toBe('observed')
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
  it('a root with members shows a caret and the member/turn summary instead of a snippet', () => {
    const row = conv('root-1', { coordinator: 'root-1', budget: { maxTurns: 6, turns: 2 } })
    renderRow(row, { memberCount: 3 })
    expect(screen.getByLabelText(`collapse ${row.title || row.name}`)).toBeInTheDocument()
    expect(screen.getByText(/turn 2 of 6/)).toBeInTheDocument()
    expect(screen.getByText(/3 members/)).toBeInTheDocument()
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
