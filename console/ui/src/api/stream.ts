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

/**
 * Connect the single stream. Idempotent: calling it twice keeps one connection,
 * because the console holds the channel op loop and every browser's SSE — a
 * second connection per tab would multiply both.
 */
export function connectStream(onResync: () => void): () => void {
  if (source) return () => undefined
  const es = new EventSource('/api/stream')
  source = es

  es.onopen = () => useStream.setState({ connected: true })
  es.onerror = () => useStream.setState({ connected: false })

  es.addEventListener('resync', () => {
    // First connect and reconnect are the SAME path: both start with a resync,
    // so there is no "have I been here before" branch to get wrong.
    useStream.setState((s) => ({ resyncs: s.resyncs + 1 }))
    onResync()
  })

  es.addEventListener('delta', (ev) => {
    const d = JSON.parse((ev as MessageEvent).data) as { kind: string }
    useStream.setState((s) => ({
      revisions: { ...s.revisions, [d.kind]: (s.revisions[d.kind] ?? 0) + 1 },
    }))
  })

  es.addEventListener('activity', (ev) => {
    const e = JSON.parse((ev as MessageEvent).data) as ActivityEvent
    useStream.setState((s) => ({
      events: [...s.events, e].slice(-MAX_LIVE_EVENTS),
      cursor: e.cursor > s.cursor ? e.cursor : s.cursor,
    }))
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

  return () => {
    es.close()
    source = undefined
    useStream.setState({ connected: false })
  }
}
