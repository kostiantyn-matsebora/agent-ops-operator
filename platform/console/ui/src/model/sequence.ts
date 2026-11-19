import type { ActivityEvent } from '../api/types'

// THE SEQUENCE VIEW'S FACTS: a conversation's recorded hops in order, each as
// a route, an offset from the first hop and a latency. Every look draws these
// rows; none recomputes them.

export interface SequenceRow {
  key: string
  /** The hop's kind (`signal`, `dispatch`, …). */
  hop: string
  /** `Kind/name → Kind/name`, with `∅` for an end the hop does not name. */
  route: string
  /** Seconds after the first hop, as `+1.2s`. */
  at: string
  /** The hop's own latency in seconds, or an em dash when it recorded none. */
  latency: string
  errored: boolean
}

function endpoint(ref: { kind: string; name: string } | undefined): string {
  return ref ? `${ref.kind}/${ref.name}` : '∅'
}

/** The rows of a conversation's sequence; empty when the buffer holds no hop for it. */
export function sequenceRows(events: ActivityEvent[]): SequenceRow[] {
  const times = events.map((e) => new Date(e.ts).getTime()).filter((t) => !Number.isNaN(t))
  const start = times.length ? Math.min(...times) : 0
  return events.map((e) => ({
    key: e.cursor,
    hop: e.kind,
    route: `${endpoint(e.from)} → ${endpoint(e.to)}`,
    at: `+${((new Date(e.ts).getTime() - start) / 1000).toFixed(1)}s`,
    latency: e.latencyMs ? `${(e.latencyMs / 1000).toFixed(2)}s` : '—',
    errored: e.status === 'error',
  }))
}
