import type { ActivityEvent, Topology } from '../api/types'
import { crossing, hopIndex } from './hops'
import type { ViewEdge, ViewGraph } from './types'
import { MANAGER } from './views/classes'
import { CONTEXT_SYNC, EGRESS_PROXY, substrate } from './views/components'
import { containerId } from './views/infrastructure'

// THE ROUTE WALK, once, for the detail fold, the pipeline selector, the route
// boxes and scope-to-route alike.
//
// A route is the element and everything it reaches along the wiring, DOWN and
// UP walked separately and unioned, so a path never turns around through a
// shared element — one runtime serves every profile, one channel receives from
// every pipeline, and an undirected walk used them as shortcuts between routes
// that have nothing to do with each other.

const SIGNAL_SIDE = /^(signalsources|signaladapters|signal-adapter)\//
const CHANNEL_ADAPTER = /^(channeladapters|channel-adapter)\//

/**
 * refused is the walk's first refusal. A channel adapter that also serves a
 * chat source is two roles on one object, and entering through the source and
 * leaving through the channel joined every pipeline posting there to every
 * pipeline reading the source.
 */
export function refused(e: Pick<ViewEdge, 'kind' | 'from' | 'to'>): boolean {
  if (e.kind !== 'served-by') return false
  return (CHANNEL_ADAPTER.test(e.from) && SIGNAL_SIDE.test(e.to)) || (CHANNEL_ADAPTER.test(e.to) && SIGNAL_SIDE.test(e.from))
}

/**
 * Oriented BY FLOW, which is not always how an edge is drawn: a signal adapter
 * FEEDS its source, while `served-by` is drawn source -> adapter.
 */
function oriented(e: Pick<ViewEdge, 'from' | 'to'>): [string, string] {
  return e.to.startsWith('signaladapters/') ? [e.to, e.from] : [e.from, e.to]
}

/** Hop distance from `from` to everything on its route. */
export function reach(from: string, edges: Pick<ViewEdge, 'kind' | 'from' | 'to'>[]): Map<string, number> {
  const down = new Map<string, string[]>()
  const up = new Map<string, string[]>()
  const push = (m: Map<string, string[]>, a: string, b: string) => m.set(a, [...(m.get(a) ?? []), b])
  for (const e of edges) {
    if (refused(e)) continue
    const [a, b] = oriented(e)
    push(down, a, b)
    push(up, b, a)
  }
  const walk = (adj: Map<string, string[]>) => {
    const seen = new Map([[from, 0]])
    let frontier = [from]
    while (frontier.length) {
      const next: string[] = []
      for (const id of frontier) {
        for (const t of adj.get(id) ?? []) {
          if (seen.has(t)) continue
          seen.set(t, (seen.get(id) ?? 0) + 1)
          next.push(t)
        }
      }
      frontier = next
    }
    return seen
  }
  const hop = walk(down)
  for (const [id, d] of walk(up)) if (!hop.has(id) || d < (hop.get(id) ?? Infinity)) hop.set(id, d)
  return hop
}

export function pipelinesOf(topo: Topology): string[] {
  return topo.nodes.filter((n) => n.kind === 'pipelines').map((n) => n.name).sort((a, b) => a.localeCompare(b))
}

/** Route ownership: the one pipeline that reaches a Model node, when exactly one does. */
export function routeOwners(topo: Topology): Map<string, string> {
  const count = new Map<string, string[]>()
  for (const p of pipelinesOf(topo)) {
    for (const id of reach(`pipelines/${p}`, topo.edges).keys()) count.set(id, [...(count.get(id) ?? []), p])
  }
  const out = new Map<string, string>()
  for (const [id, ps] of count) if (ps.length === 1) out.set(id, ps[0])
  return out
}

/**
 * routeMembers is what one pipeline's route draws on a view.
 *
 * The walk runs on the MODEL, where a runtime image is not a node, and is
 * mapped onto the view. That is the walk's second refusal: an image every
 * route shares is never walked through, so its calls reach the route only
 * along the route's own MCP configs — and along the hops the route's own
 * conversations actually made.
 */
export function routeMembers(
  topo: Topology, g: ViewGraph, pipeline: string, events: ActivityEvent[] = [],
): Set<string> {
  const model = [...reach(`pipelines/${pipeline}`, topo.edges).keys()]
  const out = new Set<string>()
  const present = new Set(g.nodes.map((n) => n.id))
  const add = (id?: string) => {
    if (id && present.has(id)) out.add(id)
  }
  if (g.view === 'model') {
    for (const id of model) add(id)
    return out
  }
  const s = substrate(topo)
  const comps = new Set<string>()
  for (const id of model) {
    const impl = s.implementer(id)
    if (impl.length) impl.forEach((c) => comps.add(c))
    else comps.add(MANAGER)
  }
  for (const c of [...comps]) {
    if (c.startsWith('channel-adapter/')) comps.add(`gateway/${c.slice(c.indexOf('/') + 1)}`)
  }
  for (const e of topo.componentEdges ?? []) {
    if (comps.has(e.from)) comps.add(e.to)
    if (comps.has(e.to)) comps.add(e.from)
  }
  const pods = (topo.pods ?? []).filter((p) => p.pipeline === pipeline)
  if (g.view === 'components') {
    comps.add(CONTEXT_SYNC)
    comps.add(EGRESS_PROXY)
    for (const c of comps) add(c)
    for (const p of pods) add(p.component)
  } else {
    const podOf = new Map<string, string>()
    for (const p of topo.pods ?? []) if (p.component && !p.conversation && !podOf.has(p.component)) podOf.set(p.component, p.id)
    for (const c of comps) {
      add(podOf.get(c) ?? podOf.get(s.byId.get(c)?.servedBy ?? '') ?? c)
    }
    for (const p of pods) {
      add(p.id)
      for (const c of p.containers) add(containerId(p, c.role === 'runtime-image' ? 'agent' : c.role || c.name))
    }
  }
  const index = hopIndex(g)
  for (const ev of events) {
    if (ev.pipeline !== pipeline) continue
    const x = crossing(g, index, ev)
    if (!x) continue
    add(x.pop)
    for (const { edge } of x.edges) {
      add(edge.from)
      add(edge.to)
    }
  }
  return out
}
