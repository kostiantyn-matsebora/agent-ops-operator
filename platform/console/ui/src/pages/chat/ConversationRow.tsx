import { forwardRef } from 'react'
import { Checkbox, Label } from '@patternfly/react-core'
import { Icon, stripLeadingIcon } from '../../components/Icon'
import { PlainText } from '../../components/Text'
import { relativeAge } from './format'
import { prefersReducedMotion } from './motion'
import { RowMenu } from './RowMenu'
import type { ConversationSummary } from '../../api/types'

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

/**
 * The small tag on the right of the snippet line — absent most of the time.
 *
 * `observed` (`!row.joined`) is GONE: measured live, it showed on every
 * unattended alert-investigator row in every screenshot, since the viewer
 * never "joins" an automated conversation by replying to it. A tag on
 * effectively 100% of rows distinguishes nothing and is pure noise.
 */
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
      {/* The SignalSource that opened this conversation — a domain entity
          (`terminology.md`: a root is reached by "a signal posted to a
          source it claims"), never to be confused with text that happens to
          sit inside some titles (a k8s alert's own namespace, embedded by
          the alert itself). NOT root-only: measured live, a plain
          cron-triggered conversation (`job-cb5vg`, "Self-heal sweep") had a
          real `source: "reaper-sweep"` but NO `coordinator` field at all —
          it was never a Coordinator root, just an ordinary conversation
          reached by a claimed source. Gating on `isRoot` hid a source that
          was genuinely there. The one row that must NEVER show it is a
          member (`depth > 0 && row.causedBy`): a member is reached by
          `causedBy`, never by a source, and already carries its own "via X"
          attribution below. Absent on anything a channel started, or
          anything older than the manager recording it — rendered as
          nothing, never guessed. */}
      {!(depth > 0 && row.causedBy) && row.source && (
        <Label isCompact color="grey">
          <PlainText>{row.source}</PlainText>
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

/**
 * The expand/collapse chevron, or the spacer a leaf keeps in its place.
 *
 * The spacer is reserved AT EVERY DEPTH, not only at depth 0. A depth-0 leaf
 * and a depth-1 leaf must sit behind the identical "toggle slot" width so the
 * guide column is the ONLY thing that moves a member's icon — measured live:
 * dropping the spacer for depth > 0 let a root's own chevron+gap (21px)
 * almost exactly cancel the guide column's 24px, leaving a member's icon
 * ~3px from its root's — visually unreadable as nested.
 *
 * The toggle BUTTON also gets the identical explicit width, for the same
 * reason one level up: unset, its width is the glyph's own metrics (measured
 * live at 6px) against the spacer's fixed 11px — two SIBLING roots, one
 * collapsible and one not, sat 5px apart despite being at the same depth.
 */
function CollapseToggle({
  memberCount, title, collapsed, onToggleCollapse,
}: Readonly<{
  memberCount: number
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
        style={{
          all: 'unset', cursor: 'pointer', color: 'var(--ao-text-subtle)', fontSize: 11, paddingTop: 3,
          width: 11, textAlign: 'center', alignSelf: 'start',
        }}
      >
        {collapsed ? '▸' : '▾'}
      </button>
    )
  }
  return <span aria-hidden style={{ width: 11, alignSelf: 'start' }} />
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
  let nameForStrip = row.pipeline
  if (isRoot) nameForStrip = row.coordinator
  else if (isMember) nameForStrip = row.causedBy?.entry
  const title = stripNamePrefix(stripLeadingIcon(row.title || row.name), nameForStrip)
  const unread = (row.unreadCount ?? 0) > 0
  const dot = phaseDot(row)
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

  // One plain vertical line per ancestor level, each spanning this row's own
  // full height — no elbow, no "is this the last child" termination. The
  // same simplification file trees, outliners and nested comment threads
  // make: indentation ALONE reads as one flat group past two levels, with
  // nothing marking which rows share a parent once a member is itself a
  // Coordinator's own root and nests a level further.
  //
  // `--ao-border` (a 1.5px fractional width) was invisible in practice: it is
  // the subtle default-divider token, barely distinct from the canvas in
  // EITHER theme (#cfd6da on a near-white light background, #3a4247 on a
  // near-black dark one) — present in the DOM with correct geometry, measured
  // live, yet unreadable in an actual screenshot. `--ao-text-subtle` is the
  // token this project already uses where something needs to read against
  // the background in both themes, and an integer 2px avoids sub-pixel
  // rounding making the line thinner than requested.
  const guides = Array.from({ length: depth }, (_, i) => (
    <span key={i} aria-hidden style={{ width: 24, flex: 'none', borderLeft: '2px solid var(--ao-text-subtle)' }} />
  ))

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
        display: 'flex',
        // A root's divider spans the FULL row — it closes out an entire
        // group. A nested row's divider (below, on the inner content only)
        // is inset and lighter: it separates rows WITHIN one family, never
        // reading as a new group's own boundary.
        borderBottom: depth === 0 ? '1px solid var(--ao-canvas)' : undefined,
        background: highlighted ? 'var(--ao-brand-soft)' : undefined,
        boxShadow: highlighted ? 'inset 3px 0 0 var(--ao-brand)' : undefined,
      }}
    >
      {selectionMode && (
        // Rendered BEFORE the guide columns, at a fixed offset from the
        // row's own left edge — never shifted by depth. Living inside the
        // depth-dependent content grid put a root's checkbox hard against
        // the row edge and a member's checkbox one guide-column further
        // right, which read as a ragged column rather than one a reader can
        // scan straight down. `alignSelf: 'start'` still anchors it to the
        // title line, matching the toggle.
        <span style={{ flex: 'none', paddingLeft: 12, paddingRight: 10, paddingTop: 13, alignSelf: 'start' }}>
          <Checkbox
            id={`select-${row.name}`}
            aria-label={`select ${title}`}
            isChecked={selected}
            isDisabled={row.deleting}
            onChange={(_e, checked) => onSelect(checked)}
          />
        </span>
      )}
      {guides}
      <div
        style={{
          flex: 1,
          minWidth: 0,
          display: 'grid',
          gridTemplateColumns: 'auto 1fr auto',
          alignItems: 'center',
          gap: 10,
          padding: '10px 12px',
          borderBottom: depth > 0 ? '1px solid var(--ao-canvas)' : undefined,
        }}
      >
      <CollapseToggle
        memberCount={memberCount}
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
          {/* Every row is exactly two lines: title + time, then chips + the
              menu. No snippet line — the last-message text added a third
              dimension of varying length that fought the chips for space,
              and the chip set itself (coordinator/source for a root, "via X"
              for a member) already says what a reader needs to decide
              whether to open the row. The native `title` attribute carries
              the FULL text for hover, since the visible text truncates. */}
          <span style={{ display: 'flex', alignItems: 'baseline', justifyContent: 'space-between', gap: 8 }}>
            <span
              title={title}
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
          <span style={{ display: 'flex', justifyContent: 'flex-end', gap: 6, marginTop: 4, flexWrap: 'wrap' }}>
            <RowBadges row={row} depth={depth} parentMissing={parentMissing} isRoot={isRoot} unread={unread} tag={tag} />
          </span>
        </span>
      </button>
      {/* Aligned to the END of the outer row (the chips' line), not centred
          across both lines, so the menu visually sits beside the chips
          rather than floating between the two lines. */}
      <span style={{ flex: 'none', alignSelf: 'end' }}>
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
      </div>
    </li>
  )
})
