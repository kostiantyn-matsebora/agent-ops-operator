import { describe, expect, it } from 'vitest'
import type { GraphNode } from '../api/types'
import { layout } from './layout'
import { NODE_STYLES, shapePath, styleFor } from './shapes'

function node(kind: string, name: string, extra: Partial<GraphNode> = {}): GraphNode {
  return { id: `${kind}/${name}`, kind, name, health: 'ok', active: 0, recent: 0, ...extra }
}

describe('shapes', () => {
  it('gives each element class its own silhouette', () => {
    // The point of the shapes is that classes are distinguishable BEFORE you
    // read a label; two kinds sharing one shape would defeat that.
    const shapes = new Set(Object.values(NODE_STYLES).map((s) => s.shape))
    expect(shapes.size).toBeGreaterThanOrEqual(6)
    expect(styleFor('signalsources').shape).not.toBe(styleFor('channels').shape)
    expect(styleFor('pipelines').shape).not.toBe(styleFor('mcptoolsets').shape)
  })

  it('falls back rather than throwing for an unknown kind', () => {
    expect(styleFor('somethingelse').shape).toBe('rect')
  })

  it('draws every shape as a closed path in the same footprint', () => {
    for (const s of new Set(Object.values(NODE_STYLES).map((v) => v.shape))) {
      const d = shapePath(s)
      expect(d.startsWith('M')).toBe(true)
      expect(d.trim().endsWith('Z')).toBe(true)
    }
  })
})

describe('layout', () => {
  const nodes = [
    node('signalsources', 'events'),
    node('signaladapters', 'k8s-events'),
    node('pipelines', 'ops'),
    node('agentprofiles', 'engineer'),
    node('agentruntimes', 'default'),
    node('channels', 'console'),
  ]

  it('groups nodes into labelled lanes, left to right', () => {
    const l = layout(nodes)
    expect(l.lanes.map((x) => x.id)).toEqual(['ingest', 'wiring', 'execution', 'delivery'])
    // lanes advance rightwards and never overlap
    for (let i = 1; i < l.lanes.length; i++) {
      expect(l.lanes[i].x).toBeGreaterThan(l.lanes[i - 1].x + l.lanes[i - 1].width - 1)
    }
  })

  it('drops empty lanes rather than drawing them', () => {
    // an empty labelled box reads as "this is broken" when usually it just means
    // the Display panel folded that class away
    const l = layout([node('pipelines', 'ops')])
    expect(l.lanes.map((x) => x.id)).toEqual(['wiring'])
  })

  it('puts every node inside its own lane box', () => {
    const l = layout(nodes)
    const laneById = new Map(l.lanes.map((x) => [x.id, x]))
    for (const n of l.nodes) {
      const lane = laneById.get(styleFor(n.kind).lane)!
      expect(n.x).toBeGreaterThanOrEqual(lane.x)
      expect(n.x).toBeLessThan(lane.x + lane.width)
      expect(n.y).toBeGreaterThanOrEqual(lane.y)
      expect(n.y).toBeLessThan(lane.y + lane.height)
    }
  })

  it('sinks detached nodes to the bottom of their lane', () => {
    const l = layout([
      node('signalsources', 'orphan', { detached: true }),
      node('signalsources', 'events'),
    ])
    const orphan = l.nodes.find((n) => n.name === 'orphan')!
    const events = l.nodes.find((n) => n.name === 'events')!
    expect(orphan.y).toBeGreaterThan(events.y)
  })

  it('gives every lane the same height so the columns line up', () => {
    const l = layout([
      node('signalsources', 'a'),
      node('signalsources', 'b'),
      node('signalsources', 'c'),
      node('pipelines', 'one'),
    ])
    expect(new Set(l.lanes.map((x) => x.height)).size).toBe(1)
    expect(l.height).toBe(l.lanes[0].height)
  })
})
