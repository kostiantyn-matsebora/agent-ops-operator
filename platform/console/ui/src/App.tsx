import { Component, type ReactNode, useState } from 'react'
import { NavLink, Navigate, Route, Routes, useLocation } from 'react-router-dom'
import {
  Alert, Badge, Button, Nav, NavItem, NavList, Masthead, MastheadBrand, MastheadContent,
  MastheadMain, Page, PageSidebar, PageSidebarBody, Label, Popover, Toolbar, ToolbarContent,
  ToolbarGroup, ToolbarItem, PageSection,
} from '@patternfly/react-core'
import {
  AngleDoubleLeftIcon, AngleDoubleRightIcon, CogIcon, CommentsIcon, ListIcon, TachometerAltIcon, TopologyIcon,
} from '@patternfly/react-icons'
import { useLiveStream, useSession, useUnreadCount } from './api/hooks'
import { api } from './api/client'
import { Logo } from './components/Logo'
import { Empty, Loading } from './components/States'
import { ThemeSwitcher } from './components/ThemeSwitcher'
import { useShell } from './shell'
import { NewConversation } from './pages/NewConversation'
import { LoginPage } from './pages/Login'
import { OverviewPage } from './pages/Overview'
import { QueuesPage } from './pages/Queues'
import { ConfigPage, ConfigDetailPage, ConfigKindPage } from './pages/Config'
import { TopologyPage } from './pages/Topology'
import { ConversationsPage } from './pages/Conversations'
import { ConversationPage } from './pages/Conversation'

/**
 * An error boundary per route. A large graph or a malformed object must degrade
 * to one broken panel, not a blank console — the operator is here BECAUSE
 * something is wrong, and this is the worst possible moment to show nothing.
 */
class Boundary extends Component<{ children: ReactNode }, { error?: Error }> {
  state: { error?: Error } = {}

  static getDerivedStateFromError(error: Error) {
    return { error }
  }

  render() {
    if (this.state.error) {
      return (
        <PageSection>
          <Alert variant="danger" isInline title="This view failed to render">
            {this.state.error.message}
            <div>
              <Button variant="link" isInline onClick={() => this.setState({ error: undefined })}>
                Try again
              </Button>
            </div>
          </Alert>
        </PageSection>
      )
    }
    return this.props.children
  }
}

const NAV = [
  { to: '/overview', label: 'Overview', icon: <TachometerAltIcon /> },
  { to: '/queues', label: 'Queues', icon: <ListIcon /> },
  { to: '/config', label: 'Configuration', icon: <CogIcon /> },
  { to: '/topology', label: 'Topology', icon: <TopologyIcon /> },
  { to: '/conversations', label: 'Conversations', icon: <CommentsIcon /> },
]

