import type { ActivityEvent, EdgeTraffic } from '../api/types'
import type { Crossing } from './hops'
import type { Curve, Pt } from './layout/curve'

// TWO KINDS OF MARK, as in Kiali. The steady STREAM is density: each edge
// emits continuously at an interval set by its rate in the window, errors in
// the edge's own proportion. A recorded hop is a PULSE on top of it: larger,
// in the direction the hop actually travelled, and clickable.
//
// Both are computed from the clock rather than kept as spawned objects, so the
// stream is a rendering of the recorded rate and never an event of its own.

export const STREAM_TRAVEL_MS = 1600

/** How often an edge emits a stream mark at this rate. */
export function streamInterval(ratePerMin: number): number {
  return Math.min(3500, Math.max(220, 5000 / ratePerMin))
}

export type MarkClass = 'ok' | 'error' | 'unconfirmed'

export interface Mark extends Pt {
  cls: MarkClass
}

/**
 * The stream marks on an edge at `now`: one emitted every interval, each
 * crossing in STREAM_TRAVEL_MS. Emission e is an error mark when the running
 * error count steps at e, which puts exactly the edge's proportion of errors in
 * the stream over time.
 */
export function streamMarks(t: EdgeTraffic | undefined, now: number, curve: Curve): Mark[] {
  if (!t || t.events === 0 || !(t.ratePerMin > 0)) return []
  const interval = streamInterval(t.ratePerMin)
  const p = t.errors / t.events
  const latest = Math.floor(now / interval)
  const out: Mark[] = []
  for (let j = 0; ; j++) {
    const frac = ((now % interval) + j * interval) / STREAM_TRAVEL_MS
    if (frac > 1) break
    const e = latest - j
    const error = Math.floor((e + 1) * p) > Math.floor(e * p)
    out.push({ ...curve.at(frac), cls: error ? 'error' : t.unconfirmed ? 'unconfirmed' : 'ok' })
  }
  return out
}

export interface Pulse {
  key: string
  ev: ActivityEvent
  x: Crossing
  t0: number
  /** Per leg. A path of several legs crosses them one after another. */
  dur: number
}

export function pulseDuration(ev: ActivityEvent): number {
  return ev.kind === 'run.completed' || ev.kind === 'run.dispatched' ? 1100 : 800
}

/** How long a pulse lives, all its legs included. */
export function pulseLife(p: Pulse): number {
  return p.x.pop ? 1200 : p.dur * Math.max(1, p.x.edges.length)
}

export interface PulseMark extends Pt {
  r: number
  cls: 'ok' | 'error'
  pop: boolean
  opacity: number
}

/** Where a pulse is at `now`, or null before it starts and after it ends. */
export function pulseAt(
  p: Pulse, now: number, curves: Map<string, Curve>, pos: Map<string, Pt>,
): PulseMark | null {
  const age = now - p.t0
  const cls = p.ev.status === 'error' ? 'error' : 'ok'
  if (age < 0 || age > pulseLife(p)) return null
  if (p.x.pop) {
    const at = pos.get(p.x.pop)
    if (!at) return null
    const t = age / 1200
    return { ...at, r: 6 + t * 16, cls, pop: true, opacity: 1 - t }
  }
  const leg = Math.min(p.x.edges.length - 1, Math.floor(age / p.dur))
  const { edge, dir } = p.x.edges[leg]
  const curve = curves.get(edge.id)
  if (!curve) return null
  const t = Math.min(1, (age - leg * p.dur) / p.dur)
  return { ...curve.at(dir === 1 ? t : 1 - t), r: 7, cls, pop: false, opacity: 1 }
}

/** A mark's outline: a dot, a diamond for an error, a ring for a pop. */
export function markPath(x: number, y: number, r: number, error: boolean): string {
  if (error) return `M${x} ${y - r - 1} L${x + r + 1} ${y} L${x} ${y + r + 1} L${x - r - 1} ${y} Z`
  return `M${x - r} ${y} a${r} ${r} 0 1 0 ${2 * r} 0 a${r} ${r} 0 1 0 ${-2 * r} 0`
}
