import { forwardRef } from 'react'
import { Checkbox, Label } from '@patternfly/react-core'
import { Icon, stripLeadingIcon } from '../../components/Icon'
import { PlainText } from '../../components/Text'
import { relativeAge } from './format'
import { prefersReducedMotion } from './motion'
import { RowMenu } from './RowMenu'
import type { ConversationBudget, ConversationSummary } from '../../api/types'

// Ported from `prototype/States.html` (row states) and `C-rail.html` /
// `D-incident.html` (the grid, the caret, the member line) — composition and
// wording read from there, behaviour from the `console-unread` and
// `console-conversation-tree` specs.

/**
 * Strips a leading "`<name>`: " from a title, ONLY when `<name>` is exactly
 * the name the row's own chip already shows (`RowBadges` above) — the
 * coordinator for a root, the pipeline otherwise. Any other colon-prefixed
 * text (an agent's own "esphome: ...", "homeassistant: ...") is content and
 * stays untouched: this never strips a prefix it cannot attribute to the chip.
 */
export function stripNamePrefix(title: string, name: string | undefined): string {
  if (!name) return title
  const prefix = `${name}: `
  return title.startsWith(prefix) ? title.slice(prefix.length) : title
}

/** The snippet line: the last counted message, or why the row has no message yet. */
export function rowSnippet(row: ConversationSummary): string {
  if (row.deleting) return 'deleting'
  if (row.phase === 'Closed') return row.closeReason ? `closed · ${row.closeReason}` : `closed · ${row.runCount} run(s)`
  if (row.blocked) return `${row.blocked.storage ? 'storage' : 'blocked'}: ${row.blocked.reason}`
  if (row.phase === 'Pending') return 'pending · waiting for a slot'
  if (row.lastMessage) {
    const who = row.lastMessage.sender ? `${row.lastMessage.sender}: ` : ''
    return row.presence ? `working · ${who}${row.lastMessage.text}` : `${who}${row.lastMessage.text}`
  }
  if (row.presence) return `${row.pipeline || 'the agent'} is working…`
  return ''
}

/** The small tag on the right of the snippet line — absent most of the time. */
export function rowTag(row: ConversationSummary, isNew: boolean): string | undefined {
  if (isNew) return 'new'
  if (row.errored) return 'run failed'
  if (row.phase === 'Closed') {
    // A coordinator's root closed WITHOUT escalating is visible and distinct
    // (console-conversation-tree: "An incident nobody was told about is
    // visible") — never confused with an ordinary closed thread.
    if (row.coordinator && !row.escalatedAt && row.closeReason) return 'nobody notified'
    return 'closed'
  }
  if (!row.joined) return 'observed'
  return undefined
}

type DotState = 'working' | 'pending' | 'failed' | 'idle' | 'none'

export function phaseDot(row: ConversationSummary): DotState {
  if (row.presence) return 'working'
  if (row.errored) return 'failed'
  if (row.phase === 'Pending' || row.phase === 'Queued') return 'pending'
  if (row.phase === 'Closed') return 'none'
  return 'idle'
}

const DOT_COLOR: Record<DotState, string | undefined> = {
  working: 'var(--ao-brand)',
  pending: 'var(--ao-warning)',
  failed: 'var(--ao-danger)',
  idle: 'var(--ao-success)',
  none: undefined,
}

/** "2 of 6" when a ceiling is set, else just the count. */
function counted(used: number | undefined, max: number | undefined): string {
  const count = used ?? 0
  return max ? `${count} of ${max}` : String(count)
}

/** The root row's extra line — turn, member count, time left (design D-F). */
export function budgetSummary(budget: ConversationBudget | undefined, memberCount: number): string {
  const parts = [`${memberCount} member${memberCount === 1 ? '' : 's'}`]
  if (budget?.maxTurns || budget?.turns) parts.unshift(`turn ${counted(budget?.turns, budget?.maxTurns)}`)
  if (budget?.deadline) {
    const msLeft = new Date(budget.deadline).getTime() - Date.now()
    if (msLeft > 0) parts.push(`${Math.round(msLeft / 60000)}m left`)
  }
  return parts.join(' · ')
}

function rowTint(row: ConversationSummary, isRoot: boolean): string {
  if (row.errored) return 'var(--ao-danger)'
  return isRoot ? 'var(--ao-accent)' : 'var(--ao-brand-strong)'
}

function tagColor(tag: string): 'red' | 'blue' | 'grey' {
  if (tag === 'run failed') return 'red'
  return tag === 'new' ? 'blue' : 'grey'
}

