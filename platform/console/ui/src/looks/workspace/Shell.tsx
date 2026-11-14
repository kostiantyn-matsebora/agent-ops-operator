import { Component, type CSSProperties, type ReactElement, type ReactNode, useEffect, useRef, useState } from 'react'
import { Navigate, Route, Routes, useLocation } from 'react-router-dom'
import type { Session } from '../../api/types'
import { useCommands } from '../../model/commands'
import { useConsoleSession, useLiveStream } from '../../model/session'
import { TESTID } from '../../model/testids'
import { ConversationsPage } from './conversations/Conversations'
import { Header } from './Header'
import {
  DEFAULT_NAV_WIDTH, MIN_NAV_WIDTH, NAV_COLLAPSED_WIDTH, useArrangement, useWorkspaceLayout,
} from './layout'
import { NavPanel } from './NavPanel'
import { LoginPage } from './pages/Login'
import {
  ConfigDetailPage, ConfigKindPage, ConfigPage, NewConversationDialog, OverviewPage, QueuesPage, TopologyPage,
} from './pages/Placeholders'
import { Splitter } from './Splitter'
import './workspace.css'

// THE WORKSPACE SHELL: the header, the navigation panel and the routes, filling
// the whole window (console-workspace-look: "The workspace fills the
// viewport"). No width cap: the panes reach both edges less the side gutter,
// stretch to the window's height, and each scrolls inside itself — the page
// never does.
//
// Wide, the navigation is a pane that folds to icons or is put away from the
// header's menu button. At tablet width it is a drawer over the page. At phone
// width the conversations page shows one pane at a time.

/** One broken route degrades to one broken panel, never a blank console. */
class Boundary extends Component<{ children: ReactNode }, { error?: Error }> {
  state: { error?: Error } = {}

  static getDerivedStateFromError(error: Error) {
    return { error }
  }

  render() {
    if (this.state.error) {
      return (
        <main className="wk-pane wk-page">
          <div className="wk-alert bad" role="alert">
            <b>This view failed to render.</b> {this.state.error.message}{' '}
            <button type="button" className="wk-btn link" onClick={() => this.setState({ error: undefined })}>
              Try again
            </button>
          </div>
        </main>
      )
    }
    return this.props.children
  }
}

function Workspace({ session }: Readonly<{ session: Session }>): ReactElement {
  const commands = useCommands()
  const arrangement = useArrangement()
  const layout = useWorkspaceLayout()
  const { pathname } = useLocation()
  const [drawerOpen, setDrawerOpen] = useState(false)
  const [newOpen, setNewOpen] = useState(false)
  const drawer = arrangement !== 'full'
  const navRef = useRef<HTMLElement>(null)

  // A drawer belongs to the moment: navigating, or widening the window, puts it away.
  useEffect(() => setDrawerOpen(false), [pathname, drawer])
  useEffect(() => {
    if (!drawerOpen) return
    navRef.current?.querySelector<HTMLElement>('a, button')?.focus()
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && setDrawerOpen(false)
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [drawerOpen])

  const navShown = drawer ? drawerOpen : !layout.navHidden
  const navWidth = layout.navCollapsed ? NAV_COLLAPSED_WIDTH : layout.navWidth
  const style = { '--wk-navw': `${navWidth}px` } as CSSProperties

  return (
    <>
      <Header
        session={session}
        connection={commands.connection}
        inFlight={commands.inFlight}
        navOpen={navShown}
        compact={arrangement === 'single'}
        onMenu={() => (drawer ? setDrawerOpen((v) => !v) : layout.update({ navHidden: !layout.navHidden }))}
        onNew={session.canOriginate ? () => setNewOpen(true) : undefined}
      />
      <div
        className="wk-cols"
        data-nav={!drawer && layout.navHidden ? 'hidden' : 'shown'}
        data-drawer={drawer && drawerOpen ? 'open' : 'closed'}
        style={style}
      >
        <nav
          id="wk-nav"
          ref={navRef}
          className={`wk-pane wk-side${layout.navCollapsed && !drawer ? ' collapsed' : ''}`}
          aria-label="Console"
          data-testid={TESTID.appNav}
          aria-hidden={navShown ? undefined : true}
          {...(navShown ? {} : { inert: '' })}
        >
          <NavPanel
            collapsed={layout.navCollapsed}
            drawer={drawer}
            onToggleCollapsed={() => layout.update({ navCollapsed: !layout.navCollapsed })}
            onPicked={() => setDrawerOpen(false)}
          />
        </nav>
        <Splitter
          label="Resize the navigation"
          width={layout.navWidth}
          min={MIN_NAV_WIDTH}
          defaultWidth={DEFAULT_NAV_WIDTH}
          disabled={layout.navCollapsed || layout.navHidden}
          onChange={(w) => layout.update({ navWidth: w })}
        />
        <div className="wk-main">
          <Boundary>
            <Routes>
              <Route path="/" element={<Navigate to="/conversations" replace />} />
              <Route path="/overview" element={<OverviewPage />} />
              <Route path="/queues" element={<QueuesPage />} />
              <Route path="/config" element={<ConfigPage />} />
              <Route path="/config/:kind" element={<ConfigKindPage />} />
              <Route path="/config/:kind/:name" element={<ConfigDetailPage />} />
              <Route path="/topology" element={<TopologyPage />} />
              <Route path="/conversations" element={<ConversationsPage />} />
              <Route path="/conversations/:name" element={<ConversationsPage />} />
              <Route
                path="*"
                element={
                  <main className="wk-pane wk-page">
                    <div className="wk-empty">
                      <h2>Not found</h2>
                      That page does not exist.
                    </div>
                  </main>
                }
              />
            </Routes>
          </Boundary>
        </div>
      </div>
      {drawer && drawerOpen && (
        <button type="button" className="wk-scrim" aria-label="Close the navigation" onClick={() => setDrawerOpen(false)} />
      )}
      <NewConversationDialog open={newOpen} onClose={() => setNewOpen(false)} />
    </>
  )
}

export function Shell(): ReactElement {
  const session = useConsoleSession()
  // The one live stream, opened for the shell's lifetime.
  useLiveStream()

  let content: ReactElement | null = null
  if (session.data?.authenticated) content = <Workspace session={session.data} />
  else if (!session.isLoading) {
    // A console that authenticates nobody itself reports every request as
    // authenticated, so a login form appears only where a token is asked for.
    content = <LoginPage configured={session.data?.configured ?? false} onLogin={() => void session.refetch()} />
  }
  return (
    <div className="wk" data-look-root="workspace">
      {content}
    </div>
  )
}
