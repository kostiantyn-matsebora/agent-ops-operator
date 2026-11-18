import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom'
import type { ConversationSummary } from '../../../api/types'
import { TESTID, testid } from '../../../model/testids'
import { buildTree } from '../../../model/tree'
import { DEFAULT_WORKSPACE_LAYOUT, useWorkspaceLayout } from '../layout'
import { ConversationsPage } from './Conversations'

// The conversations page over a mocked model: a link lands in the workspace
// with the list beside the thread, and at phone width one pane shows at a time.

const conv = (name: string, over: Partial<ConversationSummary> = {}): ConversationSummary => ({
  name, title: `title of ${name}`, phase: 'Idle', runCount: 1, queued: 0, joined: true, errored: false,
  unread: false, ageSeconds: 0, deleting: false, created: '2025-06-11T09:00:00Z', ...over,
})
const ITEMS = [conv('c1'), conv('c2', { presence: true, phase: 'Working' })]
const sent: [string, string][] = []
const latestCommand: Record<string, unknown> = {}
const retried: number[] = []
const dismissed: number[] = []
const calls: [string, unknown][] = []
const mutation = (kind: string) => ({ mutate: (arg: unknown, opts?: { onSuccess?: () => void }) => { calls.push([kind, arg]); opts?.onSuccess?.() }, isPending: false, reset: () => undefined, data: undefined, error: null })

vi.mock('../../../model/conversationList', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../../model/conversationList')>()),
  useConversationList: () => {
    const tree = buildTree(ITEMS, true)
    return { data: { items: ITEMS, total: 2 }, isLoading: false, error: null, items: ITEMS, tree, rows: tree, collapsibleNames: [] }
  },
  useArrivals: () => ({ newNames: new Set(), clear: () => undefined }),
  useInboxScopes: () => ({ general: { scope: { kind: 'general' }, label: 'General feed' }, fixed: [], targets: [], closed: { scope: { kind: 'closed' }, label: 'Archive' } }),
}))
vi.mock('../../../model/session', () => ({
  GENERAL_FEED_NAME: 'channel:general',
  useConsoleSession: () => ({ data: { canWrite: true } }),
  useGeneralFeed: () => ({ data: { channel: 'console', messages: [] }, isLoading: false }),
}))
vi.mock('../../../model/actions', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../../model/actions')>()),
  useReopen: () => mutation('reopen'),
  useMarkRead: () => mutation('markRead'),
  useMarkUnread: () => mutation('markUnread'),
  useCloseBatch: () => mutation('close'),
  useDeleteBatch: () => mutation('delete'),
  useVocabulary: () => ({ data: { entries: [] } }),
}))
vi.mock('../../../model/commands', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../../model/commands')>()),
  useCommands: () => ({ sending: () => false, latest: (name: string) => latestCommand[name], retry: (id: number) => retried.push(id), dismiss: (id: number) => dismissed.push(id), connection: 'connected' }),
}))
vi.mock('../../../model/pipelineIcon', () => ({
  usePipelineIconLookup: () => () => undefined,
}))
vi.mock('../../../model/verbs', () => ({
  sendText: (name: string, text: string) => sent.push([name, text]),
  exitRuntime: (name: string) => sent.push([name, '/exit']),
}))
vi.mock('../../../model/thread', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../../model/thread')>()),
  useThread: (name: string) => {
    const c = ITEMS.find((i) => i.name === name)
    return {
      isLoading: false, error: null, conversation: c, title: c?.title ?? '', isMember: false, isRoot: false,
      messages: [{ id: 'm', thread: 't', kind: 'agent', text: `the answer in ${name}`, at: '2025-06-11T09:01:00Z', source: '' }],
      runs: [], runDetails: new Map(), startedAt: c?.created, canWrite: true, detail: undefined,
    }
  },
  useThreadMembers: () => ({ members: [] }),
}))

function Where() {
  return <span data-testid="where">{useLocation().pathname}</span>
}

const mount = (path: string) =>
  render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route path="/conversations" element={<ConversationsPage />} />
        <Route path="/conversations/:name" element={<ConversationsPage />} />
      </Routes>
      <Where />
    </MemoryRouter>,
  )

beforeEach(() => {
  sent.length = 0
  calls.length = 0
  retried.length = 0
  dismissed.length = 0
  for (const k of Object.keys(latestCommand)) delete latestCommand[k]
  localStorage.clear()
  useWorkspaceLayout.setState(DEFAULT_WORKSPACE_LAYOUT)
  Object.defineProperty(window, 'innerWidth', { configurable: true, value: 1400 })
})

