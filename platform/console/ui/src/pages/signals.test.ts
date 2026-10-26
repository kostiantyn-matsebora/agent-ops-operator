import { describe, expect, it } from 'vitest'
import {
  FILTER_ALL, asSignalStatus, claimantStateStatus, filterSignalRows, isSignalTrouble, signalFilterOptions, signalPhaseStatus,
} from './signals'
import type { InventoryRow } from '../api/types'

function row(source: string, phase: string, over: Partial<InventoryRow> = {}): InventoryRow {
  return {
    name: `${source}-${phase}`, health: 'none', findings: 0,
    columns: { source, phase },
    ...over,
  }
}

describe('signalFilterOptions', () => {
  it('lists the distinct sources and phases, sorted, de-duplicated', () => {
    const rows = [
      row('cluster-events', 'Routed'),
      row('cluster-events', 'Pending'),
      row('prometheus-alerts', 'Routed'),
    ]
    expect(signalFilterOptions(rows)).toEqual({
      sources: ['cluster-events', 'prometheus-alerts'],
      phases: ['Pending', 'Routed'],
    })
  })

  it('skips a row with no column at all rather than filtering on an empty string', () => {
    const rows = [{ name: 'x', health: 'none' as const, findings: 0 }]
    expect(signalFilterOptions(rows)).toEqual({ sources: [], phases: [] })
  })
})

describe('filterSignalRows', () => {
  const rows = [
    row('cluster-events', 'Routed'),
    row('cluster-events', 'PartiallyRouted'),
    row('prometheus-alerts', 'Routed'),
  ]

  it("'all'/'all' is a no-op", () => {
    expect(filterSignalRows(rows, FILTER_ALL, FILTER_ALL)).toHaveLength(3)
  })

  it('filters by source alone', () => {
    const got = filterSignalRows(rows, 'cluster-events', FILTER_ALL)
    expect(got.map((r) => r.name)).toEqual(['cluster-events-Routed', 'cluster-events-PartiallyRouted'])
  })

  it('filters by phase alone — a PartiallyRouted signal is still findable beside its routed peers', () => {
    const got = filterSignalRows(rows, FILTER_ALL, 'PartiallyRouted')
    expect(got.map((r) => r.name)).toEqual(['cluster-events-PartiallyRouted'])
  })

  it('combines both filters', () => {
    const got = filterSignalRows(rows, 'prometheus-alerts', 'PartiallyRouted')
    expect(got).toHaveLength(0)
  })
})

describe('claimantStateStatus', () => {
  it('colours each claimant state, an unknown one as still routing', () => {
    expect(claimantStateStatus('Routed')).toBe('success')
    expect(claimantStateStatus('Failed')).toBe('danger')
    expect(claimantStateStatus('Skipped')).toBe('warning')
    expect(claimantStateStatus('Routing')).toBe('info')
    expect(claimantStateStatus(undefined)).toBe('info')
  })
})

describe('signalPhaseStatus', () => {
  it('maps each phase to the chip colour the review bar expects', () => {
    expect(signalPhaseStatus('Routed')).toBe('success')
    expect(signalPhaseStatus('PartiallyRouted')).toBe('warning')
    expect(signalPhaseStatus('Pending')).toBe('info')
    expect(signalPhaseStatus('Dropped')).toBe('danger')
    expect(signalPhaseStatus('Failed')).toBe('danger')
  })

  it('is undefined for an unrelated or absent phase, so a Conversation cell keeps its own rendering', () => {
    expect(signalPhaseStatus('Working')).toBeUndefined()
    expect(signalPhaseStatus(undefined)).toBeUndefined()
  })
})

describe('isSignalTrouble', () => {
  it('is true only for Dropped and Failed', () => {
    expect(isSignalTrouble('Dropped')).toBe(true)
    expect(isSignalTrouble('Failed')).toBe(true)
    expect(isSignalTrouble('PartiallyRouted')).toBe(false)
    expect(isSignalTrouble(undefined)).toBe(false)
  })
})

describe('asSignalStatus', () => {
  it('reads phase/reason/claimants/firstAttemptAt off the generic status value', () => {
    const status = {
      phase: 'PartiallyRouted',
      claimants: [{ ref: { kind: 'Pipeline', name: 'k8s-observe' }, state: 'Routed' }],
      firstAttemptAt: '2026-01-01T00:00:00Z',
    }
    expect(asSignalStatus(status)).toEqual(status)
  })

  it('degrades to an empty record rather than throwing on a missing or malformed status', () => {
    expect(asSignalStatus(undefined)).toEqual({})
    expect(asSignalStatus(null)).toEqual({})
    expect(asSignalStatus('not an object')).toEqual({})
    // claimants present but not an array — dropped, not trusted as-is.
    expect(asSignalStatus({ phase: 'Pending', claimants: 'nope' })).toEqual({ phase: 'Pending', claimants: undefined })
  })
})
