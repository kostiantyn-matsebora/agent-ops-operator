import { Component, type ReactNode, useState } from 'react'
import { NavLink, Navigate, Route, Routes, useLocation } from 'react-router-dom'
import {
  Alert, Button, Nav, NavItem, NavList, Masthead, MastheadBrand, MastheadContent,
  MastheadMain, Page, PageSidebar, PageSidebarBody, Label, Toolbar, ToolbarContent,
  ToolbarGroup, ToolbarItem, EmptyState, EmptyStateBody, Bullseye, Spinner, PageSection,
} from '@patternfly/react-core'
import { useLiveStream, useSession } from './api/hooks'
import { api } from './api/client'
import { Logo } from './components/Logo'
import { ThemeSwitcher } from './components/ThemeSwitcher'
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

export function Loading() {
  return (
    <Bullseye>
      <Spinner aria-label="loading" />
    </Bullseye>
  )
}

export function ErrorState({ title, children }: { title: string; children?: ReactNode }) {
  return (
    <EmptyState titleText={title} headingLevel="h4" status="danger">
      <EmptyStateBody>{children}</EmptyStateBody>
    </EmptyState>
  )
}

export function Empty({ title, children }: { title: string; children?: ReactNode }) {
  return (
    <EmptyState titleText={title} headingLevel="h4">
      <EmptyStateBody>{children}</EmptyStateBody>
    </EmptyState>
  )
}

const NAV = [
  { to: '/overview', label: 'Overview' },
  { to: '/queues', label: 'Queues' },
  { to: '/config', label: 'Configuration' },
  { to: '/topology', label: 'Topology' },
  { to: '/conversations', label: 'Conversations' },
]

export function App() {
  const session = useSession()
  const connected = useLiveStream()
  const location = useLocation()
  const [navOpen, setNavOpen] = useState(true)

  if (session.isLoading) return <Loading />
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
            <span style={{ fontWeight: 600 }}>agent-ops console</span>
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
                <Label color="blue">{session.data.identity || 'token'}</Label>
              </ToolbarItem>
              <ToolbarItem>
                <ThemeSwitcher />
              </ToolbarItem>
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
            </ToolbarGroup>
          </ToolbarContent>
        </Toolbar>
      </MastheadContent>
    </Masthead>
  )

  const sidebar = (
    <PageSidebar isSidebarOpen={navOpen}>
      <PageSidebarBody>
        <Nav>
          <NavList>
            {NAV.map((item) => (
              <NavItem key={item.to} isActive={location.pathname.startsWith(item.to)}>
                <NavLink to={item.to}>{item.label}</NavLink>
              </NavItem>
            ))}
          </NavList>
        </Nav>
      </PageSidebarBody>
    </PageSidebar>
  )

  return (
    <Page masthead={masthead} sidebar={sidebar} onPageResize={() => setNavOpen(true)}>
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
