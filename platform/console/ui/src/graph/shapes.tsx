import type { ReactElement } from 'react'

// Per-kind shape and glyph.
//
// Kiali gives each element class its own silhouette so a graph is readable
// before you read any label — you learn "hexagons are where traffic enters" once
// and never re-read that column again. The same idea here, mapped onto our nine
// kinds plus pods and conversations.
//
// The glyphs are hand-drawn paths on a 24×24 grid rather than PatternFly icon
// components: an icon component renders its own <svg>, and nesting one inside a
// node's <g> means fighting sizing and fill inheritance for every kind. A path
// is a path.

export type ShapeKind =
  | 'hexagon' // things traffic ENTERS through — sources
  | 'plaque' // implementations that serve them — adapters
  | 'rect' // the wiring spine — pipelines
  | 'circle' // identities — profiles
  | 'stadium' // execution — runtimes, pods
  | 'diamond' // capabilities — toolsets, MCP configs
  | 'cylinder' // surfaces work is delivered to — channels
  | 'note' // work items — conversations

export interface NodeStyle {
  shape: ShapeKind
  /** 24×24 glyph, drawn in the node's accent colour. */
  glyph: string
  label: string
  /** Which lane the node belongs to (see layout.ts). */
  lane: LaneId
}

export type LaneId = 'ingest' | 'wiring' | 'execution' | 'capability' | 'delivery' | 'work'

export const LANES: { id: LaneId; title: string; hint: string }[] = [
  { id: 'ingest', title: 'Ingest', hint: 'where signals enter' },
  { id: 'wiring', title: 'Wiring', hint: 'what claims them' },
  { id: 'execution', title: 'Execution', hint: 'who answers, and on what' },
  { id: 'capability', title: 'Capabilities', hint: 'what the route may reach' },
  { id: 'delivery', title: 'Delivery', hint: 'where answers go' },
  { id: 'work', title: 'Work', hint: 'conversations and their pods' },
]

// Glyphs: deliberately simple, legible at 14px.
const GLYPH = {
  bolt: 'M13 2 L4 14 h6 l-1 8 9-12 h-6 z',
  plug: 'M8 2 v6 M16 2 v6 M5 8 h14 v3 a7 7 0 0 1 -14 0 z M12 18 v4',
  flow: 'M3 6 h7 a4 4 0 0 1 4 4 v4 a4 4 0 0 0 4 4 h3 M18 15 l3 3 -3 3',
  person: 'M12 3 a4 4 0 1 1 0 8 a4 4 0 0 1 0-8 M4 21 a8 8 0 0 1 16 0 z',
  cpu: 'M9 3 v2 M15 3 v2 M9 19 v2 M15 19 v2 M3 9 h2 M3 15 h2 M19 9 h2 M19 15 h2 M5 5 h14 v14 h-14 z M9 9 h6 v6 h-6 z',
  wrench: 'M20 5 a5 5 0 0 1 -6.5 6.5 L5 20 l-1-1 8.5-8.5 A5 5 0 0 1 19 4 l-3 3 1 1 3-3 z',
  server: 'M3 4 h18 v6 h-18 z M3 14 h18 v6 h-18 z M6.5 7 h.01 M6.5 17 h.01',
  chat: 'M4 4 h16 v11 h-9 l-5 4 v-4 h-2 z',
  box: 'M12 2 l9 5 v10 l-9 5 -9-5 v-10 z M12 12 l9-5 M12 12 l-9-5 M12 12 v10',
  note: 'M6 3 h9 l4 4 v14 h-13 z M15 3 v4 h4 M9 12 h7 M9 16 h5',
} as const

