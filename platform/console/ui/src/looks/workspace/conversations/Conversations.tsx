import { type CSSProperties, type ReactElement, useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import type { ConversationSummary } from '../../../api/types'
import {
  listKeyAction, neighbourName, rowUnread, sameScope, scopeKey, useArrivals, useConversationList, useInboxScopes, type Scope,
} from '../../../model/conversationList'
import { useMarkRead, useMarkUnread, useReopen } from '../../../model/actions'
import { rowCommand, useCommands } from '../../../model/commands'
import { GENERAL_FEED_NAME, useConsoleSession } from '../../../model/session'
import { TESTID } from '../../../model/testids'
import { exitRuntime } from '../../../model/verbs'
import { Chevron } from '../icons'
import {
  DEFAULT_LIST_WIDTH, LIST_COLLAPSED_WIDTH, MIN_LIST_WIDTH, useArrangement, useWorkspaceLayout, useWorkspaceScope,
} from '../layout'
import { Splitter } from '../Splitter'
import { Row, type RowAction } from './Row'
import { SelectionBar } from './SelectionBar'
import { GeneralFeedPane, ThreadPane } from './Thread'

// THE CONVERSATIONS PAGE: the list and the thread side by side, the
// navigation panel beside them (the shell's). Opening a conversation replaces
// the thread and never leaves the view. At phone width one pane shows at a
// time, with a way back from the thread.

const SCOPE_LABEL: Record<Scope['kind'], string> = {
  all: 'All conversations',
  working: 'Working',
  mine: 'Mine',
  errored: 'Errored',
  pipeline: '',
  coordinator: '',
  closed: 'Archive',
  general: 'General feed',
}

export function scopeTitle(scope: Scope): string {
  return 'name' in scope ? scope.name : SCOPE_LABEL[scope.kind]
}

interface RowActionDeps {
  canWrite: boolean
  hasReader: boolean
  reopen: (name: string) => void
  markRead: (name: string) => void
  markUnread: (name: string) => void
  select: (name: string) => void
}

/** What each row's menu offers. One that the state or the viewer's rights do not allow is absent, never greyed out. */
function rowActions(row: ConversationSummary, d: RowActionDeps): RowAction[] {
  const link = `/conversations/${encodeURIComponent(row.name)}`
  const actions: RowAction[] = []
  const member = Boolean(row.causedBy)
  // A member holds no channel of its own: only navigation applies to it.
  if (!member) {
    if (rowUnread(row)) actions.push({ label: 'Mark read', run: () => d.markRead(row.name) })
    else if (d.hasReader) actions.push({ label: 'Mark unread', run: () => d.markUnread(row.name) })
  }
  actions.push({ label: 'Open in new tab', run: () => void window.open(link, '_blank', 'noopener') })
  actions.push({ label: 'Copy link', run: () => void navigator.clipboard?.writeText(new URL(link, window.location.origin).href) })
  if (!d.canWrite || row.deleting || member) return actions
  if (row.phase === 'Closed') {
    actions.push({ label: 'Reopen', run: () => d.reopen(row.name) })
    actions.push({ label: 'Delete', run: () => d.select(row.name) })
    return actions
  }
  if (row.runtimePod) actions.push({ label: 'Exit runtime', run: () => void exitRuntime(row.name) })
  if (row.joined) actions.push({ label: 'Close', run: () => d.select(row.name) })
  return actions
}

/** The scope chips shown in the list's head where the navigation panel is a drawer. */
function ScopeChips({ scope, showClosed, onPick }: Readonly<{ scope: Scope; showClosed: boolean; onPick: (s: Scope) => void }>) {
  const { fixed, closed } = useInboxScopes(showClosed)
  return (
    <div className="wk-scopes" role="group" aria-label="Conversation scopes">
      {[...fixed, closed].map((e) => (
        <button
          key={scopeKey(e.scope)}
          type="button"
          aria-pressed={sameScope(scope, e.scope)}
          onClick={() => onPick(e.scope)}
        >
          {e.label}
          {e.count !== undefined ? ` ${e.count}` : ''}
        </button>
      ))}
    </div>
  )
}

function ListPane({ current, collapsed }: Readonly<{ current?: string; collapsed: boolean }>) {
  const scope = useWorkspaceScope((s) => s.scope)
  const setScope = useWorkspaceScope((s) => s.setScope)
  const { showClosed, flatten, update } = useWorkspaceLayout()
  const [search, setSearch] = useState('')
  const [folded, setFolded] = useState<Set<string>>(new Set())
  const seen = useRef<Set<string>>(new Set())
  const session = useConsoleSession()
  const canWrite = Boolean(session.data?.canWrite)
  const hasReader = Boolean(session.data?.identity)
  const commands = useCommands()
  const reopen = useReopen()
  const markRead = useMarkRead()
  const markUnread = useMarkUnread()
  // Selecting is a mode the viewer enters: rows carry a box and opening one
  // toggles it. The selection never leaves this page's rows.
  const [selecting, setSelecting] = useState(false)
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const toggleSelected = (name: string) =>
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(name)) next.delete(name)
      else next.add(name)
      return next
    })
  const stopSelecting = () => {
    setSelecting(false)
    setSelected(new Set())
  }

  const list = useConversationList({ scope, search, showClosed, flatten, collapsedRoots: folded })
  const { newNames, clear } = useArrivals(list.data, scopeKey(scope))

  // A root with members starts folded, as it is first seen — a later arrival
  // too — and stays however the viewer leaves it.
  useEffect(() => {
    const fresh = list.collapsibleNames.filter((n) => !seen.current.has(n))
    if (fresh.length === 0) return
    for (const n of fresh) seen.current.add(n)
    setFolded((prev) => new Set([...prev, ...fresh]))
  }, [list.collapsibleNames])

  useEffect(() => {
    if (current) clear(current)
    // `clear` is a fresh function each render; the name is what decides.
  }, [current])

  // A selection never holds a row that left the page (a filter, a scope, a delete).
  useEffect(() => {
    const here = new Set(list.items.map((c) => c.name))
    setSelected((prev) => (prev.size === 0 || [...prev].every((n) => here.has(n)) ? prev : new Set([...prev].filter((n) => here.has(n)))))
  }, [list.items])

  const deps: RowActionDeps = {
    canWrite,
    hasReader,
    reopen: (name) => reopen.mutate(name),
    markRead: (name) => markRead.mutate({ names: [name] }),
    markUnread: (name) => markUnread.mutate({ names: [name] }),
    select: (name) => {
      setSelecting(true)
      setSelected(new Set([name]))
    },
  }

  const anyFolded = list.collapsibleNames.some((n) => folded.has(n))
  const toggle = (name: string) =>
    setFolded((prev) => {
      const next = new Set(prev)
      if (next.has(name)) next.delete(name)
      else next.add(name)
      return next
    })

  return (
    <section
      className={`wk-pane lp${collapsed ? ' collapsed' : ''}`}
      aria-label="Conversations"
      data-testid={collapsed ? TESTID.inboxCollapsed : TESTID.inbox}
    >
      <div className="wk-ph">
        <h2>{scopeTitle(scope)}</h2>
        <button
          type="button"
          className="wk-icon-btn collapse-list"
          aria-label={collapsed ? 'Expand the conversation list' : 'Collapse the conversation list'}
          title={collapsed ? 'Expand the conversation list' : 'Collapse the conversation list'}
          aria-expanded={!collapsed}
          onClick={() => update({ listCollapsed: !collapsed })}
        >
          <Chevron dir={collapsed ? 'doubleRight' : 'doubleLeft'} />
        </button>
      </div>
      <div className="wk-search">
        <input
          className="wk-field"
          type="search"
          placeholder="name or title"
          aria-label="Search conversations"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
      </div>
      <ScopeChips scope={scope} showClosed={showClosed} onPick={setScope} />
      <div className="wk-tools">
        <button type="button" className="wk-btn link" onClick={() => update({ flatten: !flatten })}>
          {flatten ? 'nest' : 'flatten'}
        </button>
        {!flatten && list.collapsibleNames.length > 0 && (
          <button
            type="button"
            className="wk-btn link"
            onClick={() => setFolded(anyFolded ? new Set() : new Set(list.collapsibleNames))}
          >
            {anyFolded ? 'Expand all' : 'Collapse all'}
          </button>
        )}
        {!collapsed && (
          <button
            type="button"
            className="wk-btn link"
            aria-pressed={selecting}
            onClick={() => (selecting ? stopSelecting() : setSelecting(true))}
          >
            {selecting ? 'Done selecting' : 'Select'}
          </button>
        )}
        <label>
          <input
            type="checkbox"
            data-testid={TESTID.showClosed}
            checked={showClosed}
            onChange={(e) => update({ showClosed: e.target.checked })}
          />{' '}
          Show closed
        </label>
      </div>
      {selecting && !collapsed && (
        <SelectionBar
          items={list.items}
          selected={selected}
          canWrite={canWrite}
          hasReader={hasReader}
          onClear={() => setSelected(new Set())}
          onDone={stopSelecting}
        />
      )}
      <ul
        className="wk-rows"
        aria-label={scopeTitle(scope)}
        onKeyDown={(e) => {
          // Arrows move between rows, Enter opens the focused one (a link does so natively),
          // Escape ends a selection.
          const links = [...e.currentTarget.querySelectorAll<HTMLAnchorElement>('.wk-row .open')]
          const here = links.findIndex((l) => l === document.activeElement)
          const action = listKeyAction(e.key, links.length, here >= 0 ? String(here) : undefined)
          if (action === 'clear') {
            if (selecting) stopSelecting()
          } else if (action === 'down' || action === 'up') {
            e.preventDefault()
            const next = neighbourName(links.map((_, i) => String(i)), here >= 0 ? String(here) : undefined, action === 'down')
            links[Number(next)]?.focus()
          }
        }}
      >
        {list.isLoading && !list.data && <li className="wk-empty" aria-busy="true">Loading conversations…</li>}
        {Boolean(list.error) && !list.data && (
          <li className="wk-empty">
            <h2>Could not load conversations</h2>
            {String(list.error)}
          </li>
        )}
        {list.data && list.rows.length === 0 && (
          <li className="wk-empty">
            <h2>No conversations match</h2>
            {list.data.total === 0
              ? 'Nothing has started a conversation yet.'
              : showClosed
                ? 'Every conversation was filtered out.'
                : 'Every conversation here is closed, and closed ones are hidden: turn on Show closed.'}
          </li>
        )}
        {list.rows.map((t) => (
          <Row
            key={t.row.name}
            tree={t}
            current={t.row.name === current}
            arrived={newNames.has(t.row.name)}
            folded={!flatten && t.memberCount > 0 ? folded.has(t.row.name) : undefined}
            onToggleFold={() => toggle(t.row.name)}
            actions={rowActions(t.row, deps)}
            iconOnly={collapsed}
            command={rowCommand(commands.latest(t.row.name))}
            onRetry={() => {
              const c = commands.latest(t.row.name)
              if (c) void commands.retry(c.id)
            }}
            onDismissCommand={() => {
              const c = commands.latest(t.row.name)
              if (c) commands.dismiss(c.id)
            }}
            onMultiSelect={collapsed ? undefined : () => deps.select(t.row.name)}
            selection={selecting && !collapsed ? { selected: selected.has(t.row.name), onToggle: () => toggleSelected(t.row.name) } : undefined}
          />
        ))}
      </ul>
    </section>
  )
}

