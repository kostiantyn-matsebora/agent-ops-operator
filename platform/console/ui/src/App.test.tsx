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
vi.mock('./pages/Conversations', () => ({ ConversationsPage: () => null }))
vi.mock('./pages/Conversation', () => ({ ConversationPage: () => null }))
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
