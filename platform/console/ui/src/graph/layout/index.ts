import { colaLayout } from './cola'
import { compact, evict } from './compact'
import { concentric } from './concentric'
import type { Pt } from './curve'
import { dagreLayout } from './dagre'
import {
  DEFAULT_CANVAS, fillCanvas, memberBoxes, type Box, type Canvas, type Group, type LayoutEdge, type LayoutNode,
} from './geometry'

// THREE LAYOUTS, KIALI'S, each remembered per view. Concentric for a hub, cola
// for a network, dagre for a flow. A compaction pass follows concentric and
// cola; dagre is left alone.

export type LayoutId = 'concentric' | 'cola' | 'dagre'

export const LAYOUTS: { value: LayoutId; label: string }[] = [
  { value: 'concentric', label: 'Concentric' },
  { value: 'cola', label: 'Cola' },
  { value: 'dagre', label: 'Dagre' },
]

export interface LayoutInput {
  nodes: { id: string; name: string; label?: string }[]
  edges: LayoutEdge[]
  groups: Group[]
  hub?: string
  layout: LayoutId
  canvas?: Canvas
}

export interface Placement {
  pos: Map<string, Pt>
  boxes: Box[]
  /** Dagre's routed edge points, by edge id. */
  routes: Map<string, Pt[]>
  orientation?: 'tb' | 'lr'
}

export function runLayout(input: LayoutInput): Placement {
  const canvas = input.canvas ?? DEFAULT_CANVAS
  const ids = new Set(input.nodes.map((n) => n.id))
  const edges = input.edges.filter((e) => ids.has(e.from) && ids.has(e.to))
  const groups = input.groups
    .map((g) => ({ ...g, members: g.members.filter((m) => ids.has(m)) }))
    .filter((g) => g.members.length > 0 || input.groups.some((c) => c.parent === g.id))
  // Deterministic seeds: the same graph lays out the same way.
  const seeded: LayoutNode[] = [...input.nodes]
    .sort((a, b) => a.id.localeCompare(b.id))
    .map((n, i) => ({ id: n.id, name: n.label ?? n.name, x: Math.cos(i * 2.4) * (40 + i * 18), y: Math.sin(i * 2.4) * (40 + i * 18) }))
  if (seeded.length === 0) return { pos: new Map(), boxes: [], routes: new Map() }

  if (input.layout === 'dagre') {
    const d = dagreLayout(seeded, edges, groups, canvas)
    return { pos: d.pos, boxes: memberBoxes(groups, d.pos), routes: d.routes, orientation: d.orientation }
  }
  const first = input.layout === 'concentric' ? concentric(seeded, edges, input.hub) : colaLayout(seeded, edges, groups, canvas)
  const placed = seeded.map((n) => ({ ...n, ...(first.get(n.id) ?? { x: n.x, y: n.y }) }))
  const pos = compact(placed, edges, groups, canvas, input.layout === 'concentric' && input.hub ? first.get(input.hub) : undefined)
  fillCanvas(pos, canvas)
  evict(pos, groups)
  return { pos, boxes: memberBoxes(groups, pos), routes: new Map() }
}

export { memberBoxes, pictureBounds } from './geometry'
export type { Box, Canvas, Group } from './geometry'
