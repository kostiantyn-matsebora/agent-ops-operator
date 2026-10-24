import dagre from 'dagre'
import type { Pt } from './curve'
import { layoutSize, memberBoxes, type Canvas, type Group, type LayoutEdge, type LayoutNode } from './geometry'

// DAGRE ranks along the flow, for the model read as a flow. It is left alone:
// no compaction and no stretching, since ranks ARE its picture.
//
// CYCLE BREAKING IS INTEGER-WEIGHTED. The wiring has cycles — a run goes out on
// runs-on and comes back — and the greedy acyclicer reverses the cheapest
// edges to break them. Its buckets are indexed by weight, so weights must be
// integers, and the flow's own edges weigh most so they are the last reversed.

const WEIGHT: Record<string, number> = {
  feeds: 4, posts: 4, 'served-by': 4, 'runs-on': 4, 'signal/inbound': 4, 'channel/ops': 4,
  work: 4, '/work': 4, CONTROL_URL: 4, control: 4, redirected: 4, sends: 4, calls: 4,
  answers: 3, model: 3, tool: 2, uses: 2, reconcile: 2, updates: 3, getUpdates: 2,
}

export interface DagreResult {
  pos: Map<string, Pt>
  routes: Map<string, Pt[]>
  orientation: 'tb' | 'lr'
}

function direction(nodes: LayoutNode[], edges: LayoutEdge[], groups: Group[], dir: 'tb' | 'lr', compound: boolean) {
  const g = new dagre.graphlib.Graph({ compound, multigraph: true })
  g.setGraph({ rankdir: dir === 'lr' ? 'LR' : 'TB', nodesep: 36, ranksep: 80, edgesep: 18, acyclicer: 'greedy', ranker: 'network-simplex' })
  g.setDefaultEdgeLabel(() => ({}))
  const ids = new Set(nodes.map((n) => n.id))
  for (const n of nodes) g.setNode(n.id, layoutSize(n.name))
  if (compound) {
    for (const grp of groups) g.setNode(grp.id, {})
    for (const grp of groups) {
      for (const m of grp.members) if (ids.has(m)) g.setParent(m, grp.id)
      if (grp.parent) g.setParent(grp.id, grp.parent)
    }
  }
  const flipped = new Set<string>()
  for (const e of edges) {
    if (!ids.has(e.from) || !ids.has(e.to) || e.from === e.to) continue
    // a signal adapter FEEDS its source; `served-by` is drawn against the flow
    const flip = e.to.startsWith('signaladapters/')
    if (flip) flipped.add(e.id)
    g.setEdge(flip ? e.to : e.from, flip ? e.from : e.to, { weight: WEIGHT[e.kind] ?? 1, minlen: 1 }, e.id)
  }
  dagre.layout(g)
  const pos = new Map<string, Pt>()
  for (const n of nodes) {
    const v = g.node(n.id)
    pos.set(n.id, { x: v.x, y: v.y })
  }
  const routes = new Map<string, Pt[]>()
  for (const e of g.edges()) {
    const pts = (g.edge(e) as { points?: Pt[] }).points ?? []
    const id = e.name ?? ''
    routes.set(id, flipped.has(id) ? [...pts].reverse() : pts)
  }
  return { pos, routes }
}

function run(nodes: LayoutNode[], edges: LayoutEdge[], groups: Group[], dir: 'tb' | 'lr') {
  try {
    return direction(nodes, edges, groups, dir, groups.length > 0)
  } catch {
    // Dagre's compound ranking can refuse a nesting it cannot order. The boxes
    // are drawn from the members either way, so the flat ranking is the answer.
    return direction(nodes, edges, groups, dir, false)
  }
}

/** Both orientations are laid out, and the one that fits the canvas larger wins. */
export function dagreLayout(nodes: LayoutNode[], edges: LayoutEdge[], groups: Group[], canvas: Canvas): DagreResult {
  const score = (pos: Map<string, Pt>) => {
    const ps = [...pos.values()]
    const bs = memberBoxes(groups, pos)
    const x0 = Math.min(...ps.map((p) => p.x - 50), ...bs.map((b) => b.x0))
    const x1 = Math.max(...ps.map((p) => p.x + 50), ...bs.map((b) => b.x0 + b.w))
    const y0 = Math.min(...ps.map((p) => p.y - 46), ...bs.map((b) => b.y0))
    const y1 = Math.max(...ps.map((p) => p.y + 72), ...bs.map((b) => b.y0 + b.h))
    return Math.min(canvas.w / (x1 - x0 || 1), canvas.h / (y1 - y0 || 1))
  }
  const tb = run(nodes, edges, groups, 'tb')
  const lr = run(nodes, edges, groups, 'lr')
  return score(lr.pos) > score(tb.pos) * 1.05 ? { ...lr, orientation: 'lr' } : { ...tb, orientation: 'tb' }
}
