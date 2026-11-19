import { useEffect, useMemo, useRef, useState } from 'react'
import { useConversations, useInboxCounts, useVocabulary } from '../api/hooks'
import type { ConversationSummary } from '../api/types'
import { buildTree, rootNameOf, type TreeRow } from './tree'
import { stripLeadingIcon, stripNamePrefix } from './titles'
import { preview } from './text'

// THE CONVERSATION LIST MODEL: which conversations a scope shows, in what
// order, how many each scope counts, and what each row says about itself.
// Every look reads these facts from here and none recomputes them
// (console-looks: "A fact is computed once").

// ---- scopes ------------------------------------------------------------------

// `console-chat-layout` spec named this scope "All", counting every
// conversation regardless of read state, with a separate "Unread" scope
// beside it. A later decision made every OTHER fixed scope (Working, Mine,
// Errored), each Pipeline/Coordinator, and Closed an unread count
// (TREE_PREDICATE below) — which made the dedicated "Unread" scope redundant
// (every category already had its own unread count one level down) and it was
// removed.
//
// `all` is GONE too (console-inbox-open-closed): a scope that excluded
// closed rows behind a "Show closed" checkbox had a name that lied about
// what it listed. `open` replaces it — every conversation with
// `phase≠Closed`, read or not — and is the scope the view lands on. Closed
// rows are excluded from every scope but `closed` by the ONE rule in
// `visibleItems` below, never by a per-look preference.
export type Scope =
  | { kind: 'open' }
  | { kind: 'working' }
  | { kind: 'mine' }
  | { kind: 'errored' }
  | { kind: 'pipeline' | 'coordinator'; name: string }
  | { kind: 'closed' }
  // Not a conversation filter — picking it opens the channel's general feed,
  // same as clicking a row does for an ordinary conversation. A look
  // intercepts it before it ever becomes the list's own query scope.
  | { kind: 'general' }

export function scopeKey(s: Scope): string {
  return 'name' in s ? `${s.kind}:${s.name}` : s.kind
}

export function sameScope(a: Scope, b: Scope): boolean {
  return scopeKey(a) === scopeKey(b)
}

/** How many TREES one list request asks for — a root with every member under it, never a row count (the server pages by root). */
export const PAGE_SIZE = 100

/** The server-side half of a scope: the query the list request carries. */
export function scopeParams(scope: Scope, search: string): URLSearchParams {
  const p = new URLSearchParams()
  if (scope.kind === 'closed') p.set('phase', 'Closed')
  else if (scope.kind === 'pipeline') {
    p.set('pipeline', scope.name)
    // A Pipeline conversation never causes a member (only a Coordinator
    // invokes — terminology.md), so there is no tree for `?unread=true` to
    // break here the way it would for the scopes below.
    p.set('unread', 'true')
  }
  // A Coordinator scope has no server-side filter param (the backend tracks
  // it only as a `scopes` count): narrowed CLIENT-SIDE below instead.
  //
  // All, Working, Errored and Mine are the SAME shape of problem: each is an
  // independent per-row predicate server-side, with no tree awareness at all.
  // `?mine=true` kept a root but dropped every member (a member is never Mine
  // — it was invoked, never originated). `?unread=true` would just as readily
  // keep an unread MEMBER while dropping its own root. All four are narrowed
  // CLIENT-SIDE below instead: if anything anywhere in a tree matches, the
  // WHOLE tree shows.
  if (search) p.set('q', search)
  p.set('limit', String(PAGE_SIZE))
  return p
}

// Every scope but Open and Closed is unread, ANDed with its own predicate.
const TREE_PREDICATE: Partial<Record<Scope['kind'], (c: ConversationSummary) => boolean>> = {
  working: (c) => c.phase === 'Working' && Boolean(c.unread),
  errored: (c) => Boolean(c.errored) && Boolean(c.unread),
  mine: (c) => Boolean(c.mine) && Boolean(c.unread),
  coordinator: (c) => Boolean(c.unread),
}

