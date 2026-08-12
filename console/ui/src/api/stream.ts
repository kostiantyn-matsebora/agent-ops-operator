import { create } from 'zustand'
import type { ActivityEvent, Queues } from './types'

// ONE SSE connection into a Zustand store, with cursor-driven query
// invalidation.
//
// Snapshots are authoritative and the stream carries deltas. So a CR delta does
// not update anything here — it bumps a per-kind revision, and the components
// showing that kind re-fetch. That keeps the wire format stable no matter how
// the CRDs evolve, and makes a dropped event cost one stale second rather than a
// wrong screen. Activity events are the exception: they ARE the animation, so
// they are kept whole.

/** How many live events the store keeps for animation. */
const MAX_LIVE_EVENTS = 500

/**
 * The console-local event marking LOST HISTORY in the timeline.
 *
 * The manager's activity ring is in-memory and bounded, so history goes two
 * ways: the buffer wraps, or the manager restarts and starts counting again.
 * Either way the events either side of this marker are not consecutive, and a
 * timeline that shows them adjacent claims something false. It carries no
 * from/to, so it never animates an edge.
 */
export const ACTIVITY_GAP = 'activity.gap'

export interface StreamState {
  connected: boolean
  /** Bumped per resource kind on any delta; queries key off it to refetch. */
  revisions: Record<string, number>
  /** Bumped whenever the server tells us to resync — every query refetches. */
  resyncs: number
  events: ActivityEvent[]
  cursor: string
  queues?: Queues
  /** Transcript appends, keyed by thread id. */
  messageRevision: number
  lastMessageThread?: string
}

export const useStream = create<StreamState>(() => ({
  connected: false,
  revisions: {},
  resyncs: 0,
  events: [],
  cursor: '',
  messageRevision: 0,
}))

/** Recent events touching one conversation, oldest first. */
export function eventsFor(conversation: string, events: ActivityEvent[]): ActivityEvent[] {
  return events.filter((e) => e.conversation === conversation)
}

let source: EventSource | undefined
let retryTimer: ReturnType<typeof setTimeout> | undefined
let stopped = false

/**
 * How long delta bumps are gathered before the revision moves, in ms.
 *
 * DELTAS ARRIVE IN BURSTS. Closing fifty conversations writes fifty statuses and
 * fifty deletions within a couple of seconds, and a revision is folded into a
 * query KEY — so an uncoalesced burst is fifty cold cache entries, fifty
 * refetches, and a list that flips to its loading state between each one. One
 * bump per kind per window carries the same information: a revision says "this
 * kind moved", never how many times.
 *
 * The cost is bounded and known: a delta can take a quarter second longer to
 * show. Snapshots stay authoritative, so that is the same staleness the design
 * already accepts for a dropped event.
 */
const DELTA_COALESCE_MS = 250

let pendingKinds: Set<string> | null = null
let coalesceTimer: ReturnType<typeof setTimeout> | undefined

/** flushDeltas bumps every kind seen during the window exactly once. */
function flushDeltas() {
  coalesceTimer = undefined
  const kinds = pendingKinds
  pendingKinds = null
  if (!kinds || kinds.size === 0) return
  useStream.setState((s) => {
    const revisions = { ...s.revisions }
    for (const kind of kinds) {
      revisions[kind] = (revisions[kind] ?? 0) + 1
    }
    return { revisions }
  })
}

/** noteDelta records that a kind moved. Exported for the coalescing test. */
export function noteDelta(kind: string) {
  if (!pendingKinds) pendingKinds = new Set()
  pendingKinds.add(kind)
  if (coalesceTimer === undefined) {
    coalesceTimer = setTimeout(flushDeltas, DELTA_COALESCE_MS)
  }
}

/** Reconnect backoff bounds, in ms. */
const RETRY_MIN = 1_000
const RETRY_MAX = 30_000

/**
 * Connect the single stream. Idempotent: calling it twice keeps one connection,
 * because the console holds the channel op loop and every browser's SSE — a
 * second connection per tab would multiply both.
 *
 * RECONNECTION IS OURS, not the browser's. EventSource retries a dropped
 * connection on its own, but on an HTTP error — a 502 while the console pod
 * rolls, a 401 once the session expires — it sets readyState to CLOSED and
 * gives up PERMANENTLY. Nothing then reopens it, so the masthead sat on
 * "stream disconnected" and the graph stayed frozen until somebody reloaded the
 * page. A viewer that cannot tell "the link died" from "the system is idle" is
 * the failure this whole surface exists to avoid, and a chip that only tells
 * the truth after F5 is that failure with extra steps.
 */
export function connectStream(onResync: () => void): () => void {
  if (source) return () => undefined
  stopped = false
  let backoff = RETRY_MIN

  const open = () => {
    if (stopped) return
    const es = new EventSource('/api/stream')
    source = es

    es.onopen = () => {
      backoff = RETRY_MIN // a healthy connection earns a fast first retry
      useStream.setState({ connected: true })
    }
    es.onerror = () => {
      useStream.setState({ connected: false })
      // Still CONNECTING means the browser is retrying by itself; leave it be.
      if (es.readyState !== EventSource.CLOSED || stopped) return
      es.close()
      source = undefined
      retryTimer = setTimeout(open, backoff)
      backoff = Math.min(backoff * 2, RETRY_MAX)
    }

    wire(es, onResync)
  }

  open()
  return () => {
    stopped = true
    if (retryTimer) clearTimeout(retryTimer)
    retryTimer = undefined
    if (coalesceTimer) clearTimeout(coalesceTimer)
    coalesceTimer = undefined
    pendingKinds = null
    source?.close()
    source = undefined
    useStream.setState({ connected: false })
  }
}

/** Attach the message handlers. Re-run per connection, so a reconnect is whole. */
function wire(es: EventSource, onResync: () => void) {
  es.addEventListener('resync', () => {
    // First connect and reconnect are the SAME path: both start with a resync,
    // so there is no "have I been here before" branch to get wrong.
    useStream.setState((s) => ({ resyncs: s.resyncs + 1 }))
    onResync()
  })

  es.addEventListener('delta', (ev) => {
    const d = JSON.parse((ev as MessageEvent).data) as { kind: string }
    noteDelta(d.kind)
  })

  es.addEventListener('activity', (ev) => {
    const e = JSON.parse((ev as MessageEvent).data) as ActivityEvent
    useStream.setState((s) => ({
      events: [...s.events, e].slice(-MAX_LIVE_EVENTS),
      // A gap carries no cursor — it is not a hop the manager sequenced.
      cursor: e.cursor > s.cursor ? e.cursor : s.cursor,
    }))
    // The gap itself lives on the stream HEALTH, so that a browser opened after
    // the fact still sees it. Refetching is how a browser already open learns
    // of one without a second copy of the fact in this store.
    if (e.kind === ACTIVITY_GAP) onResync()
  })

  es.addEventListener('queues', (ev) => {
    useStream.setState({ queues: JSON.parse((ev as MessageEvent).data) as Queues })
  })

  es.addEventListener('message', (ev) => {
    const m = JSON.parse((ev as MessageEvent).data) as { threadId: string }
    useStream.setState((s) => ({
      messageRevision: s.messageRevision + 1,
      lastMessageThread: m.threadId,
    }))
  })
}
