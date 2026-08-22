import type { GraphNode } from '../api/types'
import { LANES, NODE_H, NODE_W, styleFor, type LaneId } from './shapes'

// Layout: lanes left to right, with a labelled boundary around each.
//
// The boundaries are the point. A graph of eleven node classes is a soup
// without them; with them the picture reads as a sentence — signals enter on the
// left, wiring claims them, someone answers with some capabilities, answers go
// out on the right. That is the shape of the system, and it is the same shape in
// every install, so it is worth drawing even when a lane is empty of traffic.

export const LANE_GAP = 56
export const ROW_GAP = 26
export const LANE_PAD_X = 18
export const LANE_PAD_Y = 40 // room for the lane title
export const LANE_PAD_BOTTOM = 16

export interface PlacedNode extends GraphNode {
  x: number
  y: number
}

export interface LaneBox {
  id: LaneId
  title: string
  hint: string
  x: number
  y: number
  width: number
  height: number
  count: number
}

export interface Layout {
  nodes: PlacedNode[]
  lanes: LaneBox[]
  width: number
  height: number
}

/** Within a lane, kinds keep a stable vertical order so the graph never jitters. */
const KIND_ORDER = [
  'signaladapters', 'signalsources',
  'pipelines',
  'agentprofiles', 'agentruntimes',
  'mcptoolsets', 'mcpconfigs',
  'channels', 'channeladapters',
  'conversations', 'pods',
]

function kindRank(kind: string): number {
  const i = KIND_ORDER.indexOf(kind)
  return i === -1 ? KIND_ORDER.length : i
}

/**
 * layout places nodes into lane boxes.
 *
 * A lane with no nodes is DROPPED rather than drawn empty: an empty labelled box
 * reads as "this is broken", when usually it just means the Display panel folded
 * that class away.
 */
export function layout(nodes: PlacedNode[] | GraphNode[]): Layout {
  const byLane = new Map<LaneId, GraphNode[]>()
  for (const n of nodes) {
    const lane = styleFor(n.kind).lane
    const list = byLane.get(lane) ?? []
    list.push(n)
    byLane.set(lane, list)
  }

  const placed: PlacedNode[] = []
  const lanes: LaneBox[] = []
  let x = 0

  for (const lane of LANES) {
    const members = byLane.get(lane.id)
    if (!members || members.length === 0) continue

    const sorted = [...members].sort(
      (a, b) =>
        kindRank(a.kind) - kindRank(b.kind) ||
        // detached nodes sink: being off the wiring IS their state, and grouping
        // them makes "nothing claims this" findable at a glance
        Number(Boolean(a.detached)) - Number(Boolean(b.detached)) ||
        a.name.localeCompare(b.name),
    )
    const laneX = x
    sorted.forEach((n, row) => {
      placed.push({
        ...n,
        x: laneX + LANE_PAD_X,
        y: LANE_PAD_Y + row * (NODE_H + ROW_GAP),
      })
    })
    const height = LANE_PAD_Y + sorted.length * (NODE_H + ROW_GAP) - ROW_GAP + LANE_PAD_BOTTOM
    const width = NODE_W + LANE_PAD_X * 2
    lanes.push({ ...lane, x: laneX, y: 0, width, height, count: sorted.length })
    x += width + LANE_GAP
  }

  const width = Math.max(x - LANE_GAP, 320)
  const height = Math.max(...lanes.map((l) => l.height), 200)
  // every lane box is drawn to the tallest lane's height, so the boundaries line
  // up and the picture reads as columns rather than a ragged skyline
  for (const l of lanes) l.height = height
  return { nodes: placed, lanes, width, height }
}

/** Anchor points for an edge between two placed nodes. */
export function anchors(from: PlacedNode, to: PlacedNode) {
  const forward = to.x >= from.x
  return {
    x1: forward ? from.x + NODE_W : from.x,
    y1: from.y + NODE_H / 2,
    x2: forward ? to.x : to.x + NODE_W,
    y2: to.y + NODE_H / 2,
    forward,
  }
}
