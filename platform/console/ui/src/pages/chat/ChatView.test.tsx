import { describe, expect, it, vi } from 'vitest'
import { act, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { ChatView } from './ChatView'
import type { ConversationSummary } from '../../api/types'

function conv(name: string, over: Partial<ConversationSummary> = {}): ConversationSummary {
  return {
    name, title: name, runCount: 0, queued: 0, joined: true, errored: false, unread: false,
    ageSeconds: 10, deleting: false, phase: 'Idle', ...over,
  }
}

let items: ConversationSummary[] = [conv('a'), conv('b'), conv('c')]

vi.mock('../../api/hooks', () => ({
  useConversations: () => ({ data: { items, total: items.length, unreadTotal: 0, offset: 0, limit: 100, facets: {} }, isLoading: false, error: null }),
  usePipelineIcon: () => () => undefined,
  useSession: () => ({ data: { canWrite: true, identity: 'dana', canOriginate: true } }),
  useSources: () => ({ data: { sources: [] } }),
  useInboxCounts: () => ({ data: { items: [], total: items.length, unreadTotal: 0, offset: 0, limit: 0, facets: {}, scopes: {} } }),
  useVocabulary: () => ({ data: { entries: [] } }),
  useCloseConversations: () => ({ mutate: vi.fn(), data: undefined, error: null, isPending: false, reset: vi.fn() }),
  useDeleteConversations: () => ({ mutate: vi.fn(), data: undefined, error: null, isPending: false, reset: vi.fn() }),
  useMarkRead: () => ({ mutate: vi.fn(), data: undefined, error: null, isPending: false, reset: vi.fn() }),
  useMarkUnread: () => ({ mutate: vi.fn(), data: undefined, error: null, isPending: false, reset: vi.fn() }),
}))

vi.mock('./ThreadPane', () => ({ ThreadPane: ({ name }: { name: string }) => <div data-testid="thread-pane">{name}</div> }))

function renderAt(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route path="/conversations" element={<ChatView />} />
        <Route path="/conversations/:name" element={<ChatView />} />
      </Routes>
    </MemoryRouter>,
  )
}

describe('both routes render the one view', () => {
  it('shows the list with no conversation open', () => {
    renderAt('/conversations')
    expect(screen.getByLabelText('conversations')).toBeInTheDocument()
    expect(screen.queryByTestId('thread-pane')).toBeNull()
  })

  it('shows the list AND the thread when a name is given', () => {
    renderAt('/conversations/b')
    expect(screen.getByLabelText('conversations')).toBeInTheDocument()
    expect(screen.getByTestId('thread-pane')).toHaveTextContent('b')
  })
})

describe('narrow windows show one column', () => {
  it('hides the list once a conversation is open', () => {
    window.innerWidth = 600
    renderAt('/conversations/b')
    expect(screen.queryByLabelText('conversations')).toBeNull()
    expect(screen.getByTestId('thread-pane')).toBeInTheDocument()
    window.innerWidth = 1440
  })

  it('shows the list alone with nothing open', () => {
    window.innerWidth = 600
    renderAt('/conversations')
    expect(screen.getByLabelText('conversations')).toBeInTheDocument()
    expect(screen.queryByTestId('thread-pane')).toBeNull()
    window.innerWidth = 1440
  })

  it('follows a LIVE resize too, not only the width at mount', () => {
    window.innerWidth = 1440
    renderAt('/conversations/b')
    expect(screen.getByLabelText('conversations')).toBeInTheDocument()
    window.innerWidth = 600
    act(() => window.dispatchEvent(new Event('resize')))
    expect(screen.queryByLabelText('conversations')).toBeNull()
    window.innerWidth = 1440
    act(() => window.dispatchEvent(new Event('resize')))
  })
})

