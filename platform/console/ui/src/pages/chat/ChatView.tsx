import { useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { Button, Checkbox, SearchInput } from '@patternfly/react-core'
import { BarsIcon } from '@patternfly/react-icons'
import {
  useConversations, useMarkRead, useMarkUnread, usePipelineIcon, useReopenConversation, useSession,
} from '../../api/hooks'
import { api } from '../../api/client'
import { useShell } from '../../shell'
import { Empty, ErrorState, Loading } from '../../components/States'
import {
  DEFAULT_LAYOUT, INBOX_COLLAPSED_WIDTH, MIN_INBOX_WIDTH, MIN_LIST_WIDTH, MIN_THREAD_WIDTH, useLayout,
} from './layout'
import { Splitter } from './Splitter'
import { Inbox, sameScope, scopeKey, type Scope } from './Inbox'
import { ConversationRow } from './ConversationRow'
import { SelectionBar } from './SelectionBar'
import { ThreadPane } from './ThreadPane'
import { buildTree, rootNameOf } from './tree'
import type { ConversationSummary } from '../../api/types'

// `console-chat-layout`: one view, four columns, switching in place.
// Composition from `prototype/C-rail.html` and `C-collapsed.html`.

/** Below this the view shows one column at a time (console-chat-layout: "The view fits narrow windows"). */
const NARROW_WIDTH = 900
/** Below this even the Inbox panel (MIN_INBOX_WIDTH) no longer fits beside the
 * list (MIN_LIST_WIDTH) inside the remaining width once the app shell's own
 * rail is accounted for — a true phone width, not merely a narrowed window. */
const MOBILE_WIDTH = 560
const PAGE_SIZE = 100

function scopeParams(scope: Scope, search: string): URLSearchParams {
  const p = new URLSearchParams()
  if (scope.kind === 'closed') p.set('phase', 'Closed')
  else if (scope.kind === 'pipeline') p.set('pipeline', scope.name)
  // A Coordinator scope has no server-side filter param (the backend tracks
  // it only as a `scopes` count, console.go/convapi.go — not mine to add):
  // narrowed CLIENT-SIDE below instead of widening the request shape here.
  //
  // Unread, Working, Errored and Mine are the SAME shape of problem: each is
  // an independent per-row predicate server-side (convapi.go's `matches`),
  // with no tree awareness at all. `?mine=true` kept a root but dropped every
  // member (a member is never Mine — it was invoked, never originated, so it
  // carries no OriginReader). `?unread=true` would just as readily keep an
  // unread MEMBER while dropping its own root, or the reverse — either way
  // the tree has nothing to attach to. All four are narrowed CLIENT-SIDE
  // below instead: if anything anywhere in a tree matches, the WHOLE tree
  // shows, each row rendering its own real status regardless of why the tree
  // qualified (a root can show up already read, because the member beside it
  // is what made the tree match).
  if (search) p.set('q', search)
  p.set('limit', String(PAGE_SIZE))
  return p
}

function useBelowWidth(px: number): boolean {
  const [below, setBelow] = useState(() => typeof window !== 'undefined' && window.innerWidth < px)
  useEffect(() => {
    function onResize() {
      setBelow(window.innerWidth < px)
    }
    window.addEventListener('resize', onResize)
    return () => window.removeEventListener('resize', onResize)
  }, [px])
  return below
}

function useNarrow(): boolean {
  return useBelowWidth(NARROW_WIDTH)
}

/** Below this the Inbox panel can no longer fit beside the list at all —
 * measured live at 375px, where rendering both overflowed the viewport and
 * left the Inbox's own text bleeding behind the list. It becomes a
 * full-screen drawer instead of a column. */
function useMobile(): boolean {
  return useBelowWidth(MOBILE_WIDTH)
}

/**
 * Expands a per-row predicate into a per-TREE one: if anything anywhere in a
 * tree matches, every row of that tree is kept — never just the matching row
 * in isolation, which is what left a "Mine" root with none of its own
 * members, and would do the same to Unread/Working/Errored. Each kept row
 * still renders its own real status; this only decides whether the tree
 * shows at all.
 */
function withTreeContext(
  raw: ConversationSummary[],
  matches: (c: ConversationSummary) => boolean,
): ConversationSummary[] {
  const byName = new Map(raw.map((c) => [c.name, c]))
  const rootNameOf = (c: ConversationSummary): string => {
    let cur = c
    for (let hop = 0; cur.causedBy && hop < 64; hop++) {
      const parent = byName.get(cur.causedBy.parent)
      if (!parent) break
      cur = parent
    }
    return cur.name
  }
  const qualifyingRoots = new Set<string>()
  for (const c of raw) {
    if (matches(c)) qualifyingRoots.add(rootNameOf(c))
  }
  return raw.filter((c) => qualifyingRoots.has(rootNameOf(c)))
}

/** Rows whose ancestor chain passes through a collapsed root are hidden — the caret, not a filter. */
function visibleRows(
  tree: ReturnType<typeof buildTree>,
  items: ConversationSummary[],
  collapsedRoots: Set<string>,
): ReturnType<typeof buildTree> {
  const byName = new Map(items.map((c) => [c.name, c]))
  return tree.filter(({ row }) => {
    let cur: ConversationSummary | undefined = row
    while (cur?.causedBy) {
      if (collapsedRoots.has(cur.causedBy.parent)) return false
      cur = byName.get(cur.causedBy.parent)
    }
    return true
  })
}

function dropName(set: Set<string>, name: string): Set<string> {
  if (!set.has(name)) return set
  const next = new Set(set)
  next.delete(name)
  return next
}

// Arrivals move (console-thread-live-cues): a name the previous snapshot did
// not have gets the tint and the "new" tag — the row itself, plus the inbox's
// own unread badges, are the whole of the signal. A popup for every arrival
// was removed: it fired once per row on every page that happened to be open,
// which is spam rather than a cue. `seenNames` starts `null` so the FIRST
// load never fires one.
//
// `scopeKey` resets that "first load" state per SCOPE. Without it, switching
// from a narrow scope (a few rows) to a wider one (e.g. "All") compared the
// wider scope's full list against the narrow scope's small `seenNames` set —
// every row the narrow scope never had looked like a fresh arrival, so
// opening "All" tagged the entire list "new" even though nothing had.
function useArrivals(
  data: { items: ConversationSummary[] } | undefined,
  setNewNames: React.Dispatch<React.SetStateAction<Set<string>>>,
  scopeKey: string,
) {
  const seenNames = useRef<Set<string> | null>(null)
  const seenScope = useRef<string | null>(null)
  useEffect(() => {
    if (!data) return
    if (seenScope.current !== scopeKey) {
      seenScope.current = scopeKey
      seenNames.current = null
    }
    const current = new Set(data.items.map((c) => c.name))
    const prevSeen = seenNames.current
    seenNames.current = current
    if (prevSeen === null) return
    const arrived = data.items.filter((c) => !prevSeen.has(c.name))
    if (arrived.length === 0) return
    setNewNames((prev) => new Set([...prev, ...arrived.map((c) => c.name)]))
    for (const c of arrived) {
      setTimeout(() => setNewNames((prev) => dropName(prev, c.name)), 4000)
    }
  }, [data, setNewNames, scopeKey])
}

function ListBody({
  isLoading, error, data, empty, children,
}: Readonly<{
  isLoading: boolean
  error: unknown
  data: { total: number } | undefined
  empty: boolean
  children: React.ReactNode
}>) {
  if (isLoading && !data) return <Loading />
  if (error || !data) return <ErrorState title="Could not load conversations">{String(error)}</ErrorState>
  if (empty) {
    return (
      <Empty title="No conversations match">
        {data.total > 0 ? 'Every conversation was filtered out.' : 'Nothing has originated yet.'}
      </Empty>
    )
  }
  return <>{children}</>
}

/**
 * The list's client-side narrowing. The coordinator scope keeps its own
 * members. Closed conversations are hidden everywhere except the dedicated
 * Closed scope, where showing them is the whole point — narrowed CLIENT-SIDE
 * since there is no server-side "exclude closed" param to ask for instead.
 */
/** The row an arrow key lands on, clamped to the list's ends. */
function neighbourName(names: string[], current: string | undefined, down: boolean): string {
  const idx = current ? names.indexOf(current) : -1
  const next = down ? Math.min(idx + 1, names.length - 1) : Math.max(idx - 1, 0)
  return names[Math.max(next, 0)]
}

type ListKeyAction = 'clear' | 'open' | 'down' | 'up' | 'none'

/** What a key does on the list: Escape clears, arrows move, Enter opens the highlighted row. */
export function listKeyAction(key: string, rowCount: number, highlighted: string | undefined): ListKeyAction {
  if (key === 'Escape') return 'clear'
  if (rowCount === 0) return 'none'
  if (key === 'Enter') return highlighted ? 'open' : 'none'
  if (key === 'ArrowDown') return 'down'
  if (key === 'ArrowUp') return 'up'
  return 'none'
}

/** A modifier-click on a row toggles its selection instead of opening it. */
function isMultiSelectClick(e: React.MouseEvent | undefined): boolean {
  return Boolean(e && (e.metaKey || e.ctrlKey))
}

/** Keeps the name → button map in step with mounting and unmounting rows. */
function trackRowRef(refs: Map<string, HTMLButtonElement>, rowName: string, el: HTMLButtonElement | null) {
  if (el) refs.set(rowName, el)
  else refs.delete(rowName)
}

/** A copy of the set with the name added when absent, removed when present. */
function toggleName(names: Set<string>, rowName: string): Set<string> {
  const next = new Set(names)
  if (next.has(rowName)) next.delete(rowName)
  else next.add(rowName)
  return next
}

/** A copy of the set with the name present or absent, as asked. */
function withMembership(names: Set<string>, rowName: string, present: boolean): Set<string> {
  const next = new Set(names)
  if (present) next.add(rowName)
  else next.delete(rowName)
  return next
}

/** The rows a bulk action may touch, and whether every one is selected. */
function selectionState(items: ConversationSummary[], selected: Set<string>) {
  const selectableRows = items.filter((c) => !c.deleting).map((c) => c.name)
  const allSelected = selectableRows.length > 0 && selectableRows.every((n) => selected.has(n))
  return { selectableRows, allSelected }
}

/** The inbox pane's width, the narrow strip when it is collapsed. */
function inboxPaneWidth(layout: { inboxCollapsed: boolean; inboxWidth: number }): number {
  return layout.inboxCollapsed ? INBOX_COLLAPSED_WIDTH : layout.inboxWidth
}

// `/exit` is fully recoverable (invariants.md: "`/exit` RELEASES THE
// RUNTIME") so it needs no confirmation and runs the same way a thread
// command chip does — an ordinary message, never a dedicated endpoint.
function exitRuntime(rowName: string) {
  void api.send(rowName, '/exit')
}

export function ChatView() {
  const { name } = useParams<{ name?: string }>()
  const navigate = useNavigate()
  const narrow = useNarrow()
  const [layout, updateLayout] = useLayout()
  const [scope, setScope] = useState<Scope>({ kind: 'all' })
  // The Inbox panel is its own fixed-width column ALWAYS, which fits beside a
  // narrowed list down to the 900px breakpoint but not below it: at an actual
  // phone width the Inbox and the list cannot both fit, and rendering both
  // anyway overflowed the viewport and left the Inbox's own text bleeding
  // behind the list (measured live at 375px). Below a second, narrower
  // breakpoint the Inbox becomes a full-screen drawer instead of a column,
  // opened by one button and closed the moment a scope is picked.
  const mobile = useMobile()
  const [mobileInboxOpen, setMobileInboxOpen] = useState(false)
  const [search, setSearch] = useState('')
  const [flatten, setFlatten] = useState(false)
  // Persisted in `layout.ts`, same as the pane widths and the inbox's
  // collapsed state — a viewer preference, never conversation state.
  const showClosed = layout.showClosed
  const setShowClosed = (v: boolean) => updateLayout({ showClosed: v })
  const [selectionMode, setSelectionMode] = useState(false)
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [highlighted, setHighlighted] = useState<string | undefined>()
  const [collapsedRoots, setCollapsedRoots] = useState<Set<string>>(new Set())
  const [newNames, setNewNames] = useState<Set<string>>(new Set())
  const rowRefs = useRef<Map<string, HTMLButtonElement>>(new Map())
  // Every root name this component has ever applied the persisted fold
  // default to — so a root the viewer explicitly expanded (or collapsed) by
  // hand is never silently re-folded just because the tree re-rendered.
  const seededRootsRef = useRef<Set<string>>(new Set())

  const params = useMemo(() => scopeParams(scope, search), [scope, search])
  const { data, isLoading, error } = useConversations(params)
  const session = useSession()
  const iconFor = usePipelineIcon()
  const canWrite = session.data?.canWrite ?? false
  const hasReader = Boolean(session.data?.identity)
  const reopen = useReopenConversation()
  const markRead = useMarkRead()
  const markUnread = useMarkUnread()

  const items = useMemo(() => {
    let raw = data?.items ?? []
    if (scope.kind === 'coordinator') {
      // `c.causedBy` truthy alone is not enough — it admits a member of ANY
      // coordinator, not just this scope's. A member carries no field naming
      // which coordinator it ultimately belongs to, only its immediate
      // parent (`causedBy.parent`), so membership is decided by walking to
      // the uncaused root — within the UNFILTERED snapshot, since the
      // walk needs every ancestor still present — and checking THAT root's
      // own `coordinator` field.
      const byName = new Map(raw.map((c) => [c.name, c]))
      raw = raw.filter((c) => byName.get(rootNameOf(raw, c))?.coordinator === scope.name)
    }
    const TREE_PREDICATE: Partial<Record<Scope['kind'], (c: ConversationSummary) => boolean>> = {
      unread: (c) => Boolean(c.unread),
      working: (c) => c.phase === 'Working',
      errored: (c) => Boolean(c.errored),
      mine: (c) => Boolean(c.mine),
    }
    const predicate = TREE_PREDICATE[scope.kind]
    if (predicate) {
      raw = withTreeContext(raw, predicate)
    }
    // Closed conversations are hidden by default everywhere except the
    // dedicated Closed scope, where showing them is the whole point —
    // narrowed CLIENT-SIDE, like the coordinator scope above, since there is
    // no server-side "exclude closed" param to ask for instead.
    if (!showClosed && scope.kind !== 'closed') {
      raw = raw.filter((c) => c.phase !== 'Closed')
    }
    return raw
  }, [data, scope, showClosed])

  useArrivals(data, setNewNames, scopeKey(scope))

  const tree = useMemo(() => buildTree(items, !flatten), [items, flatten])
  const rows = useMemo(() => visibleRows(tree, items, collapsedRoots), [tree, items, collapsedRoots])
  const collapsibleNames = useMemo(() => tree.filter((t) => t.memberCount > 0).map((t) => t.row.name), [tree])
  const anyCollapsed = collapsibleNames.some((n) => collapsedRoots.has(n))

  // Applies the persisted fold default to every root the FIRST time it is
  // seen — on mount (item 2's "first time" requirement) and again for any
  // root that arrives later, so a reload or a live arrival both land on the
  // same preference. A root already seeded is never touched again here, so
  // expanding or collapsing one by hand sticks until the viewer changes it.
  useEffect(() => {
    const seeded = seededRootsRef.current
    const fresh = collapsibleNames.filter((n) => !seeded.has(n))
    if (fresh.length === 0) return
    for (const n of fresh) seeded.add(n)
    if (!layout.treeCollapsedByDefault) return
    setCollapsedRoots((prev) => {
      const next = new Set(prev)
      for (const n of fresh) next.add(n)
      return next
    })
  }, [collapsibleNames, layout.treeCollapsedByDefault])

  // The PatternFly sidebar collapses to icons on this view (design D-J),
  // restored to whatever it was on leaving.
  useEffect(() => {
    const prev = useShell.getState().navCollapsed
    useShell.getState().setNavCollapsed(true)
    return () => useShell.getState().setNavCollapsed(prev)
  }, [])

  function clearNew(rowName: string) {
    setNewNames((prev) => dropName(prev, rowName))
  }

  function toggleSelect(rowName: string, checked: boolean) {
    setSelected((prev) => withMembership(prev, rowName, checked))
  }

  function openRow(rowName: string, e?: React.MouseEvent) {
    if (isMultiSelectClick(e)) {
      setSelectionMode(true)
      toggleSelect(rowName, !selected.has(rowName))
      return
    }
    clearNew(rowName)
    void navigate(`/conversations/${rowName}`)
  }

  // `/close` and Delete are both destructive and already have a full,
  // confirmed flow in SelectionBar's own modals — this hands the row to that
  // SAME flow (select it, enter selection mode) rather than building a
  // second confirmation here.
  function closeRow(rowName: string) {
    setSelectionMode(true)
    setSelected(new Set([rowName]))
  }

  function deleteRow(rowName: string) {
    setSelectionMode(true)
    setSelected(new Set([rowName]))
  }

  // Attached natively: the workspace is a landmark, not a widget, and the
  // arrow-key list navigation is a shortcut layer over its rows.
  //
  // `onListKeyDown` closes over this render's own `rows`/`highlighted`/
  // `selected` and is a plain function (not `useCallback`), so it is a new
  // value every render. Reading it through a ref keeps the LISTENER itself
  // stable — attached once per mount — rather than detaching and reattaching
  // the real DOM listener on every render just to pick up a fresh closure.
  const workspaceRef = useRef<HTMLElement>(null)
  const onListKeyDownRef = useRef<(e: Pick<KeyboardEvent, 'key' | 'preventDefault'>) => void>(() => undefined)
  onListKeyDownRef.current = onListKeyDown
  useEffect(() => {
    const el = workspaceRef.current
    if (!el) return
    const handler = (e: KeyboardEvent) => onListKeyDownRef.current(e)
    el.addEventListener('keydown', handler)
    return () => el.removeEventListener('keydown', handler)
  }, [])

  function onListKeyDown(e: Pick<KeyboardEvent, 'key' | 'preventDefault'>) {
    const action = listKeyAction(e.key, rows.length, highlighted)
    if (action === 'clear') {
      setSelected(new Set())
      setSelectionMode(false)
    } else if (action === 'open') {
      openRow(highlighted as string)
    } else if (action === 'down' || action === 'up') {
      e.preventDefault()
      const nextName = neighbourName(
        rows.map((r) => r.row.name),
        highlighted,
        action === 'down',
      )
      setHighlighted(nextName)
      rowRefs.current.get(nextName)?.focus()
    }
  }

  const { selectableRows, allSelected } = selectionState(items, selected)

  const showingList = !narrow || !name
  const showingThread = !narrow || Boolean(name)
  const listWidth = layout.listWidth
  const inboxWidth = inboxPaneWidth(layout)

  return (
    <section
      ref={workspaceRef}
      data-testid="chat-view"
      aria-label="conversations workspace"
      tabIndex={-1}
      style={{ display: 'flex', flex: 1, minHeight: 0, minWidth: 0 }}
    >
      {showingList && mobile && mobileInboxOpen && (
        <div data-testid="mobile-inbox-drawer" style={{ width: '100%', flex: 1, minWidth: 0, display: 'flex', flexDirection: 'column' }}>
          <Inbox
            activeScope={scope}
            onSelectScope={(s) => {
              setScope(s)
              setMobileInboxOpen(false)
            }}
            collapsed={false}
            onToggleCollapsed={() => undefined}
          />
        </div>
      )}
      {showingList && !(mobile && mobileInboxOpen) && (
        <>
          {!mobile && (
            <>
              <div style={{ width: inboxWidth, flex: 'none', overflow: 'hidden', borderRight: '1px solid var(--ao-border)' }}>
                <Inbox
                  activeScope={scope}
                  onSelectScope={setScope}
                  collapsed={layout.inboxCollapsed}
                  onToggleCollapsed={() => updateLayout({ inboxCollapsed: !layout.inboxCollapsed })}
                />
              </div>
              {!narrow && (
                <Splitter
                  width={inboxWidth}
                  min={MIN_INBOX_WIDTH}
                  defaultWidth={DEFAULT_LAYOUT.inboxWidth}
                  ariaLabel="resize the inbox"
                  onChange={(w) => updateLayout({ inboxWidth: w, inboxCollapsed: false })}
                />
              )}
            </>
          )}
          <div
            style={{
              width: narrow ? '100%' : listWidth, flex: narrow ? 1 : 'none', minWidth: 0,
              display: 'flex', flexDirection: 'column', borderRight: narrow ? undefined : '1px solid var(--ao-border)',
            }}
          >
            <div style={{ padding: '12px 12px 8px', display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8, borderBottom: '1px solid var(--ao-border)', flexWrap: 'wrap' }}>
              {mobile && (
                <Button variant="plain" aria-label="conversation filters" onClick={() => setMobileInboxOpen(true)}>
                  <BarsIcon />
                </Button>
              )}
              <strong>{sameScope(scope, { kind: 'all' }) ? 'All conversations' : 'Conversations'}</strong>
              <SearchInput
                aria-label="search conversations"
                placeholder="name or title"
                value={search}
                onChange={(_e, v) => setSearch(v)}
                onClear={() => setSearch('')}
              />
              <Button variant="link" isInline onClick={() => setFlatten((v) => !v)}>
                {flatten ? 'group by incident' : 'flatten'}
              </Button>
              {collapsibleNames.length > 0 && (
                <Button
                  variant="link"
                  isInline
                  onClick={() => {
                    const collapsing = !anyCollapsed
                    setCollapsedRoots(collapsing ? new Set(collapsibleNames) : new Set())
                    // What was just clicked becomes the fold every root —
                    // present or still to arrive — follows from here on.
                    updateLayout({ treeCollapsedByDefault: collapsing })
                  }}
                >
                  {anyCollapsed ? 'Expand all' : 'Collapse all'}
                </Button>
              )}
              <Button
                variant="link"
                isInline
                onClick={() => {
                  setSelectionMode((v) => !v)
                  setSelected(new Set())
                }}
              >
                {selectionMode ? 'Done selecting' : 'Select'}
              </Button>
              {selectionMode && selectableRows.length > 0 && (
                <Button
                  variant="link"
                  isInline
                  onClick={() => setSelected(allSelected ? new Set() : new Set(selectableRows))}
                >
                  {allSelected ? 'Clear all' : 'Select all'}
                </Button>
              )}
              {scope.kind !== 'closed' && (
                <Checkbox
                  id="show-closed"
                  label="Show closed"
                  isChecked={showClosed}
                  onChange={(_e, checked) => setShowClosed(checked)}
                />
              )}
            </div>
            {selectionMode && (
              <SelectionBar
                items={items}
                selected={selected}
                canWrite={canWrite}
                hasReader={hasReader}
                onClear={() => setSelected(new Set())}
                onDone={() => setSelected(new Set())}
              />
            )}
            <div style={{ flex: 1, minHeight: 0, overflowY: 'auto' }}>
              <ListBody isLoading={isLoading} error={error} data={data} empty={rows.length === 0}>
                <ul style={{ listStyle: 'none', margin: 0, padding: 0 }} aria-label="conversations">
                  {rows.map(({ row, depth, memberCount, parentMissing }) => (
                    <ConversationRow
                      key={row.name}
                      ref={(el) => trackRowRef(rowRefs.current, row.name, el)}
                      row={row}
                      depth={depth}
                      memberCount={memberCount}
                      parentMissing={parentMissing}
                      isNew={newNames.has(row.name)}
                      selectionMode={selectionMode}
                      selected={selected.has(row.name)}
                      highlighted={highlighted === row.name || name === row.name}
                      onSelect={(checked) => toggleSelect(row.name, checked)}
                      onOpen={(e) => openRow(row.name, e)}
                      pipelineIcon={iconFor(row.pipeline)}
                      canWrite={canWrite}
                      hasReader={hasReader}
                      onMarkRead={() => markRead.mutate({ names: [row.name] })}
                      onMarkUnread={() => markUnread.mutate({ names: [row.name] })}
                      onReopen={() => reopen.mutate(row.name)}
                      onExitRuntime={() => exitRuntime(row.name)}
                      onClose={() => closeRow(row.name)}
                      onDelete={() => deleteRow(row.name)}
                      collapsed={collapsedRoots.has(row.name)}
                      onToggleCollapse={() => setCollapsedRoots((prev) => toggleName(prev, row.name))}
                    />
                  ))}
                </ul>
              </ListBody>
            </div>
          </div>
        </>
      )}
      {!narrow && name && (
        <Splitter
          width={listWidth}
          min={MIN_LIST_WIDTH}
          max={typeof window !== 'undefined' ? Math.max(MIN_LIST_WIDTH, window.innerWidth - inboxWidth - MIN_THREAD_WIDTH) : undefined}
          defaultWidth={DEFAULT_LAYOUT.listWidth}
          ariaLabel="resize the list"
          onChange={(w) => updateLayout({ listWidth: w })}
        />
      )}
      {showingThread && (name ? (
        <ThreadPane name={name} onBack={narrow ? () => navigate('/conversations') : undefined} />
      ) : (
        !narrow && (
          <div style={{ flex: 1, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', gap: 16 }}>
            <Empty title="Select a conversation">Choose one from the list, or start a new one.</Empty>
          </div>
        )
      ))}
    </section>
  )
}
