import type { ActivityEvent, Pod, Topology } from '../../api/types'
import { chain, edgeId, type Leg, type ViewEdge, type ViewGraph, type ViewNode } from '../types'
import { MANAGER, SPINE_CLASS } from './classes'
import { componentClass, componentNode, substrate } from './components'

// INFRASTRUCTURE: every pod that runs, and the systems outside the cluster.
// Nothing from the model is a node. A pod's pipeline and conversation are
// attributes in its panel, and its only box is the cluster node it runs on. A
// conversation pod is ONE node until it is opened into its containers.

/** Components that are never a pod here: they are outside, or not running. */
const OUTSIDE = new Set(['external', 'model', 'repository'])

/** A container's key inside its pod: what the pod's hops are routed through. */
export function containerKey(role?: string, name?: string): string {
  return role === 'runtime-image' ? 'agent' : role || name || 'container'
}

export function containerId(pod: Pod, key: string): string {
  return `containers/${pod.name}/${key}`
}

const podPreference = (p: Pod) => (p.phase === 'Running' ? 0 : p.phase === 'Pending' ? 1 : 2)

export function infrastructureView(topo: Topology, expanded: ReadonlySet<string> = new Set()): ViewGraph {
  const s = substrate(topo)
  const pods = [...(topo.pods ?? [])].sort((a, b) => podPreference(a) - podPreference(b) || a.name.localeCompare(b.name))
  // The one pod a component's hops are drawn to: a running one first. Replicas
  // beyond it are drawn, and carry no traffic of their own.
  const podOf = new Map<string, string>()
  for (const p of pods) if (p.component && !p.conversation && !podOf.has(p.component)) podOf.set(p.component, p.id)

  const nodes: ViewNode[] = []
  const convPod = new Map<string, Pod>()
  for (const p of pods) {
    const role = p.component ? s.byId.get(p.component)?.role : undefined
    const attrs: [string, string][] = []
    if (p.clusterNode) attrs.push(['Cluster node', p.clusterNode])
    if (p.phase) attrs.push(['Phase', p.phase])
    if (p.component) attrs.push(['Runs', p.component])
    if (p.pipeline) attrs.push(['Pipeline', p.pipeline])
    if (p.conversation) attrs.push(['Conversation', p.conversation])
    const base = {
      health: p.health, reason: p.reason, clusterNode: p.clusterNode,
      conversation: p.conversation, pipeline: p.pipeline,
    }
    if (p.conversation) convPod.set(p.conversation, p)
    if (p.conversation && expanded.has(p.id)) {
      for (const c of p.containers) {
        const key = containerKey(c.role, c.name)
        nodes.push({
          ...base, id: containerId(p, key), cls: 'container', name: key, pod: p.id,
          health: c.ready ? 'ok' : p.health === 'ok' ? 'unknown' : p.health,
          caption: c.role ? componentClass(c.role) : undefined,
          facts: [['Pod', p.name], ...(c.image ? [['Image', c.image] as [string, string]] : []),
            ['Ready', String(c.ready)], ['Restarts', String(c.restarts)], ...attrs],
        })
      }
      continue
    }
    nodes.push({
      ...base, id: p.id, cls: 'pod', name: p.name, collapsed: Boolean(p.conversation) || undefined,
      caption: p.conversation ? `agent · ${p.pipeline || p.conversation}` : role ? componentClass(role) : undefined,
      facts: attrs,
    })
  }
  const outside = (topo.components ?? []).filter((c) => OUTSIDE.has(c.role) || (c.role === 'mcp-server' && !podOf.has(c.id)))
  for (const c of outside) nodes.push({ ...componentNode(c), count: undefined })

  const ids = new Set(nodes.map((n) => n.id))
  const target = (compId?: string): string | undefined => {
    if (!compId) return undefined
    if (podOf.has(compId)) return podOf.get(compId)
    const c = s.byId.get(compId)
    if (c?.servedBy && podOf.has(c.servedBy)) return podOf.get(c.servedBy)
    return ids.has(compId) ? compId : undefined
  }
  const mgr = target(MANAGER)
  const k8s = target(s.kubernetes)
  const edges = new Map<string, ViewEdge>()
  const E = (from: string | undefined, to: string | undefined, kind: string, dashed = false) => {
    if (!from || !to || from === to) return
    const id = edgeId(from, to)
    if (!edges.has(id)) edges.set(id, { id, from, to, kind, dashed })
  }
  for (const e of topo.componentEdges ?? []) E(target(e.from), target(e.to), e.kind)
  for (const c of topo.components ?? []) {
    if (c.role === 'signal-adapter') E(target(c.id), mgr, 'signal/inbound')
    if (c.role === 'channel-adapter') E(mgr, target(c.id), 'channel/ops')
    if (c.role === 'housekeeping') E(target(c.id), k8s, 'list', true)
    if (c.role === 'gateway') E(target(c.id), target(`channel-adapter/${c.name}`), 'updates')
  }
  E(mgr, k8s, 'reconcile')

  const parts = (p: Pod) => {
    const open = expanded.has(p.id)
    const find = (key: string) => {
      const id = containerId(p, key)
      return open && ids.has(id) ? id : undefined
    }
    return { open, A: open ? find('agent') : p.id, S: find('context-sync'), X: find('egress-proxy') }
  }
  for (const p of convPod.values()) {
    const { open, A, S, X } = parts(p)
    const out = X ?? A
    if (open) {
      E(mgr, S ?? A, 'work')
      E(S, A, 'control')
      E(A, X, 'redirected')
    } else E(mgr, p.id, 'work')
    for (const m of p.component ? s.modelsOfImage.get(p.component) ?? [] : []) E(out, target(m), 'model')
    const pl = p.pipeline ? `pipelines/${p.pipeline}` : undefined
    const profile = pl ? s.profileOf.get(pl) : undefined
    for (const repo of profile ? s.implementer(profile) : []) if (repo.startsWith('repository/')) E(A, target(repo), 'git', true)
    for (const cfg of pl ? s.configsOf.get(pl) ?? [] : []) {
      for (const srv of s.implementer(cfg)) if (srv.startsWith('mcp-server/')) E(out, target(srv), 'tool', true)
    }
  }

  const place = (ref: ActivityEvent['from'], P?: Pod): string | undefined => {
    if (!ref?.name) return undefined
    switch (ref.kind) {
      case 'signal-adapter':
      case 'channel-adapter':
      case 'mcp-server':
      case 'model':
      case 'external':
        return target(`${ref.kind}/${ref.name}`)
      case 'runtime-image':
      case 'runtime':
        return P ? parts(P).A : undefined
    }
    return mgr
  }
  const legs = (ev: ActivityEvent): Leg[][] => {
    const P = ev.conversation ? convPod.get(ev.conversation) : undefined
    const at = (id?: string): Leg[][] => (id ? [[[id, null]]] : [])
    if (P) {
      const { open, A, S, X } = parts(P)
      const to = ev.to ? place(ev.to, P) : undefined
      switch (ev.kind) {
        case 'runtime.starting':
          return open ? [chain([mgr, S ?? A])] : [chain([mgr, k8s])]
        case 'run.dispatched':
          return [chain([mgr, S, A])]
        case 'run.completed':
          return [chain([A, S, mgr])]
        case 'model.call':
        case 'tool.call':
          return to ? [chain([A, X, to])] : at(A)
        case 'context.restored':
          return S ? [chain([S, A])] : at(A)
        case 'context.checkpoint':
        case 'context.skipped':
        case 'context.failed':
          return at(S ?? A)
      }
    } else if (ev.kind === 'runtime.starting') return [chain([mgr, k8s])]
    const from = place(ev.from, P)
    if (!from) return []
    return [[[from, ev.to ? place(ev.to, P) ?? null : null]]]
  }

  return {
    view: 'infrastructure', nodes, edges: [...edges.values()], spineClass: SPINE_CLASS.infrastructure,
    spineId: mgr, hub: mgr, legs,
  }
}
