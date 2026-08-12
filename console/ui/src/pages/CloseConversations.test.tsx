import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { CloseSelectedModal, notClosed, selectableNames, workingCount } from './CloseConversations'
import type { CloseResponse, ConversationSummary } from '../api/types'

// The confirmation is where the cost of a batch is stated: how many, how many of
// them are working, and that none of it is reversible.

function conv(name: string, over: Partial<ConversationSummary> = {}): ConversationSummary {
  return {
    name, runCount: 0, queued: 0, joined: true, errored: false, ageSeconds: 1, closing: false,
    ...over,
  }
}

describe('selection', () => {
  it('offers only the rows it was given — which is one page', () => {
    // The server paginates, so `items` IS the current page. There is no
    // "select everything matching the filter" anywhere to widen this.
    const page = [conv('a'), conv('b'), conv('c')]
    expect(selectableNames(page)).toEqual(['a', 'b', 'c'])
  })

  it('cannot select a conversation that is already closing', () => {
    const page = [conv('a'), conv('going', { closing: true }), conv('b')]
    expect(selectableNames(page)).toEqual(['a', 'b'])
  })

  it('counts the working conversations in the selection', () => {
    const page = [conv('a', { phase: 'Working' }), conv('b', { phase: 'Idle' }), conv('c', { phase: 'Working' })]
    expect(workingCount(page, new Set(['a', 'b']))).toBe(1)
    expect(workingCount(page, new Set(['a', 'b', 'c']))).toBe(2)
  })
})

describe('the confirmation', () => {
  it('states the count, the working count, and defaults the opt-in to off', async () => {
    const onConfirm = vi.fn()
    render(
      <CloseSelectedModal
        isOpen
        names={['a', 'b', 'c', 'd']}
        working={2}
        busy={false}
        onConfirm={onConfirm}
        onClose={() => {}}
      />,
    )
    expect(screen.getByText(/4 conversation\(s\) will be closed/)).toBeInTheDocument()
    expect(screen.getByTestId('close-working-count')).toHaveTextContent('2 of them are working')
    expect(screen.getByText(/cannot be undone/i)).toBeInTheDocument()

    // The toggle names what it costs, and starts off: abandoning a live run is
    // right for a deliberate single close and wrong as a batch's silent default.
    const toggle = screen.getByLabelText(/include working/i)
    expect(toggle).not.toBeChecked()

    await userEvent.click(screen.getByTestId('close-confirm'))
    expect(onConfirm).toHaveBeenCalledWith(false)
  })

  it('passes the opt-in only when it was actually turned on', async () => {
    const onConfirm = vi.fn()
    render(
      <CloseSelectedModal
        isOpen
        names={['a']}
        working={1}
        busy={false}
        onConfirm={onConfirm}
        onClose={() => {}}
      />,
    )
    await userEvent.click(screen.getByLabelText(/include working/i))
    await userEvent.click(screen.getByTestId('close-confirm'))
    expect(onConfirm).toHaveBeenCalledWith(true)
  })

  it('renders a per-conversation reason rather than one verdict', () => {
    // A mixed batch is the NORMAL outcome. "Some failed" is not an answer an
    // operator can act on; the server's own reason for each one is.
    const result: CloseResponse = {
      closed: 1,
      skipped: 2,
      failed: 1,
      results: [
        { name: 'a', outcome: 'closed' },
        { name: 'observed', outcome: 'skipped', reason: 'the console holds no thread on this conversation' },
        { name: 'busy', outcome: 'skipped', reason: 'working — closing it would abandon the run in progress' },
        { name: 'gone', outcome: 'failed', reason: 'no such conversation' },
      ],
    }
    render(
      <CloseSelectedModal
        isOpen
        names={['a', 'observed', 'busy', 'gone']}
        working={1}
        result={result}
        busy={false}
        onConfirm={() => {}}
        onClose={() => {}}
      />,
    )
    expect(screen.getByText('1 closed, 2 skipped, 1 failed')).toBeInTheDocument()
    const reasons = screen.getByTestId('close-reasons')
    expect(reasons).toHaveTextContent('observed')
    expect(reasons).toHaveTextContent('holds no thread')
    expect(reasons).toHaveTextContent('abandon the run in progress')
    expect(reasons).toHaveTextContent('no such conversation')
    // the one that CLOSED needs no explanation and gets none
    expect(notClosed(result).map((r) => r.name)).toEqual(['observed', 'busy', 'gone'])
  })
})
