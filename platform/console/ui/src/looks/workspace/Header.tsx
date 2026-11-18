import type { ReactElement } from 'react'
import type { Session } from '../../api/types'
import type { Connection } from '../../model/commands'
import { TESTID, testid } from '../../model/testids'
import { resolve, useThemeStore, type ThemeChoice } from '../../theme/useTheme'
import { Logo } from '../../components/Logo'
import { LookSwitcher } from '../shared/LookSwitcher'
import { Icon } from './icons'

// THE HEADER: the navigation's menu button, the mark, the connection and the
// work in flight, who the viewer is, the look and the colour scheme. A control
// appears only where it does something in this deployment
// (console-workspace-look): sign-out is shown only where the console holds the
// session it would end, never behind an external authenticator, where it would
// clear a cookie that authorized nothing.

const SCHEMES: { id: ThemeChoice; glyph: string; label: string }[] = [
  { id: 'light', glyph: '☀', label: 'Light' },
  { id: 'dark', glyph: '☾', label: 'Dark' },
  { id: 'system', glyph: 'Auto', label: 'Follow the system' },
]

/** Light, dark or the system's — a separate choice from the look, and every look honours it. */
export function SchemeControl(): ReactElement {
  const choice = useThemeStore((s) => s.choice)
  const setChoice = useThemeStore((s) => s.setChoice)
  return (
    <span
      className="wk-seg"
      role="group"
      aria-label="Colour scheme"
      data-testid={TESTID.themeSwitcher}
      data-applied={resolve(choice)}
    >
      {SCHEMES.map((s) => (
        <button
          key={s.id}
          type="button"
          aria-pressed={choice === s.id}
          aria-label={s.label}
          title={s.label}
          data-testid={testid.theme(s.id)}
          onClick={() => setChoice(s.id)}
        >
          <span aria-hidden="true">{s.glyph}</span>
        </button>
      ))}
    </span>
  )
}

export function Header({
  session, connection, inFlight, navOpen, onMenu, onNew, onSignOut, compact,
}: Readonly<{
  /** Absent where this console cannot start a conversation. */
  onNew?: () => void
  /** Ends the session — offered only where the console holds one. */
  onSignOut?: () => void
  session: Session
  connection: Connection
  inFlight: number
  navOpen: boolean
  onMenu: () => void
  /** Phone width: the mark without its name. */
  compact: boolean
}>): ReactElement {
  const live = connection === 'connected'
  return (
    <header className="wk-top">
      <button
        type="button"
        className="wk-btn wk-ham"
        aria-label="Toggle navigation"
        aria-expanded={navOpen}
        aria-controls="wk-nav"
        onClick={onMenu}
      >
        <Icon name="menu" />
      </button>
      <span className="wk-brand">
        <Logo size={24} />
        {!compact && <span>agent-ops console</span>}
      </span>
      {onNew && (
        <button type="button" className="wk-btn primary" data-testid={TESTID.newConversation} onClick={onNew}>
          New conversation
        </button>
      )}
      {/* The connection, stated: a stream that is down and a system that is
          idle look the same, and saying which is the difference between a
          stale console and a wrong one. */}
      <span
        className={`wk-chip ${live ? 'live' : 'off'}`}
        data-testid={TESTID.connection}
        data-state={connection}
        role="status"
      >
        <i aria-hidden="true" />
        {live ? 'live' : 'reconnecting · showing data as it was'}
      </span>
      {/* Work in flight: nothing at all while there is none. */}
      {inFlight > 0 && (
        <span className="wk-chip flight" data-testid="in-flight" role="status">
          <span className="wk-spin" aria-hidden="true" />
          <span className="wk-num">{inFlight}</span> in flight
        </span>
      )}
      <span className="wk-sp" />
      {!session.writeEnabled && <span className="wk-chip quiet">read-only</span>}
      <span
        className={`wk-chip ${session.identity ? 'who' : 'off'} hide-s`}
        title={
          session.identity
            ? undefined
            : `${session.externalAuthenticator || 'A proxy'} forwarded no identity: reads work, writes are refused.`
        }
      >
        {session.identity || 'unknown'}
      </span>
      <LookSwitcher className="wk-look" />
      <SchemeControl />
      {onSignOut && (
        <button type="button" className="wk-btn" onClick={onSignOut}>
          Sign out
        </button>
      )}
    </header>
  )
}
