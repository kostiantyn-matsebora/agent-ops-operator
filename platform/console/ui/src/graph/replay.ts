import type { ActivityEvent } from '../api/types'
import { crossing, tsOf, type HopIndex } from './hops'
import type { ViewGraph } from './types'

// REPLAY, TWO WAYS. A past window frame by frame, as a mesh graph replays
// traffic; and one conversation hop by hop with its real gaps, which a mesh
// cannot do because a request is gone once its trace is sampled.

export const FRAME_MS = 10_000
/** The minute a frame's rates are computed over. */
export const FRAME_WINDOW_MS = 60_000
export const INTERVALS = [60, 300, 600, 1800]
export const SPEEDS = [
  { label: 'slow', ms: 5000 },
  { label: 'medium', ms: 3000 },
  { label: 'fast', ms: 1000 },
]

export interface ReplayWindow {
  start: number
  end: number
  frames: number
  /** How much of the window's start the buffer does not hold. Zero when it holds all of it. */
  notHeldMs: number
  /** The first frame whose ten seconds the buffer holds. */
  firstHeldFrame: number
}

/**
 * The window replayed, bounded by the buffer: a window reaching past what the
 * buffer holds says so rather than showing those minutes as silent.
 */
export function replayWindow(now: number, intervalSeconds: number, bufferStart?: number): ReplayWindow {
  const start = now - intervalSeconds * 1000
  const frames = Math.round((intervalSeconds * 1000) / FRAME_MS)
  const held = bufferStart ?? now
  const notHeldMs = Math.max(0, Math.min(held, now) - start)
  const firstHeldFrame = Math.min(frames, Math.ceil(notHeldMs / FRAME_MS))
  return { start, end: now, frames, notHeldMs, firstHeldFrame }
}

export function frameTime(w: ReplayWindow, frame: number): number {
  return w.start + Math.max(0, Math.min(w.frames, frame)) * FRAME_MS
}

export function frameHeld(w: ReplayWindow, frame: number): boolean {
  return frame >= w.firstHeldFrame
}

/** The hops of one frame's ten seconds: what pulses. */
export function frameHops(events: ActivityEvent[], t: number): ActivityEvent[] {
  return events.filter((e) => {
    const ts = tsOf(e)
    return ts > t - FRAME_MS && ts <= t
  })
}

/** The next frame when playing, or null at the end, where play stops. */
export function nextFrame(w: ReplayWindow, frame: number): number | null {
  return frame >= w.frames ? null : frame + 1
}

export function describeNotHeld(ms: number): string {
  const min = Math.round(ms / 60_000)
  return min >= 1 ? `the first ${min} minute${min === 1 ? '' : 's'}` : `the first ${Math.round(ms / 1000)} seconds`
}

// ---- one conversation -------------------------------------------------------------

const OPENS = new Set(['input.queued', 'channel.inbound', 'conversation.created'])

/**
 * The hops of a conversation's latest run, in order. A run begins at the input
 * that brought it once the one before it completed, and keeps the ops that
 * delivered its answer.
 */
export function latestRun(events: ActivityEvent[], conversation: string): ActivityEvent[] {
  const evs = events.filter((e) => e.conversation === conversation).sort((a, b) => tsOf(a) - tsOf(b))
  let start = 0
  let completed = false
  evs.forEach((e, i) => {
    if (OPENS.has(e.kind) && completed) {
      start = i
      completed = false
    }
    if (e.kind === 'run.completed') completed = true
  })
  return evs.slice(start)
}

export interface Step {
  ev: ActivityEvent
  offsetMs: number
}

export function steps(evs: ActivityEvent[]): Step[] {
  const t0 = evs.length ? tsOf(evs[0]) : 0
  return evs.map((ev) => ({ ev, offsetMs: tsOf(ev) - t0 }))
}

/** Play compresses a gap: a long one reads as long, never as forever. */
export function playDelay(gapMs: number): number {
  return 900 + Math.min(2200, Math.log10(1 + Math.max(0, gapMs) / 1000) * 1200)
}

/** The conversations with hops in the window, most recent first. */
export function activeConversations(events: ActivityEvent[], tEnd: number, windowMs: number): string[] {
  const seen = new Map<string, number>()
  for (const e of events) {
    const t = tsOf(e)
    if (!e.conversation || t <= tEnd - windowMs || t > tEnd) continue
    seen.set(e.conversation, Math.max(seen.get(e.conversation) ?? 0, t))
  }
  return [...seen].sort((a, b) => b[1] - a[1]).map(([c]) => c)
}

/** What stays lit while a conversation replays: every node and edge its hops touch. */
export function conversationRoute(
  g: ViewGraph, index: HopIndex, evs: ActivityEvent[], conversation: string,
): { nodes: Set<string>; edges: Set<string> } {
  const nodes = new Set<string>()
  const edges = new Set<string>()
  for (const ev of evs) {
    const x = crossing(g, index, ev)
    if (!x) continue
    if (x.pop) nodes.add(x.pop)
    for (const { edge } of x.edges) {
      edges.add(edge.id)
      nodes.add(edge.from)
      nodes.add(edge.to)
    }
  }
  // the conversation itself, and whatever opened it, on the views that draw it
  const conv = `conversations/${conversation}`
  if (index.nodes.has(conv)) {
    nodes.add(conv)
    for (const e of g.edges) {
      if (e.to === conv) {
        edges.add(e.id)
        nodes.add(e.from)
      }
    }
  }
  for (const n of g.nodes) if (n.conversation === conversation) nodes.add(n.id)
  return { nodes, edges }
}
