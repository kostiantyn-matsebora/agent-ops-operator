import { afterEach, describe, expect, it, vi } from 'vitest'
import { act, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { ChatView } from './ChatView'
import type { ConversationSummary, VocabularyEntry } from '../../api/types'

function conv(name: string, over: Partial<ConversationSummary> = {}): ConversationSummary {
  return {
    name, title: name, runCount: 0, queued: 0, joined: true, errored: false, unread: false,
    ageSeconds: 10, deleting: false, phase: 'Idle', ...over,
  }
}

const DEFAULT_ITEMS: ConversationSummary[] = [conv('a'), conv('b'), conv('c')]
let items: ConversationSummary[] = DEFAULT_ITEMS
// The Inbox's own "pipelines and coordinators" section, read from the
// vocabulary — empty by default since most tests here never open it.
let vocabularyEntries: VocabularyEntry[] = []

// A manual `items = original` at the END of a test body never runs if an
// assertion earlier in that SAME test throws — which leaks that test's
// fixture into every test that runs after it in this file, as happened here
// (one failing assertion cascaded into two unrelated, previously-passing
// tests). Restoring in `afterEach` runs regardless of how the test ended.
afterEach(() => {
  items = DEFAULT_ITEMS
  vocabularyEntries = []
  // `showClosed` and the tree's fold default now live in `layout.ts`'s
  // localStorage-backed store (item 2 / item 8) — left unset, a persistence
  // test earlier in the file would leak its choice into every test after it.
  localStorage.clear()
})

vi.mock('../../api/hooks', () => ({
  useConversations: () => ({ data: { items, total: items.length, unreadTotal: 0, offset: 0, limit: 100, facets: {} }, isLoading: false, error: null }),
  usePipelineIcon: () => () => undefined,
  useSession: () => ({ data: { canWrite: true, identity: 'dana', canOriginate: true } }),
  useSources: () => ({ data: { sources: [] } }),
  useInboxCounts: () => ({ data: { items: [], total: items.length, unreadTotal: 0, offset: 0, limit: 0, facets: {}, scopes: {} } }),
  useVocabulary: () => ({ data: { entries: vocabularyEntries } }),
  useCloseConversations: () => ({ mutate: vi.fn(), data: undefined, error: null, isPending: false, reset: vi.fn() }),
  useDeleteConversations: () => ({ mutate: vi.fn(), data: undefined, error: null, isPending: false, reset: vi.fn() }),
  useMarkRead: () => ({ mutate: vi.fn(), data: undefined, error: null, isPending: false, reset: vi.fn() }),
  useMarkUnread: () => ({ mutate: vi.fn(), data: undefined, error: null, isPending: false, reset: vi.fn() }),
  useReopenConversation: () => ({ mutate: vi.fn() }),
}))

vi.mock('../../api/client', () => ({ api: { send: vi.fn() } }))

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

describe('a phone width turns the Inbox into a drawer, never a squeezed column', () => {
  // Measured live at 375px: the Inbox panel (min 200px) rendered as a fixed
  // column beside the list (min 280px) overflowed the viewport, and the
  // Inbox's own text bled behind the list rather than being hidden.
  it('does not render the Inbox column inline below the mobile breakpoint', () => {
    window.innerWidth = 375
    act(() => window.dispatchEvent(new Event('resize')))
    renderAt('/conversations')
    expect(screen.queryByTestId('inbox')).toBeNull()
    expect(screen.getByRole('button', { name: 'conversation filters' })).toBeInTheDocument()
    window.innerWidth = 1440
    act(() => window.dispatchEvent(new Event('resize')))
  })

  it('opens the Inbox full-screen on request, and returns to the list once a scope is picked', async () => {
    window.innerWidth = 375
    act(() => window.dispatchEvent(new Event('resize')))
    renderAt('/conversations')
    await userEvent.click(screen.getByRole('button', { name: 'conversation filters' }))
    expect(screen.getByTestId('inbox')).toBeInTheDocument()
    // The list is replaced, not merely covered — nothing of it renders either.
    expect(screen.queryByRole('button', { name: 'conversation filters' })).toBeNull()
    await userEvent.click(screen.getByTestId('scope-Unread'))
    expect(screen.queryByTestId('inbox')).toBeNull()
    expect(screen.getByRole('button', { name: 'conversation filters' })).toBeInTheDocument()
    window.innerWidth = 1440
    act(() => window.dispatchEvent(new Event('resize')))
  })

  it('still renders the Inbox as an ordinary column above the mobile breakpoint', () => {
    window.innerWidth = 700
    act(() => window.dispatchEvent(new Event('resize')))
    renderAt('/conversations')
    expect(screen.getByTestId('inbox')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'conversation filters' })).toBeNull()
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

  // Switching scope used to look exactly like a batch of real arrivals: the
  // "seen" set was shared across every scope, so moving from a narrow one
  // (a few rows) to a wider one (e.g. All) compared the wide list against
  // the narrow scope's small set and tagged everything the narrow scope
  // never had as "new" — even though nothing had actually arrived.
  it('does not tag every row as new just from switching to a wider scope', async () => {
    items = [conv('a', { unread: true }), conv('b'), conv('c'), conv('d'), conv('e')]
    renderAt('/conversations')
    // Narrow the view first (Unread: only 'a' qualifies), establishing a
    // small "seen" set for that scope.
    await userEvent.click(screen.getByTestId('scope-Unread'))
    expect(screen.getByTestId('row-a')).toBeInTheDocument()
    expect(screen.queryByTestId('row-b')).not.toBeInTheDocument()
    // Now open the wide scope. None of b/c/d/e actually just arrived — they
    // existed the whole time, just outside the narrow scope's own set.
    await userEvent.click(screen.getByTestId('scope-All'))
    expect(screen.getByTestId('row-b')).toBeInTheDocument()
    // The real claim: nothing is tagged "new" just from the scope widening.
    expect(screen.queryByText('new')).not.toBeInTheDocument()
  })
})

describe('tree-shaped scopes keep the whole tree (Mine, Unread, Working, Errored)', () => {
  // Each of these used to be an independent per-row filter: a member is
  // never literally "mine" (it was invoked, never originated), never
  // literally "errored" just because its root is, and so on — so opening
  // one of these scopes on a root whose MEMBER is what actually qualifies
  // (or vice versa) used to show only the one row that matched, with
  // nothing for the tree to attach to.
  it('Mine: a root the reader started keeps its member, even though the member itself is never "mine"', async () => {
    items = [
      conv('root-1', { coordinator: 'root-1', mine: true }),
      conv('member-1', { causedBy: { parent: 'root-1', entry: 'diagnose' }, mine: false }),
    ]
    renderAt('/conversations')
    await userEvent.click(screen.getByTestId('scope-Mine'))
    // A root with members starts folded (item 2's first-time default) — this
    // test is about the tree-scope keeping the member in the result at all,
    // not about the fold, so expand it to see it.
    await userEvent.click(screen.getByText('Expand all'))
    expect(screen.getByTestId('row-root-1')).toBeInTheDocument()
    expect(screen.getByTestId('row-member-1')).toBeInTheDocument()
  })

  it('Unread: a root with nothing new of its own still shows, because its member has an unread message', async () => {
    items = [
      conv('root-2', { coordinator: 'root-2', unread: false }),
      conv('member-2', { causedBy: { parent: 'root-2', entry: 'diagnose' }, unread: true, unreadCount: 1 }),
    ]
    renderAt('/conversations')
    await userEvent.click(screen.getByTestId('scope-Unread'))
    // Same fold default as above — not what this test is checking.
    await userEvent.click(screen.getByText('Expand all'))
    expect(screen.getByTestId('row-root-2')).toBeInTheDocument()
    expect(screen.getByTestId('row-member-2')).toBeInTheDocument()
    // The root itself must still render as read — the tree qualifying is not
    // the same claim as every row in it being unread.
    expect(screen.queryByTestId('unread-root-2')).not.toBeInTheDocument()
    expect(screen.getByTestId('unread-member-2')).toBeInTheDocument()
  })

  it('a tree with no match anywhere is excluded entirely', async () => {
    items = [
      conv('root-3', { coordinator: 'root-3', mine: false, unread: false, errored: false }),
      conv('member-3', { causedBy: { parent: 'root-3', entry: 'diagnose' }, mine: false, unread: false, errored: false }),
      // A control row so "Mine" isn't vacuously the only scope with zero
      // rows for an unrelated reason (e.g. the scope itself failing to
      // render) — if this one disappeared too, the test would be broken in
      // a way that could hide a real regression.
      conv('root-4', { coordinator: 'root-4', mine: true }),
    ]
    renderAt('/conversations')
    await userEvent.click(screen.getByTestId('scope-Mine'))
    expect(screen.getByTestId('row-root-4')).toBeInTheDocument()
    expect(screen.queryByTestId('row-root-3')).not.toBeInTheDocument()
    expect(screen.queryByTestId('row-member-3')).not.toBeInTheDocument()
  })
})

describe('a Coordinator scope shows only its OWN root and members', () => {
  // A member carries no field naming which coordinator it ultimately
  // belongs to — only `causedBy.parent`, one hop to its immediate parent —
  // so with TWO coordinators in the snapshot, testing `causedBy` truthiness
  // alone (rather than walking to the uncaused root and checking THAT root's
  // own `coordinator` field) leaked every coordinator's members into every
  // other coordinator's scope.
  it('never shows another coordinator\'s own root or members', async () => {
    vocabularyEntries = [
      { kind: 'coordinator', name: 'root-1', position: 'general' },
      { kind: 'coordinator', name: 'root-2', position: 'general' },
    ]
    items = [
      conv('root-1', { coordinator: 'root-1' }),
      conv('member-1', { causedBy: { parent: 'root-1', entry: 'diagnose' } }),
      conv('root-2', { coordinator: 'root-2' }),
      conv('member-2', { causedBy: { parent: 'root-2', entry: 'diagnose' } }),
    ]
    renderAt('/conversations')
    await userEvent.click(screen.getByTestId('scope-root-1'))
    await userEvent.click(screen.getByText('Expand all'))
    expect(screen.getByTestId('row-root-1')).toBeInTheDocument()
    expect(screen.getByTestId('row-member-1')).toBeInTheDocument()
    expect(screen.queryByTestId('row-root-2')).not.toBeInTheDocument()
    expect(screen.queryByTestId('row-member-2')).not.toBeInTheDocument()
  })
})

describe('expand all / collapse all (item 2)', () => {
  it('is absent with nothing collapsible to toggle', () => {
    renderAt('/conversations')
    expect(screen.queryByText('Expand all')).not.toBeInTheDocument()
    expect(screen.queryByText('Collapse all')).not.toBeInTheDocument()
  })

  it('starts folded for a first-time viewer (item 2), and expands/collapses from there', async () => {
    const original = items
    items = [
      conv('root-1', { coordinator: 'root-1' }),
      conv('member-1', { causedBy: { parent: 'root-1', entry: 'diagnose' } }),
    ]
    renderAt('/conversations')
    expect(screen.queryByTestId('row-member-1')).not.toBeInTheDocument()
    await userEvent.click(screen.getByText('Expand all'))
    expect(screen.getByTestId('row-member-1')).toBeInTheDocument()
    await userEvent.click(screen.getByText('Collapse all'))
    expect(screen.queryByTestId('row-member-1')).not.toBeInTheDocument()
    items = original
  })

  it('remembers "Expand all" across a reload, for a root that arrives afterward too', async () => {
    const original = items
    items = [
      conv('root-1', { coordinator: 'root-1' }),
      conv('member-1', { causedBy: { parent: 'root-1', entry: 'diagnose' } }),
    ]
    const { unmount } = renderAt('/conversations')
    await userEvent.click(screen.getByText('Expand all'))
    expect(screen.getByTestId('row-member-1')).toBeInTheDocument()
    unmount()

    // A second root arriving after the reload — never expanded by hand —
    // still follows the remembered preference, not the first-time default.
    items = [
      conv('root-1', { coordinator: 'root-1' }),
      conv('member-1', { causedBy: { parent: 'root-1', entry: 'diagnose' } }),
      conv('root-2', { coordinator: 'root-2' }),
      conv('member-2', { causedBy: { parent: 'root-2', entry: 'diagnose' } }),
    ]
    renderAt('/conversations')
    expect(screen.getByTestId('row-member-1')).toBeInTheDocument()
    expect(screen.getByTestId('row-member-2')).toBeInTheDocument()
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

  it('remembers the choice across a reload', async () => {
    const original = items
    items = [conv('a'), conv('b', { phase: 'Closed' })]
    const { unmount } = renderAt('/conversations')
    await userEvent.click(screen.getByLabelText('Show closed'))
    expect(screen.getByTestId('row-b')).toBeInTheDocument()
    unmount()

    renderAt('/conversations')
    expect(screen.getByLabelText('Show closed')).toBeChecked()
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