/**
 * Expands a per-row predicate into a per-TREE one: if anything anywhere in a
 * tree matches, every row of that tree is kept — never just the matching row
 * in isolation, which is what left a "Mine" root with none of its own
 * members. Each kept row still shows its own real status; this only decides
 * whether the tree shows at all.
 */
export function withTreeContext(
  raw: ConversationSummary[],
  matches: (c: ConversationSummary) => boolean,
): ConversationSummary[] {
  const byName = new Map(raw.map((c) => [c.name, c]))
  const rootOf = (c: ConversationSummary): string => {
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
    if (matches(c)) qualifyingRoots.add(rootOf(c))
  }
  return raw.filter((c) => qualifyingRoots.has(rootOf(c)))
}

/**
 * The rows a scope shows, narrowed client-side from the server's snapshot.
 *
 * ONE rule excludes closed rows: every scope but `closed` drops `phase ===
 * 'Closed'`, always — no preference, no toggle, no argument to turn it off
 * (console-chat-layout: "Closed conversations are reached through Closed
 * only"). Closed carries no predicate of its own: the server already
 * narrowed `raw` to `phase=Closed` (scopeParams above).
 */
export function visibleItems(all: ConversationSummary[], scope: Scope): ConversationSummary[] {
  let raw = all
  if (scope.kind === 'coordinator') {
    // `c.causedBy` truthy alone admits a member of ANY coordinator. A member
    // names only its immediate parent, so membership is decided by walking to
    // the uncaused root — within the UNFILTERED snapshot, since the walk needs
    // every ancestor — and checking THAT root's own `coordinator` field.
    const byName = new Map(raw.map((c) => [c.name, c]))
    raw = raw.filter((c) => byName.get(rootNameOf(all, c))?.coordinator === scope.name)
  }
  // Open is NOT an unread filter — it is every conversation a person can
  // open that is not closed. Working, Mine, Errored and each
  // Pipeline/Coordinator narrow further by their own tree predicate.
  const predicate = TREE_PREDICATE[scope.kind]
  if (predicate) raw = withTreeContext(raw, predicate)
  if (scope.kind !== 'closed') raw = raw.filter((c) => c.phase !== 'Closed')
  return raw
}

