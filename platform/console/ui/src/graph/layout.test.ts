import { describe, expect, it } from 'vitest'
import { fixtureTopology } from '../test-fixtures/topology'
import { runLayout, type Box, type LayoutId } from './layout'
import { arcCurve, basisCurve } from './layout/curve'
import { groupsOf, type BoxBy } from './layout/boxes'
import { MARK_HALF } from './layout/compact'
import { allMembers } from './layout/geometry'
import { routeOwners } from './route'
import { styleFor, shapePath, NODE_STYLES } from './shapes'
import type { ViewGraph, ViewId } from './types'
import { buildView } from './views'

const VIEWS: ViewId[] = ['model', 'components', 'infrastructure']

function lay(g: ViewGraph, layout: LayoutId, by: BoxBy = 'none') {
  const groups = groupsOf(g.nodes, g.view, by, routeOwners(fixtureTopology()))
  return { groups, ...runLayout({ nodes: g.nodes, edges: g.edges, groups, hub: g.hub, layout }) }
}

const overlap = (a: { x0: number; y0: number; x1: number; y1: number }, b: typeof a) =>
  a.x0 < b.x1 && b.x0 < a.x1 && a.y0 < b.y1 && b.y0 < a.y1
const rect = (b: Box) => ({ x0: b.x0, y0: b.y0, x1: b.x0 + b.w, y1: b.y0 + b.h })
const mark = (p: { x: number; y: number }) => ({
  x0: p.x - MARK_HALF.w, y0: p.y - MARK_HALF.h, x1: p.x + MARK_HALF.w, y1: p.y + MARK_HALF.h,
})
/** Boxes in one family nest; only unrelated boxes must not touch. */
const related = (a: Box, b: Box, boxes: Box[]) => {
  const up = (x: Box): string[] => (x.parent ? [x.parent, ...up(boxes.find((y) => y.id === x.parent)!)] : [])
  return up(a).includes(b.id) || up(b).includes(a.id)
}

describe('layouts', () => {
  for (const view of VIEWS) {
    for (const layout of ['concentric', 'cola', 'dagre'] as LayoutId[]) {
      it(`lays out ${view} with ${layout}`, () => {
        const g = buildView(view, fixtureTopology())
        const { pos } = lay(g, layout)
        expect(pos.size).toBe(g.nodes.length)
        for (const p of pos.values()) {
          expect(Number.isFinite(p.x)).toBe(true)
          expect(Number.isFinite(p.y)).toBe(true)
        }
      })
    }
  }

  it('puts the hub at the centre of a concentric layout', () => {
    const g = buildView('components', fixtureTopology())
    const { pos } = lay(g, 'concentric')
    const ps = [...pos.values()]
    const cx = (Math.min(...ps.map((p) => p.x)) + Math.max(...ps.map((p) => p.x))) / 2
    const cy = (Math.min(...ps.map((p) => p.y)) + Math.max(...ps.map((p) => p.y))) / 2
    const hub = pos.get('manager/manager')!
    const span = Math.max(...ps.map((p) => Math.hypot(p.x - cx, p.y - cy)))
    // the manager is the node nearest the middle of the picture
    const nearest = [...pos.entries()].sort(
      ([, a], [, b]) => Math.hypot(a.x - cx, a.y - cy) - Math.hypot(b.x - cx, b.y - cy),
    )[0][0]
    expect(nearest).toBe('manager/manager')
    expect(Math.hypot(hub.x - cx, hub.y - cy)).toBeLessThan(span / 3)
  })

  it('reports which dagre orientation fits the canvas larger', () => {
    const g = buildView('model', fixtureTopology())
    const out = lay(g, 'dagre', 'bundle')
    expect(['tb', 'lr']).toContain(out.orientation)
    expect(out.routes.size).toBeGreaterThan(0)
  })
})

