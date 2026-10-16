import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { Inbox } from './Inbox'
import { useInboxCounts } from '../../api/hooks'

vi.mock('../../api/hooks', () => ({
  useInboxCounts: vi.fn(() => ({
    data: {
      items: [], total: 9, unreadTotal: 2, offset: 0, limit: 0, facets: {},
      scopes: { working: 2, mine: 1, errored: 1, closed: 7, 'rollout-coordinator': 1, 'alert-triage': 1 },
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
    render(<Inbox activeScope={{ kind: 'all' }} onSelectScope={vi.fn()} collapsed={false} onToggleCollapsed={vi.fn()} showClosed={false} />)
    const labels = order()
    const index = (s: string) => labels.findIndex((l) => l?.includes(s))
    // There is no separate "Unread" scope any more — `kind: 'all'` is the
    // one with no further predicate, and it keeps its label, All, now
    // counting "all unread" since every other scope went unread-based too.
    expect(index('All')).toBeLessThan(index('Working'))
    expect(index('Working')).toBeLessThan(index('Mine'))
    expect(index('Mine')).toBeLessThan(index('Errored'))
    expect(index('Errored')).toBeLessThan(index('k8s-observe'))
    expect(index('alert-triage')).toBeLessThan(index('Archive'))
    // Item 23 — there is no COMMANDS section any more, in any position.
    expect(labels.some((l) => l?.includes('/pipelines'))).toBe(false)
    expect(labels.some((l) => l?.includes('/help'))).toBe(false)
    expect(labels.some((l) => l?.includes('/exit'))).toBe(false)
    // Item 26 — there is no Incidents scope any more.
    expect(labels.some((l) => l?.includes('Incidents'))).toBe(false)
  })

  it('shows each scope its own unread count, Archive included', () => {
    render(<Inbox activeScope={{ kind: 'all' }} onSelectScope={vi.fn()} collapsed={false} onToggleCollapsed={vi.fn()} showClosed={false} />)
    expect(screen.getByTestId('scope-All')).toHaveTextContent('2')
    expect(screen.getByTestId('scope-Working')).toHaveTextContent('2')
    expect(screen.getByTestId('scope-Mine')).toHaveTextContent('1')
    expect(screen.getByTestId('scope-Archive')).toHaveTextContent('7')
  })

  it('narrows the list when a pipeline scope is chosen', async () => {
    const onSelectScope = vi.fn()
    render(<Inbox activeScope={{ kind: 'all' }} onSelectScope={onSelectScope} collapsed={false} onToggleCollapsed={vi.fn()} showClosed={false} />)
    screen.getByTestId('scope-k8s-observe').click()
    expect(onSelectScope).toHaveBeenCalledWith({ kind: 'pipeline', name: 'k8s-observe' })
  })
})

describe('the unread-facing counts follow the list\'s own "Show closed" preference', () => {
  it('passes showClosed straight through to the counts query', () => {
    render(<Inbox activeScope={{ kind: 'all' }} onSelectScope={vi.fn()} collapsed={false} onToggleCollapsed={vi.fn()} showClosed />)
    expect(vi.mocked(useInboxCounts)).toHaveBeenCalledWith(true)

    render(<Inbox activeScope={{ kind: 'all' }} onSelectScope={vi.fn()} collapsed={false} onToggleCollapsed={vi.fn()} showClosed={false} />)
    expect(vi.mocked(useInboxCounts)).toHaveBeenCalledWith(false)
  })
})

describe('the inbox, collapsed', () => {
  it('keeps every badge on the icon strip', () => {
    render(<Inbox activeScope={{ kind: 'all' }} onSelectScope={vi.fn()} collapsed onToggleCollapsed={vi.fn()} showClosed={false} />)
    const strip = screen.getByTestId('inbox-collapsed')
    // Working's unread count (2) and the coordinator's (1) both carry over.
    expect(strip.textContent).toContain('2')
    expect(strip.textContent).toContain('1')
  })
})
