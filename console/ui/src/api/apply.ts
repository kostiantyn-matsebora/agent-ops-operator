import type { QueryClient, QueryKey } from '@tanstack/react-query'
import type {
  ConversationDetail, ConversationPage, DeltaEvent, InventoryRow, Message,
} from './types'

// APPLYING an event to what the console already holds.
//
// One place, keyed by kind, and every view reads the cache rather than the
// stream. Two views holding one object would otherwise each implement the same
// update, and the day they disagree is the day a list and a detail show
// different things about the same object. One applier cannot disagree with
// itself.
//
// Two rules bound what happens here:
//
//   1. APPLY ONLY WHERE THE CONSOLE ALREADY HOLDS IT. A view nobody has opened
//      has nothing to update, and writing it anyway would make every browser
//      accumulate every object that ever changed.
//   2. WRITE WHAT THE SERVER SENT, NEVER A SHAPE OF OUR OWN. The rows and
//      details on a delta come from the same functions the snapshot endpoints
//      use, so an applied entry cannot render something a re-fetch would never
//      produce.
//
// Correctness rests on the snapshot staying authoritative: a resync replaces
// applied state wholesale, so an applier that is ever wrong is corrected at the
// next reconnect rather than persisting.

/** held reports whether a view is loaded — the precondition for applying. */
function held(client: QueryClient, key: QueryKey): boolean {
  return client.getQueryData(key) !== undefined
}

/**
 * A view the console DERIVES rather than holds: an aggregate over many objects,
 * computed server-side from state the browser never sees whole.
 *
 * These are the stated exception to applying. A count across every kind, a
 * traffic graph, the cross-object findings and a Pipeline's resolved
 * capabilities are not recoverable from one changed object, and recomputing
 * them here would be a second implementation of the answer the server gives —
 * which is the one thing the console must never disagree with.
 *
 * They are INVALIDATED, not discarded: the query key is stable, so what is on
 * screen stays on screen while the background read lands. That is the whole
 * difference between this and what the revision-in-the-key did.
 */
interface DerivedView {
  key: QueryKey
  /** Which kinds move it. `*` means any. */
  kinds: string[]
  /** Why it cannot be applied. */
  why: string
}

export const DERIVED_VIEWS: DerivedView[] = [
  { key: ['overview'], kinds: ['*'], why: 'counts and workload health are an aggregate over every kind and the manager' },
  { key: ['kinds'], kinds: ['*'], why: 'a per-kind count is an aggregate, not a property of the object that changed' },
  { key: ['findings'], kinds: ['*'], why: 'a finding is a relation BETWEEN objects — a dangling ref is about two of them' },
  { key: ['topology'], kinds: ['*'], why: 'the graph is nodes and edges resolved across every kind' },
  { key: ['sources'], kinds: ['signalsources', 'pipelines'], why: 'wiring state joins a source to the pipelines claiming it' },
  { key: ['vocabulary'], kinds: ['pipelines'], why: 'the addressable set is Ready pipelines, resolved server-side' },
  { key: ['conversationCount'], kinds: ['conversations'], why: 'the badge is a total over conversations the browser never holds whole' },
  { key: ['conversationGraph'], kinds: ['conversations'], why: 'a conversation graph is a topology, resolved the same way' },
  // Pipeline detail is the one detail the stream cannot carry: it holds the
  // manager's resolved capabilities, which this console renders verbatim and
  // must not recompute. The BFF therefore sends a pipeline delta with no
  // detail, and that view re-reads.
  { key: ['detail', 'pipelines'], kinds: ['pipelines'], why: 'resolved capabilities are the manager\'s answer, fetched per read' },
]

/** invalidateDerived re-reads the aggregates the given kinds moved. */
export function invalidateDerived(client: QueryClient, kinds: Set<string>) {
  for (const view of DERIVED_VIEWS) {
    if (!view.kinds.some((k) => k === '*' || kinds.has(k))) continue
    void client.invalidateQueries({ queryKey: view.key })
  }
}

/** applyDelta writes one CR change into every view holding that object. */
export function applyDelta(client: QueryClient, ev: DeltaEvent) {
  // A resync is not an apply. Its whole meaning is "stop trusting what you
  // hold", and the stream answers it by reloading.
  if (ev.type === 'RESYNC') return

  if (ev.type === 'DELETED') {
    removeRow(client, ev.kind, ev.name)
    client.removeQueries({ queryKey: ['detail', ev.kind, ev.name] })
    if (ev.kind === 'conversations') {
      removeConversation(client, ev.name)
      client.removeQueries({ queryKey: ['conversation', ev.name] })
    }
    return
  }

  if (ev.row) writeRow(client, ev.kind, ev.row)
  if (ev.detail && held(client, ['detail', ev.kind, ev.name])) {
    client.setQueryData(['detail', ev.kind, ev.name], ev.detail)
  }
  if (ev.conversationRow) writeConversation(client, ev.type, ev.conversationRow)
  if (ev.conversationView) writeConversationDetail(client, ev.name, ev.conversationView)
}

