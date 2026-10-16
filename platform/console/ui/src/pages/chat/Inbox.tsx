import type { ReactNode } from 'react'
import { Badge, Tooltip } from '@patternfly/react-core'
import {
  ArchiveIcon, ExclamationCircleIcon, InProgressIcon, ListIcon, UserIcon,
} from '@patternfly/react-icons'
import { useInboxCounts, useVocabulary } from '../../api/hooks'
import { Icon } from '../../components/Icon'
import { PlainText } from '../../components/Text'
import { INBOX_COLLAPSED_WIDTH } from './layout'

// Ported from `prototype/C-rail.html` (expanded) and `C-collapsed.html`
// (the icon strip) — scope order and wording read from there.
//
// `console-chat-layout` spec named this scope "All", counting every
// conversation regardless of read state, with a separate "Unread" scope
// beside it. A later decision made every OTHER fixed scope (Working, Mine,
// Errored), each Pipeline/Coordinator, and Closed an unread count
// (ChatView.tsx's TREE_PREDICATE) — which made the dedicated "Unread" scope
// redundant (every category already had its own unread count one level
// down) and it was removed. All itself did NOT change: it is still every
// conversation a person can open, read or not, same as it always was.

export type Scope =
  | { kind: 'all' }
  | { kind: 'working' }
  | { kind: 'mine' }
  | { kind: 'errored' }
  | { kind: 'pipeline' | 'coordinator'; name: string }
  | { kind: 'closed' }

export function scopeKey(s: Scope): string {
  return 'name' in s ? `${s.kind}:${s.name}` : s.kind
}

export function sameScope(a: Scope, b: Scope): boolean {
  return scopeKey(a) === scopeKey(b)
}

interface InboxProps {
  activeScope: Scope
  onSelectScope: (s: Scope) => void
  collapsed: boolean
  onToggleCollapsed: () => void
  /** The list's own "Show closed" preference (ChatView.tsx's layout.ts) —
   * threaded through so the All/per-Pipeline/Coordinator badges can match
   * what the list below actually shows instead of promising a closed-but-
   * unread row a reader cannot reach without checking that box first. */
  showClosed: boolean
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
      data-testid={`scope-${label}`}
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

export function Inbox({ activeScope, onSelectScope, collapsed, onToggleCollapsed, showClosed }: Readonly<InboxProps>) {
  const counts = useInboxCounts(showClosed)
  const vocabulary = useVocabulary()
  const scopes = counts.data?.scopes ?? {}
  const entries = vocabulary.data?.entries ?? []
  const pipelines = entries.filter((e) => e.kind === 'pipeline' || e.kind === 'coordinator')

  // All reads `rootTotal` — every conversation a person can open, read or
  // not, respecting showClosed like every scope but its own list already
  // does. Every OTHER count below is UNREAD, ANDed with its own scope's
  // predicate: the dedicated Unread scope is GONE, because once Working,
  // Mine, Errored, each Pipeline/Coordinator and Closed all carried their own
  // unread count, a blanket one added nothing a category-scoped click
  // couldn't already answer. Every one of these still matches its own list
  // exactly (ChatView.tsx's TREE_PREDICATE), so none needs a tooltip
  // explaining a gap — there isn't one.
  const fixed: { scope: Scope; icon: ReactNode; label: string; count?: number }[] = [
    { scope: { kind: 'all' }, icon: <ListIcon />, label: 'All', count: counts.data?.rootTotal },
    { scope: { kind: 'working' }, icon: <InProgressIcon />, label: 'Working', count: scopes.working },
    { scope: { kind: 'mine' }, icon: <UserIcon />, label: 'Mine', count: scopes.mine },
    { scope: { kind: 'errored' }, icon: <ExclamationCircleIcon />, label: 'Errored', count: scopes.errored },
  ]

  if (collapsed) {
    return (
      <div
        data-testid="inbox-collapsed"
        style={{
          width: INBOX_COLLAPSED_WIDTH, flex: 'none', background: 'var(--ao-surface)',
          borderRight: '1px solid var(--ao-border)', display: 'flex', flexDirection: 'column',
          alignItems: 'center', gap: 4, padding: '10px 0', overflowY: 'auto',
        }}
      >
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
        {pipelines.map((e) => (
          <Tooltip key={e.name} content={scopes[e.name] ? `${e.name} · ${scopes[e.name]} unread` : e.name}>
            <IconButton
              active={sameScope(activeScope, { kind: e.kind as 'pipeline' | 'coordinator', name: e.name })}
              onClick={() => onSelectScope({ kind: e.kind as 'pipeline' | 'coordinator', name: e.name })}
              badge={scopes[e.name]}
            >
              <Icon icon={e.icon || 'aops:agent'} size="1.1em" />
            </IconButton>
          </Tooltip>
        ))}
        <Divider />
        <Tooltip content="Closed">
          <IconButton
            active={sameScope(activeScope, { kind: 'closed' })}
            onClick={() => onSelectScope({ kind: 'closed' })}
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
      data-testid="inbox"
      style={{
        width: '100%', background: 'var(--ao-surface)', display: 'flex', flexDirection: 'column',
        overflowY: 'auto', height: '100%',
      }}
    >
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
      {pipelines.length > 0 && (
        <Section title="PIPELINES & COORDINATORS">
          {pipelines.map((e) => (
            <Row
              key={e.name}
              icon={<Icon icon={e.icon || 'aops:agent'} size="1.1em" />}
              label={e.name}
              count={scopes[e.name]}
              active={sameScope(activeScope, { kind: e.kind as 'pipeline' | 'coordinator', name: e.name })}
              onClick={() => onSelectScope({ kind: e.kind as 'pipeline' | 'coordinator', name: e.name })}
            />
          ))}
        </Section>
      )}
      <Section title="CLOSED">
        <Row
          icon={<ArchiveIcon />}
          label="Archive"
          count={scopes.closed}
          active={sameScope(activeScope, { kind: 'closed' })}
          onClick={() => onSelectScope({ kind: 'closed' })}
        />
      </Section>
      <div
        style={{
          marginTop: 'auto', padding: '10px 14px', fontSize: 12, color: 'var(--ao-text-subtle)',
          borderTop: '1px solid var(--ao-canvas)', display: 'flex', justifyContent: 'space-between', alignItems: 'center',
        }}
      >
        <span>Counts are unread messages, not rows that changed.</span>
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
