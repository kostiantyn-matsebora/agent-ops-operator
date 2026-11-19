import { useEffect, useRef, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { Button, SearchInput } from '@patternfly/react-core'
import { BarsIcon } from '@patternfly/react-icons'
import {
  useMarkRead, useMarkUnread, usePipelineIcon, useReopenConversation, useSession,
} from '../../../../api/hooks'
import { useShell } from '../../../../shell'
import { Empty, ErrorState, Loading } from '../../components/States'
import {
  DEFAULT_LAYOUT, INBOX_COLLAPSED_WIDTH, MIN_INBOX_WIDTH, MIN_LIST_WIDTH, MIN_THREAD_WIDTH, useLayout,
} from './layout'
import { Splitter } from './Splitter'
import { Inbox } from './Inbox'
import { ConversationRow } from './ConversationRow'
import { SelectionBar } from './SelectionBar'
import { ThreadPane } from './ThreadPane'
import { GENERAL_FEED_NAME, GeneralFeedPane } from './GeneralFeedPane'
import {
  sameScope, scopeKey, selectionState, useArrivals, useConversationList, type Scope,
} from '../../../../model/conversationList'
import { exitRuntime } from '../../../../model/verbs'
import { TESTID } from '../../../../model/testids'

// `console-chat-layout`: one view, four columns, switching in place.
// Composition from `prototype/C-rail.html` and `C-collapsed.html`.

/** Below this the view shows one column at a time (console-chat-layout: "The view fits narrow windows"). */
const NARROW_WIDTH = 900
/** Below this even the Inbox panel (MIN_INBOX_WIDTH) no longer fits beside the
 * list (MIN_LIST_WIDTH) inside the remaining width once the app shell's own
 * rail is accounted for — a true phone width, not merely a narrowed window. */
const MOBILE_WIDTH = 560
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
export { listKeyAction, neighbourName } from '../../../../model/conversationList'
import { listKeyAction, neighbourName } from '../../../../model/conversationList'

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

/** The inbox pane's width, the narrow strip when it is collapsed. */
function inboxPaneWidth(layout: { inboxCollapsed: boolean; inboxWidth: number }): number {
  return layout.inboxCollapsed ? INBOX_COLLAPSED_WIDTH : layout.inboxWidth
}

/** The thread pane's content: the general feed, one conversation, or the empty prompt. */
function ThreadSlot({ name, narrow, onBack }: Readonly<{ name?: string; narrow: boolean; onBack: () => void }>) {
  if (name === GENERAL_FEED_NAME) return <GeneralFeedPane onBack={narrow ? onBack : undefined} />
  if (name) return <ThreadPane name={name} onBack={narrow ? onBack : undefined} />
  if (narrow) return null
  return (
    <div style={{ flex: 1, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', gap: 16 }}>
      <Empty title="Select a conversation">Choose one from the list, or start a new one.</Empty>
    </div>
  )
}

export function ChatView() {
  const { name } = useParams<{ name?: string }>()
  const navigate = useNavigate()
  const narrow = useNarrow()
  const [layout, updateLayout] = useLayout()
  const [scope, setScope] = useState<Scope>({ kind: 'open' })
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
  const [selectionMode, setSelectionMode] = useState(false)
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [highlighted, setHighlighted] = useState<string | undefined>()
  const [collapsedRoots, setCollapsedRoots] = useState<Set<string>>(new Set())
  const rowRefs = useRef<Map<string, HTMLButtonElement>>(new Map())
  // Every root name this component has ever applied the persisted fold
  // default to — so a root the viewer explicitly expanded (or collapsed) by
  // hand is never silently re-folded just because the tree re-rendered.
  const seededRootsRef = useRef<Set<string>>(new Set())

  const { data, isLoading, error, items, rows, collapsibleNames } = useConversationList({
    scope, search, flatten, collapsedRoots,
  })
  const session = useSession()
  const iconFor = usePipelineIcon()
  const canWrite = session.data?.canWrite ?? false
  const hasReader = Boolean(session.data?.identity)
  const reopen = useReopenConversation()
  const markRead = useMarkRead()
  const markUnread = useMarkUnread()

  // Arrivals move (console-thread-live-cues): the row itself, plus the inbox's
  // own unread badges, are the whole of the signal — no popup.
  const { newNames, clear: clearNew } = useArrivals(data, scopeKey(scope))

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

  function toggleSelect(rowName: string, checked: boolean) {
    setSelected((prev) => withMembership(prev, rowName, checked))
  }

  // `general` is not a conversation filter — picking it opens the channel's
  // general feed directly in the thread pane, the same place a conversation
  // row opens (B1, UI half). It never becomes the list's own query scope.
  function selectScope(s: Scope) {
    if (s.kind === 'general') {
      void navigate(`/conversations/${GENERAL_FEED_NAME}`)
      return
    }
    setScope(s)
  }

  // A member carries no selection control (console-conversation-tree: "A
  // member SHALL NOT be selectable in this view") — a modifier-click on one
  // opens it normally instead of starting or joining a selection.
  function openRow(rowName: string, isMember: boolean, e?: React.MouseEvent) {
    if (isMultiSelectClick(e) && !isMember) {
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
      // Enter carries no modifier key, so `isMember` never matters here.
      openRow(highlighted as string, false)
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
      data-testid={TESTID.chatView}
      aria-label="conversations workspace"
      tabIndex={-1}
      style={{ display: 'flex', flex: 1, minHeight: 0, minWidth: 0 }}
    >
      {showingList && mobile && mobileInboxOpen && (
        <div data-testid={TESTID.mobileInboxDrawer} style={{ width: '100%', flex: 1, minWidth: 0, display: 'flex', flexDirection: 'column' }}>
          <Inbox
            activeScope={scope}
            onSelectScope={(s) => {
              selectScope(s)
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
                  onSelectScope={selectScope}
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
              <strong>{sameScope(scope, { kind: 'open' }) ? 'Open conversations' : 'Conversations'}</strong>
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
                      onOpen={(e) => openRow(row.name, Boolean(row.causedBy), e)}
                      pipelineIcon={iconFor(row.pipeline)}
                      canWrite={canWrite}
                      hasReader={hasReader}
                      onMarkRead={() => markRead.mutate({ names: [row.name] })}
                      onMarkUnread={() => markUnread.mutate({ names: [row.name] })}
                      onReopen={() => reopen.mutate(row.name)}
                      onExitRuntime={() => void exitRuntime(row.name)}
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
      {showingThread && <ThreadSlot name={name} narrow={narrow} onBack={() => navigate('/conversations')} />}
    </section>
  )
}
