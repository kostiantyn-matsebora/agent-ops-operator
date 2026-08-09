import type { ActivityEvent, GraphEdge, GraphNode, Topology } from '../api/types'
import { isHidden } from './display'

// Turning the BFF's graph into what gets drawn — filtering, traffic scaling, and
// the honesty accounting that keeps a filter from concealing a failure.

export interface HiddenSummary {
  /** Nodes removed by the Display panel. */
  count: number
  /** Of those, how many are unhealthy — the number that must be surfaced. */
  failing: number
  /** Which classes those failures are in, so the indicator can name them. */
  classes: string[]
}

export interface VisibleGraph {
  nodes: GraphNode[]
  edges: GraphEdge[]
  hiddenSummary: HiddenSummary
  /** Health across the WHOLE graph, hidden nodes included. */
  health: { ok: number; bad: number; unknown: number }
}

/**
 * visibleGraph applies the Display panel to a topology.
 *
 * The health summary is computed over ALL nodes before filtering, and hidden
 * failures are counted separately. That is the rule that makes the panel safe:
 * you can simplify the picture, but you cannot make a broken component
 * disappear silently.
 */
export function visibleGraph(
  topo: Topology,
  hidden: Record<string, boolean>,
  showIdle: boolean,
): VisibleGraph {
  const health = { ok: 0, bad: 0, unknown: 0 }
  for (const n of topo.nodes) {
    if (n.health === 'bad') health.bad++
    else if (n.health === 'unknown') health.unknown++
    else if (n.health === 'ok') health.ok++
  }

  const hiddenNodes = topo.nodes.filter((n) => isHidden(hidden, n.kind))
  const failingClasses = new Set(hiddenNodes.filter((n) => n.health === 'bad').map((n) => n.kind))
  const hiddenSummary: HiddenSummary = {
    count: hiddenNodes.length,
    failing: hiddenNodes.filter((n) => n.health === 'bad').length,
    classes: [...failingClasses].sort(),
  }

  let nodes = topo.nodes.filter((n) => !isHidden(hidden, n.kind))
  const visible = new Set(nodes.map((n) => n.id))
  let edges = topo.edges.filter((e) => visible.has(e.from) && visible.has(e.to))

  if (!showIdle) {
    // "Idle" means NO EVENTS in the window — which is a different statement
    // from "not wired". Broken edges stay: a dangling ref is never idle, it is
    // wrong, and hiding it under a traffic filter would be the same mistake as
    // hiding a failing node.
    edges = edges.filter((e) => e.traffic || e.dangling)
    const touched = new Set(edges.flatMap((e) => [e.from, e.to]))
    nodes = nodes.filter((n) => touched.has(n.id) || n.health === 'bad')
  }
  return { nodes, edges, hiddenSummary, health }
}

/**
 * animationDuration maps an edge's event rate to a dash-animation period.
 *
 * Faster traffic, faster dashes — the Kiali idiom. Clamped at both ends: below
 * the floor a busy edge would strobe unreadably, above the ceiling a trickle
 * would look stopped, and "stopped" already means something else here.
 */
export function animationDuration(ratePerMin: number): number {
  if (ratePerMin <= 0) return 0
  const seconds = 6 / Math.max(ratePerMin, 0.1)
  return Math.min(Math.max(seconds, 0.4), 6)
}

export type EdgeTone = 'idle' | 'ok' | 'error' | 'unconfirmed'

/** edgeTone classifies an edge for rendering. */
export function edgeTone(e: GraphEdge): EdgeTone {
  if (e.dangling) return 'error'
  if (!e.traffic) return 'idle'
  if (e.traffic.errors > 0) return 'error'
  // Enqueued with no delivery confirmation is NOT success. Adapter reporting is
  // optional, so an adapter that reports nothing must not look like one that
  // delivered — it looks like "sent, unconfirmed", which is what it is.
  if (e.traffic.unconfirmed) return 'unconfirmed'
  return 'ok'
}

/** edgeLabel renders the Display panel's chosen label for an edge. */
export function edgeLabel(e: GraphEdge, mode: 'none' | 'rate' | 'latency'): string {
  if (mode === 'none' || !e.traffic) return ''
  if (mode === 'rate') {
    const r = e.traffic.ratePerMin
    return r >= 1 ? `${r.toFixed(1)}/min` : `${(r * 60).toFixed(1)}/h`
  }
  const ms = e.traffic.p50LatencyMs
  if (!ms) return ''
  return ms >= 1000 ? `${(ms / 1000).toFixed(1)}s` : `${ms}ms`
}

/**
 * liveEdgeIds maps recent activity events onto graph edge ids, so an event
 * arriving on the stream flashes the edge it crossed WITHOUT waiting for the
 * next topology refetch.
 *
 * It resolves against the DRAWN edges, in either direction and through the
 * conversation's pipeline, for the same three reasons the server-side rate
 * mapping does (see applyTraffic in topology.go): a signal travels adapter →
 * source while the graph draws source → adapter; a run crosses pipeline →
 * profile → runtime in one hop; and ops name a conversation, which is not a
 * wiring node. Matching only exact from→to pairs left almost the whole flow
 * unanimated.
 */
export function liveEdgeIds(
  events: ActivityEvent[],
  eventNodeKinds: Record<string, string>,
  edges: GraphEdge[],
): Set<string> {
  // Undirected index of what is actually drawn.
  const drawn = new Map<string, string>()
  for (const e of edges) {
    drawn.set(`${e.from}|${e.to}`, edgeId(e))
    drawn.set(`${e.to}|${e.from}`, edgeId(e))
  }
  const out = new Set<string>()

  const node = (ref?: { kind: string; name: string }): string | undefined => {
    if (!ref) return undefined
    const kind = eventNodeKinds[ref.kind]
    return kind ? `${kind}/${ref.name}` : undefined
  }
  const link = (a?: string, b?: string) => {
    if (!a || !b) return false
    const id = drawn.get(`${a}|${b}`)
    if (id) out.add(id)
    return Boolean(id)
  }

  for (const e of events) {
    const from = node(e.from)
    const to = node(e.to)
    if (from && to) {
      // A pipeline↔runtime hop crossed the profile in between; light the whole
      // path rather than an edge the graph never drew.
      if (!link(from, to) && e.pipeline) {
        const p = `pipelines/${e.pipeline}`
        for (const [k, id] of drawn) {
          const [a, b] = k.split('|')
          if ((a === p || b === p) && (a === from || b === from || a === to || b === to)) out.add(id)
        }
      }
      continue
    }
    // One endpoint is a conversation: credit its pipeline's edge.
    const wiring = from ?? to
    if (wiring && e.pipeline) link(wiring, `pipelines/${e.pipeline}`)
  }
  return out
}

export function edgeId(e: GraphEdge): string {
  return `${e.from}->${e.to}`
}
