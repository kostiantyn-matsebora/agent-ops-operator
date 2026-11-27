import { type ReactElement, useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import type { ConversationSummary } from '../../../api/types'
import type { RowCommand } from '../../../model/commands'
import { rowSnippet, rowStatus, rowTag, rowTitle, rowUnread, waitingSnippet, type RowStatus } from '../../../model/conversationList'
import { testid } from '../../../model/testids'
import type { TreeRow } from '../../../model/tree'
import { Chevron, Icon } from '../icons'
import { ago } from '../format'
import { prefersReducedMotion } from '../motion'

// ONE LIST ROW, ONE HEIGHT (console-workspace-look: "A list row has a stable
// shape"). Whatever it shows — a status, a badge, the lifecycle bar, a longer
// preview — its height does not change: every line is reserved and clipped,
// and the height is fixed in workspace.css.
//
// Left to right: the fold control for a root with members, the avatar with
// its status dot, three lines of text over a reserved lifecycle line, and on
// the right the time with the unread badge beside it and the status pill
// below.

export interface RowAction {
  label: string
  run: () => void
}

export interface RowProps {
  tree: TreeRow
  current: boolean
  /** Just arrived — a brief emphasis, no size change. */
  arrived?: boolean
  /** Set for a root with members while members nest under it: whether they are folded. */
  folded?: boolean
  onToggleFold?: () => void
  actions?: RowAction[]
  /** The list is folded to icons: the row is its avatar and badge. */
  iconOnly?: boolean
  /** Set while the viewer is selecting: the row carries a box and opening it toggles the box instead. */
  selection?: { selected: boolean; onToggle: () => void }
  /** A modifier-click on the row: starts a selection with this row in it, instead of opening it. */
  onMultiSelect?: () => void
  /** The newest action on this row: the verb in progress, or why it did not happen. */
  command?: RowCommand
  onRetry?: () => void
  onDismissCommand?: () => void
  now?: number
  /** The pipeline's icon, when available. */
  pipelineIcon?: string
}

/** The lifecycle segments a status fills: queued, working, answered. Empty while idle. */
export function lifecycle(status: RowStatus): ('on' | 'cur' | '')[] | undefined {
  if (status === 'pending') return ['cur', '', '']
  if (status === 'working' || status === 'waiting') return ['on', 'cur', '']
  return undefined
}

function StatusPill({
  row, status, command, onRetry, onDismiss,
}: Readonly<{ row: ConversationSummary; status: RowStatus; command?: RowCommand; onRetry?: () => void; onDismiss?: () => void }>) {
  // An action the viewer took speaks before the conversation's own state.
  if (command?.kind === 'progress') {
    return (
      <span className="wk-pill working" data-testid={`command-${row.name}`} data-state="progress">
        <span className="wk-spin" aria-hidden="true" />
        {command.label}
      </span>
    )
  }
  if (command?.kind === 'failed') {
    return (
      <span className="wk-cmdfail" data-testid={`command-${row.name}`} data-state="failed" title={command.error}>
        <span className="wk-pill bad">{command.label}</span>
        {command.retryable && onRetry && (
          <button type="button" className="wk-lnk" onClick={onRetry}>Retry</button>
        )}
        {onDismiss && (
          <button type="button" className="wk-lnk" aria-label={`Dismiss: ${command.error ?? command.label}`} onClick={onDismiss}>
            Dismiss
          </button>
        )}
      </span>
    )
  }
  const tag = rowTag(row, false)
  if (status === 'working') {
    return (
      <span className="wk-pill working">
        <span className="wk-spin" aria-hidden="true" />
        Working
      </span>
    )
  }
  if (status === 'waiting') {
    return (
      <span className="wk-pill working" data-testid={`waiting-${row.name}`} title="its members are still at work">
        <span className="wk-spin" aria-hidden="true" />
        Waiting
      </span>
    )
  }
  if (status === 'failed') return <span className="wk-pill bad">Run failed</span>
  if (status === 'pending') return <span className="wk-pill warn">{row.phase}</span>
  if (tag === 'nobody notified') return <span className="wk-pill warn">Nobody notified</span>
  if (status === 'none') return <span className="wk-pill">Closed</span>
  return null
}

function RowMenu({ title, actions }: Readonly<{ title: string; actions: RowAction[] }>) {
  const [open, setOpen] = useState(false)
  const button = useRef<HTMLButtonElement>(null)
  const menu = useRef<HTMLUListElement>(null)
  useEffect(() => {
    if (!open) return
    menu.current?.querySelector('button')?.focus()
    const onDoc = (e: MouseEvent) => {
      if (!menu.current?.contains(e.target as Node) && e.target !== button.current) setOpen(false)
    }
    document.addEventListener('mousedown', onDoc)
    return () => document.removeEventListener('mousedown', onDoc)
  }, [open])
  const close = () => {
    setOpen(false)
    button.current?.focus()
  }
  return (
    <>
      <button
        ref={button}
        type="button"
        className="wk-icon-btn kb"
        aria-label={`Actions for ${title}`}
        aria-haspopup="menu"
        aria-expanded={open}
        onClick={() => setOpen((v) => !v)}
      >
        <Icon name="more" />
      </button>
      {open && (
        <ul
          ref={menu}
          className="wk-menu"
          role="menu"
          aria-label={`Actions for ${title}`}
          onKeyDown={(e) => {
            if (e.key === 'Escape') close()
            if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
              e.preventDefault()
              const items = [...(menu.current?.querySelectorAll('button') ?? [])]
              const at = items.indexOf(document.activeElement as HTMLButtonElement)
              const next = (at + (e.key === 'ArrowDown' ? 1 : items.length - 1)) % items.length
              items[next]?.focus()
            }
          }}
        >
          {actions.map((a) => (
            <li key={a.label} role="none">
              <button
                type="button"
                role="menuitem"
                onClick={() => {
                  close()
                  a.run()
                }}
              >
                {a.label}
              </button>
            </li>
          ))}
        </ul>
      )}
    </>
  )
}