describe('the conversations page', () => {
  it('lands a link on the thread, with the list beside it and the row marked', () => {
    mount('/conversations/c2')
    const view = screen.getByTestId(TESTID.chatView)
    expect(view).toHaveAttribute('data-view', 'thread')
    expect(within(view).getByRole('heading', { name: 'title of c2' })).toBeInTheDocument()
    expect(screen.getByText('the answer in c2')).toBeInTheDocument()
    expect(screen.getByTestId(testid.row('c1'))).toBeInTheDocument()
    expect(screen.getByTestId(testid.row('c2'))).toHaveAttribute('aria-current', 'true')
  })

  it('opens another conversation in place, never leaving the view', async () => {
    mount('/conversations/c2')
    await userEvent.click(screen.getByTestId(testid.open('c1')))
    expect(screen.getByTestId('where')).toHaveTextContent('/conversations/c1')
    expect(screen.getByText('the answer in c1')).toBeInTheDocument()
    expect(screen.getByTestId(testid.row('c2'))).toBeInTheDocument()
  })

  it('folds the list to icons, and remembers it', async () => {
    mount('/conversations/c1')
    await userEvent.click(screen.getByRole('button', { name: 'Collapse the conversation list' }))
    expect(screen.getByTestId(TESTID.inboxCollapsed)).toHaveClass('collapsed')
    expect(useWorkspaceLayout.getState().listCollapsed).toBe(true)
  })

  it('resizes the list from the keyboard, and remembers the width', async () => {
    mount('/conversations')
    const splitter = screen.getByRole('separator', { name: 'Resize the conversation list' })
    splitter.focus()
    await userEvent.keyboard('{ArrowRight}{ArrowRight}')
    expect(useWorkspaceLayout.getState().listWidth).toBe(DEFAULT_WORKSPACE_LAYOUT.listWidth + 32)
    await userEvent.keyboard('{Home}')
    expect(useWorkspaceLayout.getState().listWidth).toBe(DEFAULT_WORKSPACE_LAYOUT.listWidth)
  })

  it('offers a row\u2019s actions, and Close hands the row to the confirmed batch flow', async () => {
    mount('/conversations')
    await userEvent.click(within(screen.getByTestId(testid.row('c1'))).getByRole('button', { name: /Actions for/ }))
    await userEvent.click(screen.getByRole('menuitem', { name: 'Close' }))
    // Nothing is sent: the row is selected and the bar's own dialog confirms.
    expect(sent).toEqual([])
    expect(screen.getByTestId(TESTID.selectionBar)).toHaveTextContent('1 selected')
    expect(screen.getByTestId(testid.select('c1'))).toBeChecked()
    await userEvent.click(screen.getByTestId(TESTID.closeSelected))
    await userEvent.click(screen.getByTestId('close-confirm'))
    expect(calls).toEqual([['close', { names: ['c1'], includeWorking: false }]])
  })

  it('offers Reopen and Delete on a closed conversation, and neither on an open one', async () => {
    ITEMS.push(conv('c3', { phase: 'Closed' }))
    try {
      mount('/conversations')
      const menu = async (name: string) => {
        await userEvent.click(within(screen.getByTestId(testid.row(name))).getByRole('button', { name: /Actions for/ }))
        return screen.getAllByRole('menuitem').map((m) => m.textContent)
      }
      expect(await menu('c3')).toEqual(expect.arrayContaining(['Reopen', 'Delete']))
      await userEvent.click(screen.getByRole('menuitem', { name: 'Reopen' }))
      expect(calls).toEqual([['reopen', 'c3']])
      expect(await menu('c1')).not.toEqual(expect.arrayContaining(['Reopen']))
    } finally {
      ITEMS.pop()
    }
  })

  it('selects rows without opening them, and offers the bulk actions on the selection', async () => {
    mount('/conversations')
    await userEvent.click(screen.getByRole('button', { name: 'Select' }))
    await userEvent.click(screen.getByTestId(testid.open('c1')))
    await userEvent.click(screen.getByTestId(testid.open('c2')))
    expect(screen.getByTestId('where')).toHaveTextContent(/^\/conversations$/)
    expect(screen.getByTestId(TESTID.selectionBar)).toHaveTextContent('2 selected')
    // A working conversation is only closed once the viewer says so.
    await userEvent.click(screen.getByTestId(TESTID.closeSelected))
    expect(screen.getByTestId('close-working')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('checkbox', { name: /Also close 1 working/ }))
    await userEvent.click(screen.getByTestId('close-confirm'))
    expect(calls).toEqual([['close', { names: ['c1', 'c2'], includeWorking: true }]])
    // The batch ends the mode.
    expect(screen.queryByTestId(TESTID.selectionBar)).toBeNull()
  })

  it('selects every row at once, and clears them again', async () => {
    mount('/conversations')
    expect(screen.queryByRole('button', { name: 'Select all' })).toBeNull()
    await userEvent.click(screen.getByRole('button', { name: 'Select' }))
    await userEvent.click(screen.getByRole('button', { name: 'Select all' }))
    expect(screen.getByTestId(TESTID.selectionBar)).toHaveTextContent(`${ITEMS.length} selected`)
    await userEvent.click(screen.getByRole('button', { name: 'Clear all' }))
    expect(screen.getByTestId(TESTID.selectionBar)).toHaveTextContent('0 selected')
  })

  it('starts a selection from a modifier-click, without opening the row', async () => {
    mount('/conversations')
    fireEvent.click(screen.getByTestId(testid.open('c1')), { ctrlKey: true })
    expect(screen.getByTestId('where')).toHaveTextContent(/^\/conversations$/)
    expect(screen.getByTestId(TESTID.selectionBar)).toHaveTextContent('1 selected')
  })

  it('marks a selection read, and offers delete only when every selected row is closed', async () => {
    mount('/conversations')
    await userEvent.click(screen.getByRole('button', { name: 'Select' }))
    await userEvent.click(screen.getByTestId(testid.open('c1')))
    await userEvent.click(screen.getByTestId(TESTID.markRead))
    expect(calls).toEqual([['markRead', { names: ['c1'] }]])
    expect(screen.queryByTestId(TESTID.selectionBar)).toBeNull()
    await userEvent.click(screen.getByRole('button', { name: 'Select' }))
    await userEvent.click(screen.getByTestId(testid.open('c1')))
    expect(screen.getByTestId(TESTID.deleteSelected)).toBeDisabled()
  })

  it('says a close is in progress on its row, and not on the others', () => {
    latestCommand.c1 = { id: 1, target: 'c1', verb: 'close', state: 'accepted', retryable: false, startedAt: 0 }
    mount('/conversations')
    expect(screen.getByTestId('command-c1')).toHaveTextContent('closing…')
    expect(screen.queryByTestId('command-c2')).toBeNull()
  })

  it('says why a close failed, and offers retry and dismiss', async () => {
    latestCommand.c1 = { id: 7, target: 'c1', verb: 'close', state: 'failed', error: 'manager unreachable', retryable: true, startedAt: 0 }
    mount('/conversations')
    const note = screen.getByTestId('command-c1')
    expect(note).toHaveTextContent('Not closed')
    expect(note).toHaveAttribute('title', 'manager unreachable')
    await userEvent.click(within(note).getByRole('button', { name: 'Retry' }))
    await userEvent.click(within(note).getByRole('button', { name: /Dismiss/ }))
    expect(retried).toEqual([7])
    expect(dismissed).toEqual([7])
  })

  it('offers no retry for a failure that is unsafe to repeat', () => {
    latestCommand.c1 = { id: 8, target: 'c1', verb: 'start', state: 'failed', error: 'x', retryable: false, startedAt: 0 }
    mount('/conversations')
    expect(within(screen.getByTestId('command-c1')).queryByRole('button', { name: 'Retry' })).toBeNull()
  })

  it('moves between rows with the arrow keys and ends a selection with Escape', async () => {
    mount('/conversations')
    screen.getByTestId(testid.open('c1')).focus()
    await userEvent.keyboard('{ArrowDown}')
    expect(screen.getByTestId(testid.open('c2'))).toHaveFocus()
    await userEvent.keyboard('{ArrowUp}')
    expect(screen.getByTestId(testid.open('c1'))).toHaveFocus()
    await userEvent.click(screen.getByRole('button', { name: 'Select' }))
    screen.getByTestId(testid.open('c1')).focus()
    await userEvent.keyboard('{Escape}')
    expect(screen.queryByTestId(TESTID.selectionBar)).toBeNull()
  })

  it('remembers the show-closed choice', async () => {
    mount('/conversations')
    await userEvent.click(screen.getByTestId(TESTID.showClosed))
    expect(useWorkspaceLayout.getState().showClosed).toBe(true)
  })

  it('asks for a conversation where none is open', () => {
    mount('/conversations')
    expect(screen.getByText('Select a conversation')).toBeInTheDocument()
  })

  it('opens the general feed in the thread pane', () => {
    mount('/conversations/channel:general')
    expect(screen.getByRole('heading', { name: 'General feed' })).toBeInTheDocument()
    expect(screen.getByText('No messages on the console thread yet')).toBeInTheDocument()
  })
})

describe('at phone width', () => {
  beforeEach(() => Object.defineProperty(window, 'innerWidth', { configurable: true, value: 390 }))

  it('shows one pane at a time, the thread with a way back to the list', async () => {
    mount('/conversations/c1')
    expect(screen.getByTestId(TESTID.chatView)).toHaveAttribute('data-view', 'thread')
    await userEvent.click(screen.getByRole('button', { name: 'Back to the list' }))
    expect(screen.getByTestId('where')).toHaveTextContent(/^\/conversations$/)
    expect(screen.getByTestId(TESTID.chatView)).toHaveAttribute('data-view', 'list')
  })

  it('never folds the list, where it is the only pane', () => {
    useWorkspaceLayout.setState({ listCollapsed: true })
    mount('/conversations')
    expect(screen.getByTestId(TESTID.inbox)).not.toHaveClass('collapsed')
  })
})
