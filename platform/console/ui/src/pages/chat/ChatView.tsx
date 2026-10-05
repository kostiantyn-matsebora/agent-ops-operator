import { useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { Button, Checkbox, SearchInput } from '@patternfly/react-core'
import { useConversations, usePipelineIcon, useSession, useSources } from '../../api/hooks'
import { useShell } from '../../shell'
import { Empty, ErrorState, Loading } from '../../components/States'
import {
  DEFAULT_LAYOUT, INBOX_COLLAPSED_WIDTH, MIN_INBOX_WIDTH, MIN_LIST_WIDTH, MIN_THREAD_WIDTH, useLayout,
} from './layout'
import { Splitter } from './Splitter'
import { Inbox, sameScope, type Scope } from './Inbox'
import { ConversationRow } from './ConversationRow'
import { SelectionBar } from './SelectionBar'
import { ThreadPane } from './ThreadPane'
import { QuickChips } from './QuickChips'
import { buildTree } from './tree'
import type { ConversationSummary } from '../../api/types'

// `console-chat-layout`: one view, four columns, switching in place.
// Composition from `prototype/C-rail.html` and `C-collapsed.html`.

/** Below this the view shows one column at a time (console-chat-layout: "The view fits narrow windows"). */
const NARROW_WIDTH = 900
const PAGE_SIZE = 100

function scopeParams(scope: Scope, search: string): URLSearchParams {
  const p = new URLSearchParams()
  if (scope.kind === 'unread') p.set('unread', 'true')
  else if (scope.kind === 'working') p.set('phase', 'Working')
  else if (scope.kind === 'mine') p.set('mine', 'true')
  else if (scope.kind === 'errored') p.set('errored', 'true')
  else if (scope.kind === 'closed') p.set('phase', 'Closed')
  else if (scope.kind === 'pipeline') p.set('pipeline', scope.name)
  // A Coordinator scope has no server-side filter param (the backend tracks
  // it only as a `scopes` count, console.go/convapi.go — not mine to add):
  // narrowed CLIENT-SIDE below instead of widening the request shape here.
  if (search) p.set('q', search)
  p.set('limit', String(PAGE_SIZE))
  return p
}

function useNarrow(): boolean {
  const [narrow, setNarrow] = useState(() => typeof window !== 'undefined' && window.innerWidth < NARROW_WIDTH)
  useEffect(() => {
    function onResize() {
      setNarrow(window.innerWidth < NARROW_WIDTH)
    }
    window.addEventListener('resize', onResize)
    return () => window.removeEventListener('resize', onResize)
  }, [])
  return narrow
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
function useArrivals(
  data: { items: ConversationSummary[] } | undefined,
  setNewNames: React.Dispatch<React.SetStateAction<Set<string>>>,
) {
  const seenNames = useRef<Set<string> | null>(null)
  useEffect(() => {
    if (!data) return
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
  }, [data, setNewNames])
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
function narrowItems<T extends { coordinator?: string; causedBy?: string; phase?: string }>(
  items: T[],
  scope: Scope,
  showClosed: boolean,
): T[] {
  let raw = items
  if (scope.kind === 'coordinator') {
    raw = raw.filter((c) => c.coordinator === scope.name || c.causedBy)
  }
  if (!showClosed && scope.kind !== 'closed') {
    raw = raw.filter((c) => c.phase !== 'Closed')
  }
  return raw
}

export function ChatView() {
  const { name } = useParams<{ name?: string }>()
  const navigate = useNavigate()
  const narrow = useNarrow()
  const [layout, updateLayout] = useLayout()
  const [scope, setScope] = useState<Scope>({ kind: 'all' })
  const [search, setSearch] = useState('')
  const [flatten, setFlatten] = useState(false)
  // No existing layout preference carries a per-scope toggle like this one,
  // so it is plain component state rather than a new persistence mechanism —
  // `layout.ts` owns only pane widths and the inbox's collapsed state.
  const [showClosed, setShowClosed] = useState(false)
  const [selectionMode, setSelectionMode] = useState(false)
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [highlighted, setHighlighted] = useState<string | undefined>()
  const [collapsedRoots, setCollapsedRoots] = useState<Set<string>>(new Set())
  const [newNames, setNewNames] = useState<Set<string>>(new Set())
  const rowRefs = useRef<Map<string, HTMLButtonElement>>(new Map())

  const params = useMemo(() => scopeParams(scope, search), [scope, search])
  const { data, isLoading, error } = useConversations(params)
  const session = useSession()
  const sources = useSources()
  const iconFor = usePipelineIcon()
  // Same gates `NewConversation.tsx` uses to decide whether its own button is
  // live — a starter chip here must offer nothing a conversation could not
  // actually be started from.
  const canWriteHere = session.data?.canWrite ?? false
  const canStartHere = Boolean(session.data?.canOriginate) && (sources.data?.sources ?? []).some((s) => s.wired)

  const items = useMemo(() => narrowItems(data?.items ?? [], scope, showClosed), [data, scope, showClosed])

  useArrivals(data, setNewNames)

  const tree = useMemo(() => buildTree(items, !flatten), [items, flatten])
  const rows = useMemo(() => visibleRows(tree, items, collapsedRoots), [tree, items, collapsedRoots])
  const collapsibleNames = useMemo(() => tree.filter((t) => t.memberCount > 0).map((t) => t.row.name), [tree])
  const anyCollapsed = collapsibleNames.some((n) => collapsedRoots.has(n))

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
    setSelected((prev) => {
      const next = new Set(prev)
      if (checked) next.add(rowName)
      else next.delete(rowName)
      return next
    })
  }

  function openRow(rowName: string, e?: React.MouseEvent) {
    if (e && (e.metaKey || e.ctrlKey)) {
      setSelectionMode(true)
      toggleSelect(rowName, !selected.has(rowName))
      return
    }
    clearNew(rowName)
    void navigate(`/conversations/${rowName}`)
  }

  // Attached natively: the workspace is a landmark, not a widget, and the
  // arrow-key list navigation is a shortcut layer over its rows.
  const workspaceRef = useRef<HTMLElement>(null)
  useEffect(() => {
    const el = workspaceRef.current
    if (!el) return
    el.addEventListener('keydown', onListKeyDown)
    return () => el.removeEventListener('keydown', onListKeyDown)
  })

  function onListKeyDown(e: Pick<KeyboardEvent, 'key' | 'preventDefault'>) {
    if (e.key === 'Escape') {
      setSelected(new Set())
      setSelectionMode(false)
      return
    }
    if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp' && e.key !== 'Enter') return
    const names = rows.map((r) => r.row.name)
    if (names.length === 0) return
    if (e.key === 'Enter') {
      if (highlighted) openRow(highlighted)
      return
    }
    e.preventDefault()
    const idx = highlighted ? names.indexOf(highlighted) : -1
    const next = e.key === 'ArrowDown' ? Math.min(idx + 1, names.length - 1) : Math.max(idx - 1, 0)
    const nextName = names[Math.max(next, 0)]
    setHighlighted(nextName)
    rowRefs.current.get(nextName)?.focus()
  }

  const selectableRows = items.filter((c) => !c.deleting).map((c) => c.name)
  const allSelected = selectableRows.length > 0 && selectableRows.every((n) => selected.has(n))

  const showingList = !narrow || !name
  const showingThread = !narrow || Boolean(name)
  const listWidth = layout.listWidth
  const inboxWidth = layout.inboxCollapsed ? INBOX_COLLAPSED_WIDTH : layout.inboxWidth

  return (
    <section
      ref={workspaceRef}
      data-testid="chat-view"
      aria-label="conversations workspace"
      tabIndex={-1}
      style={{ display: 'flex', flex: 1, minHeight: 0, minWidth: 0 }}
    >
      {showingList && (
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
          <div
            style={{
              width: narrow ? '100%' : listWidth, flex: narrow ? 1 : 'none', minWidth: 0,
              display: 'flex', flexDirection: 'column', borderRight: narrow ? undefined : '1px solid var(--ao-border)',
            }}
          >
            <div style={{ padding: '12px 12px 8px', display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8, borderBottom: '1px solid var(--ao-border)', flexWrap: 'wrap' }}>
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
                  onClick={() => setCollapsedRoots(anyCollapsed ? new Set() : new Set(collapsibleNames))}
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
                canWrite={session.data?.canWrite ?? false}
                hasReader={Boolean(session.data?.identity)}
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
                      ref={(el) => {
                        if (el) rowRefs.current.set(row.name, el)
                        else rowRefs.current.delete(row.name)
                      }}
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
                      collapsed={collapsedRoots.has(row.name)}
                      onToggleCollapse={() =>
                        setCollapsedRoots((prev) => {
                          const next = new Set(prev)
                          if (next.has(row.name)) next.delete(row.name)
                          else next.add(row.name)
                          return next
                        })
                      }
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
            {/* No thread is open yet, so there is nothing to insert a thread
                command into — only the starter chips apply, and choosing one
                opens the composer through the shared composer intent
                (`NewConversation.tsx` is the one place that listens). */}
            <QuickChips canWrite={canWriteHere} canStart={canStartHere} />
          </div>
        )
      ))}
    </section>
  )
}