export function Row({
  tree, current, arrived, folded, onToggleFold, actions = [], iconOnly, selection, onMultiSelect, command, onRetry, onDismissCommand, now, pipelineIcon,
}: Readonly<RowProps>): ReactElement {
  const row = tree.row
  const title = rowTitle(row)
  const status = rowStatus(row, tree.membersActive)
  const unread = rowUnread(row)
  const member = tree.depth > 0 || Boolean(row.causedBy)
  const snippet = status === 'waiting' ? waitingSnippet(tree.activeEntries, tree.membersActive) : rowSnippet(row)
  const attribution = row.causedBy ? `via ${row.causedBy.entry}` : row.coordinator || row.pipeline || row.profile
  const third = [attribution, tree.memberCount > 0 && folded !== undefined ? `${tree.memberCount} members` : '']
    .filter(Boolean)
    .join(' · ')
  const segments = lifecycle(status)
  void prefersReducedMotion
  const cls = ['wk-row', unread && 'unread', tree.depth > 0 && 'mem', status === 'failed' && 'fail', arrived && 'arrived', selection && 'sel']
    .filter(Boolean)
    .join(' ')

  const self = useRef<HTMLLIElement>(null)
  // The open conversation's row is brought into view, once, as it becomes current.
  useEffect(() => {
    if (current) self.current?.scrollIntoView?.({ block: 'nearest' })
  }, [current])

  return (
    <li
      ref={self}
      className={cls}
      data-testid={testid.row(row.name)}
      data-status={status}
      aria-current={current ? 'true' : undefined}
    >
      <Link
        className="open"
        to={`/conversations/${encodeURIComponent(row.name)}`}
        data-testid={testid.open(row.name)}
        aria-label={`Open ${title}${unread ? `, ${row.unreadCount} unread` : ''}`}
        title={iconOnly ? title : undefined}
        onClick={(e) => {
          if (selection) {
            e.preventDefault()
            selection.onToggle()
          } else if ((e.ctrlKey || e.metaKey) && onMultiSelect) {
            e.preventDefault()
            onMultiSelect()
          }
        }}
      />
      {selection && !iconOnly && (
        <input
          type="checkbox"
          className="pick"
          data-testid={testid.select(row.name)}
          aria-label={`Select ${title}`}
          checked={selection.selected}
          onChange={selection.onToggle}
        />
      )}
      {onToggleFold && folded !== undefined && !iconOnly ? (
        <button
          type="button"
          className="exp"
          aria-expanded={!folded}
          aria-label={`${folded ? 'Expand' : 'Collapse'} ${title}`}
          onClick={onToggleFold}
        >
          <Chevron dir={folded ? 'right' : 'down'} />
        </button>
      ) : (
        <span aria-hidden="true" />
      )}
      <span className={`wk-av${member ? ' agent' : ''}`} aria-hidden="true">
        <Icon name={pipelineIcon ? undefined : (member ? 'member' : 'bot')} icon={pipelineIcon} />
        <span className={`d ${status}`} />
        {iconOnly && unread && <span className="bd">{row.unreadCount}</span>}
      </span>
      <div className="txt">
        <span className="name">{title}</span>
        <span className={`l2${row.errored && !row.presence ? ' err' : ''}`}>{snippet || '\u00a0'}</span>
        <span className="l3">{third || '\u00a0'}</span>
        <span className="lc" aria-hidden="true">
          {segments?.map((s, i) => <s key={i} className={s || undefined} />)}
        </span>
      </div>
      <div className="st">
        <span className="tl">
          {unread && (
            <span className="wk-pill ok" data-testid={testid.unread(row.name)}>
              {row.unreadCount}
            </span>
          )}
          <time className="time" dateTime={row.lastActivity ?? row.created}>
            {ago(row.lastActivity ?? row.created, now)}
          </time>
        </span>
        <StatusPill row={row} status={status} command={command} onRetry={onRetry} onDismiss={onDismissCommand} />
      </div>
      {actions.length > 0 && !iconOnly && <RowMenu title={title} actions={actions} />}
    </li>
  )
}
