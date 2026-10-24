import { Layout as ColaLayout, type Group as ColaGroup, type Link, type Node as ColaNode } from 'webcola'
import type { Pt } from './curve'
import { layoutSize, type Canvas, type Group, type LayoutEdge, type LayoutNode } from './geometry'

// COLA is WebCola, the constraint layout behind Kiali's option of that name,
// with the ownership groups kept together.

export function colaLayout(nodes: LayoutNode[], edges: LayoutEdge[], groups: Group[], canvas: Canvas): Map<string, Pt> {
  const idx = new Map(nodes.map((n, i) => [n.id, i]))
  const cn: ColaNode[] = nodes.map((n) => ({ ...layoutSize(n.name), x: n.x, y: n.y }))
  const cl: Link<number>[] = edges
    .filter((e) => idx.has(e.from) && idx.has(e.to) && e.from !== e.to)
    .map((e) => ({ source: idx.get(e.from)!, target: idx.get(e.to)! }))
  const gi = new Map(groups.map((g, i) => [g.id, i]))
  const cg: { leaves: number[]; groups: number[]; padding: number }[] = groups.map((g) => ({
    leaves: g.members.filter((m) => idx.has(m)).map((m) => idx.get(m)!),
    groups: [],
    padding: 24,
  }))
  for (const g of groups) if (g.parent && gi.has(g.parent)) cg[gi.get(g.parent)!].groups.push(gi.get(g.id)!)
  // handleDisconnected packs components AFTER the group bounds are computed,
  // which moves a disconnected bundle out of its own box. Off, and the boxes
  // are drawn from where the members ended up.
  const layout = new ColaLayout()
    .nodes(cn)
    .links(cl)
    .groups(cg as unknown as ColaGroup[])
    .avoidOverlaps(true)
    .handleDisconnected(false)
    .linkDistance(136)
    .size([canvas.w, canvas.h])
  layout.start(60, 40, 90, 0, false)
  const pos = new Map<string, Pt>()
  nodes.forEach((n, i) => pos.set(n.id, { x: cn[i].x, y: cn[i].y }))
  return pos
}
