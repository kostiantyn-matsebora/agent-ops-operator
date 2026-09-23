import type { ActivityEvent, Health } from '../api/types'

// The shapes every view shares. A view is a function from the base graph to
// these, plus the mapping of a hop onto its own nodes: three identities over one
// feed, which is what lets one event animate three pictures.

export type ViewId = 'model' | 'components' | 'infrastructure'

export const VIEWS: { id: ViewId; label: string }[] = [
  { id: 'model', label: 'Model' },
  { id: 'components', label: 'Components' },
  { id: 'infrastructure', label: 'Infrastructure' },
]

/** One leg of a hop in a view: from a node to a node, or to nothing. */
export type Leg = [string, string | null]

export interface ViewNode {
  id: string
  /** The element class: what the display control hides, and what picks the mark. */
  cls: string
  name: string
  /** What is drawn under the mark, when the name is too long to read. */
  label?: string
  health: Health
  reason?: string
  message?: string
  detached?: boolean
  /** Instances running, for a component. */
  count?: number
  /** A conversation pod drawn as one node, which opens into its containers. */
  collapsed?: boolean
  caption?: string
  bundle?: string
  clusterNode?: string
  /** A container's pod, as a pod id. */
  pod?: string
  conversation?: string
  pipeline?: string
  /** Panel rows, verbatim facts. */
  facts: [string, string][]
  /** The Config page this node opens, as `<kind>/<name>`. */
  config?: string
}

export interface ViewEdge {
  id: string
  from: string
  to: string
  kind: string
  dangling?: boolean
  dashed?: boolean
  /** The BFF's own windowed traffic, Model only: a fallback while no hop is held. */
  traffic?: import('../api/types').EdgeTraffic
}

export interface ViewGraph {
  view: ViewId
  nodes: ViewNode[]
  edges: ViewEdge[]
  /** The one class the view cannot do without. */
  spineClass: string
  /** On Infrastructure the spine is one node, the manager's pod. */
  spineId?: string
  /** The concentric layout's centre. */
  hub?: string
  /**
   * The paths a hop may have taken in this view, most direct first. The first
   * one whose every leg is drawn is the one credited.
   */
  legs: (ev: ActivityEvent) => Leg[][]
}

export function edgeId(from: string, to: string): string {
  return `${from}->${to}`
}

export function isSpine(g: Pick<ViewGraph, 'spineClass' | 'spineId' | 'view'>, n: ViewNode): boolean {
  return g.spineId !== undefined ? n.id === g.spineId : n.cls === g.spineClass
}

/** Consecutive legs through the nodes that exist, skipping a missing hop in the middle. */
export function chain(ids: (string | undefined)[]): Leg[] {
  const present = ids.filter((id): id is string => Boolean(id))
  if (present.length === 1) return [[present[0], null]]
  const out: Leg[] = []
  for (let i = 1; i < present.length; i++) out.push([present[i - 1], present[i]])
  return out
}
