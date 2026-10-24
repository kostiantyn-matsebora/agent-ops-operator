import { forceCollide, forceLink, forceSimulation, forceX, forceY, type SimulationNodeDatum } from 'd3-force'
import type { Pt } from './curve'
import {
  allMembers, memberBoxes, settleRadius, type Canvas, type Group, type LayoutEdge, type LayoutNode,
} from './geometry'

// COMPACTION, IN TWO STAGES, after concentric or cola and never after dagre.
//
// First every node is pulled toward the centre under collision, held to its
// group, until the density inside each group is even. Then each group is ONE
// RIGID RECTANGLE, ungrouped nodes are small rectangles, and the rectangles
// are packed under a gravity shaped to the canvas. Circles pack with holes
// between them; rectangles do not, and the holes were the empty half of the
// picture. Last, positions are separated directly until nothing intersects.

interface Sim extends SimulationNodeDatum {
  id: string
  name: string
}

interface Body {
  members: Sim[]
  x: number
  y: number
  hw: number
  hh: number
  vx: number
  vy: number
}

const MARGIN = 22
/** Half a mark with its label, for the no-overlap guarantee. */
export const MARK_HALF = { w: 44, h: 40 }

export function compact(
  nodes: LayoutNode[], edges: LayoutEdge[], groups: Group[], canvas: Canvas, centre?: Pt,
): Map<string, Pt> {
  const ns: Sim[] = nodes.map((n) => ({ id: n.id, name: n.name, x: n.x, y: n.y }))
  const pos = new Map<string, Pt>()
  if (ns.length < 2) {
    for (const n of ns) pos.set(n.id, { x: n.x ?? 0, y: n.y ?? 0 })
    return pos
  }
  // A hub's layout gravitates to its hub, so packing keeps the hub central.
  const cx = centre?.x ?? mean(ns.map((n) => n.x ?? 0))
  const cy = centre?.y ?? mean(ns.map((n) => n.y ?? 0))
  const owner = new Map<string, string>()
  for (const g of groups) for (const m of g.members) owner.set(m, g.id)
  const byId = new Map(ns.map((n) => [n.id, n]))
  const links = edges
    .filter((e) => byId.has(e.from) && byId.has(e.to) && e.from !== e.to)
    .map((e) => ({ source: byId.get(e.from)!, target: byId.get(e.to)! }))

  // stage one: members settle inside their groups
  const cluster = (alpha: number) => {
    const c = new Map<string, { x: number; y: number; k: number }>()
    for (const n of ns) {
      const o = owner.get(n.id)
      if (!o) continue
      const a = c.get(o) ?? { x: 0, y: 0, k: 0 }
      a.x += n.x ?? 0
      a.y += n.y ?? 0
      a.k++
      c.set(o, a)
    }
    for (const n of ns) {
      const o = owner.get(n.id)
      const a = o ? c.get(o) : undefined
      if (!a || a.k < 2) continue
      n.vx = (n.vx ?? 0) + (a.x / a.k - (n.x ?? 0)) * alpha * 0.6
      n.vy = (n.vy ?? 0) + (a.y / a.k - (n.y ?? 0)) * alpha * 0.6
    }
  }
  const sim = forceSimulation(ns)
    .force('link', forceLink<Sim, { source: Sim; target: Sim }>(links).distance(140).strength(0.2))
    .force('collide', forceCollide<Sim>((n) => settleRadius(n.name)).strength(1).iterations(3))
    .force('x', forceX<Sim>(cx).strength(0.08))
    .force('y', forceY<Sim>(cy).strength(0.08))
    .force('cluster', cluster)
    .alpha(1)
    .alphaDecay(0.025)
    .stop()
  for (let i = 0; i < 220; i++) sim.tick()

  // stage two: rigid rectangles
  const bodies: Body[] = []
  for (const g of groups.filter((x) => !x.parent)) {
    const m = allMembers(g, groups).map((id) => byId.get(id)).filter((n): n is Sim => Boolean(n))
    if (m.length === 0) continue
    const x0 = Math.min(...m.map((n) => n.x!)) - 54
    const x1 = Math.max(...m.map((n) => n.x!)) + 54
    const y0 = Math.min(...m.map((n) => n.y!)) - 70
    const y1 = Math.max(...m.map((n) => n.y!)) + 64
    bodies.push({ members: m, x: (x0 + x1) / 2, y: (y0 + y1) / 2, hw: (x1 - x0) / 2, hh: (y1 - y0) / 2, vx: 0, vy: 0 })
  }
  const grouped = new Set(bodies.flatMap((b) => b.members.map((m) => m.id)))
  for (const n of ns) {
    if (grouped.has(n.id)) continue
    const r = settleRadius(n.name)
    bodies.push({ members: [n], x: n.x!, y: n.y!, hw: r * 0.9, hh: r * 0.78, vx: 0, vy: 0 })
  }
  const bodyOf = new Map<string, number>()
  bodies.forEach((b, i) => b.members.forEach((m) => bodyOf.set(m.id, i)))
  const blinks = new Map<string, number>()
  for (const l of links) {
    const a = bodyOf.get(l.source.id)
    const b = bodyOf.get(l.target.id)
    if (a === undefined || b === undefined || a === b) continue
    const k = a < b ? `${a}|${b}` : `${b}|${a}`
    blinks.set(k, (blinks.get(k) ?? 0) + 1)
  }
  const gx = 0.045 * Math.min(1, canvas.h / canvas.w)
  const gy = 0.045 * Math.min(1, canvas.w / canvas.h)
  for (let it = 0; it < 420; it++) {
    const damp = it < 300 ? 0.82 : 0.6
    for (const b of bodies) {
      b.vx += (cx - b.x) * gx
      b.vy += (cy - b.y) * gy
    }
    for (const [k, w] of blinks) {
      const [i, j] = k.split('|').map(Number)
      const a = bodies[i]
      const b = bodies[j]
      const f = 0.004 * Math.min(w, 3)
      a.vx += (b.x - a.x) * f
      a.vy += (b.y - a.y) * f
      b.vx += (a.x - b.x) * f
      b.vy += (a.y - b.y) * f
    }
    for (let i = 0; i < bodies.length; i++) {
      for (let j = i + 1; j < bodies.length; j++) push(bodies[i], bodies[j], 0.5, false)
    }
    for (const b of bodies) {
      b.x += b.vx
      b.y += b.vy
      b.vx *= damp
      b.vy *= damp
    }
  }
  // final: no gravity, positions moved directly until nothing intersects
  for (let pass = 0; pass < 300; pass++) {
    let moved = false
    for (let i = 0; i < bodies.length; i++) {
      for (let j = i + 1; j < bodies.length; j++) moved = push(bodies[i], bodies[j], 1, true) || moved
    }
    if (!moved) break
  }
  for (const b of bodies) {
    const xs = b.members.map((m) => m.x!)
    const ys = b.members.map((m) => m.y!)
    const ox = b.x - (Math.min(...xs) + Math.max(...xs)) / 2
    const oy = b.y - (Math.min(...ys) + Math.max(...ys)) / 2
    for (const m of b.members) pos.set(m.id, { x: m.x! + ox, y: m.y! + oy })
  }
  return pos
}