/** Rows whose ancestor chain passes through a collapsed root are hidden — the fold, not a filter. */
export function visibleRows(tree: TreeRow[], items: ConversationSummary[], collapsedRoots: Set<string>): TreeRow[] {
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

/** The rows a bulk action may touch, and whether every one is selected. */
export function selectionState(items: ConversationSummary[], selected: Set<string>) {
  const selectableRows = items.filter((c) => !c.deleting).map((c) => c.name)
  const allSelected = selectableRows.length > 0 && selectableRows.every((n) => selected.has(n))
  return { selectableRows, allSelected }
}

// ---- one row ------------------------------------------------------------------

/** The row's title: no lane glyph, no prefix repeating the name its own chip already shows. */
export function rowTitle(row: ConversationSummary): string {
  // A root is attributed by its coordinator, a member by the entry it was
  // invoked as (`row.pipeline` is unset on a member), and everything else by
  // its pipeline.
  let attributedTo = row.pipeline
  if (row.coordinator) attributedTo = row.coordinator
  else if (row.causedBy) attributedTo = row.causedBy.entry
  return stripNamePrefix(stripLeadingIcon(row.title || row.name), attributedTo)
}

/** Whether a row has anything unread — the count a look draws, never its own guess. */
export function rowUnread(row: ConversationSummary): boolean {
  return (row.unreadCount ?? 0) > 0
}

/** The snippet line: the last counted message, or why the row has no message yet. */
export function rowSnippet(row: ConversationSummary): string {
  if (row.deleting) return 'deleting'
  if (row.phase === 'Closed') return row.closeReason ? `closed · ${row.closeReason}` : `closed · ${row.runCount} run(s)`
  if (row.blocked) return `${row.blocked.storage ? 'storage' : 'blocked'}: ${row.blocked.reason}`
  if (row.phase === 'Pending') return 'pending · waiting for a slot'
  if (row.lastMessage) {
    const who = row.lastMessage.sender ? `${row.lastMessage.sender}: ` : ''
    const text = preview(row.lastMessage.text)
    return row.presence ? `working · ${who}${text}` : `${who}${text}`
  }
  if (row.presence) return `${row.pipeline || 'the agent'} is working…`
  return ''
}

/**
 * The small tag beside a row — absent most of the time.
 *
 * `observed` (`!row.joined`) is GONE: measured live, it showed on every
 * unattended row, since the viewer never "joins" an automated conversation by
 * replying to it. A tag on effectively every row distinguishes nothing.
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

export type RowStatus = 'working' | 'waiting' | 'pending' | 'failed' | 'idle' | 'none'

/** The row's one status, in precedence order. */
export function phaseDot(row: ConversationSummary): RowStatus {
  if (row.presence) return 'working'
  if (row.errored) return 'failed'
  if (row.phase === 'Pending' || row.phase === 'Queued') return 'pending'
  if (row.phase === 'Closed') return 'none'
  return 'idle'
}

/**
 * The row's status once its members are known. A conversation with nothing of
 * its own running but members still at work is WAITING on them — not idle: its
 * answer is not in yet, and the list must say so.
 */
export function rowStatus(row: ConversationSummary, membersActive: number): RowStatus {
  const own = phaseDot(row)
  return own === 'idle' && membersActive > 0 ? 'waiting' : own
}

/** What a waiting row says on its second line: who it is waiting on. */
export function waitingSnippet(entries: string[], count: number): string {
  const who = entries.slice(0, 3).join(', ')
  const more = entries.length > 3 ? ` +${entries.length - 3}` : ''
  return `waiting on ${who || `${count} member${count === 1 ? '' : 's'}`}${more}…`
}

// ---- the hooks ----------------------------------------------------------------

export interface ListOptions {
  scope: Scope
  search: string
  /** Every row its own line, in the server's order, instead of grouped under its root. */
  flatten: boolean
  collapsedRoots: Set<string>
}

/**
 * The list a scope shows. Ordering is the server's — newest activity first —
 * with each member nested depth-first under its own root unless flattened.
 */
export function useConversationList({ scope, search, flatten, collapsedRoots }: ListOptions) {
  const params = useMemo(() => scopeParams(scope, search), [scope, search])
  const { data, isLoading, error } = useConversations(params)
  const items = useMemo(() => visibleItems(data?.items ?? [], scope), [data, scope])
  const tree = useMemo(() => buildTree(items, !flatten), [items, flatten])
  // A root waiting on a member is never folded: the work is the news, and the
  // member that is doing it must be in view (an explicit fold returns once the wait ends).
  const collapsed = useMemo(() => {
    const waitingRoots = new Set(tree.filter((t) => t.membersActive > 0).map((t) => t.row.name))
    return waitingRoots.size === 0 ? collapsedRoots : new Set([...collapsedRoots].filter((n) => !waitingRoots.has(n)))
  }, [tree, collapsedRoots])
  const rows = useMemo(() => visibleRows(tree, items, collapsed), [tree, items, collapsed])
  const collapsibleNames = useMemo(() => tree.filter((t) => t.memberCount > 0).map((t) => t.row.name), [tree])
  return { data, isLoading, error, items, tree, rows, collapsibleNames }
}

/** One inbox entry: the scope it selects, the label it shows and the count beside it. */
export interface ScopeEntry {
  scope: Scope
  label: string
  count?: number
  /** A Pipeline's or Coordinator's declared icon, for the look to draw. */
  icon?: string
}

/**
 * The inbox's scopes and their counts, in spec order: Open, Working, Mine,
 * Errored, then every Ready Pipeline/Coordinator, then Closed
 * (console-chat-layout: "The inbox column lists scopes, each with its
 * count").
 *
 * Every count is UNREAD, ANDed with its own scope's predicate, and each
 * matches its own list exactly (TREE_PREDICATE above): Open and every scope
 * but Closed exclude closed rows, Closed counts closed rows only. No
 * argument changes which conversations a count includes — the server
 * computes every one with the same rule (console-unread).
 */
export function useInboxScopes() {
  const counts = useInboxCounts()
  const vocabulary = useVocabulary()
  const scopes = counts.data?.scopes ?? {}
  const fixed: ScopeEntry[] = [
    { scope: { kind: 'open' }, label: 'Open', count: scopes.open },
    { scope: { kind: 'working' }, label: 'Working', count: scopes.working },
    { scope: { kind: 'mine' }, label: 'Mine', count: scopes.mine },
    { scope: { kind: 'errored' }, label: 'Errored', count: scopes.errored },
  ]
  const targets: ScopeEntry[] = (vocabulary.data?.entries ?? [])
    .filter((e) => e.kind === 'pipeline' || e.kind === 'coordinator')
    .map((e) => ({
      scope: { kind: e.kind as 'pipeline' | 'coordinator', name: e.name },
      label: e.name,
      count: scopes[e.name],
      icon: e.icon,
    }))
  const general: ScopeEntry = { scope: { kind: 'general' }, label: 'General feed' }
  const closed: ScopeEntry = { scope: { kind: 'closed' }, label: 'Closed', count: scopes.closed }
  return { general, fixed, targets, closed }
}

function dropName(set: Set<string>, name: string): Set<string> {
  if (!set.has(name)) return set
  const next = new Set(set)
  next.delete(name)
  return next
}

/** How long a row counts as newly arrived, in ms. */
export const ARRIVAL_HOLD_MS = 4000

/**
 * Which rows just arrived (console-thread-live-cues: "Arrivals move"). A name
 * the previous snapshot did not have is new for ARRIVAL_HOLD_MS, or until the
 * viewer opens it. The FIRST snapshot of a scope announces nothing: switching
 * from a narrow scope to a wider one would otherwise call every row the
 * narrow scope never had an arrival.
 */
export function useArrivals(data: { items: ConversationSummary[] } | undefined, key: string) {
  const [newNames, setNewNames] = useState<Set<string>>(new Set())
  const seenNames = useRef<Set<string> | null>(null)
  const seenScope = useRef<string | null>(null)
  useEffect(() => {
    if (!data) return
    if (seenScope.current !== key) {
      seenScope.current = key
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
      setTimeout(() => setNewNames((prev) => dropName(prev, c.name)), ARRIVAL_HOLD_MS)
    }
  }, [data, key])
  const clear = (name: string) => setNewNames((prev) => dropName(prev, name))
  return { newNames, clear }
}

export type ListKeyAction = 'clear' | 'open' | 'down' | 'up' | 'none'

/** What a key does on the list: Escape clears, arrows move, Enter opens the highlighted row. */
export function listKeyAction(key: string, rowCount: number, highlighted: string | undefined): ListKeyAction {
  if (key === 'Escape') return 'clear'
  if (rowCount === 0) return 'none'
  if (key === 'Enter') return highlighted ? 'open' : 'none'
  if (key === 'ArrowDown') return 'down'
  if (key === 'ArrowUp') return 'up'
  return 'none'
}

/** The row an arrow key lands on, clamped at both ends. */
export function neighbourName(names: string[], current: string | undefined, down: boolean): string {
  const idx = current ? names.indexOf(current) : -1
  const next = down ? Math.min(idx + 1, names.length - 1) : Math.max(idx - 1, 0)
  return names[Math.max(next, 0)]
}
