import type { ReactNode } from 'react'
import { Badge, Tooltip } from '@patternfly/react-core'
import {
  ArchiveIcon, ExclamationCircleIcon, HashtagIcon, InProgressIcon, ListIcon, UserIcon,
} from '@patternfly/react-icons'
import { sameScope, scopeKey, useInboxScopes, type Scope } from '../../../../model/conversationList'
import { TESTID, testid } from '../../../../model/testids'
import { Icon } from '../../components/Icon'
import { PlainText } from '../../components/Text'
import { INBOX_COLLAPSED_WIDTH } from './layout'

// Ported from `prototype/C-rail.html` (expanded) and `C-collapsed.html`
// (the icon strip) — scope order and wording read from there. The scopes and
// their counts are the list model's (model/conversationList.ts); this file
// only draws them.

export { sameScope, scopeKey, type Scope } from '../../../../model/conversationList'

interface InboxProps {
  activeScope: Scope
  onSelectScope: (s: Scope) => void
  collapsed: boolean
  onToggleCollapsed: () => void
}

function Section({ title, children }: Readonly<{ title: string; children: ReactNode }>) {
  return (
    <>
      <div
        style={{
          fontSize: 11, letterSpacing: '0.08em', color: 'var(--ao-text-subtle)', fontWeight: 500,
          padding: '14px 14px 6px',
        }}
      >
        {title}
      </div>
      {children}
    </>
  )
}

function Row({
  icon, label, count, active, onClick, mono,
}: Readonly<{
  icon: ReactNode
  label: string
  count?: number
  active: boolean
  onClick: () => void
  mono?: boolean
}>) {
  const badge = typeof count === 'number' && count > 0 && (
    <span style={{ marginLeft: 'auto' }}>
      <Badge isRead={!active}>{count}</Badge>
    </span>
  )
  return (
    <button
      type="button"
      onClick={onClick}
      aria-current={active || undefined}
      data-testid={testid.scope(label)}
      style={{
        all: 'unset', display: 'flex', alignItems: 'center', gap: 8, padding: '7px 14px',
        fontSize: '0.9em', cursor: 'pointer', width: '100%', boxSizing: 'border-box',
        color: active ? 'var(--ao-brand-strong)' : 'var(--ao-text)',
        background: active ? 'var(--ao-brand-soft)' : 'transparent',
        fontWeight: active ? 500 : 400,
        fontFamily: mono ? 'var(--pf-t--global--font--family--mono)' : undefined,
      }}
    >
      <span aria-hidden style={{ display: 'flex', flex: 'none' }}>{icon}</span>
      <PlainText>{label}</PlainText>
      {badge}
    </button>
  )
}

