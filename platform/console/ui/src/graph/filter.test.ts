import { describe, expect, it } from 'vitest'
import { CLAUDE_IMAGE, fixtureTopology, hop } from '../test-fixtures/topology'
import { compile, depthLevels, visible, type VisibleOptions } from './filter'
import { routeMembers } from './route'
import type { ViewGraph, ViewNode } from './types'
import { buildView } from './views'

const ctx = (counts: Record<string, number> = {}) => ({ counts: new Map(Object.entries(counts)), windowSeconds: 300 })
const model = () => buildView('model', fixtureTopology())
const find = (g: ViewGraph, id: string) => g.nodes.find((n) => n.id === id)!

function opts(over: Partial<VisibleOptions> = {}): VisibleOptions {
  return {
    hiddenClasses: new Set(), idleNodes: true, idleEdges: true, busyEdges: new Set(), busyNodes: new Set(), ...over,
  }
}

describe('find and hide expressions', () => {
  const g = model()
  const ids = (expr: string, counts?: Record<string, number>) =>
    g.nodes.filter((n) => compile(expr, ctx(counts))!.test(n)).map((n) => n.id)

  it('reads each form', () => {
    expect(ids('kind=pipeline')).toContain('pipelines/alert-triage')
    expect(ids('kind=signal-source')).toContain('signalsources/nightly')
    expect(ids('name~k8s')).toEqual(expect.arrayContaining(['pipelines/k8s-observe', 'signaladapters/k8s-events']))
    expect(ids('name=console').every((id) => id.endsWith('/console'))).toBe(true)
    expect(ids('!healthy')).toEqual(['signalsources/bench-sensors'])
    expect(ids('healthy')).not.toContain('signalsources/bench-sensors')
    expect(ids('detached')).toEqual(['signalsources/bench-sensors'])
    expect(ids('bundle=release')).toEqual(expect.arrayContaining(['pipelines/nightly-report', 'signaladapters/cron']))
    expect(ids('idle', { 'pipelines/alert-triage': 3 })).not.toContain('pipelines/alert-triage')
    expect(ids('rate>0.5', { 'pipelines/alert-triage': 3 })).toEqual(['pipelines/alert-triage'])
    expect(ids('kind=pipeline and name~k8s')).toEqual(['pipelines/k8s-observe'])
  })

  it('reads cluster node and pipeline on Infrastructure', () => {
    const infra = buildView('infrastructure', fixtureTopology())
    const on = (expr: string) => infra.nodes.filter((n) => compile(expr, ctx())!.test(n)).map((n) => n.id)
    expect(on('node=node-b')).toContain('pods/agentops-conv-nightly-d2')
    expect(on('pipeline=alert-triage')).toEqual(['pods/agentops-conv-prometheus-alerts-a1'])
  })

  it('names the terms it does not know, and matches nothing with them', () => {
    const c = compile('lane=ingest', ctx())!
    expect(c.unknown).toEqual(['lane=ingest'])
    expect(g.nodes.some((n) => c.test(n))).toBe(false)
    expect(compile('   ', ctx())).toBeNull()
  })
})

