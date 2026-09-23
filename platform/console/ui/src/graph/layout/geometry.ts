import type { Pt } from './curve'

// The shared vocabulary of the three layouts: what they lay out, the canvas they
// fill, the groups they keep together and the boxes drawn around them. The
// numbers are the mockup's, where a mark was sized against its spacing.

export interface LayoutNode {
  id: string
  name: string
  x: number
  y: number
}

export interface LayoutEdge {
  id: string
  from: string
  to: string
  kind: string
}

export interface Canvas {
  w: number
  h: number
}

export const DEFAULT_CANVAS: Canvas = { w: 1400, h: 700 }

/** An owner and what it holds. Nested boxes name their parent. */
export interface Group {
  id: string
  title: string
  hint: string
  members: string[]
  parent?: string
}

export interface Box extends Group {
  x0: number
  y0: number
  w: number
  h: number
}

export interface Bounds {
  x0: number
  y0: number
  x1: number
  y1: number
}

/** A node's footprint when laying out: wide enough for its label under the mark. */
export function layoutSize(name: string): { width: number; height: number } {
  return { width: Math.max(104, Math.min(name.length, 20) * 8.2 + 30), height: 118 }
}

/** The collision radius compaction settles nodes with. */
export function settleRadius(name: string): number {
  return Math.max(66, Math.min(name.length, 20) * 4.4 + 30)
}

/** Every member of a group, its nested groups' members included. */
export function allMembers(g: Group, groups: Group[]): string[] {
  let m = [...g.members]
  for (const c of groups) if (c.parent === g.id) m = m.concat(allMembers(c, groups))
  return m
}

/** Boxes drawn from where the members ENDED UP: no rule positions a box. */
export function memberBoxes(groups: Group[], pos: Map<string, Pt>): Box[] {
  const out: Box[] = []
  for (const g of groups) {
    const m = allMembers(g, groups).map((id) => pos.get(id)).filter((p): p is Pt => Boolean(p))
    if (m.length === 0) continue
    const x0 = Math.min(...m.map((p) => p.x)) - 50
    const x1 = Math.max(...m.map((p) => p.x)) + 50
    const y0 = Math.min(...m.map((p) => p.y)) - 66
    const y1 = Math.max(...m.map((p) => p.y)) + 60
    out.push({ ...g, x0, y0, w: x1 - x0, h: y1 - y0 })
  }
  // a parent grows to hold its children's boxes with room for its own title
  for (const b of [...out].sort((a, c) => depthOf(c, groups) - depthOf(a, groups))) {
    for (const child of out.filter((c) => c.parent === b.id)) {
      const x0 = Math.min(b.x0, child.x0 - 12)
      const y0 = Math.min(b.y0, child.y0 - 34)
      const x1 = Math.max(b.x0 + b.w, child.x0 + child.w + 12)
      const y1 = Math.max(b.y0 + b.h, child.y0 + child.h + 12)
      Object.assign(b, { x0, y0, w: x1 - x0, h: y1 - y0 })
    }
  }
  return out
}

function depthOf(g: Group, groups: Group[]): number {
  let d = 0
  let p = g.parent
  while (p) {
    d++
    p = groups.find((x) => x.id === p)?.parent
  }
  return d
}

/**
 * fillCanvas stretches the slack axis, bounded, so a picture squarer than the
 * canvas is not fitted by height with the sides empty. The topology is
 * unchanged, only the aspect.
 */
export function fillCanvas(pos: Map<string, Pt>, canvas: Canvas): void {
  const ps = [...pos.values()]
  if (ps.length < 2) return
  const x0 = Math.min(...ps.map((p) => p.x))
  const x1 = Math.max(...ps.map((p) => p.x))
  const y0 = Math.min(...ps.map((p) => p.y))
  const y1 = Math.max(...ps.map((p) => p.y))
  const pw = x1 - x0 + 180
  const ph = y1 - y0 + 140
  const want = canvas.w / canvas.h
  const have = pw / ph
  let sx = 1
  let sy = 1
  if (have < want) sx = Math.min(1.45, want / have)
  else sy = Math.min(1.45, have / want)
  const cx = (x0 + x1) / 2
  const cy = (y0 + y1) / 2
  for (const [id, p] of pos) pos.set(id, { x: cx + (p.x - cx) * sx, y: cy + (p.y - cy) * sy })
}

/** The picture: every mark with its label, and every box with its title. */
export function pictureBounds(pos: Map<string, Pt>, boxes: Box[]): Bounds {
  const ps = [...pos.values()]
  if (ps.length === 0) return { x0: 0, y0: 0, x1: 1, y1: 1 }
  return {
    x0: Math.min(...ps.map((p) => p.x - 96), ...boxes.map((b) => b.x0 - 6)),
    x1: Math.max(...ps.map((p) => p.x + 96), ...boxes.map((b) => b.x0 + b.w + 6)),
    y0: Math.min(...ps.map((p) => p.y - 54), ...boxes.map((b) => b.y0 - 6)),
    y1: Math.max(...ps.map((p) => p.y + 80), ...boxes.map((b) => b.y0 + b.h + 26)),
  }
}
