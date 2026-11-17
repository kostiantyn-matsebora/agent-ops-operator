import { describe, expect, it } from 'vitest'
import type { ActivityEvent } from '../api/types'
import { sequenceRows } from './sequence'

const ev = (cursor: string, ts: string, over: Partial<ActivityEvent> = {}): ActivityEvent => ({ cursor, ts, kind: 'dispatch', status: 'ok', ...over })

describe('sequenceRows', () => {
  it('is empty when no hop was recorded', () => {
    expect(sequenceRows([])).toEqual([])
  })

  it('offsets every hop from the first and names the route', () => {
    const rows = sequenceRows([
      ev('1', '2025-06-10T22:00:00Z', { from: { kind: 'SignalSource', name: 'cron' }, to: { kind: 'Pipeline', name: 'ops' } }),
      ev('2', '2025-06-10T22:00:01.500Z', { latencyMs: 2340, status: 'error' }),
    ])
    expect(rows[0]).toMatchObject({ route: 'SignalSource/cron → Pipeline/ops', at: '+0.0s', latency: '—', errored: false })
    expect(rows[1]).toMatchObject({ route: '∅ → ∅', at: '+1.5s', latency: '2.34s', errored: true })
  })

  it('measures from the earliest hop even when they arrive out of order', () => {
    const rows = sequenceRows([ev('2', '2025-06-10T22:00:05Z'), ev('1', '2025-06-10T22:00:00Z')])
    expect(rows.map((r) => r.at)).toEqual(['+5.0s', '+0.0s'])
  })
})
