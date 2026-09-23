import type { ActivityEvent } from '../../api/types'
import type { Leg, ViewGraph } from '../types'
import { ROUTE_CLASSES } from './classes'

// ROUTES ONLY: one node per pipeline, its profile, runtime, toolsets, MCP
// configs and conversations folded inside it — Kiali's service graph. Only the
// sources, channels and adapters around it stay nodes.

export function foldToRoutes(g: ViewGraph): ViewGraph {
  const kept = new Set(g.nodes.filter((n) => ROUTE_CLASSES.includes(n.cls)).map((n) => n.id))
  // A folded object belongs to the pipeline pointing at it. A runtime two
  // pipelines share has two owners, and a hop then names which one it crossed.
  const owners = new Map<string, string[]>()
  for (const e of g.edges) {
    if (!e.from.startsWith('pipelines/') || kept.has(e.to)) continue
    owners.set(e.to, [...(owners.get(e.to) ?? []), e.from])
  }
  const fold = (id: string | null, ev: ActivityEvent): string | null => {
    if (!id || kept.has(id)) return id
    const own = owners.get(id) ?? []
    const named = ev.pipeline ? `pipelines/${ev.pipeline}` : undefined
    if (named && own.includes(named)) return named
    return own.length === 1 ? own[0] : named ?? null
  }
  const folded = new Map<string, string[]>()
  for (const [id, own] of owners) for (const p of own) folded.set(p, [...(folded.get(p) ?? []), id])

  return {
    ...g,
    nodes: g.nodes
      .filter((n) => kept.has(n.id))
      .map((n) =>
        folded.has(n.id)
          ? { ...n, facts: [...n.facts, ['Folds in', folded.get(n.id)!.map(short).sort().join(', ')]] }
          : n,
      ),
    edges: g.edges.filter((e) => kept.has(e.from) && kept.has(e.to)),
    legs: (ev) =>
      g.legs(ev).map((path) =>
        path
          .map(([a, b]): Leg | null => {
            const fa = fold(a, ev)
            return fa ? [fa, fold(b, ev)] : null
          })
          .filter((l): l is Leg => l !== null),
      ),
  }
}

/** What the fold put inside a pipeline, as Model ids. */
export function foldedInto(g: ViewGraph, pipeline: string): string[] {
  return g.edges.filter((e) => e.from === pipeline && !ROUTE_CLASSES.includes(clsOf(e.to))).map((e) => e.to)
}

const clsOf = (id: string) => id.split('/')[0]
const short = (id: string) => id.slice(id.indexOf('/') + 1)
