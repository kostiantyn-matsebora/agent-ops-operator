import { describe, expect, it } from 'vitest'
import { CLAUDE_IMAGE, fixtureTopology, hop } from '../test-fixtures/topology'
import { crossing, hopIndex } from './hops'
import { reach, routeMembers, routeOwners } from './route'
import { isSpine, type ViewGraph } from './types'
import { buildView, componentsView, foldToRoutes, infrastructureView, modelView } from './views'
import { SPINE_CLASS, VIEW_CLASSES } from './views/classes'

const IMG = `runtime-image/${CLAUDE_IMAGE}`
const A1 = 'agentops-conv-prometheus-alerts-a1'
const MGR_POD = 'pods/agentops-manager-7c9d'

/** The drawn path a hop takes on a view, as `from>to` in travel order. */
function path(g: ViewGraph, ev: Parameters<ViewGraph['legs']>[0]): string[] {
  const x = crossing(g, hopIndex(g), ev)
  if (!x) return []
  if (x.pop) return [`@${x.pop}`]
  return x.edges.map(({ edge, dir }) => (dir === 1 ? `${edge.from}>${edge.to}` : `${edge.to}>${edge.from}`))
}

const dispatch = hop('run.dispatched', 'pipeline/alert-triage', 'runtime/default', {
  conversation: 'prometheus-alerts-a1', pipeline: 'alert-triage', runId: 'r1',
})

describe('one hop, three pictures', () => {
  it('moves the pipeline runtime edge, manager to context-sync to image, and manager pod to conversation pod', () => {
    const topo = fixtureTopology()
    expect(path(modelView(topo), dispatch)).toEqual(['pipelines/alert-triage>agentruntimes/default'])
    expect(path(componentsView(topo), dispatch)).toEqual([
      'manager/manager>context-sync/context-sync',
      `context-sync/context-sync>${IMG}`,
    ])
    expect(path(infrastructureView(topo), dispatch)).toEqual([`${MGR_POD}>pods/${A1}`])
  })

  it('maps a signal, an op and a model call onto every view', () => {
    const topo = fixtureTopology()
    const signal = hop('signal.received', 'signal-adapter/alertmanager', 'signal-source/prometheus-alerts')
    expect(path(modelView(topo), signal)).toEqual(['signaladapters/alertmanager>signalsources/prometheus-alerts'])
    expect(path(componentsView(topo), signal)).toEqual(['signal-adapter/alertmanager>manager/manager'])
    expect(path(infrastructureView(topo), signal)).toEqual([`pods/agentops-signal-alertmanager-6b7d>${MGR_POD}`])

    const op = hop('channel.op.enqueued', 'conversation/prometheus-alerts-a1', 'channel/ops-chat', {
      conversation: 'prometheus-alerts-a1',
    })
    // The conversation has no edge to a channel: its pipeline's wiring is what moved.
    expect(path(modelView(topo), op)).toEqual(['pipelines/alert-triage>channels/ops-chat'])

    const call = hop('model.call', IMG, 'model/claude-sonnet-5', {
      conversation: 'prometheus-alerts-a1', pipeline: 'alert-triage', data: { tokensIn: '1200' },
    })
    expect(path(modelView(topo), call)).toEqual(['@agentruntimes/default'])
    expect(path(componentsView(topo), call)).toEqual([
      `${IMG}>egress-proxy/egress-proxy`, 'egress-proxy/egress-proxy>model/claude-sonnet-5',
    ])
    expect(path(infrastructureView(topo), call)).toEqual([`pods/${A1}>model/claude-sonnet-5`])
  })

  it('pulses a dropped signal on its source', () => {
    const topo = fixtureTopology()
    const drop = hop('signal.dropped', 'signal-source/bench-sensors', null, { status: 'error' })
    expect(path(modelView(topo), drop)).toEqual(['@signalsources/bench-sensors'])
  })
})