export function Inbox({ activeScope, onSelectScope, collapsed, onToggleCollapsed }: Readonly<InboxProps>) {
  const { general, fixed: fixedScopes, targets, closed } = useInboxScopes()
  const FIXED_ICONS: Record<string, ReactNode> = {
    open: <ListIcon />, working: <InProgressIcon />, mine: <UserIcon />, errored: <ExclamationCircleIcon />,
  }
  const fixed = fixedScopes.map((f) => ({ ...f, icon: FIXED_ICONS[f.scope.kind] }))

  if (collapsed) {
    return (
      <div
        data-testid={TESTID.inboxCollapsed}
        style={{
          width: INBOX_COLLAPSED_WIDTH, flex: 'none', background: 'var(--ao-surface)',
          borderRight: '1px solid var(--ao-border)', display: 'flex', flexDirection: 'column',
          alignItems: 'center', gap: 4, padding: '10px 0', overflowY: 'auto',
        }}
      >
        <Tooltip content="General feed">
          <IconButton
            active={sameScope(activeScope, general.scope)}
            onClick={() => onSelectScope(general.scope)}
          >
            <HashtagIcon />
          </IconButton>
        </Tooltip>
        <Divider />
        {fixed.map((f) => (
          <Tooltip
            key={scopeKey(f.scope)}
            content={f.count ? `${f.label} · ${f.count}` : f.label}
          >
            <IconButton active={sameScope(activeScope, f.scope)} onClick={() => onSelectScope(f.scope)} badge={f.count}>
              {f.icon}
            </IconButton>
          </Tooltip>
        ))}
        <Divider />
        {targets.map((e) => (
          <Tooltip key={e.label} content={e.count ? `${e.label} · ${e.count} unread` : e.label}>
            <IconButton
              active={sameScope(activeScope, e.scope)}
              onClick={() => onSelectScope(e.scope)}
              badge={e.count}
            >
              <Icon icon={e.icon || 'aops:agent'} size="1.1em" />
            </IconButton>
          </Tooltip>
        ))}
        <Divider />
        <Tooltip content={closed.count ? `Closed · ${closed.count}` : 'Closed'}>
          <IconButton
            active={sameScope(activeScope, closed.scope)}
            onClick={() => onSelectScope(closed.scope)}
            badge={closed.count}
          >
            <ArchiveIcon />
          </IconButton>
        </Tooltip>
        <button
          type="button"
          aria-label="expand the inbox"
          onClick={onToggleCollapsed}
          style={{
            all: 'unset', marginTop: 'auto', cursor: 'pointer', color: 'var(--ao-text-subtle)', fontSize: 16,
          }}
        >
          »
        </button>
      </div>
    )
  }

  return (
    <div
      data-testid={TESTID.inbox}
      style={{
        width: '100%', background: 'var(--ao-surface)', display: 'flex', flexDirection: 'column',
        overflowY: 'auto', height: '100%',
      }}
    >
      <Section title="CHANNEL">
        <Row
          icon={<HashtagIcon />}
          label={general.label}
          active={sameScope(activeScope, general.scope)}
          onClick={() => onSelectScope(general.scope)}
        />
      </Section>
      <Section title="INBOX">
        {fixed.map((f) => (
          <Row
            key={scopeKey(f.scope)}
            icon={f.icon}
            label={f.label}
            count={f.count}
            active={sameScope(activeScope, f.scope)}
            onClick={() => onSelectScope(f.scope)}
          />
        ))}
      </Section>
      {targets.length > 0 && (
        <Section title="PIPELINES & COORDINATORS">
          {targets.map((e) => (
            <Row
              key={e.label}
              icon={<Icon icon={e.icon || 'aops:agent'} size="1.1em" />}
              label={e.label}
              count={e.count}
              active={sameScope(activeScope, e.scope)}
              onClick={() => onSelectScope(e.scope)}
            />
          ))}
        </Section>
      )}
      <Section title="CLOSED">
        <Row
          icon={<ArchiveIcon />}
          label={closed.label}
          count={closed.count}
          active={sameScope(activeScope, closed.scope)}
          onClick={() => onSelectScope(closed.scope)}
        />
      </Section>
      <div
        style={{
          marginTop: 'auto', padding: '10px 14px', fontSize: 12, color: 'var(--ao-text-subtle)',
          borderTop: '1px solid var(--ao-canvas)', display: 'flex', justifyContent: 'space-between', alignItems: 'center',
        }}
      >
        <span>Counts are unread conversations, not rows that changed.</span>
        <button
          type="button"
          aria-label="collapse the inbox"
          onClick={onToggleCollapsed}
          style={{ all: 'unset', cursor: 'pointer', fontSize: 14 }}
        >
          «
        </button>
      </div>
    </div>
  )
}

function Divider() {
  return <div aria-hidden style={{ width: 28, height: 1, background: 'var(--ao-border)', margin: '6px 0' }} />
}

function IconButton({
  active, onClick, badge, children,
}: Readonly<{
  active: boolean
  onClick: () => void
  badge?: number
  children: ReactNode
}>) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-current={active || undefined}
      style={{
        all: 'unset', width: 40, height: 40, borderRadius: 10, display: 'flex', alignItems: 'center',
        justifyContent: 'center', cursor: 'pointer', position: 'relative',
        color: active ? 'var(--ao-brand-strong)' : 'var(--ao-text-subtle)',
        background: active ? 'var(--ao-brand-soft)' : 'transparent',
      }}
    >
      {children}
      {typeof badge === 'number' && badge > 0 && (
        <span
          style={{
            position: 'absolute', top: -4, right: -6, minWidth: 16, height: 16, fontSize: 10, padding: '0 4px',
            borderRadius: 8, background: 'var(--ao-brand)', color: 'var(--ao-surface)', display: 'flex',
            alignItems: 'center', justifyContent: 'center',
          }}
        >
          {badge}
        </span>
      )}
    </button>
  )
}
