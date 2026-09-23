import { MARK_RADIUS } from '../shapes'

// An edge's drawn path and the points along it, from one description. A pulse
// travels the same curve the edge is drawn with, sampled by arc length here
// rather than measured from the DOM — which also keeps it testable.

export interface Pt {
  x: number
  y: number
}

type Seg = { kind: 'L'; to: Pt } | { kind: 'C'; c1: Pt; c2: Pt; to: Pt }

export interface Curve {
  d: string
  /** The point a fraction t of the way along, by length. */
  at: (t: number) => Pt
  mid: Pt
}

function build(start: Pt, segs: Seg[]): Curve {
  const f = (v: number) => Math.round(v * 10) / 10
  const d = `M${f(start.x)} ${f(start.y)} ${segs
    .map((s) => (s.kind === 'L' ? `L${f(s.to.x)} ${f(s.to.y)}` : `C${f(s.c1.x)} ${f(s.c1.y)} ${f(s.c2.x)} ${f(s.c2.y)} ${f(s.to.x)} ${f(s.to.y)}`))
    .join(' ')}`
  const samples: Pt[] = [start]
  let from = start
  for (const s of segs) {
    if (s.kind === 'L') samples.push(s.to)
    else {
      for (let i = 1; i <= 12; i++) {
        const t = i / 12
        const u = 1 - t
        samples.push({
          x: u * u * u * from.x + 3 * u * u * t * s.c1.x + 3 * u * t * t * s.c2.x + t * t * t * s.to.x,
          y: u * u * u * from.y + 3 * u * u * t * s.c1.y + 3 * u * t * t * s.c2.y + t * t * t * s.to.y,
        })
      }
    }
    from = s.to
  }
  const cum = [0]
  for (let i = 1; i < samples.length; i++) {
    cum.push(cum[i - 1] + Math.hypot(samples[i].x - samples[i - 1].x, samples[i].y - samples[i - 1].y))
  }
  const total = cum[cum.length - 1] || 1
  const at = (t: number): Pt => {
    const want = Math.min(1, Math.max(0, t)) * total
    let i = 1
    while (i < cum.length - 1 && cum[i] < want) i++
    const span = cum[i] - cum[i - 1] || 1
    const k = (want - cum[i - 1]) / span
    const a = samples[i - 1]
    const b = samples[i] ?? a
    return { x: a.x + (b.x - a.x) * k, y: a.y + (b.y - a.y) * k }
  }
  return { d, at, mid: at(0.5) }
}

/** Trim a straight run to the marks at either end, leaving room for the arrow. */
export function trimmed(a: Pt, b: Pt): [Pt, Pt] {
  const dx = b.x - a.x
  const dy = b.y - a.y
  const L = Math.hypot(dx, dy) || 1
  const ux = dx / L
  const uy = dy / L
  return [
    { x: a.x + ux * MARK_RADIUS, y: a.y + uy * MARK_RADIUS },
    { x: b.x - ux * (MARK_RADIUS + 4), y: b.y - uy * (MARK_RADIUS + 4) },
  ]
}

/**
 * An unrouted edge: a gentle arc bowed to the right of travel, so two opposite
 * edges between one pair never lie on one line.
 */
export function arcCurve(a: Pt, b: Pt): Curve {
  const [p, q] = trimmed(a, b)
  const mx = (p.x + q.x) / 2
  const my = (p.y + q.y) / 2
  const dx = q.x - p.x
  const dy = q.y - p.y
  const L = Math.hypot(dx, dy) || 1
  const k = Math.min(30, L * 0.12)
  const c = { x: mx - (dy / L) * k, y: my + (dx / L) * k }
  // a quadratic is a cubic with both controls two thirds toward its one
  const c1 = { x: p.x + (2 / 3) * (c.x - p.x), y: p.y + (2 / 3) * (c.y - p.y) }
  const c2 = { x: q.x + (2 / 3) * (c.x - q.x), y: q.y + (2 / 3) * (c.y - q.y) }
  return build(p, [{ kind: 'C', c1, c2, to: q }])
}

/**
 * A routed edge through dagre's points, as a uniform B-spline — d3's
 * curveBasis, ported. Dagre ends a path at the node's LAYOUT box, wider than
 * the drawn mark to hold the label, so the ends are pulled in to the mark.
 */
export function basisCurve(a: Pt, b: Pt, points: Pt[]): Curve {
  if (points.length < 2) return arcCurve(a, b)
  const towards = (from: Pt, to: Pt, d: number): Pt => {
    const dx = to.x - from.x
    const dy = to.y - from.y
    const L = Math.hypot(dx, dy) || 1
    return { x: from.x + (dx / L) * d, y: from.y + (dy / L) * d }
  }
  const pts = points.slice()
  pts[0] = towards(a, pts.length > 2 ? pts[1] : b, MARK_RADIUS - 2)
  pts[pts.length - 1] = towards(b, pts.length > 2 ? pts[pts.length - 2] : a, MARK_RADIUS + 4)
  const segs: Seg[] = []
  let [x0, y0, x1, y1] = [NaN, NaN, NaN, NaN]
  let state = 0
  const cubic = (x: number, y: number) =>
    segs.push({
      kind: 'C',
      c1: { x: (2 * x0 + x1) / 3, y: (2 * y0 + y1) / 3 },
      c2: { x: (x0 + 2 * x1) / 3, y: (y0 + 2 * y1) / 3 },
      to: { x: (x0 + 4 * x1 + x) / 6, y: (y0 + 4 * y1 + y) / 6 },
    })
  for (const p of pts) {
    if (state === 0) state = 1
    else if (state === 1) state = 2
    else {
      if (state === 2) {
        state = 3
        segs.push({ kind: 'L', to: { x: (5 * x0 + x1) / 6, y: (5 * y0 + y1) / 6 } })
      }
      cubic(p.x, p.y)
    }
    x0 = x1
    x1 = p.x
    y0 = y1
    y1 = p.y
  }
  if (state === 3) cubic(x1, y1)
  segs.push({ kind: 'L', to: { x: x1, y: y1 } })
  return build(pts[0], segs)
}
