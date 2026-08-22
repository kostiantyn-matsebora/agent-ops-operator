import { describe, expect, it } from 'vitest'
import type { GraphEdge, GraphNode, Topology } from '../api/types'
import { animationDuration, edgeId, edgeLabel, edgeTone, liveEdgeIds, scopedGraph, visibleGraph } from './model'

const topo: Topology = {
  eventNodeKinds: {
    'signal-source': 'signalsources',
    pipeline: 'pipelines',
    channel: 'channels',
    toolset: 'mcptoolsets',
  },
  nodes: [
    { id: 'signalsources/events', kind: 'signalsources', name: 'events', health: 'ok', active: 0, recent: 0 },
    { id: 'pipelines/ops', kind: 'pipelines', name: 'ops', health: 'ok', active: 1, recent: 3 },
    { id: 'channels/console', kind: 'channels', name: 'console', health: 'ok', active: 0, recent: 0 },
    { id: 'mcptoolsets/admin', kind: 'mcptoolsets', name: 'admin', health: 'bad', active: 0, recent: 0 },
  ],
  edges: [
    {
      from: 'signalsources/events', to: 'pipelines/ops', kind: 'feeds',
      traffic: { events: 12, errors: 0, ratePerMin: 4, p50LatencyMs: 250 },
    },
    { from: 'pipelines/ops', to: 'channels/console', kind: 'posts' },
    { from: 'pipelines/ops', to: 'mcptoolsets/admin', kind: 'uses' },
  ],
}

describe('visibleGraph', () => {
  it('hides a class without removing it from the health summary', () => {
    const view = visibleGraph(topo, { mcptoolsets: true }, true)
    expect(view.nodes.map((n) => n.kind)).not.toContain('mcptoolsets')
    // The failing toolset is hidden but STILL counted — a filter that could
    // conceal a broken component silently is the one way this view could mislead.
    expect(view.health.bad).toBe(1)
    expect(view.hiddenSummary.failing).toBe(1)
    expect(view.hiddenSummary.classes).toEqual(['mcptoolsets'])
  })

  it('drops edges whose endpoints are hidden', () => {
    const view = visibleGraph(topo, { mcptoolsets: true }, true)
    expect(view.edges.some((e) => e.to === 'mcptoolsets/admin')).toBe(false)
    expect(view.edges).toHaveLength(2)
  })

  it('keeps broken edges when idle elements are hidden', () => {
    const withDangling: Topology = {
      ...topo,
      edges: [...topo.edges, { from: 'pipelines/ops', to: 'agentprofiles/ghost', kind: 'answers', dangling: true }],
      nodes: [
        ...topo.nodes,
        { id: 'agentprofiles/ghost', kind: 'agentprofiles', name: 'ghost', health: 'bad', active: 0, recent: 0 },
      ],
    }
    const view = visibleGraph(withDangling, {}, false)
    // A dangling ref is never "idle" — it is wrong, and hiding it under a
    // traffic filter would be the same mistake as hiding a failing node.
    expect(view.edges.some((e) => e.dangling)).toBe(true)
    // ...while a wired-but-quiet edge is filtered out
    expect(view.edges.some((e) => e.to === 'channels/console')).toBe(false)
  })
})

describe('edgeTone', () => {
  it('separates unconfirmed delivery from success', () => {
    const unconfirmed: GraphEdge = {
      from: 'a', to: 'b', kind: 'posts',
      traffic: { events: 1, errors: 0, ratePerMin: 1, unconfirmed: true },
    }
    // Adapter reporting is OPTIONAL, so an adapter that reports nothing must not
    // look like one that delivered.
    expect(edgeTone(unconfirmed)).toBe('unconfirmed')
    expect(edgeTone({ from: 'a', to: 'b', kind: 'posts' })).toBe('idle')
    expect(edgeTone({ from: 'a', to: 'b', kind: 'posts', dangling: true })).toBe('error')
    expect(
      edgeTone({ from: 'a', to: 'b', kind: 'posts', traffic: { events: 2, errors: 1, ratePerMin: 1 } }),
    ).toBe('error')
  })
})

describe('animationDuration', () => {
  it('is faster for busier edges and clamped at both ends', () => {
    expect(animationDuration(0)).toBe(0)
    expect(animationDuration(60)).toBeLessThan(animationDuration(1))
    expect(animationDuration(10_000)).toBeGreaterThanOrEqual(0.4)
    expect(animationDuration(0.001)).toBeLessThanOrEqual(6)
  })
})