function RowBadges({
  row, depth, parentMissing, isRoot, unread, tag,
}: Readonly<{
  row: ConversationSummary
  depth: number
  parentMissing: boolean
  isRoot: boolean
  unread: boolean
  tag: string | undefined
}>) {
  // Every row names what answers it: the coordinator by name for a root, the
  // addressed entry for a member (already carried by `causedBy.entry`), and
  // the pipeline by name for an ordinary conversation — previously only an
  // icon, with nothing to read if the icon didn't say enough on its own.
  const plainPipeline = !isRoot && !(depth > 0 && row.causedBy) && row.pipeline

  return (
    <>
      {isRoot && (
        <Label isCompact color="purple">
          <PlainText>{row.coordinator}</PlainText>
        </Label>
      )}
      {depth > 0 && row.causedBy && (
        <Label isCompact color="purple">
          <PlainText>{`via ${row.causedBy.entry}`}</PlainText>
        </Label>
      )}
      {plainPipeline && (
        <Label isCompact color="grey">
          <PlainText>{row.pipeline}</PlainText>
        </Label>
      )}
      {parentMissing && row.causedBy && (
        <Label isCompact color="grey" title={`parent ${row.causedBy.parent} is not in view`}>
          parent missing
        </Label>
      )}
      {unread && (
        <span
          data-testid={`unread-${row.name}`}
          aria-label={`${row.unreadCount} unread`}
          style={{
            minWidth: 20, height: 20, padding: '0 6px', borderRadius: 10, background: 'var(--ao-brand)',
            color: 'var(--ao-surface)', fontSize: '0.75em', fontWeight: 700, display: 'inline-flex',
            alignItems: 'center', justifyContent: 'center',
          }}
        >
          {row.unreadCount}
        </span>
      )}
      {tag && (
        <Label isCompact color={tagColor(tag)}>
          {tag}
        </Label>
      )}
    </>
  )
}

export interface ConversationRowProps {
  row: ConversationSummary
  depth: number
  memberCount: number
  parentMissing: boolean
  /** True for the few seconds after arrival (console-thread-live-cues: "Arrivals move"). */
  isNew: boolean
  selectionMode: boolean
  selected: boolean
  /** The keyboard-navigated row — distinct from `selected`, which is the bulk-action set. */
  highlighted: boolean
  onSelect: (checked: boolean) => void
  /** `⌘`/`Ctrl`-click adds to the selection instead of opening (design D-A). */
  onOpen: (e: React.MouseEvent) => void
  pipelineIcon?: string
  collapsed?: boolean
  onToggleCollapse?: () => void
  /** Whether the viewer may write at all — RowMenu omits every write action otherwise. */
  canWrite: boolean
  /** A per-person watermark exists to rewind — absent under a shared token (RowMenu, design D-E). */
  hasReader: boolean
  onMarkRead: () => void
  onMarkUnread: () => void
  onReopen: () => void
  onExitRuntime: () => void
  onClose: () => void
  onDelete: () => void
}

/** The expand/collapse chevron of a root, or the spacer a top-level leaf keeps. */
function CollapseToggle({
  memberCount, depth, title, collapsed, onToggleCollapse,
}: Readonly<{
  memberCount: number
  depth: number
  title: string
  collapsed?: boolean
  onToggleCollapse?: () => void
}>) {
  if (memberCount > 0) {
    return (
      <button
        type="button"
        aria-label={collapsed ? `expand ${title}` : `collapse ${title}`}
        aria-expanded={!collapsed}
        onClick={onToggleCollapse}
        style={{ all: 'unset', cursor: 'pointer', color: 'var(--ao-text-subtle)', fontSize: 11, paddingTop: 10 }}
      >
        {collapsed ? '▸' : '▾'}
      </button>
    )
  }
  return depth === 0 ? <span aria-hidden style={{ width: 11 }} /> : null
}

