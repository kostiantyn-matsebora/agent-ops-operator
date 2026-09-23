import type { ActivityEvent, NodeRef, Topology } from '../../api/types'
import { edgeId, type Leg, type ViewEdge, type ViewGraph, type ViewNode } from '../types'
import { DEFAULT_EVENT_NODE_KINDS, SPINE_CLASS } from './classes'

// THE MODEL: the declared objects and what references what. A runtime's image,
// harness and vendor are facts in its panel, never nodes.

const DASHED = new Set(['uses', 'opened'])

export function modelView(topo: Topology): ViewGraph {
  const nodes: ViewNode[] = topo.nodes.map((n) => {
    const facts: [string, string][] = []
    if (n.bundle) facts.push(['Bundle', n.bundle])
    if (n.image) facts.push(['Image', n.image])
    if (n.harness) facts.push(['Harness', n.harness])
    if (n.vendor) facts.push(['Vendor', n.vendor])
    for (const x of n.externals ?? []) facts.push(['Faces', `${x.name} (${x.kind})`])
    if (n.servedBy) facts.push(['Served by', n.servedBy])
    if (n.phase) facts.push(['Phase', n.phase])
    if (n.runtimePod) facts.push(['Runtime pod', n.runtimePod.replace(/^pods\//, '')])
    if (n.active || n.recent) facts.push(['Conversations', `${n.active} active, ${n.recent} recent`])
    return {
      id: n.id, cls: n.kind, name: n.name, health: n.health, reason: n.reason, message: n.message,
      detached: n.detached, bundle: n.bundle, facts, config: `${n.kind}/${n.name}`,
    }
  })
  const ids = new Set(nodes.map((n) => n.id))
  const edges: ViewEdge[] = topo.edges.map((e) => ({
    id: edgeId(e.from, e.to), from: e.from, to: e.to, kind: e.kind, dangling: e.dangling,
    dashed: DASHED.has(e.kind), traffic: e.traffic,
  }))
  // A signal adapter served by a channel adapter's process is one object in
  // two roles. The reference is drawn so the walk can refuse it by name.
  for (const n of topo.nodes) {
    if (n.kind === 'signaladapters' && n.servedBy && ids.has(n.servedBy)) {
      edges.push({ id: edgeId(n.id, n.servedBy), from: n.id, to: n.servedBy, kind: 'served-by', dashed: true })
    }
  }

  const kinds = Object.keys(topo.eventNodeKinds ?? {}).length ? topo.eventNodeKinds : DEFAULT_EVENT_NODE_KINDS
  const idOf = (ref?: NodeRef): string | null => {
    if (!ref?.name) return null
    const kind = kinds[ref.kind]
    return kind ? `${kind}/${ref.name}` : null
  }
  const pipelineOfConv = new Map<string, string>()
  const runtimeOf = new Map<string, string>()
  const configsOf = new Map<string, string[]>()
  for (const e of topo.edges) {
    if (e.kind === 'opened') pipelineOfConv.set(e.to, e.from)
    if (e.kind === 'runs-on') runtimeOf.set(e.from, e.to)
    if (e.kind === 'uses' && e.to.startsWith('mcpconfigs/')) {
      configsOf.set(e.from, [...(configsOf.get(e.from) ?? []), e.to])
    }
  }
  const serverConfigs = new Map<string, Set<string>>()
  for (const c of topo.components ?? []) {
    if (c.role === 'mcp-server') serverConfigs.set(c.name, new Set(c.implements ?? []))
  }

  const legs = (ev: ActivityEvent): Leg[][] => {
    const conv = ev.conversation ? `conversations/${ev.conversation}` : undefined
    const p = (conv && pipelineOfConv.get(conv)) || (ev.pipeline ? `pipelines/${ev.pipeline}` : undefined)
    const rt = p ? runtimeOf.get(p) : undefined
    const on = (id?: string): Leg[][] => (id ? [[[id, null]]] : [])
    switch (ev.kind) {
      case 'model.call':
        return on(rt ?? p)
      case 'tool.call': {
        // A shared image's calls are the ROUTE's: credited along the route's
        // own MCP config, never along the image every route shares.
        const cfgs = p ? configsOf.get(p) ?? [] : []
        const serves = ev.to?.name ? serverConfigs.get(ev.to.name) : undefined
        const cfg = cfgs.find((c) => serves?.has(c))
        return cfg && p ? [[[p, cfg]]] : on(rt ?? p)
      }
      case 'runtime.starting':
        return on(conv ?? rt)
      case 'context.restored':
      case 'context.checkpoint':
      case 'context.skipped':
      case 'context.failed':
        return on(rt ?? conv)
    }
    const from = idOf(ev.from)
    const to = idOf(ev.to)
    if (from && !ev.to) return [[[from, null]]]
    const out: Leg[][] = []
    if (from && to) out.push([[from, to]])
    // One endpoint is a conversation with no edge of its own to the other: the
    // movement crossed its pipeline's wiring, exactly as the BFF credits it.
    if (from && to && ev.to?.kind === 'conversation' && p) out.push([[from, p]])
    if (from && to && ev.from?.kind === 'conversation' && p) out.push([[p, to]])
    return out
  }

  return { view: 'model', nodes, edges, spineClass: SPINE_CLASS.model, legs }
}