describe('edgeLabel', () => {
  it('renders rate and latency, and nothing for an idle edge', () => {
    const e = topo.edges[0]
    expect(edgeLabel(e, 'rate')).toBe('4.0/min')
    expect(edgeLabel(e, 'latency')).toBe('250ms')
    expect(edgeLabel(e, 'none')).toBe('')
    expect(edgeLabel(topo.edges[1], 'rate')).toBe('')
  })
})

describe('liveEdgeIds', () => {
  it('maps activity events onto graph edges', () => {
    const ids = liveEdgeIds(
      [
        {
          cursor: '1', ts: '', kind: 'signal.claimed', status: 'ok',
          from: { kind: 'signal-source', name: 'events' },
          to: { kind: 'pipeline', name: 'ops' },
        },
      ],
      topo.eventNodeKinds,
      topo.edges,
    )
    expect(ids.has('signalsources/events->pipelines/ops')).toBe(true)
  })

  it('matches an edge regardless of which way the graph drew it', () => {
    // A signal travels adapter → source; the graph draws that relationship as
    // source → adapter. Traffic means "these two exchanged something".
    const withAdapter: Topology = {
      ...topo,
      eventNodeKinds: { ...topo.eventNodeKinds, 'signal-adapter': 'signaladapters' },
      edges: [
        ...topo.edges,
        { from: 'signalsources/events', to: 'signaladapters/k8s', kind: 'served-by' },
      ],
    }
    const ids = liveEdgeIds(
      [
        {
          cursor: '1', ts: '', kind: 'signal.received', status: 'ok',
          from: { kind: 'signal-adapter', name: 'k8s' },
          to: { kind: 'signal-source', name: 'events' },
        },
      ],
      withAdapter.eventNodeKinds,
      withAdapter.edges,
    )
    expect(ids.has('signalsources/events->signaladapters/k8s')).toBe(true)
  })

  it('credits a conversation hop to its pipeline edge', () => {
    // An op names a conversation, which the wiring graph has no node for; the
    // movement still crossed the pipeline's edge to that channel.
    const ids = liveEdgeIds(
      [
        {
          cursor: '1', ts: '', kind: 'channel.op.enqueued', status: 'ok',
          from: { kind: 'conversation', name: 'chat-1' },
          to: { kind: 'channel', name: 'console' },
          pipeline: 'ops',
        },
      ],
      topo.eventNodeKinds,
      topo.edges,
    )
    expect(ids.has('pipelines/ops->channels/console')).toBe(true)
  })

  it('lights nothing for a hop that resolves to no drawn edge', () => {
    const ids = liveEdgeIds(
      [
        {
          cursor: '1', ts: '', kind: 'signal.received', status: 'ok',
          from: { kind: 'signal-adapter', name: 'k8s' },
          to: { kind: 'manager', name: 'manager' },
        },
      ],
      topo.eventNodeKinds,
      topo.edges,
    )
    expect(ids.size).toBe(0)
  })
})

