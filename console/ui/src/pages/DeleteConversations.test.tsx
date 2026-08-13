import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { DeleteSelectedModal, deletableNames, notDeleted } from './DeleteConversations'
import type { DeleteResponse, ConversationSummary } from '../api/types'

// Deleting is the irreversible half, so the tests here are about what it
// REFUSES and what it SAYS — the two things that keep it from being a
// one-click way to destroy a record.

function conv(name: string, over: Partial<ConversationSummary> = {}): ConversationSummary {
  return {
    name, runCount: 0, queued: 0, joined: true, errored: false, ageSeconds: 1, closing: false,
    ...over,
  }
}

describe('selection', () => {
  it('offers only CLOSED conversations', () => {
    // The two-step IS the safety property: a live conversation is not a
    // candidate at all, rather than a candidate that gets closed on the way
    // through.
    const page = [
      conv('idle', { phase: 'Idle' }),
      conv('done', { phase: 'Closed' }),
      conv('busy', { phase: 'Working' }),
      conv('also-done', { phase: 'Closed' }),
    ]
    expect(deletableNames(page)).toEqual(['done', 'also-done'])
  })

  it('excludes one already on its way out', () => {
    const page = [conv('going', { phase: 'Closed', closing: true }), conv('done', { phase: 'Closed' })]
    expect(deletableNames(page)).toEqual(['done'])
  })
})

describe('the confirmation', () => {
  it('names what is destroyed, not just how many', async () => {
    render(
      <DeleteSelectedModal
        isOpen
        names={['a', 'b']}
        busy={false}
        onConfirm={vi.fn()}
        onClose={vi.fn()}
      />,
    )
    const text = screen.getByTestId('delete-consequences').textContent ?? ''
    // The recorded answers are the only durable copy of what the agent said,
    // and the workspace is the other half. A count alone does not tell anyone
    // what they are about to lose.
    expect(text).toContain('recorded answers')
    expect(text).toContain('workspace')
    expect(text).toContain('cannot be undone')
    expect(text).toContain('2 closed conversation(s)')
  })

  it('points at closing as the reversible alternative', () => {
    render(
      <DeleteSelectedModal isOpen names={['a']} busy={false} onConfirm={vi.fn()} onClose={vi.fn()} />,
    )
    expect(screen.getByText(/can be reopened/)).toBeTruthy()
  })

  it('confirms with the count and nothing else to decide', async () => {
    const onConfirm = vi.fn()
    render(
      <DeleteSelectedModal isOpen names={['a', 'b']} busy={false} onConfirm={onConfirm} onClose={vi.fn()} />,
    )
    await userEvent.click(screen.getByTestId('delete-confirm'))
    expect(onConfirm).toHaveBeenCalledTimes(1)
    // No includeWorking equivalent: there is no flag that makes deleting reach
    // further, because it never reaches a conversation that is not closed.
    expect(onConfirm).toHaveBeenCalledWith()
  })

  it('cannot be confirmed with an empty selection', () => {
    render(
      <DeleteSelectedModal isOpen names={[]} busy={false} onConfirm={vi.fn()} onClose={vi.fn()} />,
    )
    expect(screen.getByTestId('delete-confirm')).toHaveProperty('disabled', true)
  })
})

describe('the result', () => {
  const mixed: DeleteResponse = {
    deleted: 1,
    skipped: 1,
    failed: 1,
    results: [
      { name: 'gone', outcome: 'deleted' },
      { name: 'live', outcome: 'skipped', reason: 'close it first — deleting removes its recorded answers' },
      { name: 'broken', outcome: 'failed', reason: 'deleting failed' },
    ],
  }

  it('reports per conversation, because a partial batch is the normal outcome', () => {
    expect(notDeleted(mixed).map((r) => r.name)).toEqual(['live', 'broken'])
  })

  it('shows the totals and every reason it was given', () => {
    render(
      <DeleteSelectedModal
        isOpen
        names={['gone', 'live', 'broken']}
        result={mixed}
        busy={false}
        onConfirm={vi.fn()}
        onClose={vi.fn()}
      />,
    )
    expect(screen.getByText(/1 deleted, 1 skipped, 1 failed/)).toBeTruthy()
    // the server's own reason, passed through rather than flattened
    expect(screen.getByText(/close it first/)).toBeTruthy()
  })
})