export function App() {
  const session = useSession()
  const connected = useLiveStream()
  // The badge is the count-only form of the list query: it is computed
  // server-side BEFORE any filter, so it says the same thing wherever you are
  // and whatever the list page happens to be showing.
  const unread = useUnreadCount()
  const location = useLocation()
  const [navOpen, setNavOpen] = useState(true)
  // Folded to its icons, and kept so across reloads: a wide view, the
  // topology first of all, is what the width is wanted for, and a choice made
  // for it should not need making again on every visit.
  const navCollapsed = useShell((s) => s.navCollapsed)
  const toggleNav = () => useShell.getState().setNavCollapsed(!navCollapsed)

  if (session.isLoading) return <Loading />
  // A console that authenticates nobody itself reports every request as
  // authenticated, so this branch is never taken there — which is the point: a
  // login form that accepts no token is a dead end that reads as a broken
  // install.
  if (!session.data?.authenticated) {
    return <LoginPage configured={session.data?.configured ?? false} onLogin={() => session.refetch()} />
  }

  const masthead = (
    <Masthead>
      <MastheadMain>
        <MastheadBrand>
          {/* Inline SVG, not <Brand src="">: an empty src renders the browser's
              broken-image glyph, which is exactly what the masthead was showing. */}
          <span style={{ display: 'inline-flex', alignItems: 'center', gap: 8 }}>
            <Logo />
            {/* Folded, the masthead's brand column is as narrow as the icon
                strip under it, so the name would spill under the toolbar.
                The mark alone stands for it then, as the icons stand for the
                links. */}
            {!navCollapsed && <span style={{ fontWeight: 600 }}>agent-ops console</span>}
          </span>
        </MastheadBrand>
      </MastheadMain>
      <MastheadContent>
        <Toolbar isStatic isFullHeight>
          <ToolbarContent>
            {/* Starting work is a GLOBAL action — you decide to ask an agent
                something while looking at whatever made you want to. Living in
                the masthead means it is one click from every page instead of
                only from the two that happened to render it. */}
            <ToolbarItem>
              <NewConversation />
            </ToolbarItem>

            <ToolbarGroup align={{ default: 'alignEnd' }}>
              <ToolbarItem>
                {/* A stream that is down and a system that is idle look
                    identical on a graph. Saying which is the difference between
                    a stale console and a wrong one. */}
                <Label status={connected ? 'success' : 'warning'}>
                  {connected ? 'live' : 'stream disconnected'}
                </Label>
              </ToolbarItem>
              {!session.data.writeEnabled && (
                <ToolbarItem>
                  <Label color="grey">read-only</Label>
                </ToolbarItem>
              )}
              <ToolbarItem>
                {/* WHO the console thinks you are. Under external
                    authentication an unresolved identity is not cosmetic — it
                    is the reason writes are refused — so it says "unknown" and
                    explains itself instead of quietly reading "token". */}
                {session.data.identity ? (
                  <Label color="blue">{session.data.identity}</Label>
                ) : (
                  <Popover
                    headerContent="Nobody said who you are"
                    bodyContent={
                      <>
                        {session.data.externalAuthenticator || 'A proxy'} authenticated this
                        request, but forwarded no identity header, so there is nothing to record a
                        write against. Reads work; writes are refused. Configure it to forward
                        X-Forwarded-Email or X-Auth-Request-User.
                      </>
                    }
                  >
                    <Label color="orange" isCompact>
                      unknown
                    </Label>
                  </Popover>
                )}
              </ToolbarItem>
              <ToolbarItem>
                <ThemeSwitcher />
              </ToolbarItem>
              {/* No sign-out where there is no session to end: with
                  authentication in front of the console, the button would clear
                  a cookie that authorized nothing and leave you signed in. */}
              {session.data.authMode !== 'external' && (
                <ToolbarItem>
                  <Button
                    variant="secondary"
                    onClick={async () => {
                      await api.logout()
                      session.refetch()
                    }}
                  >
                    Sign out
                  </Button>
                </ToolbarItem>
              )}
            </ToolbarGroup>
          </ToolbarContent>
        </Toolbar>
      </MastheadContent>
    </Masthead>
  )

  const sidebar = (
    <PageSidebar isSidebarOpen={navOpen}>
      <PageSidebarBody isFilled>
        <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
          <Nav aria-label="console">
            <NavList>
              {NAV.map((item) => (
                <NavItem key={item.to} isActive={location.pathname.startsWith(item.to)}>
                  <NavLink to={item.to} aria-label={item.label} title={navCollapsed ? item.label : undefined}>
                    <span className="pf-v6-c-nav__link-icon" aria-hidden="true">{item.icon}</span>
                    {!navCollapsed && <span className="pf-v6-c-nav__link-text">{item.label}</span>}
                    {item.to === '/conversations' && (unread.data?.unreadTotal ?? 0) > 0 && (
                      <Badge isRead={false} data-testid="unread-badge" style={{ marginLeft: navCollapsed ? 4 : 8 }}>
                        {unread.data?.unreadTotal}
                      </Badge>
                    )}
                  </NavLink>
                </NavItem>
              ))}
            </NavList>
          </Nav>
          <div style={{ marginTop: 'auto', padding: 8, textAlign: navCollapsed ? 'center' : 'right' }}>
            <Button
              variant="plain"
              aria-label={navCollapsed ? 'Expand navigation' : 'Collapse navigation'}
              aria-expanded={!navCollapsed}
              onClick={toggleNav}
              icon={navCollapsed ? <AngleDoubleRightIcon /> : <AngleDoubleLeftIcon />}
            />
          </div>
        </div>
      </PageSidebarBody>
    </PageSidebar>
  )

  return (
    <Page
      masthead={masthead}
      sidebar={sidebar}
      className={navCollapsed ? 'ao-nav-collapsed' : undefined}
      onPageResize={() => setNavOpen(true)}
    >
      <Boundary>
        <Routes>
          <Route path="/" element={<Navigate to="/overview" replace />} />
          <Route path="/overview" element={<OverviewPage />} />
          <Route path="/queues" element={<QueuesPage />} />
          <Route path="/config" element={<ConfigPage />} />
          <Route path="/config/:kind" element={<ConfigKindPage />} />
          <Route path="/config/:kind/:name" element={<ConfigDetailPage />} />
          <Route path="/topology" element={<TopologyPage />} />
          <Route path="/conversations" element={<ConversationsPage />} />
          <Route path="/conversations/:name" element={<ConversationPage />} />
          <Route
            path="*"
            element={<Empty title="Not found">That page does not exist.</Empty>}
          />
        </Routes>
      </Boundary>
    </Page>
  )
}