describe('compaction', () => {
  const cases: [ViewId, BoxBy][] = [['model', 'bundle'], ['model', 'route'], ['components', 'none'], ['infrastructure', 'node']]
  for (const [view, by] of cases) {
    for (const layout of ['cola', 'concentric'] as LayoutId[]) {
      it(`leaves no two boxes and no two marks intersecting: ${view} by ${by}, ${layout}`, () => {
        const g = buildView(view, fixtureTopology(), view === 'infrastructure' ? { expanded: new Set(['pods/agentops-conv-nightly-d2']) } : {})
        const { pos, boxes, groups } = lay(g, layout, by)
        const marks = [...pos.entries()]
        for (let i = 0; i < marks.length; i++) {
          for (let j = i + 1; j < marks.length; j++) {
            expect(overlap(mark(marks[i][1]), mark(marks[j][1])), `${marks[i][0]} / ${marks[j][0]}`).toBe(false)
          }
        }
        for (let i = 0; i < boxes.length; i++) {
          for (let j = i + 1; j < boxes.length; j++) {
            if (related(boxes[i], boxes[j], boxes)) continue
            expect(overlap(rect(boxes[i]), rect(boxes[j])), `${boxes[i].id} / ${boxes[j].id}`).toBe(false)
          }
        }
        // a mark in no box never sits inside one
        for (const b of boxes) {
          const members = new Set(allMembers(b, groups))
          for (const [id, p] of marks) {
            if (!members.has(id)) expect(overlap(mark(p), rect(b)), `${id} in ${b.id}`).toBe(false)
          }
        }
      })
    }
  }
})

describe('boxes', () => {
  it('boxes the Model by bundle, leaving shared substrate unboxed', () => {
    const g = buildView('model', fixtureTopology())
    const groups = groupsOf(g.nodes, 'model', 'bundle')
    const prom = groups.find((x) => x.id === 'box:bundle:prometheus')!
    expect(prom.members).toContain('pipelines/alert-triage')
    expect(groups.some((x) => x.members.includes('mcptoolsets/agentops-observe'))).toBe(false)
  })

  it('boxes the Model by route, holding what only one pipeline reaches', () => {
    const g = buildView('model', fixtureTopology())
    const groups = groupsOf(g.nodes, 'model', 'route', routeOwners(fixtureTopology()))
    const route = groups.find((x) => x.id === 'box:route:nightly-report')!
    expect(route.members).toContain('agentruntimes/sandbox')
    expect(groups.some((x) => x.members.includes('agentruntimes/default'))).toBe(false)
  })

  it('draws no box on Components', () => {
    const g = buildView('components', fixtureTopology())
    expect(groupsOf(g.nodes, 'components', 'bundle')).toEqual([])
  })

  it('keeps every external outside every cluster node box', () => {
    const g = buildView('infrastructure', fixtureTopology(), { expanded: new Set(['pods/agentops-conv-prometheus-alerts-a1']) })
    const outside = g.nodes.filter((n) => ['external', 'model', 'repository', 'mcp-server'].includes(n.cls))
    expect(outside.length).toBeGreaterThan(0)
    for (const layout of ['cola', 'concentric', 'dagre'] as LayoutId[]) {
      const { pos, boxes, groups } = lay(g, layout, 'node')
      expect(boxes.filter((b) => b.id.startsWith('box:node:')).length).toBe(2)
      for (const b of boxes) {
        const members = new Set(allMembers(b, groups))
        for (const n of outside) {
          expect(members.has(n.id)).toBe(false)
          if (layout !== 'dagre') expect(overlap(mark(pos.get(n.id)!), rect(b)), `${n.id} in ${b.id}`).toBe(false)
        }
      }
      // an opened pod is a box inside its node's box
      const podBox = boxes.find((b) => b.id === 'box:pod:agentops-conv-prometheus-alerts-a1')!
      expect(podBox.parent).toBe('box:node:node-b')
    }
  })
})

describe('marks and curves', () => {
  it('gives each Model class its own silhouette', () => {
    const shapes = new Set(['signalsources', 'pipelines', 'agentprofiles', 'agentruntimes', 'mcptoolsets', 'mcpconfigs', 'channels', 'conversations'].map((k) => styleFor(k).shape))
    expect(shapes.size).toBe(8)
    expect(styleFor('somethingelse').shape).toBe('rect')
    for (const s of Object.values(NODE_STYLES)) expect(shapePath(s.shape).startsWith('M')).toBe(true)
  })

  it('samples a curve from end to end by length', () => {
    const c = arcCurve({ x: 0, y: 0 }, { x: 300, y: 0 })
    expect(c.at(0).x).toBeCloseTo(38, 0)
    expect(c.at(1).x).toBeCloseTo(258, 0)
    const b = basisCurve({ x: 0, y: 0 }, { x: 0, y: 400 }, [{ x: 0, y: 30 }, { x: 60, y: 200 }, { x: 0, y: 370 }])
    expect(b.d.startsWith('M')).toBe(true)
    expect(b.at(1).y).toBeGreaterThan(b.at(0).y)
  })
})
