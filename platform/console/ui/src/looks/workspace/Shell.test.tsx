import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import type { Session } from '../../api/types'
import { TESTID, testid } from '../../model/testids'
import { LookContext } from '../shared/LookContext'
import { DEFAULT_WORKSPACE_LAYOUT, useWorkspaceLayout } from './layout'
import { Shell } from './Shell'

// The shell over a mocked model: the header's controls, the navigation's
// counts folded and unfolded, and the sign-in gate.

const SESSION: Session = {
  authenticated: true, configured: true, identity: 'dana@example.com', identitySource: 'forward-auth',
  authMode: 'token', externalAuthenticator: '', writeEnabled: true, canWrite: true, canOriginate: true, metrics: false,
}
let session: { data?: Session; isLoading: boolean } = { data: SESSION, isLoading: false }
let inFlight = 0
let connection: 'connected' | 'disconnected' = 'connected'

vi.mock('../../model/session', () => ({
  GENERAL_FEED_NAME: 'channel:general',
  useConsoleSession: () => ({ ...session, refetch: vi.fn() }),
  useLiveStream: () => connection === 'connected',
  signIn: vi.fn(async () => true),
}))
vi.mock('../../model/actions', async (importOriginal) => {
  const idle = { mutate: () => undefined, isPending: false, reset: () => undefined, data: undefined, error: null }
  return {
    ...(await importOriginal<typeof import('../../model/actions')>()),
    useReopen: () => idle, useMarkRead: () => idle, useMarkUnread: () => idle,
    useCloseBatch: () => idle, useDeleteBatch: () => idle, useVocabulary: () => ({ data: { entries: [] } }),
  }
})
vi.mock('../../model/newConversation', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../model/newConversation')>()),
  useNewConversation: () => ({
    availability: 'ready', externalAuthenticator: '', wired: [], unwired: [], ambiguous: false, vocabularyLoading: false,
    startable: [], busy: false, start: async () => ({ ok: true }),
  }),
}))
vi.mock('../../model/commands', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../model/commands')>()),
  useCommands: () => ({ inFlight, connection, sending: () => false }),
}))
vi.mock('../../model/conversationList', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../model/conversationList')>()),
  useConversationList: () => ({ data: { items: [], total: 0 }, isLoading: false, error: null, items: [], tree: [], rows: [], collapsibleNames: [] }),
  useArrivals: () => ({ newNames: new Set(), clear: () => undefined }),
  useInboxScopes: () => ({
    general: { scope: { kind: 'general' }, label: 'General feed' },
    fixed: [
      { scope: { kind: 'all' }, label: 'All', count: 7 },
      { scope: { kind: 'working' }, label: 'Working', count: 2 },
      { scope: { kind: 'mine' }, label: 'Mine', count: 0 },
      { scope: { kind: 'errored' }, label: 'Errored', count: 1 },
    ],
    targets: [{ scope: { kind: 'pipeline', name: 'k8s-observe' }, label: 'k8s-observe', count: 3 }],
    closed: { scope: { kind: 'closed' }, label: 'Archive', count: 4 },
  }),
}))

const LOOKS = [
  { id: 'classic', label: 'Classic', load: () => Promise.reject(new Error('unused')) },
  { id: 'workspace', label: 'Workspace', load: () => Promise.reject(new Error('unused')) },
]

const mount = (path = '/overview') =>
  render(
    <LookContext.Provider value={{ active: 'workspace', looks: LOOKS }}>
      <MemoryRouter initialEntries={[path]}>
        <Shell />
      </MemoryRouter>
    </LookContext.Provider>,
  )

beforeEach(() => {
  // A wide window: the navigation is a pane, not a drawer.
  Object.defineProperty(window, 'innerWidth', { configurable: true, value: 1400 })
  session = { data: SESSION, isLoading: false }
  inFlight = 0
  connection = 'connected'
  localStorage.clear()
  useWorkspaceLayout.setState(DEFAULT_WORKSPACE_LAYOUT)
})