export const NODE_STYLES: Record<string, NodeStyle> = {
  signalsources: { shape: 'hexagon', glyph: GLYPH.bolt, label: 'Signal source', lane: 'ingest' },
  signaladapters: { shape: 'plaque', glyph: GLYPH.plug, label: 'Signal adapter', lane: 'ingest' },
  pipelines: { shape: 'rect', glyph: GLYPH.flow, label: 'Pipeline', lane: 'wiring' },
  agentprofiles: { shape: 'circle', glyph: GLYPH.person, label: 'Profile', lane: 'execution' },
  agentruntimes: { shape: 'stadium', glyph: GLYPH.cpu, label: 'Runtime', lane: 'execution' },
  mcptoolsets: { shape: 'diamond', glyph: GLYPH.wrench, label: 'Toolset', lane: 'capability' },
  mcpconfigs: { shape: 'diamond', glyph: GLYPH.server, label: 'MCP config', lane: 'capability' },
  channels: { shape: 'cylinder', glyph: GLYPH.chat, label: 'Channel', lane: 'delivery' },
  channeladapters: { shape: 'plaque', glyph: GLYPH.plug, label: 'Channel adapter', lane: 'delivery' },
  conversations: { shape: 'note', glyph: GLYPH.note, label: 'Conversation', lane: 'work' },
  pods: { shape: 'stadium', glyph: GLYPH.box, label: 'Runtime pod', lane: 'work' },
}

export function styleFor(kind: string): NodeStyle {
  return NODE_STYLES[kind] ?? { shape: 'rect', glyph: GLYPH.box, label: kind, lane: 'wiring' }
}

export const NODE_W = 176
export const NODE_H = 56

/**
 * shapePath returns the outline for a shape at the origin. Every shape occupies
 * the same NODE_W×NODE_H box so the layout does not have to know which is which
 * — silhouettes differ, footprints do not.
 */
export function shapePath(shape: ShapeKind, w = NODE_W, h = NODE_H): string {
  const r = 8
  switch (shape) {
    case 'hexagon': {
      const c = 14
      return `M ${c} 0 H ${w - c} L ${w} ${h / 2} L ${w - c} ${h} H ${c} L 0 ${h / 2} Z`
    }
    case 'plaque': {
      // clipped top-left / bottom-right corners: an implementation, not a thing
      const c = 12
      return `M ${c} 0 H ${w} V ${h - c} L ${w - c} ${h} H 0 V ${c} Z`
    }
    case 'circle': {
      // a wide ellipse, so a long name still fits
      return `M 0 ${h / 2} A ${w / 2} ${h / 2} 0 1 0 ${w} ${h / 2} A ${w / 2} ${h / 2} 0 1 0 0 ${h / 2} Z`
    }
    case 'stadium':
      return `M ${h / 2} 0 H ${w - h / 2} A ${h / 2} ${h / 2} 0 0 1 ${w - h / 2} ${h} H ${h / 2} A ${h / 2} ${h / 2} 0 0 1 ${h / 2} 0 Z`
    case 'diamond': {
      const c = 18
      return `M ${c} 0 H ${w - c} L ${w} ${h / 2} L ${w - c} ${h} H ${c} L 0 ${h / 2} Z`
    }
    case 'cylinder': {
      const e = 9
      return `M 0 ${e} A ${w / 2} ${e} 0 0 1 ${w} ${e} V ${h - e} A ${w / 2} ${e} 0 0 1 0 ${h - e} Z`
    }
    case 'note': {
      const f = 14
      return `M 0 ${r} q 0 -${r} ${r} -${r} H ${w - f} L ${w} ${f} V ${h - r} q 0 ${r} -${r} ${r} H ${r} q -${r} 0 -${r} -${r} Z`
    }
    default:
      return `M ${r} 0 H ${w - r} q ${r} 0 ${r} ${r} V ${h - r} q 0 ${r} -${r} ${r} H ${r} q -${r} 0 -${r} -${r} V ${r} q 0 -${r} ${r} -${r} Z`
  }
}

/** The cylinder's top ellipse, drawn as a separate stroke so it reads as 3D. */
export function shapeDecoration(shape: ShapeKind, w = NODE_W): ReactElement | null {
  if (shape !== 'cylinder') return null
  const e = 9
  return <path d={`M 0 ${e} A ${w / 2} ${e} 0 0 0 ${w} ${e}`} fill="none" stroke="currentColor" strokeWidth={1.5} />
}

/** Glyph rendered at 14px inside the node, scaled from the 24×24 grid. */
export function Glyph({ d, x, y, size = 15 }: { d: string; x: number; y: number; size?: number }) {
  const s = size / 24
  return (
    <g transform={`translate(${x},${y}) scale(${s})`} aria-hidden="true">
      <path d={d} fill="none" stroke="currentColor" strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round" />
    </g>
  )
}
