import type { ReactElement } from 'react'
import { NavLink, useLocation, useNavigate } from 'react-router-dom'
import { sameScope, scopeKey, useInboxScopes, type Scope, type ScopeEntry } from '../../model/conversationList'
import { GENERAL_FEED_NAME } from '../../model/session'
import { testid } from '../../model/testids'
import { Chevron, Icon, type IconName } from './icons'
import { useWorkspaceLayout, useWorkspaceScope } from './layout'

// THE NAVIGATION PANEL: the pages, the channel, the conversation scopes with
// their counts, the pipelines and coordinators, and the archive
// (console-workspace-look). Folded, it is a strip of icons — and every count
// stays, as a badge on its icon.

const PAGES: { to: string; label: string; icon: IconName }[] = [
  { to: '/overview', label: 'Overview', icon: 'overview' },
  { to: '/queues', label: 'Queues', icon: 'queues' },
  { to: '/config', label: 'Configuration', icon: 'config' },
  { to: '/topology', label: 'Topology', icon: 'topology' },
]

const SCOPE_ICON: Record<Scope['kind'], IconName> = {
  all: 'all',
  working: 'working',
  mine: 'mine',
  errored: 'errored',
  pipeline: 'pipeline',
  coordinator: 'pipeline',
  closed: 'archive',
  general: 'hash',
}

function Label({ icon, label }: Readonly<{ icon: IconName; label: string }>) {
  return (
    <span className="lb">
      <Icon name={icon} />
      <span>{label}</span>
    </span>
  )
}

export function NavPanel({
  collapsed, drawer, onToggleCollapsed, onPicked,
}: Readonly<{
  collapsed: boolean
  /** Shown as a drawer: never folded, and it closes once something is picked. */
  drawer: boolean
  onToggleCollapsed: () => void
  onPicked: () => void
}>): ReactElement {
  const { pathname } = useLocation()
  const navigate = useNavigate()
  const scope = useWorkspaceScope((s) => s.scope)
  const setScope = useWorkspaceScope((s) => s.setScope)
  const showClosed = useWorkspaceLayout((s) => s.showClosed)
  const { general, fixed, targets, closed } = useInboxScopes(showClosed)
  const folded = collapsed && !drawer
  const onConversations = pathname.startsWith('/conversations')
  const onFeed = pathname === `/conversations/${GENERAL_FEED_NAME}` || pathname === `/conversations/${encodeURIComponent(GENERAL_FEED_NAME)}`

  const heading = (text: string) => (folded ? null : <h3>{text}</h3>)

  const pick = (s: Scope) => {
    if (s.kind === 'general') {
      void navigate(`/conversations/${GENERAL_FEED_NAME}`)
    } else {
      setScope(s)
      if (!onConversations || onFeed) void navigate('/conversations')
    }
    onPicked()
  }

  const scopeButton = (e: ScopeEntry) => {
    const active = e.scope.kind === 'general' ? onFeed : onConversations && !onFeed && sameScope(scope, e.scope)
    const label = e.label
    return (
      <li key={scopeKey(e.scope)}>
        <button
          type="button"
          className="wk-nv"
          aria-pressed={active}
          aria-label={folded ? `${label}${e.count ? `, ${e.count}` : ''}` : undefined}
          title={folded ? label : undefined}
          data-testid={testid.scope(label)}
          onClick={() => pick(e.scope)}
        >
          <Label icon={SCOPE_ICON[e.scope.kind]} label={label} />
          {e.count !== undefined && (!folded || e.count > 0) && <b className="wk-num">{e.count}</b>}
        </button>
      </li>
    )
  }

  return (
    <>
      <div className="wk-sidein">
        {heading('Pages')}
        <ul>
          {PAGES.map((p) => (
            <li key={p.to}>
              <NavLink
                to={p.to}
                className="wk-nv"
                aria-label={folded ? p.label : undefined}
                title={folded ? p.label : undefined}
                onClick={onPicked}
              >
                <Label icon={p.icon} label={p.label} />
              </NavLink>
            </li>
          ))}
        </ul>
        {heading('Channel')}
        <ul>{scopeButton(general)}</ul>
        {heading('Conversations')}
        <ul>{fixed.map(scopeButton)}</ul>
        {targets.length > 0 && heading('Pipelines & coordinators')}
        {targets.length > 0 && <ul>{targets.map(scopeButton)}</ul>}
        {heading('Closed')}
        <ul>{scopeButton(closed)}</ul>
      </div>
      {!drawer && (
        <div className="wk-sidefoot">
          {!folded && <span>All counts conversations. The rest count unread.</span>}
          <button
            type="button"
            className="wk-icon-btn"
            aria-label={folded ? 'Expand the navigation' : 'Collapse the navigation'}
            title={folded ? 'Expand the navigation' : 'Collapse the navigation'}
            aria-expanded={!folded}
            onClick={onToggleCollapsed}
          >
            <Chevron dir={folded ? 'doubleRight' : 'doubleLeft'} />
          </button>
        </div>
      )}
    </>
  )
}