describe('the workspace header', () => {
  it('carries the look switcher, the colour scheme, the connection and who the viewer is', () => {
    mount()
    const header = screen.getByRole('banner')
    expect(within(header).getByTestId(TESTID.lookSwitcher)).toHaveAttribute('data-active', 'workspace')
    expect(within(header).getByRole('combobox', { name: 'Look' })).toHaveValue('workspace')
    expect(within(header).getByTestId(TESTID.themeSwitcher)).toBeInTheDocument()
    for (const s of ['light', 'dark', 'system']) expect(within(header).getByTestId(testid.theme(s))).toBeInTheDocument()
    expect(within(header).getByTestId(TESTID.connection)).toHaveAttribute('data-state', 'connected')
    expect(within(header).getByText('dana@example.com')).toBeInTheDocument()
  })

  it('shows no sign-out control', () => {
    mount()
    expect(screen.queryByRole('button', { name: /sign out/i })).toBeNull()
  })

  it('counts work in flight, and shows nothing while there is none', () => {
    const { unmount } = mount()
    expect(screen.queryByTestId('in-flight')).toBeNull()
    unmount()
    inFlight = 3
    mount()
    expect(screen.getByTestId('in-flight')).toHaveTextContent('3 in flight')
  })

  it('states a lost connection', () => {
    connection = 'disconnected'
    mount()
    expect(screen.getByTestId(TESTID.connection)).toHaveAttribute('data-state', 'disconnected')
    expect(screen.getByTestId(TESTID.connection)).toHaveTextContent('reconnecting')
  })

  it('switches the colour scheme from its own control', async () => {
    mount()
    await userEvent.click(screen.getByTestId(testid.theme('dark')))
    expect(screen.getByTestId(testid.theme('dark'))).toHaveAttribute('aria-pressed', 'true')
    expect(document.documentElement).toHaveAttribute('data-theme', 'dark')
    await userEvent.click(screen.getByTestId(testid.theme('system')))
  })

  it('offers New conversation only where this console may start one', () => {
    const { unmount } = mount()
    expect(screen.getByTestId(TESTID.newConversation)).toBeInTheDocument()
    unmount()
    session = { data: { ...SESSION, canOriginate: false }, isLoading: false }
    mount()
    expect(screen.queryByTestId(TESTID.newConversation)).toBeNull()
  })
})

describe('the navigation panel', () => {
  it('carries the pages, the channel, the scopes with their counts, the pipelines and the archive', () => {
    mount()
    const nav = screen.getByTestId(TESTID.appNav)
    for (const page of ['Overview', 'Queues', 'Configuration', 'Topology']) expect(within(nav).getByRole('link', { name: page })).toBeInTheDocument()
    expect(within(nav).getByRole('link', { name: 'Overview' })).toHaveAttribute('aria-current', 'page')
    expect(within(nav).getByTestId(testid.scope('General feed'))).toBeInTheDocument()
    expect(within(nav).getByTestId(testid.scope('All'))).toHaveTextContent('7')
    expect(within(nav).getByTestId(testid.scope('k8s-observe'))).toHaveTextContent('3')
    expect(within(nav).getByTestId(testid.scope('Archive'))).toHaveTextContent('4')
  })

  it('folds to icons and keeps every count as a badge, and the fold is remembered', async () => {
    mount()
    await userEvent.click(screen.getByRole('button', { name: 'Collapse the navigation' }))
    const nav = screen.getByTestId(TESTID.appNav)
    expect(nav).toHaveClass('collapsed')
    expect(within(nav).queryByText('Pages')).toBeNull()
    expect(within(nav).getByTestId(testid.scope('All'))).toHaveTextContent('7')
    expect(within(nav).getByTestId(testid.scope('Working'))).toHaveAccessibleName('Working, 2')
    expect(within(nav).getByTestId(testid.scope('k8s-observe'))).toHaveTextContent('3')
    expect(useWorkspaceLayout.getState().navCollapsed).toBe(true)
  })

  it('picks a scope and opens the conversations page', async () => {
    mount()
    await userEvent.click(screen.getByTestId(testid.scope('Working')))
    expect(screen.getByTestId(testid.scope('Working'))).toHaveAttribute('aria-pressed', 'true')
  })

  it('is put away from the header and announced as such', async () => {
    mount()
    await userEvent.click(screen.getByRole('button', { name: 'Toggle navigation' }))
    expect(screen.getByTestId(TESTID.appNav)).toHaveAttribute('aria-hidden', 'true')
    expect(screen.getByRole('button', { name: 'Toggle navigation' })).toHaveAttribute('aria-expanded', 'false')
    expect(useWorkspaceLayout.getState().navHidden).toBe(true)
  })
})

describe('the sign-in gate', () => {
  it('asks for a token where the console has a session to start', () => {
    session = { data: { ...SESSION, authenticated: false, configured: false }, isLoading: false }
    mount()
    expect(screen.getByLabelText('Token')).toBeInTheDocument()
    expect(screen.getByRole('note')).toHaveTextContent('Nobody can sign in')
    expect(screen.queryByRole('banner')).toBeNull()
  })

  it('draws nothing while the session loads', () => {
    session = { isLoading: true }
    const { container } = mount()
    expect(container.querySelector('.wk')?.childElementCount).toBe(0)
  })
})

describe('at tablet width', () => {
  it('makes the navigation a drawer that opens from the header and closes on Escape', async () => {
    Object.defineProperty(window, 'innerWidth', { configurable: true, value: 1024 })
    mount('/conversations')
    const nav = screen.getByTestId(TESTID.appNav)
    expect(nav).toHaveAttribute('aria-hidden', 'true')
    await userEvent.click(screen.getByRole('button', { name: 'Toggle navigation' }))
    expect(nav).not.toHaveAttribute('aria-hidden')
    expect(screen.getByRole('button', { name: 'Close the navigation' })).toBeInTheDocument()
    // Never folded as a drawer, and its first control takes focus.
    expect(nav).not.toHaveClass('collapsed')
    expect(within(nav).getByRole('link', { name: 'Overview' })).toHaveFocus()
    await userEvent.keyboard('{Escape}')
    expect(nav).toHaveAttribute('aria-hidden', 'true')
  })
})
