import { describe, expect, it } from 'vitest'
import { EDGE_CLEARANCE, arcCurve } from './curve'

// An unrouted edge is drawn as one arc, and that arc goes around whatever
// mark sits between its ends rather than through it.

const clearance = (avoid: { x: number; y: number }, a = { x: 0, y: 0 }, b = { x: 400, y: 0 }) => {
  const c = arcCurve(a, b, [avoid])
  let min = Infinity
  for (let i = 0; i <= 100; i++) {
    const p = c.at(i / 100)
    min = Math.min(min, Math.hypot(p.x - avoid.x, p.y - avoid.y))
  }
  return min
}

describe('an unrouted edge', () => {
  it('bows gently to the right of travel with nothing in the way', () => {
    const c = arcCurve({ x: 0, y: 0 }, { x: 400, y: 0 })
    expect(c.mid.y).toBeGreaterThan(5)
    expect(c.mid.y).toBeLessThan(20)
  })

  it('bows clear of a mark sitting on its line', () => {
    expect(clearance({ x: 200, y: 0 })).toBeGreaterThanOrEqual(EDGE_CLEARANCE - 1)
  })

  it('bows to the side away from a mark just off its line', () => {
    const c = arcCurve({ x: 0, y: 0 }, { x: 400, y: 0 }, [{ x: 200, y: 20 }])
    expect(c.mid.y).toBeLessThan(0)
    expect(clearance({ x: 200, y: 20 })).toBeGreaterThanOrEqual(EDGE_CLEARANCE - 1)
  })

  it('ignores marks beyond either end and far from the line', () => {
    const plain = arcCurve({ x: 0, y: 0 }, { x: 400, y: 0 })
    const c = arcCurve({ x: 0, y: 0 }, { x: 400, y: 0 }, [{ x: -80, y: 0 }, { x: 480, y: 0 }, { x: 200, y: 300 }])
    expect(c.d).toBe(plain.d)
  })
})
