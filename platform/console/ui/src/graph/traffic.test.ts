import { describe, expect, it } from 'vitest'
import { hop } from '../test-fixtures/topology'
import { arcCurve } from './layout/curve'
import { pulseAt, pulseLife, streamInterval, streamMarks, type Pulse } from './traffic'

const curve = arcCurve({ x: 0, y: 0 }, { x: 400, y: 0 })

describe('the stream', () => {
  it('follows the rate: a busier edge carries more marks', () => {
    const quiet = streamMarks({ events: 4, errors: 0, ratePerMin: 4 }, 10_000, curve)
    const busy = streamMarks({ events: 60, errors: 0, ratePerMin: 60 }, 10_000, curve)
    expect(quiet.length).toBeGreaterThan(0)
    expect(busy.length).toBeGreaterThan(quiet.length)
    expect(streamInterval(0.1)).toBe(3500)
    expect(streamInterval(1000)).toBe(220)
  })

  it('renders an idle edge with no marks', () => {
    expect(streamMarks(undefined, 0, curve)).toEqual([])
    expect(streamMarks({ events: 0, errors: 0, ratePerMin: 0 }, 0, curve)).toEqual([])
  })

  it('puts the edge\'s own proportion of errors in the stream', () => {
    const t = { events: 50, errors: 10, ratePerMin: 60 }
    const interval = streamInterval(60)
    let errors = 0
    let total = 0
    for (let now = 0; now < interval * 500; now += interval) {
      const m = streamMarks(t, now, curve)[0]
      total++
      if (m.cls === 'error') errors++
    }
    expect(errors / total).toBeCloseTo(0.2, 2)
  })

  it('marks unconfirmed delivery distinctly', () => {
    const m = streamMarks({ events: 3, errors: 0, ratePerMin: 3, unconfirmed: true }, 0, curve)
    expect(m.every((x) => x.cls === 'unconfirmed')).toBe(true)
  })
})

describe('a pulse', () => {
  const edge = { id: 'a->b', from: 'a', to: 'b', kind: 'feeds' }
  const curves = new Map([['a->b', curve]])
  const pos = new Map([['a', { x: 0, y: 0 }], ['b', { x: 400, y: 0 }]])

  it('travels the direction the hop did', () => {
    const forward: Pulse = { key: '1', ev: hop('x', null, null), x: { edges: [{ edge, dir: 1 }] }, t0: 0, dur: 1000 }
    const back: Pulse = { ...forward, x: { edges: [{ edge, dir: -1 }] } }
    expect(pulseAt(forward, 100, curves, pos)!.x).toBeLessThan(pulseAt(forward, 900, curves, pos)!.x)
    expect(pulseAt(back, 100, curves, pos)!.x).toBeGreaterThan(pulseAt(back, 900, curves, pos)!.x)
    expect(pulseAt(forward, 1001, curves, pos)).toBeNull()
  })

  it('pulses on its node when the hop has no destination', () => {
    const p: Pulse = { key: '2', ev: hop('signal.dropped', null, null, { status: 'error' }), x: { edges: [], pop: 'a' }, t0: 0, dur: 800 }
    const m = pulseAt(p, 600, curves, pos)!
    expect(m.pop).toBe(true)
    expect(m.cls).toBe('error')
    expect(pulseLife(p)).toBe(1200)
  })
})