// A scope is another filter, so it answers to the same rule the display panel
// does: it may simplify the picture, never conceal a broken component.
describe('scopedGraph', () => {
  // adapter — source — pipeline — channel, plus a toolset off the pipeline and
  // a second pipeline sharing the channel. Two components: the strand above,
  // and an unclaimed source that nothing joins.
  const nodes: GraphNode[] = [
    { id: 'signaladapters/k8s', kind: 'signaladapters', name: 'k8s', health: 'ok', active: 0, recent: 0 },
    { id: 'signalsources/events', kind: 'signalsources', name: 'events', health: 'ok', active: 0, recent: 0 },
    { id: 'pipelines/ops', kind: 'pipelines', name: 'ops', health: 'ok', active: 0, recent: 0 },
    { id: 'pipelines/alerts', kind: 'pipelines', name: 'alerts', health: 'ok', active: 0, recent: 0 },
    { id: 'channels/console', kind: 'channels', name: 'console', health: 'ok', active: 0, recent: 0 },
    { id: 'mcptoolsets/admin', kind: 'mcptoolsets', name: 'admin', health: 'bad', active: 0, recent: 0 },
    { id: 'signalsources/orphan', kind: 'signalsources', name: 'orphan', health: 'bad', active: 0, recent: 0 },
  ]
  const edges: GraphEdge[] = [
    // AS THE BFF EMITS IT: the served CR points at its adapter. Writing this the
    // intuitive way round is what let the adapter defect through — the fixture
    // agreed with the code instead of with the cluster.
    { from: 'signalsources/events', to: 'signaladapters/k8s', kind: 'served-by' },
    { from: 'signalsources/events', to: 'pipelines/ops', kind: 'feeds' },
    { from: 'pipelines/ops', to: 'channels/console', kind: 'posts' },
    { from: 'pipelines/ops', to: 'mcptoolsets/admin', kind: 'uses' },
    { from: 'pipelines/alerts', to: 'channels/console', kind: 'posts' },
  ]
  const ids = (r: { nodes: GraphNode[] }) => r.nodes.map((n) => n.id).sort()

  it('scopes to the whole ROUTE through the element, not to whatever it can be walked to', () => {
    const r = scopedGraph(nodes, edges, { id: 'pipelines/ops', depth: 'all' })
    expect(ids(r)).toEqual([
      'channels/console', 'mcptoolsets/admin',
      'pipelines/ops', 'signaladapters/k8s', 'signalsources/events',
    ])
    // pipelines/alerts posts to the SAME channel, and that is the whole point:
    // a shared object must not become a shortcut between two routes that have
    // nothing to do with each other. Undirected traversal pulled it in.
    expect(ids(r)).not.toContain('pipelines/alerts')
  })

  it('runs UPSTREAM as well as downstream', () => {
    // A channel reaches nothing downstream. Scoping it must still answer which
    // pipelines post to it, or the scope is useless on half the graph — and
    // both of them are ancestors, so both belong.
    const r = scopedGraph(nodes, edges, { id: 'channels/console', depth: 1 })
    expect(ids(r)).toEqual(['channels/console', 'pipelines/alerts', 'pipelines/ops'])
  })

  it('cuts by hop distance, not by discovery order', () => {
    const r = scopedGraph(nodes, edges, { id: 'pipelines/ops', depth: 1 })
    expect(ids(r)).toEqual([
      'channels/console', 'mcptoolsets/admin', 'pipelines/ops', 'signalsources/events',
    ])
    // the adapter is 2 hops upstream: on the route, but beyond this depth, and
    // reported as such rather than simply absent
    expect(r.beyondDepth).toBe(1)
  })

  it('keeps every edge whose ends both survive, and drops the rest', () => {
    const r = scopedGraph(nodes, edges, { id: 'signalsources/events', depth: 1 })
    expect(r.edges.map(edgeId).sort()).toEqual([
      'signalsources/events->pipelines/ops',
      'signalsources/events->signaladapters/k8s',
    ])
  })

  it('names the class of a failing element it put out of view', () => {
    const r = scopedGraph(nodes, edges, { id: 'channels/console', depth: 1 })
    expect(r.outOfScope.failing).toBe(2)
    expect(r.outOfScope.classes).toEqual(['mcptoolsets', 'signalsources'])
  })

  it('puts a signal adapter at the HEAD of its route, not at a dead end', () => {
    // The adapter feeds the source, so everything the source feeds is
    // downstream of the adapter — even though the edge is drawn the other way.
    const r = scopedGraph(nodes, edges, { id: 'signaladapters/k8s', depth: 'all' })
    expect(ids(r)).toEqual([
      'channels/console', 'mcptoolsets/admin', 'pipelines/ops',
      'signaladapters/k8s', 'signalsources/events',
    ])
  })

  it('keeps a channel adapter at the TAIL, where the flow already points', () => {
    const r = scopedGraph(nodes, edges, { id: 'channels/console', depth: 1 })
    expect(ids(r)).toEqual(['channels/console', 'pipelines/alerts', 'pipelines/ops'])
  })

  it('leaves the whole graph alone when there is no scope', () => {
    const r = scopedGraph(nodes, edges, undefined)
    expect(r.nodes).toHaveLength(nodes.length)
    expect(r.outOfScope.count).toBe(0)
  })

  it('falls back to the whole graph when the focused id is not on it', () => {
    // A node can vanish under the operator — hide its class while it is
    // focused — and an empty canvas would present that as the answer.
    const r = scopedGraph(nodes, edges, { id: 'pipelines/gone', depth: 'all' })
    expect(r.nodes).toHaveLength(nodes.length)
  })
})