// ---- configuration listings ---------------------------------------------------

/** writeRow replaces or inserts a row in a held listing, keeping name order. */
function writeRow(client: QueryClient, kind: string, row: InventoryRow) {
  const key = ['inventory', kind]
  if (!held(client, key)) return
  client.setQueryData<InventoryRow[]>(key, (prev) => {
    if (!prev) return prev
    const next = prev.filter((r) => r.name !== row.name)
    next.push(row)
    // The server lists by name, so an inserted row lands where a re-fetch
    // would have put it rather than at whichever end was convenient.
    next.sort((a, b) => a.name.localeCompare(b.name))
    return next
  })
}

function removeRow(client: QueryClient, kind: string, name: string) {
  const key = ['inventory', kind]
  if (!held(client, key)) return
  client.setQueryData<InventoryRow[]>(key, (prev) => prev?.filter((r) => r.name !== name))
}

// ---- the conversations listing ------------------------------------------------

/**
 * Whether a listing query is the unfiltered first page.
 *
 * Membership, ordering and the facet lists are the SERVER's decisions — it
 * filters, sorts by activity and pages. So an update to a row already on screen
 * is applied, a delete removes it, and a conversation APPEARING is inserted
 * only here, where "newest first" and "no filter to satisfy" together make its
 * place unambiguous. Anywhere else the page is re-read instead, which changes
 * the rows without blanking them.
 */
function unfilteredFirstPage(params: string): boolean {
  const p = new URLSearchParams(params)
  for (const key of p.keys()) {
    if (key !== 'limit' && key !== 'offset') return false
  }
  return (p.get('offset') ?? '0') === '0'
}

function writeConversation(
  client: QueryClient,
  type: 'ADDED' | 'MODIFIED',
  row: ConversationPage['items'][number],
) {
  for (const [key, page] of client.getQueriesData<ConversationPage>({ queryKey: ['conversations'] })) {
    if (!page) continue
    const present = page.items.some((c) => c.name === row.name)
    if (present) {
      client.setQueryData<ConversationPage>(key, {
        ...page,
        items: page.items.map((c) => (c.name === row.name ? row : c)),
      })
      continue
    }
    if (type !== 'ADDED') continue
    const params = String(key[1] ?? '')
    if (!unfilteredFirstPage(params)) {
      // It may or may not belong on this page. The server decides, and a
      // stable key means the re-read swaps the rows under an unchanged table.
      void client.invalidateQueries({ queryKey: key })
      continue
    }
    const limit = page.limit || page.items.length + 1
    client.setQueryData<ConversationPage>(key, {
      ...page,
      items: [row, ...page.items].slice(0, limit),
      total: page.total + 1,
    })
  }
}

function removeConversation(client: QueryClient, name: string) {
  for (const [key, page] of client.getQueriesData<ConversationPage>({ queryKey: ['conversations'] })) {
    if (!page || !page.items.some((c) => c.name === name)) continue
    client.setQueryData<ConversationPage>(key, {
      ...page,
      items: page.items.filter((c) => c.name !== name),
      total: Math.max(0, page.total - 1),
    })
  }
}

// ---- one conversation ---------------------------------------------------------

/**
 * The page keeps the two parts that arrive on streams of their own.
 *
 * The transcript is appended to by message events and the events list by
 * activity events, so a delta replaces everything ELSE about the conversation
 * and leaves those alone. Overwriting them with what a delta does not carry is
 * how an open thread would empty itself every time its object changed.
 */
function writeConversationDetail(
  client: QueryClient,
  name: string,
  view: Omit<ConversationDetail, 'transcript' | 'events'>,
) {
  const key = ['conversation', name]
  if (!held(client, key)) return
  client.setQueryData<ConversationDetail>(key, (prev) =>
    prev ? { ...prev, ...view } : prev,
  )
}

/**
 * applyMessage puts a transcript append where it belongs.
 *
 * The message carries its whole self, so nothing is fetched to learn what it
 * said — the console used to parse exactly this payload, throw it away, and ask
 * the server what had just arrived.
 *
 * It is addressed by THREAD, and the conversation holding that thread is the
 * one that gets it. A thread nobody has open matches nothing, which is correct:
 * the first read of that conversation brings the whole transcript.
 */
export function applyMessage(client: QueryClient, msg: Message) {
  for (const [key, detail] of client.getQueriesData<ConversationDetail>({ queryKey: ['conversation'] })) {
    if (!detail || detail.conversation.consoleThread !== msg.thread) continue
    const prev = detail.transcript ?? []
    // Replace by id rather than append: a locally-typed message becomes the
    // confirmed one, and appending would show it twice.
    const transcript = prev.some((m) => m.id === msg.id)
      ? prev.map((m) => (m.id === msg.id ? msg : m))
      : [...prev, msg]
    client.setQueryData<ConversationDetail>(key, { ...detail, transcript })
  }
}