describe('what the cuts hide, and say they hide', () => {
  it('counts a hidden class and names its failures, leaving the spine', () => {
    const g = model()
    const v = visible(g, opts({ hiddenClasses: new Set(['signalsources', 'pipelines']) }))
    expect(v.nodes.some((n) => n.cls === 'signalsources')).toBe(false)
    // the spine is listed and never hidden
    expect(v.nodes.some((n) => n.cls === 'pipelines')).toBe(true)
    expect(v.hidden.count).toBe(5)
    expect(v.hidden.failing).toBe(1)
    expect(v.hidden.classes).toEqual(['Signal source'])
  })

  it('counts what the hide expression takes', () => {
    const g = model()
    const v = visible(g, opts({ hide: compile('kind=toolset', ctx()) }))
    expect(v.hidden.count).toBe(2)
    expect(v.nodes.some((n) => n.cls === 'mcptoolsets')).toBe(false)
  })

  it('counts idle elements removed, keeping failures', () => {
    const g = model()
    const busy = 'signalsources/prometheus-alerts->pipelines/alert-triage'
    const v = visible(g, opts({ idleNodes: false, idleEdges: false, busyEdges: new Set([busy]) }))
    const ids = v.nodes.map((n) => n.id)
    expect(ids).toEqual(expect.arrayContaining(['signalsources/prometheus-alerts', 'pipelines/alert-triage', 'signalsources/bench-sensors']))
    expect(v.edges.map((e) => e.id)).toEqual([busy])
    expect(v.hidden.count).toBe(g.nodes.length - v.nodes.length)
  })

  it('selecting a pipeline removes the others and counts them separately', () => {
    const topo = fixtureTopology()
    const g = buildView('model', topo)
    const v = visible(g, opts({ routes: routeMembers(topo, g, 'nightly-report') }))
    const ids = v.nodes.map((n) => n.id)
    expect(ids).toContain('pipelines/nightly-report')
    expect(ids).toContain('channels/console')
    expect(ids).not.toContain('mcpconfigs/prometheus')
    expect(v.outOfRoutes.count).toBeGreaterThan(0)
    expect(v.outOfRoutes.failing).toBe(1)
    expect(v.hidden.count).toBe(0)
  })

  it('narrows a scope by depth and reports what lies beyond', () => {
    const g = model()
    const all = visible(g, opts({ scope: { id: 'channels/console', depth: 'all' } }))
    // connection runs both ways: every pipeline posting there is on its route
    for (const p of ['alert-triage', 'k8s-observe', 'nightly-report']) {
      expect(all.nodes.map((n) => n.id)).toContain(`pipelines/${p}`)
    }
    const one = visible(g, opts({ scope: { id: 'channels/console', depth: 1 } }))
    expect(one.nodes.length).toBeLessThan(all.nodes.length)
    expect(one.beyondDepth).toBe(all.nodes.length - one.nodes.length)
    expect(depthLevels(all.maxDepth).at(-1)).toBe('all')
    expect(one.outOfScope.count).toBeGreaterThan(all.outOfScope.count)
  })

  it('a scope never reaches another pipeline through a shared channel', () => {
    const v = visible(model(), opts({ scope: { id: 'pipelines/nightly-report', depth: 'all' } }))
    const ids = v.nodes.map((n) => n.id)
    expect(ids).toContain('channels/console')
    expect(ids).not.toContain('pipelines/k8s-observe')
    expect(ids).not.toContain('mcptoolsets/agentops-shell')
  })

  it('scopes a Components element to the routes through it, with depth that means something', () => {
    const topo = fixtureTopology()
    const g = buildView('components', topo)
    const img = `runtime-image/${CLAUDE_IMAGE}`
    const calls = [hop('tool.call', img, 'mcp-server/prometheus', { pipeline: 'alert-triage', conversation: 'prometheus-alerts-a1' })]
    const members = routeMembers(topo, g, 'alert-triage', calls)
    const all = visible(g, opts({ scope: { id: 'mcp-server/prometheus', depth: 'all' }, scopeMembers: members }))
    expect(all.nodes.map((n) => n.id)).not.toContain('mcp-server/kubernetes')
    const one = visible(g, opts({ scope: { id: 'mcp-server/prometheus', depth: 1 }, scopeMembers: members }))
    expect(one.nodes.length).toBeLessThan(all.nodes.length)
  })

  it('offers only the depth levels a route has', () => {
    expect(depthLevels(0)).toEqual([])
    expect(depthLevels(1)).toEqual([])
    expect(depthLevels(3)).toEqual([1, 2, 'all'])
    expect(depthLevels(9)).toEqual([1, 2, 3, 'all'])
  })

  it('leaves an unscoped view whole when the scoped element is gone', () => {
    const g = model()
    const v = visible(g, opts({ scope: { id: 'pipelines/nope', depth: 'all' } }))
    expect(v.nodes.length).toBe(g.nodes.length)
    const bad: ViewNode = find(g, 'signalsources/bench-sensors')
    expect(bad.reason).toBe('NoPipelineClaim')
  })
})
