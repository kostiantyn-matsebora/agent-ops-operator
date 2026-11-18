import { afterEach, describe, expect, it, vi } from 'vitest'
import { act, renderHook } from '@testing-library/react'
import { curatedConversations as page } from '../test-fixtures/conversations'
import type { ConversationPage, ConversationSummary, VocabularyEntry } from '../api/types'
import {
  ARRIVAL_HOLD_MS, phaseDot, rowSnippet, rowTag, rowTitle, rowUnread, sameScope, scopeKey, scopeParams,
  selectionState, useArrivals, useConversationList, useInboxScopes, visibleItems, visibleRows, type Scope,
} from './conversationList'
import { buildTree } from './tree'

// The list model, pinned against the curated install the published screenshots
// show (screenshots/fixture.ts). A change to any of these answers changes what
// every look shows, so it changes here first, on purpose.

let served: ConversationPage | undefined = page
let counts: Partial<ConversationPage> | undefined
let vocabulary: VocabularyEntry[] = []

vi.mock('../api/hooks', () => ({
  useConversations: () => ({ data: served, isLoading: served === undefined, error: null }),
  useInboxCounts: () => ({ data: counts }),
  useVocabulary: () => ({ data: { entries: vocabulary } }),
}))

afterEach(() => {
  served = page
  counts = undefined
  vocabulary = []
})

const names = (rows: ConversationSummary[]) => rows.map((c) => c.name)
const ROOT = 'incident-response-4a8f21'
const TREE = [ROOT, `${ROOT}-triage`, `${ROOT}-mitigate`, `${ROOT}-mitigate-drain`, `${ROOT}-mitigate-verify`]

describe('a scope', () => {
  it('asks the server only what the server can answer per tree', () => {
    expect(scopeParams({ kind: 'all' }, '').toString()).toBe('limit=100')
    expect(scopeParams({ kind: 'closed' }, '').toString()).toBe('phase=Closed&limit=100')
    expect(scopeParams({ kind: 'pipeline', name: 'k8s-observe' }, 'oom').toString())
      .toBe('pipeline=k8s-observe&unread=true&q=oom&limit=100')
    // Working, Mine, Errored and a Coordinator are narrowed client-side, per tree.
    for (const kind of ['working', 'mine', 'errored'] as const) {
      expect(scopeParams({ kind }, '').toString()).toBe('limit=100')
    }
    expect(scopeParams({ kind: 'coordinator', name: 'incident-response' }, '').toString()).toBe('limit=100')
  })

  it('is keyed by kind and name', () => {
    expect(scopeKey({ kind: 'all' })).toBe('all')
    expect(scopeKey({ kind: 'coordinator', name: 'x' })).toBe('coordinator:x')
    expect(sameScope({ kind: 'pipeline', name: 'x' }, { kind: 'pipeline', name: 'x' })).toBe(true)
    expect(sameScope({ kind: 'pipeline', name: 'x' }, { kind: 'coordinator', name: 'x' })).toBe(false)
  })
})

describe('which conversations a scope shows, in the server’s order', () => {
  it('All is every open conversation, read or not', () => {
    expect(names(visibleItems(page.items, { kind: 'all' }, false))).toEqual([
      'cluster-events-7c1d4e', 'console-3f9a2b', 'prometheus-alerts-91b7fd', 'console-b48e10', 'nightly-2f60c8',
      'console-5a81cc', ...TREE,
    ])
  })

  it('Show closed adds the closed ones in place', () => {
    expect(names(visibleItems(page.items, { kind: 'all' }, true))).toEqual([
      'cluster-events-7c1d4e', 'console-3f9a2b', 'prometheus-alerts-91b7fd', 'console-b48e10', 'nightly-2f60c8',
      'cluster-events-d902a3', 'console-5a81cc', 'node-pressure-c81b4f', ROOT, `${ROOT}-triage`,
      `${ROOT}-mitigate`, `${ROOT}-notify`, `${ROOT}-mitigate-drain`, `${ROOT}-mitigate-verify`,
    ])
  })

  it('Working keeps every tree with an unread working row, whole — and not a read working one', () => {
    const rows = names(visibleItems(page.items, { kind: 'working' }, false))
    expect(rows).toEqual(['cluster-events-7c1d4e', 'console-3f9a2b', ...TREE])
    // console-5a81cc is working but read: Working is unread AND working.
    expect(rows).not.toContain('console-5a81cc')
  })

  it('a Coordinator scope is its root and every member, at any depth', () => {
    expect(names(visibleItems(page.items, { kind: 'coordinator', name: 'incident-response' }, false))).toEqual(TREE)
  })

  it('Mine and Errored are empty on an install where nothing qualifies — an empty list, not an error', () => {
    expect(visibleItems(page.items, { kind: 'mine' }, true)).toEqual([])
    expect(visibleItems(page.items, { kind: 'errored' }, true)).toEqual([])
  })

  it('Closed shows what the server narrowed, closed rows included regardless of the toggle', () => {
    expect(visibleItems(page.items, { kind: 'closed' }, false)).toHaveLength(page.items.length)
  })

  it('a sparse install shows nothing, and says nothing is selectable', () => {
    expect(visibleItems([], { kind: 'all' }, false)).toEqual([])
    expect(selectionState([], new Set())).toEqual({ selectableRows: [], allSelected: false })
  })
})

