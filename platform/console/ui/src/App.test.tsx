import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { App } from './App'
import { useShell } from './shell'

// The shell: the navigation folds to its icons and remembers it, so a wide
// view keeps the width across reloads.

vi.mock('./api/hooks', () => ({
  useSession: () => ({ isLoading: false, data: { authenticated: true, configured: true }, refetch: () => {} }),
  useLiveStream: () => true,
  useUnreadCount: () => ({ data: { unreadTotal: 3 } }),
}))
vi.mock('./api/client', () => ({ api: {} }))
vi.mock('./components/ThemeSwitcher', () => ({ ThemeSwitcher: () => null }))
vi.mock('./pages/NewConversation', () => ({ NewConversation: () => null }))
vi.mock('./pages/Overview', () => ({ OverviewPage: () => <div>overview page</div> }))
vi.mock('./pages/Queues', () => ({ QueuesPage: () => null }))
vi.mock('./pages/Config', () => ({ ConfigPage: () => null, ConfigDetailPage: () => null, ConfigKindPage: () => null }))
vi.mock('./pages/Topology', () => ({ TopologyPage: () => <div>topology page</div> }))
vi.mock('./pages/chat/ChatView', () => ({ ChatView: () => <div>chat view</div> }))
vi.mock('./pages/Login', () => ({ LoginPage: () => null }))

const shell = (path = '/topology') =>
  render(
    <MemoryRouter initialEntries={[path]}>
      <App />
    </MemoryRouter>,
  )

beforeEach(() => {
  localStorage.clear()
  useShell.setState({ navCollapsed: false })
  // jsdom never lays anything out, so `clientWidth` is always 0 — and
  // PatternFly's `Page` reads ITS OWN ref's `clientWidth` in a real browser
  // rather than `window.innerWidth` once it has mounted, to measure the
  // space actually available rather than the whole window. Left unstubbed,
  // `Page` would call this file's `onPageResize` with `mobileView: true` on
  // every mount regardless of `window.innerWidth`, closing the nav this
  // file's tests need open — this file is about the nav's CONTENT, not its
  // responsive behavior (ChatView.test.tsx covers a real narrow width).
  Object.defineProperty(HTMLElement.prototype, 'clientWidth', { configurable: true, value: 1440 })
  window.innerWidth = 1440
})

describe('the navigation', () => {
  it('names every view, with the unread count on Conversations', () => {
    shell()
    for (const label of ['Overview', 'Queues', 'Configuration', 'Topology', 'Conversations']) {
      expect(screen.getByRole('link', { name: new RegExp(`^${label}`) })).toHaveTextContent(label)
    }
    expect(screen.getByTestId('unread-badge')).toHaveTextContent('3')
  })

  it('folds to icons, keeps every link reachable by name, and remembers the fold', async () => {
    const { unmount } = shell()
    await userEvent.click(screen.getByLabelText('Collapse navigation'))
    expect(document.querySelector('.pf-v6-c-page')).toHaveClass('ao-nav-collapsed')
    const topology = screen.getByRole('link', { name: 'Topology' })
    expect(topology).not.toHaveTextContent('Topology')
    expect(topology).toHaveAttribute('title', 'Topology')
    // The masthead keeps the mark alone, since its brand column is as narrow as the strip.
    expect(screen.queryByText('agent-ops console')).not.toBeInTheDocument()
    expect(screen.getByTestId('unread-badge')).toBeInTheDocument()
    expect(localStorage.getItem('agentops-console-shell')).toContain('"navCollapsed":true')
    unmount()

    shell()
    expect(document.querySelector('.pf-v6-c-page')).toHaveClass('ao-nav-collapsed')
    await userEvent.click(screen.getByLabelText('Expand navigation'))
    expect(document.querySelector('.pf-v6-c-page')).not.toHaveClass('ao-nav-collapsed')
    expect(screen.getByRole('link', { name: 'Topology' })).toHaveTextContent('Topology')
  })
})

describe('a narrow width turns the nav into a closed drawer, never an overlay stuck open', () => {
  // Measured live at 375px: `onPageResize` unconditionally forced the nav
  // back OPEN on every resize, so it stayed `pf-m-expanded` over the content
  // at any narrow width and its links intercepted clicks meant for what was
  // underneath. PatternFly reports a real resize here as `mobileView: true`.
  it('starts closed and hidden from the accessibility tree at a narrow width', () => {
    Object.defineProperty(HTMLElement.prototype, 'clientWidth', { configurable: true, value: 375 })
    window.innerWidth = 375
    shell()
    expect(screen.queryByRole('link', { name: /^Overview/ })).not.toBeInTheDocument()
    expect(document.querySelector('.pf-v6-c-page__sidebar')).toHaveAttribute('aria-hidden', 'true')
    expect(screen.getByRole('button', { name: 'Conversations and other sections' })).toBeInTheDocument()
  })

  it('opens on request via the masthead toggle', async () => {
    Object.defineProperty(HTMLElement.prototype, 'clientWidth', { configurable: true, value: 375 })
    window.innerWidth = 375
    shell()
    await userEvent.click(screen.getByRole('button', { name: 'Conversations and other sections' }))
    expect(screen.getByRole('link', { name: /^Overview/ })).toBeInTheDocument()
  })
})
