import { useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { Alert, AlertGroup, Button, SearchInput } from '@patternfly/react-core'
import { useConversations, usePipelineIcon, useSession } from '../../api/hooks'
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
  else if (scope.kind === 'incidents') p.set('incidents', 'true')
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

export function ChatView() {
  const { name } = useParams<{ name?: string }>()
  const navigate = useNavigate()
  const narrow = useNarrow()
  const [layout, updateLayout] = useLayout()
  const [scope, setScope] = useState<Scope>({ kind: 'all' })
  const [search, setSearch] = useState('')
  const [flatten, setFlatten] = useState(false)
  const [selectionMode, setSelectionMode] = useState(false)
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [highlighted, setHighlighted] = useState<string | undefined>()
  const [collapsedRoots, setCollapsedRoots] = useState<Set<string>>(new Set())
  const [toasts, setToasts] = useState<{ id: string; title: string }[]>([])
  const [newNames, setNewNames] = useState<Set<string>>(new Set())
  const seenNames = useRef<Set<string> | null>(null)
  const rowRefs = useRef<Map<string, HTMLButtonElement>>(new Map())

  const params = useMemo(() => scopeParams(scope, search), [scope, search])
  const { data, isLoading, error } = useConversations(params)
  const session = useSession()
  const iconFor = usePipelineIcon()

  const items = useMemo(() => {
    const raw = data?.items ?? []
    if (scope.kind === 'coordinator') {
      return raw.filter((c) => c.coordinator === scope.name || c.causedBy)
    }
    return raw
  }, [data, scope])

  // Arrivals move (console-thread-live-cues): a name the previous snapshot
  // did not have gets the tint, the "new" tag and a toast naming its
  // pipeline. `seenNames` starts `null` so the FIRST load never fires one.
  useEffect(() => {
    if (!data) return
    const current = new Set(data.items.map((c) => c.name))
    const prevSeen = seenNames.current
    if (prevSeen === null) {
      seenNames.current = current
      return
    }
    const arrived = data.items.filter((c) => !prevSeen.has(c.name))
    if (arrived.length > 0) {
      setNewNames((prev) => {
        const next = new Set(prev)
        for (const c of arrived) next.add(c.name)
        return next
      })
      setToasts((prev) => [
        ...prev,
        ...arrived.map((c) => ({
          id: `${c.name}-${Date.now()}`,
          title: `${c.pipeline || c.coordinator || 'A pipeline'} opened ${c.title || c.name}`,
        })),
      ])
      for (const c of arrived) {
        setTimeout(() => {
          setNewNames((prev) => {
            if (!prev.has(c.name)) return prev
            const next = new Set(prev)
            next.delete(c.name)
            return next
          })
        }, 4000)
      }
    }
    seenNames.current = current
  }, [data])

  const tree = useMemo(() => buildTree(items, !flatten), [items, flatten])
  const rows = useMemo(() => visibleRows(tree, items, collapsedRoots), [tree, items, collapsedRoots])

  // The PatternFly sidebar collapses to icons on this view (design D-J),
  // restored to whatever it was on leaving.
  useEffect(() => {
    const prev = useShell.getState().navCollapsed
    useShell.getState().setNavCollapsed(true)
    return () => useShell.getState().setNavCollapsed(prev)
  }, [])

  function clearNew(rowName: string) {
    setNewNames((prev) => {
      if (!prev.has(rowName)) return prev
      const next = new Set(prev)
      next.delete(rowName)
      return next
    })
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
    navigate(`/conversations/${rowName}`)
  }

  function onListKeyDown(e: React.KeyboardEvent) {
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
    <div
      data-testid="chat-view"
      tabIndex={-1}
      onKeyDown={onListKeyDown}
      style={{ display: 'flex', flex: 1, minHeight: 0, minWidth: 0 }}
    >
      <AlertGroup isToast isLiveRegion>
        {toasts.slice(-3).map((t) => (
          <Alert
            key={t.id}
            variant="info"
            title={t.title}
            timeout={5000}
            onTimeout={() => setToasts((prev) => prev.filter((x) => x.id !== t.id))}
          />
        ))}
      </AlertGroup>
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
              {isLoading && !data ? (
                <Loading />
              ) : error || !data ? (
                <ErrorState title="Could not load conversations">{String(error)}</ErrorState>
              ) : rows.length === 0 ? (
                <Empty title="No conversations match">
                  {data.total > 0 ? 'Every conversation was filtered out.' : 'Nothing has originated yet.'}
                </Empty>
              ) : (
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
              )}
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
          <div style={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
            <Empty title="Select a conversation">Choose one from the list, or start a new one.</Empty>
          </div>
        )
      ))}
    </div>
  )
}
