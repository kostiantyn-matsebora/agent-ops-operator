import type { Pt } from './curve'
import type { LayoutEdge, LayoutNode } from './geometry'

// CONCENTRIC draws a hub as a hub: the view's hub at the centre, rings by graph
// distance, and each node at the angle of its inner neighbours so the spokes
// stay straight. Neighbours are then pushed apart until the arc between them
// holds a node.

export function concentric(nodes: LayoutNode[], edges: LayoutEdge[], hubId?: string): Map<string, Pt> {
  const pos = new Map<string, Pt>()
  const adj = new Map<string, Set<string>>()
  const link = (a: string, b: string) => adj.set(a, (adj.get(a) ?? new Set()).add(b))
  for (const e of edges) {
    link(e.from, e.to)
    link(e.to, e.from)
  }
  const hub =
    nodes.find((n) => n.id === hubId) ??
    [...nodes].sort((a, b) => (adj.get(b.id)?.size ?? 0) - (adj.get(a.id)?.size ?? 0))[0]
  if (!hub) return pos

  const hop = new Map([[hub.id, 0]])
  let frontier = [hub.id]
  while (frontier.length) {
    const next: string[] = []
    for (const id of frontier) {
      for (const t of adj.get(id) ?? []) {
        if (hop.has(t)) continue
        hop.set(t, (hop.get(id) ?? 0) + 1)
        next.push(t)
      }
    }
    frontier = next
  }
  const far = Math.max(1, ...hop.values())
  for (const n of nodes) if (!hop.has(n.id)) hop.set(n.id, far + 1)
  const rings = new Map<number, LayoutNode[]>()
  for (const n of nodes) {
    const k = hop.get(n.id)!
    rings.set(k, [...(rings.get(k) ?? []), n])
  }

  const angle = new Map([[hub.id, 0]])
  pos.set(hub.id, { x: 0, y: 0 })
  let radius = 0
  for (const k of [...rings.keys()].sort((a, b) => a - b)) {
    if (k === 0) continue
    const ring = rings.get(k)!
    const want = (n: LayoutNode) => {
      const inner = [...(adj.get(n.id) ?? [])].filter((id) => hop.get(id) === k - 1 && angle.has(id))
      if (inner.length === 0) return Math.PI
      const a = inner.map((id) => angle.get(id)!)
      return Math.atan2(a.reduce((t, x) => t + Math.sin(x), 0), a.reduce((t, x) => t + Math.cos(x), 0))
    }
    // The hub has no direction, so the first ring wanting "the hub's angle"
    // bunched on one side of it. That ring is spread evenly instead, and every
    // ring after it leans toward its own inner neighbours.
    const first = [...ring].sort((a, b) => a.name.localeCompare(b.name))
    const w = new Map(
      k === 1 ? first.map((n, i) => [n.id, (2 * Math.PI * i) / first.length - Math.PI]) : ring.map((n) => [n.id, want(n)]),
    )
    ring.sort((a, b) => w.get(a.id)! - w.get(b.id)! || a.name.localeCompare(b.name))
    radius += Math.max(210, (ring.length * 176) / (2 * Math.PI) + 60)
    const gap = Math.min((2 * Math.PI) / ring.length, 168 / radius)
    const a = ring.map((n) => w.get(n.id)!)
    for (let pass = 0; pass < 6; pass++) {
      for (let i = 1; i < a.length; i++) {
        if (a[i] - a[i - 1] < gap) {
          const mid = (a[i] + a[i - 1]) / 2
          a[i - 1] = mid - gap / 2
          a[i] = mid + gap / 2
        }
      }
      if (a.length > 1 && a[0] + 2 * Math.PI - a[a.length - 1] < gap) {
        const over = gap - (a[0] + 2 * Math.PI - a[a.length - 1])
        a[0] += over / 2
        a[a.length - 1] -= over / 2
      }
    }
    ring.forEach((n, i) => {
      angle.set(n.id, a[i])
      pos.set(n.id, { x: Math.cos(a[i]) * radius, y: Math.sin(a[i]) * radius })
    })
  }
  return pos
}
