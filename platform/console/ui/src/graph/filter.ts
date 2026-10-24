import { styleFor } from './shapes'
import { reach, refused } from './route'
import { isSpine, type ViewEdge, type ViewGraph, type ViewNode } from './types'

// What gets drawn, and the honesty accounting that keeps a filter from
// concealing a failure. Every cut reports what it removed on the same terms:
// how many, how many of those are failing, and in which classes.

export interface HiddenSummary {
  count: number
  failing: number
  classes: string[]
}

export const NOTHING_HIDDEN: HiddenSummary = { count: 0, failing: 0, classes: [] }

export function summarize(removed: ViewNode[]): HiddenSummary {
  const failing = removed.filter((n) => n.health === 'bad')
  return {
    count: removed.length,
    failing: failing.length,
    classes: [...new Set(failing.map((n) => styleFor(n.cls).label))].sort((a, b) => a.localeCompare(b)),
  }
}

// ---- find and hide ---------------------------------------------------------------

export interface ExprContext {
  /** Hops per node in the window. */
  counts: Map<string, number>
  windowSeconds: number
}

export interface Compiled {
  test: (n: ViewNode) => boolean
  /** Terms the grammar does not know. They match nothing, and are named. */
  unknown: string[]
}

/**
 * compile reads an expression over the view's facts, terms joined by `and`:
 * `healthy`, `!healthy`, `idle`, `detached`, `kind=pipeline`, `name~k8s`,
 * `name=ops`, `bundle=kubernetes`, `node=node-a`, `pipeline=ops`, `rate>1`.
 */
export function compile(expr: string, ctx: ExprContext): Compiled | null {
  const terms = expr.trim().toLowerCase().split(/\s+and\s+/).filter(Boolean)
  if (terms.length === 0) return null
  const unknown: string[] = []
  const preds = terms.map((t): ((n: ViewNode) => boolean) => {
    let m: RegExpMatchArray | null
    if (t === '!healthy' || t === 'unhealthy') return (n) => n.health === 'bad'
    if (t === 'healthy') return (n) => n.health === 'ok'
    if (t === 'idle') return (n) => !((ctx.counts.get(n.id) ?? 0) > 0)
    if (t === '!idle') return (n) => (ctx.counts.get(n.id) ?? 0) > 0
    if (t === 'detached') return (n) => Boolean(n.detached)
    if ((m = t.match(/^kind\s*=\s*([a-z-]+)$/))) {
      const k = m[1]
      return (n) => {
        const label = styleFor(n.cls).label.toLowerCase().replace(/\s+/g, '-')
        return n.cls === k || n.cls === `${k}s` || label === k || n.cls === k.replace(/s$/, '')
      }
    }
    if ((m = t.match(/^name\s*~\s*(\S+)$/))) {
      const v = m[1]
      return (n) => n.name.toLowerCase().includes(v)
    }
    if ((m = t.match(/^name\s*=\s*(\S+)$/))) {
      const v = m[1]
      return (n) => n.name.toLowerCase() === v
    }
    if ((m = t.match(/^bundle\s*=\s*(\S+)$/))) {
      const v = m[1]
      return (n) => (n.bundle ?? '').toLowerCase() === v
    }
    if ((m = t.match(/^node\s*=\s*(\S+)$/))) {
      const v = m[1]
      return (n) => (n.clusterNode ?? '').toLowerCase() === v
    }
    if ((m = t.match(/^pipeline\s*=\s*(\S+)$/))) {
      const v = m[1]
      return (n) => (n.pipeline ?? (n.cls === 'pipelines' ? n.name : '')).toLowerCase() === v
    }
    if ((m = t.match(/^rate\s*([<>])\s*([\d.]+)$/))) {
      const [op, v] = [m[1], Number(m[2])]
      return (n) => {
        const r = (ctx.counts.get(n.id) ?? 0) / (ctx.windowSeconds / 60)
        return op === '>' ? r > v : r < v
      }
    }
    unknown.push(t)
    return () => false
  })
  return { test: (n) => preds.every((p) => p(n)), unknown }
}

// ---- the cuts ----------------------------------------------------------------------

export type ScopeDepth = number | 'all'

export interface Scope {
  id: string
  depth: ScopeDepth
}

export interface VisibleOptions {
  hiddenClasses: ReadonlySet<string>
  hide?: Compiled | null
  /** Route members of the selected pipelines, or undefined for all. */
  routes?: ReadonlySet<string>
  idleNodes: boolean
  idleEdges: boolean
  /** Edges and nodes with events in the window. */
  busyEdges: ReadonlySet<string>
  busyNodes: ReadonlySet<string>
  scope?: Scope
  /**
   * What a scope reaches, when the view's own edges are not the route: on
   * Components and Infrastructure it is the routes through the element.
   */
  scopeMembers?: ReadonlySet<string>
}