describe('ordering and folding', () => {
  it('nests each member depth-first under its own root', () => {
    const tree = buildTree(visibleItems(page.items, { kind: 'all' }, false), true)
    expect(tree.map((r) => [r.row.name, r.depth, r.memberCount])).toEqual([
      ['cluster-events-7c1d4e', 0, 0],
      ['console-3f9a2b', 0, 0],
      ['prometheus-alerts-91b7fd', 0, 0],
      ['console-b48e10', 0, 0],
      ['nightly-2f60c8', 0, 0],
      ['console-5a81cc', 0, 0],
      [ROOT, 0, 4],
      [`${ROOT}-triage`, 1, 0],
      [`${ROOT}-mitigate`, 1, 2],
      [`${ROOT}-mitigate-drain`, 2, 0],
      [`${ROOT}-mitigate-verify`, 2, 0],
    ])
  })

  it('a folded root hides every descendant, the nested root’s own members included', () => {
    const items = visibleItems(page.items, { kind: 'all' }, false)
    const rows = visibleRows(buildTree(items, true), items, new Set([ROOT]))
    expect(rows.map((r) => r.row.name)).not.toContain(`${ROOT}-mitigate-drain`)
    expect(rows.map((r) => r.row.name)).toContain(ROOT)
    const inner = visibleRows(buildTree(items, true), items, new Set([`${ROOT}-mitigate`]))
    expect(inner.map((r) => r.row.name)).toEqual(expect.arrayContaining([ROOT, `${ROOT}-triage`, `${ROOT}-mitigate`]))
    expect(inner.map((r) => r.row.name)).not.toContain(`${ROOT}-mitigate-verify`)
  })
})

describe('what each row says about itself', () => {
  it('pins title, unread, status, tag and snippet for every curated row', () => {
    const facts = page.items.map((r) => [r.name, rowTitle(r), rowUnread(r), phaseDot(r), rowTag(r, false), rowSnippet(r)])
    expect(facts).toEqual([
      ['cluster-events-7c1d4e', 'checkout-api is restarting', true, 'working', undefined,
        'working · ops-chat/dana: Does the same setting affect the other two services in that namespace?'],
      ['console-3f9a2b', 'Why is the payments deployment not rolling out?', true, 'working', undefined,
        'working · Checking the rollout status for the payments deployment…'],
      ['prometheus-alerts-91b7fd', 'HighMemoryPressure on node-3', false, 'pending', undefined, 'pending · waiting for a slot'],
      ['console-b48e10', 'Which namespaces have no resource quota?', false, 'idle', undefined, ''],
      ['nightly-2f60c8', 'Nightly capacity report', false, 'idle', undefined, ''],
      ['cluster-events-d902a3', 'ingress-nginx admission webhook timed out', false, 'none', 'closed', 'closed · 4 run(s)'],
      ['console-5a81cc', 'Is the ingress controller logging TLS handshake failures?', false, 'working', undefined,
        'working · Checked the ingress logs — no TLS handshake failures in the last hour.'],
      ['node-pressure-c81b4f', 'MemoryPressure on node-7', false, 'none', 'nobody notified',
        'closed · evicted 2 pods, pressure cleared on its own — no mitigation needed'],
      [ROOT, 'Elevated 5xx rate on checkout-api', true, 'working', undefined, 'the agent is working…'],
      [`${ROOT}-triage`, 'Triage: checkout-api restart loop', false, 'idle', undefined, ''],
      [`${ROOT}-mitigate`, 'Mitigate: roll checkout-api off node-7', false, 'idle', undefined, ''],
      [`${ROOT}-notify`, 'Notify: ops-chat summary', false, 'none', 'closed', 'closed · handed back to the root after posting the summary'],
      [`${ROOT}-mitigate-drain`, 'Drain node-7', false, 'idle', undefined, ''],
      [`${ROOT}-mitigate-verify`, 'Verify checkout-api on node-9', false, 'idle', undefined, ''],
    ])
  })

  it('an arrival is tagged new before anything else', () => {
    expect(rowTag(page.items[5], true)).toBe('new')
  })

  it('a title drops the lane glyph and the prefix its own chip already shows', () => {
    const member = { ...page.items[9], title: '🤝 triage: Triage: checkout-api restart loop' }
    expect(rowTitle(member)).toBe('Triage: checkout-api restart loop')
    const plain = { ...page.items[3], title: 'esphome: a prefix nobody shows stays' }
    expect(rowTitle(plain)).toBe('esphome: a prefix nobody shows stays')
  })
})