describe('arrivals (console-thread-live-cues)', () => {
  // The toast popup is gone (item 9) — the tinted row and its "new" tag are
  // the whole of the signal now, never a popup naming the pipeline.
  it('tags the new row, and raises no toast', () => {
    const original = items
    items = [conv('a'), conv('b'), conv('c')]
    const { rerender } = render(
      <MemoryRouter initialEntries={['/conversations']}>
        <Routes>
          <Route path="/conversations" element={<ChatView />} />
        </Routes>
      </MemoryRouter>,
    )
    items = [conv('fresh', { pipeline: 'alert-triage', title: 'a brand new incident' }), ...items]
    rerender(
      <MemoryRouter initialEntries={['/conversations']}>
        <Routes>
          <Route path="/conversations" element={<ChatView />} />
        </Routes>
      </MemoryRouter>,
    )
    expect(screen.getByText('new')).toBeInTheDocument()
    expect(screen.queryByText(/alert-triage opened a brand new incident/)).not.toBeInTheDocument()
    items = original
  })
})

describe('expand all / collapse all (item 2)', () => {
  it('is absent with nothing collapsible to toggle', () => {
    renderAt('/conversations')
    expect(screen.queryByText('Expand all')).not.toBeInTheDocument()
    expect(screen.queryByText('Collapse all')).not.toBeInTheDocument()
  })

  it('collapses every root with members, then expands them again', async () => {
    const original = items
    items = [
      conv('root-1', { coordinator: 'root-1' }),
      conv('member-1', { causedBy: { parent: 'root-1', entry: 'diagnose' } }),
    ]
    renderAt('/conversations')
    await userEvent.click(screen.getByText('Collapse all'))
    expect(screen.queryByTestId('row-member-1')).not.toBeInTheDocument()
    await userEvent.click(screen.getByText('Expand all'))
    expect(screen.getByTestId('row-member-1')).toBeInTheDocument()
    items = original
  })
})

describe('show closed (item 8)', () => {
  it('hides closed conversations by default, and reveals them when checked', async () => {
    const original = items
    items = [conv('a'), conv('b', { phase: 'Closed' })]
    renderAt('/conversations')
    expect(screen.queryByTestId('row-b')).not.toBeInTheDocument()
    await userEvent.click(screen.getByLabelText('Show closed'))
    expect(screen.getByTestId('row-b')).toBeInTheDocument()
    items = original
  })
})

describe('keyboard navigation', () => {
  it('walks the rows with the arrow keys and opens the highlighted one on Enter', async () => {
    renderAt('/conversations')
    const view = screen.getByTestId('chat-view')
    view.focus()
    await userEvent.keyboard('{ArrowDown}{ArrowDown}')
    expect(document.activeElement).toHaveAttribute('data-testid', 'open-b')
    await userEvent.keyboard('{Enter}')
    expect(screen.getByTestId('thread-pane')).toHaveTextContent('b')
  })

  it('clamps at both ends of the list', async () => {
    renderAt('/conversations')
    screen.getByTestId('chat-view').focus()
    await userEvent.keyboard('{ArrowUp}')
    expect(document.activeElement).toHaveAttribute('data-testid', 'open-a')
    await userEvent.keyboard('{ArrowDown}{ArrowDown}{ArrowDown}{ArrowDown}')
    expect(document.activeElement).toHaveAttribute('data-testid', 'open-c')
    await userEvent.keyboard('{ArrowUp}')
    expect(document.activeElement).toHaveAttribute('data-testid', 'open-b')
  })

  it('ignores Enter with nothing highlighted and other keys entirely', async () => {
    renderAt('/conversations')
    screen.getByTestId('chat-view').focus()
    await userEvent.keyboard('{Enter}x')
    expect(screen.queryByTestId('thread-pane')).toBeNull()
  })

  it('Escape leaves selection mode', async () => {
    renderAt('/conversations')
    await userEvent.click(screen.getByText('Select'))
    expect(screen.getByText('Done selecting')).toBeInTheDocument()
    screen.getByTestId('chat-view').focus()
    await userEvent.keyboard('{Escape}')
    expect(screen.getByText('Select')).toBeInTheDocument()
  })
})