export interface Visible {
  nodes: ViewNode[]
  edges: ViewEdge[]
  /** The display control's cut: classes, the hide expression and idle filtering. */
  hidden: HiddenSummary
  /** The pipeline selector's cut. */
  outOfRoutes: HiddenSummary
  /** The scope's cut. */
  outOfScope: HiddenSummary
  /** Connected to the scoped element but past the depth. */
  beyondDepth: number
  /** The furthest hop the scope's route has: what the depth control offers. */
  maxDepth: number
}

export function visible(g: ViewGraph, o: VisibleOptions): Visible {
  const within = (nodes: ViewNode[], edges: ViewEdge[]) => {
    const ids = new Set(nodes.map((n) => n.id))
    return edges.filter((e) => ids.has(e.from) && ids.has(e.to))
  }
  const displayed = g.nodes.filter(
    (n) => isSpine(g, n) || (!o.hiddenClasses.has(n.cls) && !(o.hide && o.hide.test(n))),
  )
  // Route selection works over the whole view, so hiding a class never
  // disconnects a route from its pipeline. It removes other pipelines — that
  // is what it is for — and the manager is on every route already.
  let nodes = o.routes ? displayed.filter((n) => o.routes!.has(n.id)) : displayed
  const routed = new Set(nodes.map((n) => n.id))
  let edges = within(nodes, g.edges)
  if (!o.idleEdges) edges = edges.filter((e) => o.busyEdges.has(e.id) || e.dangling)
  if (!o.idleNodes) {
    // A broken reference is never idle, it is wrong; and a failing element
    // stays whatever its traffic, like every other cut here.
    const touched = new Set(edges.flatMap((e) => [e.from, e.to]))
    nodes = nodes.filter((n) => touched.has(n.id) || o.busyNodes.has(n.id) || n.health === 'bad' || isSpine(g, n))
    edges = within(nodes, edges)
  }
  const shown = new Set(nodes.map((n) => n.id))
  const displayedIds = new Set(displayed.map((n) => n.id))
  const hidden = summarize(
    g.nodes.filter((n) => !displayedIds.has(n.id) || (routed.has(n.id) && !shown.has(n.id))),
  )
  const outOfRoutes = summarize(displayed.filter((n) => !routed.has(n.id)))

  let outOfScope = NOTHING_HIDDEN
  let beyondDepth = 0
  let maxDepth = 0
  if (o.scope && shown.has(o.scope.id)) {
    const hop = scopeDistances(o.scope.id, nodes, edges, o.scopeMembers)
    maxDepth = Math.max(0, ...hop.values())
    const depth = o.scope.depth
    const keep = (id: string) => hop.has(id) && (depth === 'all' || (hop.get(id) ?? 0) <= depth)
    const removed = nodes.filter((n) => !keep(n.id))
    beyondDepth = removed.filter((n) => hop.has(n.id)).length
    outOfScope = summarize(removed)
    nodes = nodes.filter((n) => keep(n.id))
    edges = within(nodes, edges)
  }
  return { nodes, edges, hidden, outOfRoutes, outOfScope, beyondDepth, maxDepth }
}

/**
 * Distances along the scoped element's route. Where a view supplies the route's
 * members, the distance is the shortest path inside them; otherwise it is the
 * oriented walk over what is drawn.
 */
function scopeDistances(
  id: string, nodes: ViewNode[], edges: ViewEdge[], members?: ReadonlySet<string>,
): Map<string, number> {
  if (!members) return reach(id, edges)
  const adj = new Map<string, string[]>()
  for (const e of edges) {
    if (refused(e) || !members.has(e.from) || !members.has(e.to)) continue
    adj.set(e.from, [...(adj.get(e.from) ?? []), e.to])
    adj.set(e.to, [...(adj.get(e.to) ?? []), e.from])
  }
  const seen = new Map([[id, 0]])
  let frontier = [id]
  while (frontier.length) {
    const next: string[] = []
    for (const a of frontier) {
      for (const b of adj.get(a) ?? []) {
        if (seen.has(b)) continue
        seen.set(b, (seen.get(a) ?? 0) + 1)
        next.push(b)
      }
    }
    frontier = next
  }
  // A member the drawn edges do not join is still on the route: it sits one
  // past the furthest, never out of scope.
  const far = Math.max(0, ...seen.values()) + 1
  const ids = new Set(nodes.map((n) => n.id))
  for (const m of members) if (ids.has(m) && !seen.has(m)) seen.set(m, far)
  return seen
}

/** The depth levels worth offering: the rings strictly inside the route, then all. */
export function depthLevels(maxDepth: number): ScopeDepth[] {
  const rings = Math.max(0, Math.min(maxDepth - 1, 3))
  return rings === 0 ? [] : [...Array.from({ length: rings }, (_, i) => i + 1), 'all' as const]
}