export function ConversationsPage(): ReactElement {
  const { name } = useParams<{ name?: string }>()
  const navigate = useNavigate()
  const arrangement = useArrangement()
  const { listWidth, listCollapsed, update } = useWorkspaceLayout()
  const single = arrangement === 'single'
  const collapsed = listCollapsed && !single
  const back = single ? () => void navigate('/conversations') : undefined

  const style = useMemo(
    () => ({ '--wk-listw': `${collapsed ? LIST_COLLAPSED_WIDTH : listWidth}px` }) as CSSProperties,
    [collapsed, listWidth],
  )

  let thread: ReactElement
  if (name === GENERAL_FEED_NAME) thread = <GeneralFeedPane onBack={back} />
  else if (name) thread = <ThreadPane name={name} onBack={back} />
  else {
    thread = (
      <div className="wk-empty">
        <h2>Select a conversation</h2>
        Choose one from the list, or start a new one.
      </div>
    )
  }

  return (
    <div className="wk-conv" data-testid={TESTID.chatView} data-view={name ? 'thread' : 'list'} style={style}>
      <ListPane current={name} collapsed={collapsed} />
      <Splitter
        label="Resize the conversation list"
        width={listWidth}
        min={MIN_LIST_WIDTH}
        defaultWidth={DEFAULT_LIST_WIDTH}
        disabled={collapsed}
        onChange={(w) => update({ listWidth: w })}
      />
      <section className="wk-pane tp" aria-label="Conversation" data-thread={name ?? ''}>
        {thread}
      </section>
    </div>
  )
}
