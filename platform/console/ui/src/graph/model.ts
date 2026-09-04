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
    classes: [...failingClasses].sort((a, b) => a.localeCompare(b)),
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
/** How far a scope reaches: a number of hops, or the whole connected component. */
export type ScopeDepth = number | 'all'

export interface Scope {
  /** The node the graph is scoped to. */
  id: string
  depth: ScopeDepth
}

export interface ScopedGraph {
  nodes: GraphNode[]
  edges: GraphEdge[]
  /** What the scope removed, in the same shape the display panel's filter reports. */
  outOfScope: HiddenSummary
  /** Connected to the focused node but beyond the current depth. */
  beyondDepth: number
  /**
   * The furthest hop on the route. It is what the depth control offers: a
   * Pipeline sits at the CENTRE of its own route — sources, profile, toolsets,
   * MCP configs and channels are all one hop, and only the runtime and the
   * adapters are two — so offering four levels there gives four buttons and two
   * pictures, which reads as broken rather than as shallow.
   */
  maxDepth: number
}

/**
 * scopedGraph narrows a visible graph to one node and what it is connected to.
 *
 * THE SCOPE IS THE ROUTE THROUGH THE NODE: everything UPSTREAM of it plus
 * everything DOWNSTREAM of it. Both directions, because a scope answers "what is
 * this part of" as much as "what does this reach" — following edges forward only
 * would scope a channel to nothing, which is the opposite of the question.
 *
 * But a path may not TURN AROUND. Undirected traversal was tried and is wrong,
 * and only a real install shows why: agent-ops shares objects on purpose — one
 * AgentRuntime serves every profile, one Channel receives from every pipeline —
 * so an undirected walk uses them as shortcuts between things that have nothing
 * to do with each other. Measured on a 30-node install, `k8s-ops` reached 29 of
 * 30 nodes and the Home Assistant toolsets sat 3 hops away, via the console
 * channel. By route it reaches 15, and no `ha-*` at all. A hop count is only a
 * proxy for "related" in a graph without hubs, and this graph is all hubs.
 *
 * Down and up are computed SEPARATELY and unioned, so a node's distance is its
 * distance along whichever route reaches it. The focused node's own direction is
 * never mixed mid-path.
 *
 * It runs over the ALREADY-VISIBLE graph, so a class the display panel is
 * hiding never acts as an invisible stepping stone joining two nodes the
 * operator cannot see. Hiding a class can therefore split a component and
 * shrink a scope — which is the honest reading of "connected to what I am
 * looking at".
 *
 * A scope is a filter, so it reports what it removed on the same terms class
 * hiding does: an out-of-scope node that is FAILING is counted and its class
 * named. Health totals are computed elsewhere, over the whole topology, and are
 * not affected by scoping at all.
 */
export function scopedGraph(
  nodes: GraphNode[],
  edges: GraphEdge[],
  scope: Scope | undefined,
): ScopedGraph {
  const none: ScopedGraph = {
    nodes,
    edges,
    outOfScope: { count: 0, failing: 0, classes: [] },
    beyondDepth: 0,
    maxDepth: 0,
  }
  // An id that is not on the graph scopes to nothing, which would present an
  // empty canvas as the answer. The whole graph is the truthful fallback.
  if (!scope || !nodes.some((n) => n.id === scope.id)) return none

  const downstream = new Map<string, string[]>()
  const upstream = new Map<string, string[]>()
  const join = (m: Map<string, string[]>, a: string, b: string) => {
    const list = m.get(a)
    if (list) list.push(b)
    else m.set(a, [b])
  }

  // ORIENTED BY FLOW, which is not always how the edge is DRAWN.
  //
  // `served-by` runs from the served CR to its adapter for both kinds, and reads
  // correctly either way round — "this source is served by that adapter". But
  // the two adapter kinds sit at OPPOSITE ends of the flow: a channel adapter
  // delivers outward, so channel → adapter is already the direction work moves,
  // while a signal adapter FEEDS its source, so source → adapter is drawn
  // against it. Left as drawn, every signal adapter is a dead end and scopes to
  // itself plus its source — two nodes, on an install where the same adapter is
  // on the route of twenty-three.
  const kindOf = new Map(nodes.map((n) => [n.id, n.kind]))
  for (const e of edges) {
    const flip = kindOf.get(e.to) === 'signaladapters'
    const from = flip ? e.to : e.from
    const to = flip ? e.from : e.to
    join(downstream, from, to)
    join(upstream, to, from)
  }

  // Breadth-first, recording the hop at which each node is FIRST reached, so a
  // depth limit cuts by distance rather than by discovery order.
  const walk = (adjacency: Map<string, string[]>): Map<string, number> => {
    const seen = new Map<string, number>([[scope.id, 0]])
    let frontier = [scope.id]
    while (frontier.length > 0) {
      const next: string[] = []
      for (const id of frontier) {
        for (const to of adjacency.get(id) ?? []) {
          if (seen.has(to)) continue
          seen.set(to, (seen.get(id) ?? 0) + 1)
          next.push(to)
        }
      }
      frontier = next
    }
    return seen
  }

  // The nearer of the two routes wins, so a node reachable both ways is placed
  // at its shortest honest distance.
  const hop = walk(downstream)
  for (const [id, d] of walk(upstream)) {
    const had = hop.get(id)
    if (had === undefined || d < had) hop.set(id, d)
  }

  const within = (id: string): boolean => {
    const d = hop.get(id)
    return d !== undefined && (scope.depth === 'all' || d <= scope.depth)
  }

  const kept = nodes.filter((n) => within(n.id))
  const keptIds = new Set(kept.map((n) => n.id))
  const removed = nodes.filter((n) => !keptIds.has(n.id))
  const failingClasses = new Set(removed.filter((n) => n.health === 'bad').map((n) => n.kind))

  return {
    nodes: kept,
    edges: edges.filter((e) => keptIds.has(e.from) && keptIds.has(e.to)),
    outOfScope: {
      count: removed.length,
      failing: removed.filter((n) => n.health === 'bad').length,
      classes: [...failingClasses].sort((a, b) => a.localeCompare(b)),
    },
    // Connected, but cut off by the depth limit rather than by not being
    // connected at all — two different reasons to be missing.
    beyondDepth: removed.filter((n) => hop.has(n.id)).length,
    maxDepth: Math.max(0, ...nodes.filter((n) => hop.has(n.id)).map((n) => hop.get(n.id) ?? 0)),
  }
}

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
