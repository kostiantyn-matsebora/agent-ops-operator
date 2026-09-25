import type { ActivityEvent, EdgeTraffic } from '../api/types'
import type { ViewEdge, ViewGraph } from './types'

// ONE CREDITING CORE FOR EVERY VIEW. A view says which paths a hop may have
// taken, most direct first; this takes the first one it actually draws, which
// is the rule the BFF's applyTraffic already applies to the Model. Matching is
// UNDIRECTED, since traffic on an edge means the two ends exchanged something,
// and the pulse still travels the direction the hop did.

export interface HopIndex {
  pairs: Map<string, ViewEdge>
  nodes: Set<string>
}

export function hopIndex(g: Pick<ViewGraph, 'nodes' | 'edges'>): HopIndex {
  const pairs = new Map<string, ViewEdge>()
  for (const e of g.edges) {
    pairs.set(`${e.from}|${e.to}`, e)
    if (!pairs.has(`${e.to}|${e.from}`)) pairs.set(`${e.to}|${e.from}`, e)
  }
  return { pairs, nodes: new Set(g.nodes.map((n) => n.id)) }
}

export interface Crossing {
  /** The drawn edges crossed, in order, each with the direction it was travelled. */
  edges: { edge: ViewEdge; dir: 1 | -1 }[]
  /** A hop with no destination pulses on its node instead. */
  pop?: string
}

export function crossing(g: Pick<ViewGraph, 'legs'>, index: HopIndex, ev: ActivityEvent): Crossing | null {
  for (const path of g.legs(ev)) {
    const legs = path.filter(([a, b]) => a && a !== b)
    if (legs.length === 0) continue
    const moving = legs.filter(([, b]) => b !== null)
    if (moving.length === 0) {
      if (index.nodes.has(legs[0][0])) return { edges: [], pop: legs[0][0] }
      continue
    }
    const found = moving.map(([a, b]) => index.pairs.get(`${a}|${b}`))
    if (found.every(Boolean)) {
      return { edges: found.map((edge, i) => ({ edge: edge!, dir: edge!.from === moving[i][0] ? 1 : -1 })) }
    }
  }
  return null
}

export interface EdgeStats extends EdgeTraffic {
  latencies: number[]
  last: number
}

export interface WindowStats {
  edges: Map<string, EdgeStats>
  /** Hops touching each node, whether they moved or pulsed on it. */
  nodes: Map<string, number>
}

export function tsOf(ev: ActivityEvent): number {
  const t = Date.parse(ev.ts)
  return Number.isNaN(t) ? 0 : t
}

/**
 * windowStats credits every hop in (tEnd - windowMs, tEnd] to the drawn edges
 * it crossed. Enqueued-with-no-confirmation is marked on the edge exactly as
 * the BFF marks it: intent seen, and no adapter report.
 */
export function windowStats(
  g: Pick<ViewGraph, 'legs'>, index: HopIndex, events: ActivityEvent[], tEnd: number, windowMs: number,
): WindowStats {
  const acc = new Map<string, EdgeStats & { intent: boolean; confirmed: boolean }>()
  const nodes = new Map<string, number>()
  const tStart = tEnd - windowMs
  for (const ev of events) {
    const t = tsOf(ev)
    if (t <= tStart || t > tEnd) continue
    const x = crossing(g, index, ev)
    if (!x) continue
    if (x.pop) nodes.set(x.pop, (nodes.get(x.pop) ?? 0) + 1)
    for (const { edge } of x.edges) {
      for (const id of [edge.from, edge.to]) nodes.set(id, (nodes.get(id) ?? 0) + 1)
      let s = acc.get(edge.id)
      if (!s) {
        s = { events: 0, errors: 0, ratePerMin: 0, latencies: [], last: 0, intent: false, confirmed: false }
        acc.set(edge.id, s)
      }
      s.events++
      if (ev.status === 'error') s.errors++
      if (ev.latencyMs) s.latencies.push(ev.latencyMs)
      if (t > s.last) s.last = t
      if (ev.kind === 'channel.op.enqueued') s.intent = true
      if (ev.kind === 'channel.op.completed') s.confirmed = true
    }
  }
  const edges = new Map<string, EdgeStats>()
  for (const [id, s] of acc) {
    const lat = [...s.latencies].sort((a, b) => a - b)
    edges.set(id, {
      events: s.events, errors: s.errors, ratePerMin: s.events / (windowMs / 60000),
      p50LatencyMs: lat.length ? lat[Math.floor(lat.length / 2)] : undefined,
      maxLatencyMs: lat.length ? lat[lat.length - 1] : undefined,
      lastTs: s.last ? new Date(s.last).toISOString() : undefined,
      unconfirmed: s.intent && !s.confirmed, latencies: lat, last: s.last,
    })
  }
  return { edges, nodes }
}

export type EdgeTone = 'idle' | 'ok' | 'error' | 'unconfirmed'

export function edgeTone(e: ViewEdge, t?: EdgeTraffic): EdgeTone {
  if (e.dangling) return 'error'
  if (!t || t.events === 0) return 'idle'
  if (t.errors > 0) return 'error'
  // Enqueued with no delivery confirmation is NOT success. Adapter reporting is
  // optional, so one that reports nothing must not look like one that delivered.
  if (t.unconfirmed) return 'unconfirmed'
  return 'ok'
}

export type EdgeLabel = 'none' | 'rate' | 'latency'

/** The chosen label, and nothing — never a zero — on an edge with no events. */
export function edgeLabel(t: EdgeTraffic | undefined, mode: EdgeLabel): string {
  if (mode === 'none' || !t || t.events === 0) return ''
  if (mode === 'rate') {
    const r = t.ratePerMin
    return r >= 1 ? `${r.toFixed(1)}/min` : `${(r * 60).toFixed(0)}/h`
  }
  const ms = t.p50LatencyMs
  if (!ms) return ''
  return ms >= 1000 ? `${(ms / 1000).toFixed(1)}s` : `${ms}ms`
}

/**
 * mergeEvents joins what the buffer held when the page opened with what the
 * stream has delivered since, once each, in cursor order. A console-local
 * marker carries no cursor and is not a hop.
 */
export function mergeEvents(...lists: ActivityEvent[][]): ActivityEvent[] {
  const byCursor = new Map<string, ActivityEvent>()
  for (const list of lists) for (const e of list) if (e.cursor) byCursor.set(e.cursor, e)
  return [...byCursor.values()].sort((a, b) => (a.cursor < b.cursor ? -1 : a.cursor > b.cursor ? 1 : 0))
}
