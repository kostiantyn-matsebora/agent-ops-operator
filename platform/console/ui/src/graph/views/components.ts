import type { ActivityEvent, Component, Topology } from '../../api/types'
import { chain, edgeId, type Leg, type ViewEdge, type ViewGraph, type ViewNode } from '../types'
import { MANAGER, SPINE_CLASS } from './classes'

// ONE NODE PER COMPONENT: the directories that build a container, plus the
// systems outside. A runtime image running in several pods is a count on one
// node. Model objects fold into the manager, runtimes into their image, pods
// into the component they run.

/** The component roles a hop's endpoint already names, verbatim. */
const NAMED = new Set(['signal-adapter', 'channel-adapter', 'runtime-image', 'model', 'mcp-server', 'external', 'manager'])

export const CONTEXT_SYNC = 'context-sync/context-sync'
export const EGRESS_PROXY = 'egress-proxy/egress-proxy'

export function componentClass(role: string): string {
  return role === 'context-sync' || role === 'egress-proxy' ? 'sidecar' : role
}

/** The last segment of an image or a URL, which is what a reader recognises. */
export function shortName(name: string): string {
  const bare = name.replace(/[?#].*$/, '').replace(/\/+$/, '')
  const seg = bare.slice(bare.lastIndexOf('/') + 1)
  return seg.replace(/\.git$/, '') || name
}

export function componentNode(c: Component): ViewNode {
  const facts: [string, string][] = []
  if (c.image) facts.push(['Image', c.image])
  if (c.harness) facts.push(['Harness', c.harness])
  if (c.vendor) facts.push(['Vendor', c.vendor])
  if (c.url) facts.push(['URL', c.url])
  if (c.externalKind) facts.push(['Faces as', c.externalKind])
  if (c.workload) facts.push(['Runs as', c.workload])
  if (c.servedBy) facts.push(['Served by', c.servedBy])
  if (c.bundle) facts.push(['Bundle', c.bundle])
  if (c.implements?.length) facts.push(['Implements', c.implements.join(', ')])
  const long = c.role === 'runtime-image' || c.role === 'repository'
  return {
    id: c.id, cls: componentClass(c.role), name: c.name, label: long ? shortName(c.name) : undefined,
    health: c.health, reason: c.reason, message: c.message, bundle: c.bundle,
    count: c.role === 'runtime-image' || componentClass(c.role) === 'sidecar' ? c.count : undefined,
    facts,
  }
}

/** Facts every non-Model view reads off the base graph. */
export interface Substrate {
  byId: Map<string, Component>
  /** The component implementing a Model id: an adapter's CR, a runtime's image. */
  implementer: (modelId: string) => string[]
  kubernetes?: string
  /** Model ids a pipeline runs on, answers with and binds. */
  runtimeOf: Map<string, string>
  profileOf: Map<string, string>
  configsOf: Map<string, string[]>
  /** Pipelines opened conversations: conversation id -> pipeline id. */
  pipelineOfConv: Map<string, string>
  /** Models each runtime image was observed calling. */
  modelsOfImage: Map<string, Set<string>>
}

export function substrate(topo: Topology): Substrate {
  const comps = topo.components ?? []
  const byId = new Map(comps.map((c) => [c.id, c]))
  const impl = new Map<string, string[]>()
  for (const c of comps) for (const m of c.implements ?? []) impl.set(m, [...(impl.get(m) ?? []), c.id])
  const runtimeOf = new Map<string, string>()
  const profileOf = new Map<string, string>()
  const configsOf = new Map<string, string[]>()
  const pipelineOfConv = new Map<string, string>()
  for (const e of topo.edges) {
    if (e.kind === 'runs-on') runtimeOf.set(e.from, e.to)
    if (e.kind === 'answers') profileOf.set(e.from, e.to)
    if (e.kind === 'opened') pipelineOfConv.set(e.to, e.from)
    if (e.kind === 'uses' && e.to.startsWith('mcpconfigs/')) configsOf.set(e.from, [...(configsOf.get(e.from) ?? []), e.to])
  }
  const modelsOfImage = new Map<string, Set<string>>()
  for (const h of topo.hops ?? []) {
    if (h.from.kind !== 'runtime-image' || h.to.kind !== 'model') continue
    const img = `runtime-image/${h.from.name}`
    modelsOfImage.set(img, (modelsOfImage.get(img) ?? new Set()).add(`model/${h.to.name}`))
  }
  return {
    byId,
    implementer: (id) => impl.get(id) ?? [],
    kubernetes: comps.find((c) => c.role === 'external' && c.externalKind === 'kubernetes')?.id,
    runtimeOf, profileOf, configsOf, pipelineOfConv, modelsOfImage,
  }
}

/** The runtime image a hop's conversation ran on: its pod, then its route's runtime. */
export function imageOf(topo: Topology, s: Substrate, ev: ActivityEvent): string | undefined {
  if (ev.from?.kind === 'runtime-image') return `runtime-image/${ev.from.name}`
  const pod = ev.conversation ? (topo.pods ?? []).find((p) => p.conversation === ev.conversation) : undefined
  if (pod?.component && s.byId.has(pod.component)) return pod.component
  const p = routeOfEvent(s, ev)
  const rt = p ? s.runtimeOf.get(p) : ev.to?.kind === 'runtime' ? `agentruntimes/${ev.to.name}` : undefined
  return rt ? s.implementer(rt).find((id) => id.startsWith('runtime-image/')) : undefined
}

export function routeOfEvent(s: Substrate, ev: ActivityEvent): string | undefined {
  const byConv = ev.conversation ? s.pipelineOfConv.get(`conversations/${ev.conversation}`) : undefined
  return byConv ?? (ev.pipeline ? `pipelines/${ev.pipeline}` : undefined)
}

export function componentsView(topo: Topology): ViewGraph {
  const s = substrate(topo)
  const comps = topo.components ?? []
  const nodes = comps.map(componentNode)
  const has = (id?: string) => Boolean(id && s.byId.has(id))
  const mgr = has(MANAGER) ? MANAGER : undefined
  const ctx = has(CONTEXT_SYNC) ? CONTEXT_SYNC : undefined
  const egr = has(EGRESS_PROXY) ? EGRESS_PROXY : undefined
  const k8s = s.kubernetes
  const edges = new Map<string, ViewEdge>()
  const E = (from: string | undefined, to: string | undefined, kind: string, dashed = false) => {
    if (!from || !to || from === to) return
    const id = edgeId(from, to)
    if (!edges.has(id)) edges.set(id, { id, from, to, kind, dashed })
  }
  for (const e of topo.componentEdges ?? []) E(e.from, e.to, e.kind)
  const byRole = (role: string) => comps.filter((c) => c.role === role)
  for (const c of byRole('signal-adapter')) E(c.id, mgr, 'signal/inbound')
  for (const c of byRole('channel-adapter')) E(mgr, c.id, 'channel/ops')
  E(mgr, k8s, 'reconcile')
  for (const c of byRole('housekeeping')) E(c.id, k8s, 'list', true)
  for (const g of byRole('gateway')) {
    const ca = `channel-adapter/${g.name}`
    if (!has(ca)) continue
    E(g.id, ca, 'updates')
    for (const e of topo.componentEdges ?? []) if (e.from === ca && e.kind === 'calls') E(g.id, e.to, 'getUpdates')
  }
  E(ctx, mgr, '/work')
  const images = byRole('runtime-image')
  for (const img of images) {
    if (ctx) E(img.id, ctx, 'CONTROL_URL')
    else E(mgr, img.id, '/work')
    E(img.id, egr, 'redirected')
    if (!egr) for (const m of s.modelsOfImage.get(img.id) ?? []) E(img.id, m, 'model')
  }
  for (const m of byRole('model')) E(egr, m.id, 'model')
  for (const srv of byRole('mcp-server')) E(egr, srv.id, 'tool', true)
  // An image reaches the repositories its routes' profiles check out, and —
  // with no egress proxy between — the servers its routes' configs name.
  for (const [p, rt] of s.runtimeOf) {
    for (const img of s.implementer(rt).filter((id) => id.startsWith('runtime-image/'))) {
      const profile = s.profileOf.get(p)
      for (const repo of profile ? s.implementer(profile) : []) if (repo.startsWith('repository/')) E(img, repo, 'git', true)
      if (!egr) for (const cfg of s.configsOf.get(p) ?? []) for (const srv of s.implementer(cfg)) if (srv.startsWith('mcp-server/')) E(img, srv, 'tool', true)
    }
  }

  const place = (ref: ActivityEvent['from'], ev: ActivityEvent): string | undefined => {
    if (!ref?.name) return undefined
    if (NAMED.has(ref.kind)) {
      const id = `${ref.kind}/${ref.name}`
      if (has(id)) return id
    }
    if (ref.kind === 'runtime') return imageOf(topo, s, ev)
    return mgr
  }
  const legs = (ev: ActivityEvent): Leg[][] => {
    const img = imageOf(topo, s, ev)
    const to = ev.to ? place(ev.to, ev) : undefined
    switch (ev.kind) {
      case 'runtime.starting':
        return [chain([mgr, k8s])]
      case 'run.dispatched':
        return [chain([mgr, ctx, img])]
      case 'run.completed':
        return [chain([img, ctx, mgr])]
      case 'model.call':
      case 'tool.call':
        return ev.to ? [chain([img, egr, to])] : img ? [[[img, null]]] : []
      case 'context.restored':
        return [chain([ctx, img])]
      case 'context.checkpoint':
      case 'context.skipped':
      case 'context.failed': {
        const at = ctx ?? img
        return at ? [[[at, null]]] : []
      }
    }
    const from = place(ev.from, ev)
    if (!from) return []
    return [[[from, ev.to ? to ?? null : null]]]
  }

  return {
    view: 'components', nodes, edges: [...edges.values()], spineClass: SPINE_CLASS.components,
    hub: mgr, legs,
  }
}