/** Two overlapping bodies pushed apart on the cheaper axis, the smaller one moving more. */
function push(a: Body, b: Body, k: number, direct: boolean): boolean {
  const ox = a.hw + b.hw + MARGIN - Math.abs(a.x - b.x)
  const oy = a.hh + b.hh + MARGIN - Math.abs(a.y - b.y)
  if (ox <= 0 || oy <= 0) return false
  const wa = (b.hw * b.hh) / (a.hw * a.hh + b.hw * b.hh)
  const wb = 1 - wa
  if (ox < oy) {
    const d = Math.sign(a.x - b.x) || 1
    const amt = direct ? ox + 1 : ox * k
    if (direct) {
      a.x += d * amt * wa
      b.x -= d * amt * wb
    } else {
      a.vx += d * amt * wa
      b.vx -= d * amt * wb
    }
  } else {
    const d = Math.sign(a.y - b.y) || 1
    const amt = direct ? oy + 1 : oy * k
    if (direct) {
      a.y += d * amt * wa
      b.y -= d * amt * wb
    } else {
      a.vy += d * amt * wa
      b.vy -= d * amt * wb
    }
  }
  return true
}

/**
 * evict moves a mark that belongs to no box out of every box it landed in, and
 * pulls apart any two marks that still touch. An element that belongs to no
 * box never sits inside one, whatever the layout did.
 */
export function evict(pos: Map<string, Pt>, groups: Group[]): void {
  for (let pass = 0; pass < 40; pass++) {
    let moved = false
    const boxes = memberBoxes(groups, pos)
    for (const b of boxes) {
      const inside = new Set(allMembers(b, groups))
      for (const [id, p] of pos) {
        if (inside.has(id)) continue
        const ox = Math.min(p.x + MARK_HALF.w - b.x0, b.x0 + b.w - (p.x - MARK_HALF.w))
        const oy = Math.min(p.y + MARK_HALF.h - b.y0, b.y0 + b.h - (p.y - MARK_HALF.h))
        if (ox <= 0 || oy <= 0) continue
        moved = true
        const cx = b.x0 + b.w / 2
        const cy = b.y0 + b.h / 2
        if (ox < oy) pos.set(id, { x: p.x + (p.x < cx ? -ox - 2 : ox + 2), y: p.y })
        else pos.set(id, { x: p.x, y: p.y + (p.y < cy ? -oy - 2 : oy + 2) })
      }
    }
    const ps = [...pos.entries()]
    for (let i = 0; i < ps.length; i++) {
      for (let j = i + 1; j < ps.length; j++) {
        const [ia, a] = ps[i]
        const [ib, b] = ps[j]
        const ox = 2 * MARK_HALF.w - Math.abs(a.x - b.x)
        const oy = 2 * MARK_HALF.h - Math.abs(a.y - b.y)
        if (ox <= 0 || oy <= 0) continue
        moved = true
        if (ox < oy) {
          const d = Math.sign(a.x - b.x) || 1
          ps[i][1] = { x: a.x + (d * (ox + 1)) / 2, y: a.y }
          ps[j][1] = { x: b.x - (d * (ox + 1)) / 2, y: b.y }
        } else {
          const d = Math.sign(a.y - b.y) || 1
          ps[i][1] = { x: a.x, y: a.y + (d * (oy + 1)) / 2 }
          ps[j][1] = { x: b.x, y: b.y - (d * (oy + 1)) / 2 }
        }
        pos.set(ia, ps[i][1])
        pos.set(ib, ps[j][1])
      }
    }
    if (!moved) return
  }
}

function mean(xs: number[]): number {
  return xs.reduce((a, b) => a + b, 0) / (xs.length || 1)
}