describe('the views', () => {
  it('wires the runtime from the pipeline, never from the profile', () => {
    const g = modelView(fixtureTopology())
    expect(g.edges.some((e) => e.from === 'pipelines/alert-triage' && e.to === 'agentruntimes/default')).toBe(true)
    expect(g.edges.some((e) => e.from.startsWith('agentprofiles/') && e.to.startsWith('agentruntimes/'))).toBe(false)
    // image, harness and vendor are panel facts, not nodes
    const rt = g.nodes.find((n) => n.id === 'agentruntimes/default')!
    expect(rt.facts).toContainEqual(['Harness', 'Claude Code'])
    expect(g.nodes.some((n) => n.cls === 'runtime-image')).toBe(false)
  })

  it('draws a runtime image in several pods once, with the count', () => {
    const g = componentsView(fixtureTopology())
    const images = g.nodes.filter((n) => n.cls === 'runtime-image')
    expect(images.map((n) => n.id)).toContain(IMG)
    expect(images.find((n) => n.id === IMG)!.count).toBe(2)
    expect(images.find((n) => n.id === IMG)!.label).toBe('agentops-runtime-claude:0.9.3')
  })

  it('draws the MCP servers the configs point at on Components', () => {
    const g = componentsView(fixtureTopology())
    expect(g.edges.map((e) => e.id)).toContain('egress-proxy/egress-proxy->mcp-server/kubernetes')
  })

  it('draws no Model object on Infrastructure, and keeps pod attributes in the panel', () => {
    const g = infrastructureView(fixtureTopology())
    expect(g.nodes.some((n) => n.id.startsWith('pipelines/') || n.id.startsWith('conversations/'))).toBe(false)
    const p = g.nodes.find((n) => n.id === `pods/${A1}`)!
    expect(p.collapsed).toBe(true)
    expect(p.facts).toContainEqual(['Pipeline', 'alert-triage'])
    expect(p.facts).toContainEqual(['Cluster node', 'node-b'])
  })

  it('declares each view its own classes and one spine', () => {
    const topo = fixtureTopology()
    for (const v of ['model', 'components', 'infrastructure'] as const) {
      const g = buildView(v, topo)
      expect(VIEW_CLASSES[v]).toContain(SPINE_CLASS[v])
      for (const n of g.nodes) expect(VIEW_CLASSES[v]).toContain(n.cls)
    }
    const infra = infrastructureView(topo)
    expect(infra.spineId).toBe(MGR_POD)
    // on Infrastructure only the manager's pod is the spine, never every pod
    expect(infra.nodes.filter((n) => isSpine(infra, n)).map((n) => n.id)).toEqual([MGR_POD])
    const comps = componentsView(topo)
    expect(comps.nodes.filter((n) => isSpine(comps, n)).map((n) => n.id)).toEqual(['manager/manager'])
  })
})

describe('routes only', () => {
  it('folds a pipeline profile, runtime and capabilities into it', () => {
    const g = foldToRoutes(modelView(fixtureTopology()))
    const classes = new Set(g.nodes.map((n) => n.cls))
    expect([...classes].sort()).toEqual(['channeladapters', 'channels', 'pipelines', 'signaladapters', 'signalsources'])
    const p = g.nodes.find((n) => n.id === 'pipelines/k8s-observe')!
    const folds = p.facts.find(([k]) => k === 'Folds in')![1]
    for (const x of ['k8s-engineer', 'default', 'agentops-observe', 'agentops-shell', 'kubernetes', 'cluster-events-b7']) {
      expect(folds).toContain(x)
    }
    // a model call inside the route pulses on the pipeline that holds its runtime
    const call = hop('model.call', IMG, 'model/claude-sonnet-5', { pipeline: 'k8s-observe', conversation: 'cluster-events-b7' })
    expect(path(g, call)).toEqual(['@pipelines/k8s-observe'])
    // and the route's own edges survive the fold
    expect(g.edges.map((e) => e.id)).toContain('pipelines/k8s-observe->channels/console')
  })
})

