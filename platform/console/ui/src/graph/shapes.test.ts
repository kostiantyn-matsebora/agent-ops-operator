import { describe, expect, it } from 'vitest'
import { CLEARANCE, type Clearance, ICON_HALF, NODE_STYLES, ROLE_GLYPH, shapePath } from './shapes'

// A glyph never touches its outline. Both are one path each, so the check is
// geometric: flatten the path to points, and every glyph point lies inside one
// of the shape's clearance rectangles while no outline point lies inside any.
// The cloud shipped once with the globe crossing its bottom line, and no
// screenshot fixture happened to show an external system.

type Pt = [number, number]

function flatten(d: string): Pt[] {
  const out: Pt[] = []
  const tokens = d.match(/[MmLlHhVvAaCcZz]|-?\d*\.?\d+(?:e-?\d+)?/g) ?? []
  let i = 0
  let cmd = ''
  let x = 0
  let y = 0
  let sx = 0
  let sy = 0
  const num = () => Number(tokens[i++])
  const push = (px: number, py: number) => {
    out.push([px, py])
    x = px
    y = py
  }
  while (i < tokens.length) {
    const t = tokens[i]
    if (/^[A-Za-z]$/.test(t)) {
      cmd = t
      i++
      if (cmd === 'z' || cmd === 'Z') {
        push(sx, sy)
        continue
      }
    }
    const rel = cmd === cmd.toLowerCase()
    switch (cmd.toUpperCase()) {
      case 'M': {
        const nx = num()
        const ny = num()
        push(rel ? x + nx : nx, rel ? y + ny : ny)
        sx = x
        sy = y
        cmd = rel ? 'l' : 'L'
        break
      }
      case 'L': {
        const nx = num()
        const ny = num()
        push(rel ? x + nx : nx, rel ? y + ny : ny)
        break
      }
      case 'H': {
        const nx = num()
        push(rel ? x + nx : nx, y)
        break
      }
      case 'V': {
        const ny = num()
        push(x, rel ? y + ny : ny)
        break
      }
      case 'C': {
        const [x1, y1, x2, y2, ex, ey] = [num(), num(), num(), num(), num(), num()].map((v, k) => (rel ? v + (k % 2 ? y : x) : v))
        const [x0, y0] = [x, y]
        for (let s = 1; s <= 16; s++) {
          const u = s / 16
          const a = (1 - u) ** 3
          const b = 3 * (1 - u) ** 2 * u
          const c = 3 * (1 - u) * u ** 2
          const e = u ** 3
          out.push([a * x0 + b * x1 + c * x2 + e * ex, a * y0 + b * y1 + c * y2 + e * ey])
        }
        x = ex
        y = ey
        break
      }
      case 'A': {
        // SVG implementation notes F.6.5: endpoint to centre parameterisation.
        let rx = Math.abs(num())
        let ry = Math.abs(num())
        const phi = (num() * Math.PI) / 180
        const large = num() !== 0
        const sweep = num() !== 0
        const nx = num()
        const ny = num()
        const ex = rel ? x + nx : nx
        const ey = rel ? y + ny : ny
        const [x0, y0] = [x, y]
        const cos = Math.cos(phi)
        const sin = Math.sin(phi)
        const dx = (x0 - ex) / 2
        const dy = (y0 - ey) / 2
        const x1p = cos * dx + sin * dy
        const y1p = -sin * dx + cos * dy
        const lambda = (x1p * x1p) / (rx * rx) + (y1p * y1p) / (ry * ry)
        if (lambda > 1) {
          rx *= Math.sqrt(lambda)
          ry *= Math.sqrt(lambda)
        }
        const num2 = Math.max(0, rx * rx * ry * ry - rx * rx * y1p * y1p - ry * ry * x1p * x1p)
        const den = rx * rx * y1p * y1p + ry * ry * x1p * x1p
        const coef = (large === sweep ? -1 : 1) * Math.sqrt(den ? num2 / den : 0)
        const cxp = (coef * rx * y1p) / ry
        const cyp = (-coef * ry * x1p) / rx
        const cx = cos * cxp - sin * cyp + (x0 + ex) / 2
        const cy = sin * cxp + cos * cyp + (y0 + ey) / 2
        const ang = (ux: number, uy: number, vx: number, vy: number) => {
          const dot = ux * vx + uy * vy
          const len = Math.hypot(ux, uy) * Math.hypot(vx, vy)
          const a = Math.acos(Math.max(-1, Math.min(1, dot / len)))
          return ux * vy - uy * vx < 0 ? -a : a
        }
        const theta1 = ang(1, 0, (x1p - cxp) / rx, (y1p - cyp) / ry)
        let dtheta = ang((x1p - cxp) / rx, (y1p - cyp) / ry, (-x1p - cxp) / rx, (-y1p - cyp) / ry)
        if (!sweep && dtheta > 0) dtheta -= 2 * Math.PI
        if (sweep && dtheta < 0) dtheta += 2 * Math.PI
        for (let s = 1; s <= 24; s++) {
          const th = theta1 + (dtheta * s) / 24
          const px = rx * Math.cos(th)
          const py = ry * Math.sin(th)
          out.push([cos * px - sin * py + cx, sin * px + cos * py + cy])
        }
        x = ex
        y = ey
        break
      }
      default:
        throw new Error(`unhandled path command ${cmd} in ${d}`)
    }
  }
  return out
}

const EPS = 1e-6
const inside = ([x, y]: Pt, [x0, y0, x1, y1]: Clearance, strict: boolean) =>
  strict ? x > x0 + EPS && x < x1 - EPS && y > y0 + EPS && y < y1 - EPS : x >= x0 - EPS && x <= x1 + EPS && y >= y0 - EPS && y <= y1 + EPS

const glyphs = [
  ...Object.entries(NODE_STYLES).map(([cls, s]) => ({ what: cls, shape: s.shape, glyph: s.glyph })),
  // A pod wears its role's glyph on the box shape.
  ...Object.entries(ROLE_GLYPH).map(([role, glyph]) => ({ what: `pod as ${role}`, shape: 'box' as const, glyph })),
]

describe('marks', () => {
  it.each(Object.keys(CLEARANCE))('keeps the %s outline out of its clearance', (shape) => {
    const rects = CLEARANCE[shape as keyof typeof CLEARANCE]
    const offenders = flatten(shapePath(shape as keyof typeof CLEARANCE)).filter((p) => rects.some((r) => inside(p, r, true)))
    expect(offenders).toEqual([])
  })

  it.each(glyphs.map((g) => [g.what, g] as const))('draws the %s glyph inside its clearance', (_what, g) => {
    const rects = CLEARANCE[g.shape]
    const offenders = flatten(g.glyph).filter((p) => !rects.some((r) => inside(p, r, false)))
    expect(offenders).toEqual([])
  })

  it('leaves a pipeline mark room for the icon it declares', () => {
    const [x0, y0, x1, y1] = CLEARANCE[NODE_STYLES.pipelines.shape][0]
    expect([x0 <= -ICON_HALF, y0 <= -ICON_HALF, x1 >= ICON_HALF, y1 >= ICON_HALF]).toEqual([true, true, true, true])
  })

  it('flattens an arc through its bulge, not only its endpoints', () => {
    const pts = flatten('M-10 0 a10 10 0 0 1 20 0')
    expect(Math.min(...pts.map((p) => p[1]))).toBeCloseTo(-10, 5)
  })
})
