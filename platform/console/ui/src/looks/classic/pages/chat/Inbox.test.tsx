import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { Inbox } from './Inbox'

vi.mock('../../../../api/hooks', () => ({
  useInboxCounts: vi.fn(() => ({
    data: {
      items: [], total: 9, unreadTotal: 5, offset: 0, limit: 0, facets: {},
      scopes: { open: 5, working: 2, mine: 1, errored: 1, closed: 7, 'rollout-coordinator': 1, 'alert-triage': 1 },
    },
  })),
  useVocabulary: () => ({
    data: {
      entries: [
        { kind: 'pipeline', name: 'k8s-observe', position: 'general', icon: 'aops:kubernetes' },
        { kind: 'pipeline', name: 'alert-triage', position: 'general' },
        { kind: 'coordinator', name: 'rollout-coordinator', position: 'general' },
        { kind: 'builtin', name: 'pipelines', position: 'general', description: 'List the pipelines' },
        { kind: 'builtin', name: 'help', position: 'general' },
        { kind: 'builtin', name: 'exit', position: 'thread' },
        { kind: 'builtin', name: 'close', position: 'thread' },
      ],
    },
  }),
}))

function order() {
  return screen.getAllByRole('button').map((b) => b.textContent)
}

describe('the inbox, expanded', () => {
  it('lists the fixed scopes, then pipelines and coordinators, then Closed, in spec order', () => {
    render(<Inbox activeScope={{ kind: 'open' }} onSelectScope={vi.fn()} collapsed={false} onToggleCollapsed={vi.fn()} />)
    const labels = order()
    const index = (s: string) => labels.findIndex((l) => l?.includes(s))
    // There is no separate "Unread" scope any more — every other scope
    // (Working, Mine, Errored, each Pipeline/Coordinator, Closed) already
    // carries its own unread count, which made a blanket one redundant.
    expect(index('Open')).toBeLessThan(index('Working'))
    expect(index('Working')).toBeLessThan(index('Mine'))
    expect(index('Mine')).toBeLessThan(index('Errored'))
    expect(index('Errored')).toBeLessThan(index('k8s-observe'))
    expect(index('alert-triage')).toBeLessThan(index('Closed'))
    // Item 23 — there is no COMMANDS section any more, in any position.
    expect(labels.some((l) => l?.includes('/pipelines'))).toBe(false)
    expect(labels.some((l) => l?.includes('/help'))).toBe(false)
    expect(labels.some((l) => l?.includes('/exit'))).toBe(false)
    // Item 26 — there is no Incidents scope any more.
    expect(labels.some((l) => l?.includes('Incidents'))).toBe(false)
  })

  it('shows Open and Closed their own unread counts, and every other scope its own', () => {
    render(<Inbox activeScope={{ kind: 'open' }} onSelectScope={vi.fn()} collapsed={false} onToggleCollapsed={vi.fn()} />)
    expect(screen.getByTestId('scope-Open')).toHaveTextContent('5')
    expect(screen.getByTestId('scope-Working')).toHaveTextContent('2')
    expect(screen.getByTestId('scope-Mine')).toHaveTextContent('1')
    expect(screen.getByTestId('scope-Closed')).toHaveTextContent('7')
  })

  it('offers no "Show closed" control anywhere', () => {
    render(<Inbox activeScope={{ kind: 'open' }} onSelectScope={vi.fn()} collapsed={false} onToggleCollapsed={vi.fn()} />)
    expect(screen.queryByLabelText(/show closed/i)).toBeNull()
  })

  it('narrows the list when a pipeline scope is chosen', async () => {
    const onSelectScope = vi.fn()
    render(<Inbox activeScope={{ kind: 'open' }} onSelectScope={onSelectScope} collapsed={false} onToggleCollapsed={vi.fn()} />)
    screen.getByTestId('scope-k8s-observe').click()
    expect(onSelectScope).toHaveBeenCalledWith({ kind: 'pipeline', name: 'k8s-observe' })
  })
})

describe('the inbox, collapsed', () => {
  it('keeps every badge on the icon strip, Open and Closed included', () => {
    render(<Inbox activeScope={{ kind: 'open' }} onSelectScope={vi.fn()} collapsed onToggleCollapsed={vi.fn()} />)
    const strip = screen.getByTestId('inbox-collapsed')
    // Open's own count (5), Working's (2), the coordinator's (1) and Closed's (7) all carry over.
    expect(strip.textContent).toContain('5')
    expect(strip.textContent).toContain('2')
    expect(strip.textContent).toContain('1')
    expect(strip.textContent).toContain('7')
  })
})
