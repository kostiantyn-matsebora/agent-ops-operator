import { describe, expect, it } from 'vitest'
import { buildTree, descendantsOf, partitionSelection, rootNameOf } from './tree'
import type { ConversationSummary } from '../../api/types'

function conv(name: string, over: Partial<ConversationSummary> = {}): ConversationSummary {
  return {
    name, runCount: 0, queued: 0, joined: true, errored: false, unread: false,
    ageSeconds: 1, deleting: false, phase: 'Idle', ...over,
  }
}

describe('buildTree — one level', () => {
  const items = [
    conv('root-1', { coordinator: 'root-1' }),
    conv('member-1', { causedBy: { parent: 'root-1', entry: 'triage' } }),
    conv('member-2', { causedBy: { parent: 'root-1', entry: 'logs' } }),
    conv('plain'),
  ]

  it('nests both members under the root when grouped', () => {
    const rows = buildTree(items, true)
    const byName = new Map(rows.map((r) => [r.row.name, r]))
    expect(byName.get('root-1')?.depth).toBe(0)
    expect(byName.get('member-1')?.depth).toBe(1)
    expect(byName.get('member-2')?.depth).toBe(1)
    expect(byName.get('member-1')?.rootName).toBe('root-1')
    expect(byName.get('plain')?.depth).toBe(0)
  })

  it('counts both members on the root', () => {
    const rows = buildTree(items, true)
    expect(rows.find((r) => r.row.name === 'root-1')?.memberCount).toBe(2)
    expect(rows.find((r) => r.row.name === 'plain')?.memberCount).toBe(0)
  })

  it('flattened, every row sits at depth 0 in the original order', () => {
    const rows = buildTree(items, false)
    expect(rows.map((r) => r.depth)).toEqual([0, 0, 0, 0])
    expect(rows.map((r) => r.row.name)).toEqual(['root-1', 'member-1', 'member-2', 'plain'])
  })
})

describe('buildTree — two levels', () => {
  const items = [
    conv('root-1', { coordinator: 'root-1' }),
    conv('mid-1', { causedBy: { parent: 'root-1', entry: 'diagnose' }, coordinator: 'mid-1' }),
    conv('leaf-1', { causedBy: { parent: 'mid-1', entry: 'logs' } }),
    conv('leaf-2', { causedBy: { parent: 'mid-1', entry: 'metrics' } }),
  ]

  it('nests a coordinating member one level deeper than its own members', () => {
    const rows = buildTree(items, true)
    const byName = new Map(rows.map((r) => [r.row.name, r]))
    expect(byName.get('mid-1')?.depth).toBe(1)
    expect(byName.get('leaf-1')?.depth).toBe(2)
    expect(byName.get('leaf-2')?.depth).toBe(2)
    expect(byName.get('leaf-1')?.rootName).toBe('root-1')
  })

  it('counts every descendant at any depth on the uncaused root', () => {
    const rows = buildTree(items, true)
    expect(rows.find((r) => r.row.name === 'root-1')?.memberCount).toBe(3)
    expect(rows.find((r) => r.row.name === 'mid-1')?.memberCount).toBe(2)
  })
})

describe('a missing parent', () => {
  it('sits at the top level with a marker, never dropped', () => {
    const items = [conv('member-1', { causedBy: { parent: 'not-on-this-page', entry: 'triage' } })]
    const rows = buildTree(items, true)
    expect(rows).toHaveLength(1)
    expect(rows[0].depth).toBe(0)
    expect(rows[0].parentMissing).toBe(true)
  })
})

describe('rootNameOf', () => {
  const items = [
    conv('root-1'),
    conv('mid-1', { causedBy: { parent: 'root-1', entry: 'diagnose' } }),
    conv('leaf-1', { causedBy: { parent: 'mid-1', entry: 'logs' } }),
  ]

  it('walks the chain to the uncaused root', () => {
    expect(rootNameOf(items, items[2])).toBe('root-1')
  })

  it('a conversation with no causedBy is its own root', () => {
    expect(rootNameOf(items, items[0])).toBe('root-1')
  })
})

describe('partitionSelection', () => {
  const items = [
    conv('root-1', { coordinator: 'root-1' }),
    conv('member-1', { causedBy: { parent: 'root-1', entry: 'diagnose' } }),
    conv('plain'),
  ]

  it('sends an ordinary conversation straight through', () => {
    const { send, skipped } = partitionSelection(items, new Set(['plain']))
    expect(send).toEqual(['plain'])
    expect(skipped).toEqual([])
  })

  it('skips a member reached directly, naming its parent', () => {
    const { send, skipped } = partitionSelection(items, new Set(['member-1']))
    expect(send).toEqual([])
    expect(skipped).toEqual([{ name: 'member-1', parentName: 'root-1' }])
  })

  it('drops a member silently when its root is also selected — the cascade already reaches it', () => {
    const { send, skipped } = partitionSelection(items, new Set(['root-1', 'member-1']))
    expect(send).toEqual(['root-1'])
    expect(skipped).toEqual([])
  })
})

describe('descendantsOf', () => {
  it('returns every nested descendant, not only direct children', () => {
    const items = [
      conv('root-1'),
      conv('mid-1', { causedBy: { parent: 'root-1', entry: 'diagnose' } }),
      conv('leaf-1', { causedBy: { parent: 'mid-1', entry: 'logs' } }),
    ]
    expect(descendantsOf(items, 'root-1').map((c) => c.name).sort()).toEqual(['leaf-1', 'mid-1'])
  })
})