export const ConversationRow = forwardRef<HTMLButtonElement, ConversationRowProps>(function ConversationRow(
  {
    row, depth, memberCount, parentMissing, isNew, selectionMode, selected, highlighted,
    onSelect, onOpen, pipelineIcon, collapsed, onToggleCollapse,
    canWrite, hasReader, onMarkRead, onMarkUnread, onReopen, onExitRuntime, onClose, onDelete,
  },
  ref,
) {
  const isRoot = Boolean(row.coordinator)
  const isMember = Boolean(row.causedBy)
  // A root is attributed by its coordinator, a member by the entry it was
  // invoked as (`causedBy.entry` — `row.pipeline` is unset on a member, so
  // comparing against it left the prefix in place), and everything else by
  // its pipeline.
  const nameForStrip = isRoot ? row.coordinator : isMember ? row.causedBy?.entry : row.pipeline
  const title = stripNamePrefix(stripLeadingIcon(row.title || row.name), nameForStrip)
  const unread = (row.unreadCount ?? 0) > 0
  const dot = phaseDot(row)
  const snippet = rowSnippet(row)
  const tag = rowTag(row, isNew)
  const tint = rowTint(row, isRoot)

  // Pure navigation, derived from the row's own name — no mutation, so no
  // hook and no prop from the caller (prototype/States.html's "Row menu").
  function openInNewTab() {
    window.open(`${window.location.origin}/conversations/${row.name}`, '_blank', 'noopener')
  }
  function copyLink() {
    void navigator.clipboard?.writeText(`${window.location.origin}/conversations/${row.name}`)
  }

  return (
    <li
      // The arrival TINT is motion and is skipped under reduced motion; the
      // "new" tag below is content and stays either way
      // (console-thread-live-cues: "rows ... appear without animation, and
      // the marker ... still appear"). `isNew` is cleared the moment the row
      // is first opened, by whoever tracks arrivals.
      className={isNew && !prefersReducedMotion() ? 'ao-row-arrive' : undefined}
      data-testid={`row-${row.name}`}
      style={{
        display: 'grid',
        gridTemplateColumns: selectionMode ? 'auto auto 1fr auto' : 'auto 1fr auto',
        alignItems: 'start',
        gap: 10,
        padding: '10px 12px',
        paddingLeft: 12 + depth * 24,
        borderBottom: '1px solid var(--ao-canvas)',
        background: highlighted ? 'var(--ao-brand-soft)' : undefined,
        boxShadow: highlighted ? 'inset 3px 0 0 var(--ao-brand)' : undefined,
      }}
    >
      {selectionMode && (
        <Checkbox
          id={`select-${row.name}`}
          aria-label={`select ${title}`}
          isChecked={selected}
          isDisabled={row.deleting}
          onChange={(_e, checked) => onSelect(checked)}
        />
      )}
      <CollapseToggle
        memberCount={memberCount}
        depth={depth}
        title={title}
        collapsed={collapsed}
        onToggleCollapse={onToggleCollapse}
      />
      <button
        ref={ref}
        type="button"
        onClick={onOpen}
        aria-label={`open ${title}`}
        data-testid={`open-${row.name}`}
        style={{
          all: 'unset',
          display: 'grid',
          gridTemplateColumns: '36px 1fr',
          columnGap: 10,
          minWidth: 0,
          width: '100%',
          cursor: 'pointer',
          textAlign: 'left',
        }}
      >
        <span
          aria-hidden
          style={{
            width: 36, height: 36, borderRadius: '50%', background: 'var(--ao-surface-alt)',
            border: `1px solid ${tint}`, color: tint, display: 'flex', alignItems: 'center', justifyContent: 'center',
            position: 'relative', flex: 'none',
          }}
        >
          <Icon icon={pipelineIcon || 'aops:agent'} size="1.1em" />
          {dot !== 'none' && (
            <span
              aria-hidden
              style={{
                position: 'absolute', right: -2, bottom: -2, width: 11, height: 11, borderRadius: '50%',
                border: '2px solid var(--ao-surface)', background: DOT_COLOR[dot],
              }}
            />
          )}
        </span>
        <span style={{ minWidth: 0 }}>
          <span style={{ display: 'flex', alignItems: 'baseline', justifyContent: 'space-between', gap: 8 }}>
            <span
              style={{
                fontSize: '0.95em', fontWeight: unread ? 700 : 400,
                color: unread ? 'var(--ao-brand-strong)' : 'var(--ao-text)',
                overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
              }}
            >
              <PlainText>{title}</PlainText>
            </span>
            <time style={{ fontSize: '0.8em', color: 'var(--ao-text-subtle)', flex: 'none' }}>
              {relativeAge(row.ageSeconds)}
            </time>
          </span>
          <span style={{ display: 'flex', alignItems: 'baseline', justifyContent: 'space-between', gap: 8, marginTop: 2 }}>
            <span
              style={{
                fontSize: '0.85em', color: 'var(--ao-text-subtle)',
                overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
              }}
            >
              {isRoot && memberCount > 0 ? (
                <PlainText>{budgetSummary(row.budget, memberCount)}</PlainText>
              ) : (
                <PlainText>{snippet}</PlainText>
              )}
            </span>
            <span style={{ display: 'flex', alignItems: 'center', gap: 6, flex: 'none' }}>
              <RowBadges row={row} depth={depth} parentMissing={parentMissing} isRoot={isRoot} unread={unread} tag={tag} />
            </span>
          </span>
        </span>
      </button>
      <span style={{ paddingTop: 4 }}>
        <RowMenu
          row={row}
          canWrite={canWrite}
          hasReader={hasReader}
          onMarkRead={onMarkRead}
          onMarkUnread={onMarkUnread}
          onOpenNewTab={openInNewTab}
          onCopyLink={copyLink}
          onReopen={onReopen}
          onExitRuntime={onExitRuntime}
          onClose={onClose}
          onDelete={onDelete}
        />
      </span>
    </li>
  )
})
