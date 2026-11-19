import { create } from 'zustand'
import { notifyManager, type QueryClient } from '@tanstack/react-query'
import { applyDeltas, applyMessage, invalidateDerived } from './apply'
import type { ActivityEvent, DeltaEvent, Message, Queues } from './types'

// ONE SSE connection into a Zustand store and the react-query cache.
//
// EVERY EVENT CARRIES ITS CONTENT, so an event is APPLIED rather than used as a
// signal to go and ask. A CR delta brings the changed object in the shapes the
// snapshots serve, a message brings the message, activity events and queue
// state travel whole. Nothing here bumps a counter that a query key reads —
// that mechanism is what made every change a cache miss, and a cache miss is a
// spinner where content used to be.
//
// Snapshots stay AUTHORITATIVE. A resync reloads everything, so a browser that
// misses events converges, and an applier that is ever wrong is corrected at
// the next reconnect rather than persisting.

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
  events: ActivityEvent[]
  cursor: string
  queues?: Queues
}

export const useStream = create<StreamState>(() => ({
  connected: false,
  events: [],
  cursor: '',
}))

/** Recent events touching one conversation, oldest first. */
export function eventsFor(conversation: string, events: ActivityEvent[]): ActivityEvent[] {
  return events.filter((e) => e.conversation === conversation)
}

let source: EventSource | undefined
let retryTimer: ReturnType<typeof setTimeout> | undefined
let stopped = false

/**
 * How long derived-view invalidations are gathered before they are issued, in ms.
 *
 * DELTAS ARRIVE IN BURSTS. Closing fifty conversations writes fifty statuses and
 * fifty deletions within a couple of seconds. The rows themselves are applied
 * one by one — that costs nothing and is what keeps the table live — but the
 * views the server DERIVES from many objects cannot be applied, and re-reading
 * the overview fifty times to learn one new count is a burst of requests for one
 * answer.
 *
 * The cost is bounded and known: a derived view can be a quarter second behind
 * the row that moved. Snapshots stay authoritative, so that is the same
 * staleness the design already accepts for a dropped event.
 */
const DERIVED_COALESCE_MS = 250

let pendingKinds: Set<string> | null = null
let coalesceTimer: ReturnType<typeof setTimeout> | undefined

/** flushDerived re-reads every derived view the window's kinds moved. */
function flushDerived(client: QueryClient) {
  coalesceTimer = undefined
  const kinds = pendingKinds
  pendingKinds = null
  if (!kinds || kinds.size === 0) return
  invalidateDerived(client, kinds)
}

/** noteDerived records that a kind moved. Exported for the coalescing test. */
export function noteDerived(client: QueryClient, kind: string) {
  if (!pendingKinds) pendingKinds = new Set()
  pendingKinds.add(kind)
  if (coalesceTimer === undefined) {
    coalesceTimer = setTimeout(() => flushDerived(client), DERIVED_COALESCE_MS)
  }
}

/**
 * How long stream events are gathered before they are applied, in ms.
 *
 * A busy install streams hundreds of changes a second. Applying each as its own
 * task puts a full render of the list behind every one, and a person clicking
 * meanwhile waits behind all of them — the console looks frozen although every
 * single change is cheap. Events are applied IN ORDER, but in one batch per
 * window, so the list renders once for the burst, not once per change.
 */
export const STREAM_BATCH_MS = 50

let queue: Array<() => void> = []
let pendingDeltas: DeltaEvent[] = []
let pendingActivity: ActivityEvent[] = []
let batchTimer: ReturnType<typeof setTimeout> | undefined
let liveClient: QueryClient | undefined

function enqueue(client: QueryClient, apply: () => void) {
  liveClient = client
  queue.push(apply)
  if (batchTimer === undefined) batchTimer = setTimeout(flushStream, STREAM_BATCH_MS)
}

/** Applies everything gathered so far, in arrival order. Exported for tests. */
export function flushStream() {
  if (batchTimer !== undefined) clearTimeout(batchTimer)
  batchTimer = undefined
  const work = queue
  const deltas = pendingDeltas
  const activity = pendingActivity
  queue = []
  pendingDeltas = []
  pendingActivity = []
  if (work.length === 0 && deltas.length === 0 && activity.length === 0) return
  const run = () => {
    if (deltas.length > 0) {
      applyDeltas(liveClient!, deltas)
      for (const kind of new Set(deltas.map((d) => d.kind))) noteDerived(liveClient!, kind)
    }
    for (const apply of work) apply()
    if (activity.length > 0) {
      useStream.setState((s) => {
        const last = activity[activity.length - 1]
        const newest = activity.reduce((c, e) => (e.cursor > c ? e.cursor : c), s.cursor)
        return { events: [...s.events, ...activity].slice(-MAX_LIVE_EVENTS), cursor: last ? newest : s.cursor }
      })
    }
  }
  if (liveClient) notifyManager.batch(run)
  else run()
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
export function connectStream(client: QueryClient): () => void {
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

    wire(es, client)
  }

  open()
  return () => {
    flushStream()
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
function wire(es: EventSource, client: QueryClient) {
  es.addEventListener('resync', () => {
    // First connect and reconnect are the SAME path: both start with a resync,
    // so there is no "have I been here before" branch to get wrong.
    //
    // This is one of the four reasons a refetch still happens: the client has
    // provably missed events and cannot know which.
    void client.invalidateQueries()
  })

  es.addEventListener('delta', (ev) => {
    const d = JSON.parse((ev as MessageEvent).data) as DeltaEvent
    // What one object cannot answer for — counts, graphs, cross-object findings
    // — is re-read on a stable key (noteDerived, at the flush), so the page
    // updates without blanking.
    liveClient = client
    pendingDeltas.push(d)
    if (batchTimer === undefined) batchTimer = setTimeout(flushStream, STREAM_BATCH_MS)
  })

  es.addEventListener('activity', (ev) => {
    const e = JSON.parse((ev as MessageEvent).data) as ActivityEvent
    // A gap carries no cursor — it is not a hop the manager sequenced; the cursor
    // only moves for events that have one (an empty one never sorts above a held one).
    liveClient = client
    pendingActivity.push(e)
    if (batchTimer === undefined) batchTimer = setTimeout(flushStream, STREAM_BATCH_MS)
    // The gap itself lives on the stream HEALTH, so that a browser opened after
    // the fact still sees it. Reloading is how a browser already open learns of
    // one without a second copy of the fact in this store — and it is a resync
    // in every sense: history the client provably missed.
    if (e.kind === ACTIVITY_GAP) void client.invalidateQueries()
  })

  es.addEventListener('queues', (ev) => {
    useStream.setState({ queues: JSON.parse((ev as MessageEvent).data) as Queues })
  })

  es.addEventListener('message', (ev) => {
    const m = JSON.parse((ev as MessageEvent).data) as Message
    enqueue(client, () => applyMessage(client, m))
  })
}
