import type { ViewId, ViewNode } from '../types'
import type { Group } from './geometry'

// BOXES ARE OWNERSHIP, NEVER KIND. On the Model, the bundle that installs an
// object or the one route that reaches it. On Infrastructure, the cluster node
// a pod runs on, with an opened pod a box of its own inside it. On Components,
// nothing. Shared substrate stays unboxed, as a shared service does in a mesh,
// and nothing positions a box but its members' edges.

export type BoxBy = 'bundle' | 'route' | 'node' | 'none'

export const BOX_OPTIONS: Record<ViewId, { value: BoxBy; label: string }[]> = {
  model: [
    { value: 'bundle', label: 'Bundle' },
    { value: 'route', label: 'Route' },
    { value: 'none', label: 'None' },
  ],
  components: [],
  infrastructure: [
    { value: 'node', label: 'Cluster node' },
    { value: 'none', label: 'None' },
  ],
}

/**
 * groupsOf boxes the drawn nodes by their owner. `routeOwners` maps a node to
 * the one pipeline reaching it, and a node two pipelines reach has no entry.
 */
export function groupsOf(
  nodes: ViewNode[], view: ViewId, by: BoxBy, routeOwners: ReadonlyMap<string, string> = new Map(),
): Group[] {
  const groups = new Map<string, Group>()
  const into = (id: string, title: string, hint: string, member?: string, parent?: string) => {
    const g = groups.get(id) ?? { id, title, hint, members: [], parent }
    if (member) g.members.push(member)
    groups.set(id, g)
    return g
  }
  for (const n of nodes) {
    if (view === 'model') {
      const owner = by === 'bundle' ? n.bundle : by === 'route' ? routeOwners.get(n.id) : undefined
      if (owner) into(`box:${by}:${owner}`, owner, by, n.id)
      continue
    }
    if (view !== 'infrastructure') continue
    const nodeBox = by === 'node' && n.clusterNode ? `box:node:${n.clusterNode}` : undefined
    if (nodeBox) into(nodeBox, n.clusterNode!, 'cluster node')
    if (n.cls === 'container' && n.pod) {
      const pod = n.pod.slice(n.pod.indexOf('/') + 1)
      into(`box:pod:${pod}`, pod.replace(/^agentops-conv-/, ''), 'pod · click to collapse', n.id, nodeBox)
    } else if (nodeBox && n.cls === 'pod') into(nodeBox, n.clusterNode!, 'cluster node', n.id)
  }
  return [...groups.values()]
}