describe('the route walk', () => {
  it('does not use a shared adapter as a shortcut', () => {
    const topo = fixtureTopology()
    const g = modelView(topo)
    // alert-triage posts to the console channel; the console adapter also
    // serves the chat source chat-helper reads. Scoping one reaches no other.
    const route = reach('pipelines/alert-triage', g.edges)
    expect(route.has('channeladapters/console')).toBe(true)
    expect(route.has('signaladapters/console')).toBe(false)
    expect(route.has('signalsources/console')).toBe(false)
    expect(route.has('pipelines/chat-helper')).toBe(false)
  })

  it('refuses the mockup form of the same shortcut', () => {
    const edges = [
      { from: 'pipelines/a', to: 'channels/console', kind: 'posts' },
      { from: 'channels/console', to: 'channeladapters/console', kind: 'served-by' },
      { from: 'channeladapters/console', to: 'signalsources/console', kind: 'served-by' },
      { from: 'signalsources/console', to: 'pipelines/b', kind: 'feeds' },
    ]
    expect(reach('pipelines/a', edges).has('pipelines/b')).toBe(false)
  })

  it('never turns around through a shared channel', () => {
    const route = reach('pipelines/nightly-report', modelView(fixtureTopology()).edges)
    expect(route.has('channels/console')).toBe(true)
    expect(route.has('pipelines/alert-triage')).toBe(false)
    expect(route.has('mcpconfigs/prometheus')).toBe(false)
  })

  it('credits a shared image along the route, not along the image', () => {
    const topo = fixtureTopology()
    const calls = [
      hop('tool.call', IMG, 'mcp-server/kubernetes', { pipeline: 'k8s-observe', conversation: 'cluster-events-b7' }),
      hop('tool.call', IMG, 'mcp-server/prometheus', { pipeline: 'alert-triage', conversation: 'prometheus-alerts-a1' }),
    ]
    for (const v of ['components', 'infrastructure'] as const) {
      const members = routeMembers(topo, buildView(v, topo), 'alert-triage', calls)
      expect(members.has('mcp-server/prometheus')).toBe(true)
      expect([...members].some((id) => id.includes('kubernetes-mcp') || id === 'mcp-server/kubernetes')).toBe(false)
    }
    // and on the Model a tool call lands on the route's own MCP config
    expect(path(modelView(topo), calls[1])).toEqual(['pipelines/alert-triage>mcpconfigs/prometheus'])
  })

  it('owns a node to a route only when one pipeline reaches it', () => {
    const owners = routeOwners(fixtureTopology())
    expect(owners.get('mcpconfigs/prometheus')).toBe('alert-triage')
    expect(owners.get('agentruntimes/sandbox')).toBe('nightly-report')
    expect(owners.has('agentruntimes/default')).toBe(false)
    expect(owners.has('channels/console')).toBe(false)
  })
})

describe('an expanded conversation pod', () => {
  it('opens into its containers and routes the hops through the sidecars', () => {
    const topo = fixtureTopology()
    const open = infrastructureView(topo, new Set([`pods/${A1}`]))
    const ids = open.nodes.map((n) => n.id)
    expect(ids).not.toContain(`pods/${A1}`)
    for (const k of ['agent', 'context-sync', 'egress-proxy']) expect(ids).toContain(`containers/${A1}/${k}`)

    const call = hop('model.call', IMG, 'model/claude-sonnet-5', { conversation: 'prometheus-alerts-a1', pipeline: 'alert-triage' })
    expect(path(open, call)).toEqual([
      `containers/${A1}/agent>containers/${A1}/egress-proxy`,
      `containers/${A1}/egress-proxy>model/claude-sonnet-5`,
    ])
    expect(path(open, dispatch)).toEqual([
      `${MGR_POD}>containers/${A1}/context-sync`,
      `containers/${A1}/context-sync>containers/${A1}/agent`,
    ])

    const closed = infrastructureView(topo, new Set())
    expect(closed.nodes.map((n) => n.id)).toContain(`pods/${A1}`)
    expect(path(closed, call)).toEqual([`pods/${A1}>model/claude-sonnet-5`])
  })
})
