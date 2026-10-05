import { describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Inbox } from './Inbox'

vi.mock('../../api/hooks', () => ({
  useInboxCounts: () => ({
    data: {
      items: [], total: 7, unreadTotal: 2, offset: 0, limit: 0, facets: {},
      scopes: { working: 2, mine: 1, errored: 1, 'rollout-coordinator': 1, 'alert-triage': 1 },
    },
  }),
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
    render(<Inbox activeScope={{ kind: 'all' }} onSelectScope={vi.fn()} collapsed={false} onToggleCollapsed={vi.fn()} />)
    const labels = order()
    const index = (s: string) => labels.findIndex((l) => l?.includes(s))
    expect(index('All')).toBeLessThan(index('Unread'))
    expect(index('Unread')).toBeLessThan(index('Working'))
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

  it('shows each scope its own unread count', () => {
    render(<Inbox activeScope={{ kind: 'all' }} onSelectScope={vi.fn()} collapsed={false} onToggleCollapsed={vi.fn()} />)
    expect(screen.getByTestId('scope-All')).toHaveTextContent('7')
    expect(screen.getByTestId('scope-Unread')).toHaveTextContent('2')
    expect(screen.getByTestId('scope-Working')).toHaveTextContent('2')
    expect(screen.getByTestId('scope-Mine')).toHaveTextContent('1')
  })

  it('narrows the list when a pipeline scope is chosen', async () => {
    const onSelectScope = vi.fn()
    render(<Inbox activeScope={{ kind: 'all' }} onSelectScope={onSelectScope} collapsed={false} onToggleCollapsed={vi.fn()} />)
    screen.getByTestId('scope-k8s-observe').click()
    expect(onSelectScope).toHaveBeenCalledWith({ kind: 'pipeline', name: 'k8s-observe' })
  })
})

describe('the "All" count explains the gap to visible rows (item 24)', () => {
  it('names closed conversations and invoked members on hover, never a bare number with no explanation', async () => {
    render(<Inbox activeScope={{ kind: 'all' }} onSelectScope={vi.fn()} collapsed={false} onToggleCollapsed={vi.fn()} />)
    await userEvent.hover(screen.getByTestId('scope-All').querySelector('.pf-v6-c-badge')!)
    await waitFor(() => {
      expect(screen.getByText(/closed ones and invoked members/)).toBeInTheDocument()
    })
  })

  it('carries no such explanation on an ordinary scope — that count really does match its rows', async () => {
    render(<Inbox activeScope={{ kind: 'all' }} onSelectScope={vi.fn()} collapsed={false} onToggleCollapsed={vi.fn()} />)
    await userEvent.hover(screen.getByTestId('scope-Working').querySelector('.pf-v6-c-badge')!)
    // No tooltip at all is wired to an ordinary scope's badge in the expanded
    // list — unlike the collapsed icon strip below, which tips every entry.
    expect(screen.queryByText(/closed ones and invoked members/)).not.toBeInTheDocument()
  })

  it('still names the gap on the collapsed icon strip, beside its own label · count tip', async () => {
    render(<Inbox activeScope={{ kind: 'all' }} onSelectScope={vi.fn()} collapsed onToggleCollapsed={vi.fn()} />)
    const strip = screen.getByTestId('inbox-collapsed')
    await userEvent.hover(strip.querySelectorAll('button')[0])
    await waitFor(() => {
      expect(screen.getByText(/closed ones and invoked members/)).toBeInTheDocument()
    })
  })
})

describe('the inbox, collapsed', () => {
  it('keeps every badge on the icon strip', () => {
    render(<Inbox activeScope={{ kind: 'all' }} onSelectScope={vi.fn()} collapsed onToggleCollapsed={vi.fn()} />)
    const strip = screen.getByTestId('inbox-collapsed')
    // Working's unread count (2) and the coordinator's (1) both carry over.
    expect(strip.textContent).toContain('2')
    expect(strip.textContent).toContain('1')
  })
})