describe('useConversationList', () => {
  const base = { search: '', showClosed: false, flatten: false, collapsedRoots: new Set<string>() }

  it('returns the scope’s items, its tree and the roots that fold', () => {
    const { result } = renderHook(() => useConversationList({ ...base, scope: { kind: 'all' } }))
    expect(result.current.rows).toHaveLength(11)
    expect(result.current.collapsibleNames).toEqual([ROOT, `${ROOT}-mitigate`])
  })

  it('flattened, every row is its own line in the server’s order', () => {
    const { result } = renderHook(() => useConversationList({ ...base, flatten: true, scope: { kind: 'all' } }))
    expect(result.current.rows.every((r) => r.depth === 0)).toBe(true)
    expect(result.current.rows.map((r) => r.row.name)).toEqual(names(visibleItems(page.items, { kind: 'all' }, false)))
  })

  it('while loading, it is empty and says so — never a stale or invented row', () => {
    served = undefined
    const { result } = renderHook(() => useConversationList({ ...base, scope: { kind: 'all' } }))
    expect(result.current.isLoading).toBe(true)
    expect(result.current.items).toEqual([])
    expect(result.current.rows).toEqual([])
  })
})

describe('useInboxScopes', () => {
  it('counts All by every conversation and every other scope by unread', () => {
    counts = { rootTotal: 14, scopes: { working: 2, mine: 0, errored: 1, closed: 3, 'k8s-observe': 2 } }
    vocabulary = [
      { kind: 'pipeline', name: 'k8s-observe', position: 'general', icon: 'aops:k8s' },
      { kind: 'coordinator', name: 'incident-response', position: 'general' },
      { kind: 'builtin', name: 'exit', position: 'thread' },
    ]
    const { result } = renderHook(() => useInboxScopes(false))
    expect(result.current.fixed.map((f) => [f.label, f.count])).toEqual([
      ['All', 14], ['Working', 2], ['Mine', 0], ['Errored', 1],
    ])
    expect(result.current.targets).toEqual([
      { scope: { kind: 'pipeline', name: 'k8s-observe' }, label: 'k8s-observe', count: 2, icon: 'aops:k8s' },
      { scope: { kind: 'coordinator', name: 'incident-response' }, label: 'incident-response', count: undefined, icon: undefined },
    ])
    expect(result.current.closed).toEqual({ scope: { kind: 'closed' }, label: 'Archive', count: 3 })
    expect(result.current.general.scope).toEqual({ kind: 'general' })
  })

  it('before the counts arrive, every scope is listed with no count', () => {
    const { result } = renderHook(() => useInboxScopes(true))
    expect(result.current.fixed.map((f) => f.count)).toEqual([undefined, undefined, undefined, undefined])
    expect(result.current.targets).toEqual([])
  })
})

describe('useArrivals', () => {
  afterEach(() => vi.useRealTimers())

  it('announces nothing on a scope’s first snapshot, then names what arrived, for a while', () => {
    vi.useFakeTimers()
    const first = { items: page.items.slice(1) }
    const { result, rerender } = renderHook(({ data, key }) => useArrivals(data, key), {
      initialProps: { data: first, key: 'all' },
    })
    expect(result.current.newNames.size).toBe(0)
    rerender({ data: { items: page.items }, key: 'all' })
    expect([...result.current.newNames]).toEqual(['cluster-events-7c1d4e'])
    act(() => vi.advanceTimersByTime(ARRIVAL_HOLD_MS))
    expect(result.current.newNames.size).toBe(0)
  })

  it('switching scope is not an arrival, and opening a row clears it at once', () => {
    const scope: Scope = { kind: 'working' }
    const { result, rerender } = renderHook(({ data, key }) => useArrivals(data, key), {
      initialProps: { data: { items: page.items.slice(0, 2) }, key: scopeKey(scope) },
    })
    rerender({ data: { items: page.items }, key: 'all' })
    expect(result.current.newNames.size).toBe(0)
    rerender({ data: { items: [page.items[13], ...page.items] }, key: 'all' })
    rerender({ data: { items: [{ ...page.items[0], name: 'fresh' }, ...page.items] }, key: 'all' })
    expect(result.current.newNames.has('fresh')).toBe(true)
    act(() => result.current.clear('fresh'))
    expect(result.current.newNames.has('fresh')).toBe(false)
  })
})
