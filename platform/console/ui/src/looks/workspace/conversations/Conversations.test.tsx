import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, within } from '@testing-library/react'
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
vi.mock('../../../model/commands', () => ({ useCommands: () => ({ sending: () => false }) }))
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

  it('offers a row\u2019s actions, and runs them through the model', async () => {
    mount('/conversations')
    await userEvent.click(within(screen.getByTestId(testid.row('c1'))).getByRole('button', { name: /Actions for/ }))
    await userEvent.click(screen.getByRole('menuitem', { name: 'Close' }))
    expect(sent).toEqual([['c1', '/close']])
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
