import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { ConversationRow, phaseDot, rowSnippet, rowTag } from './ConversationRow'
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

  it('OBSERVED with nothing else to say falls back to the word itself', () => {
    const row = conv('watched', { joined: false, phase: 'Idle' })
    expect(rowSnippet(row)).toBe('observed')
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
