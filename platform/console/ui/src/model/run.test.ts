import { afterEach, describe, expect, it, vi } from 'vitest'
import { act, renderHook } from '@testing-library/react'
import { curatedDetail, curatedNow } from '../test-fixtures/conversations'
import type { ActivityEvent, ConversationSummary, Message, Run } from '../api/types'
import {
  ELAPSED_TICK_MS, attachRunDetails, groupRunEvents, runDuration, runEventLabel, runEventRows, runIdOfMessage,
  useWorkingRun, workingRun,
} from './run'

// Design D8: a working run is `{ elapsed, latest, count }`, and when the reply
// lands the run's tools and events attach to it. Pinned for a run the viewer
// started with a reply AND for one a signal started — a run is keyed by its
// id, never by what caused it.

const T0 = Date.parse('2026-01-01T10:00:00Z')
const at = (s: number) => new Date(T0 + s * 1000).toISOString()

function conv(over: Partial<ConversationSummary> = {}): ConversationSummary {
  return {
    name: 'c', runCount: 0, queued: 0, joined: true, errored: false, unread: false, ageSeconds: 0,
    deleting: false, phase: 'Idle', ...over,
  }
}

let cursor = 0
function ev(kind: string, s: number, runId?: string): ActivityEvent {
  cursor += 1
  return { cursor: String(1000 + cursor), ts: at(s), kind, status: 'ok', conversation: 'c', runId }
}
const msg = (id: string, kind: string, s: number, text = id): Message => ({ id, thread: 'c', kind, text, at: at(s) })

describe('a run the viewer started with a reply', () => {
  const question = msg('local:1', 'local', 0, 'is the pod healthy?')
  const dispatched = ev('run.dispatched', 1, 'r1')
  const starting = ev('runtime.starting', 3, 'r1')
  const completed = ev('run.completed', 9, 'r1')
  const run: Run = {
    runId: 'r1', status: 'succeeded', startedAt: at(1), finishedAt: at(9), result: 'healthy',
    toolCalls: [{ tool: 'Bash', server: '', durationMs: 120, resultBytes: 64 }],
  }

  it('while it works, it is one fact: elapsed, the latest event, and the count', () => {
    const working = conv({ presence: true, phase: 'Working', inflight: { runId: 'r1', dispatchedAt: at(1) } })
    expect(workingRun(working, [dispatched, starting], T0 + 6000)).toEqual({
      runId: 'r1',
      elapsed: 5000,
      latest: { kind: 'runtime.starting', label: 'runtime starting', at: at(3) },
      count: 2,
    })
    // Nothing is attached anywhere yet: there is no reply to attach to.
    expect(attachRunDetails([question], [], [dispatched, starting]).size).toBe(0)
  })

  it('when the reply lands, its tools and events attach to that reply and the working run is gone', () => {
    const reply = msg('send:c:console:r1', 'agent', 9, 'healthy')
    const details = attachRunDetails([question, reply], [run], [dispatched, starting, completed])
    expect([...details.keys()]).toEqual([reply.id])
    expect(details.get(reply.id)).toEqual({
      runId: 'r1', run, tools: run.toolCalls, events: [dispatched, starting, completed],
    })
    expect(workingRun(conv({ phase: 'Idle' }), [dispatched, starting, completed], T0 + 10_000)).toBeUndefined()
  })
})

describe('a run a signal started', () => {
  const signal = msg('input:c:sig-1:console', 'signal', 0, 'disk pressure on node-3')
  const dispatched = ev('run.dispatched', 1, 'r2')
  const restored = ev('context.restored', 2, 'r2')

  it('works the same way: one fact while working', () => {
    const working = conv({ presence: true, inflight: { runId: 'r2', dispatchedAt: at(1) } })
    const w = workingRun(working, [dispatched, restored], T0 + 31_000)
    expect(w).toMatchObject({ runId: 'r2', elapsed: 30_000, count: 2, latest: { label: 'context restored' } })
    expect(attachRunDetails([signal], undefined, [dispatched, restored]).size).toBe(0)
  })

  it('a reply rebuilt from the run record carries the run in its id', () => {
    const reply = msg('run:r2', 'agent', 40, 'evicted two pods')
    const completed = ev('run.completed', 39, 'r2')
    const run: Run = { runId: 'r2', status: 'succeeded', startedAt: at(1), finishedAt: at(39), result: 'evicted two pods' }
    const details = attachRunDetails([signal, reply], [run], [dispatched, restored, completed])
    expect(details.get('run:r2')).toEqual({ runId: 'r2', run, tools: [], events: [dispatched, restored, completed] })
  })

  it('the curated install: run-3 answered with m2, matched by time, and run-4 is working with no event yet', () => {
    const details = attachRunDetails(curatedDetail.transcript ?? [], curatedDetail.conversation.runs, curatedDetail.events ?? [])
    expect([...details.keys()]).toEqual(['m2'])
    expect(details.get('m2')?.runId).toBe('run-3')
    expect(details.get('m2')?.events.map((e) => e.kind)).toEqual(['run.dispatched', 'run.completed'])
    expect(details.get('m2')?.tools).toEqual([])
    expect(workingRun(curatedDetail.conversation, curatedDetail.events ?? [], curatedNow)).toEqual({
      runId: 'run-4', elapsed: 42_000, latest: undefined, count: 0,
    })
  })
})

describe('matching replies to runs', () => {
  it('reads the run id a message id carries, and nothing else', () => {
    expect(runIdOfMessage('run:r7')).toBe('r7')
    expect(runIdOfMessage('send:conv:console:r7')).toBe('r7')
    expect(runIdOfMessage('send:conv:r7')).toBeUndefined()
    expect(runIdOfMessage('m2')).toBeUndefined()
    expect(runIdOfMessage('run:')).toBeUndefined()
  })

  it('two serial runs hand each reply to the run that began before it, never one reply to both', () => {
    const a = [ev('run.dispatched', 0, 'ra'), ev('run.completed', 5, 'ra')]
    const b = [ev('run.dispatched', 10, 'rb'), ev('run.completed', 15, 'rb')]
    const replies = [msg('m-a', 'agent', 5), msg('m-b', 'agent', 15)]
    const details = attachRunDetails(replies, undefined, [...a, ...b])
    expect(details.get('m-a')?.runId).toBe('ra')
    expect(details.get('m-b')?.runId).toBe('rb')
  })

  it('a reply only a person wrote is never claimed by a run', () => {
    const details = attachRunDetails([msg('relay-1', 'relay', 5)], undefined, [ev('run.dispatched', 0, 'r')])
    expect(details.size).toBe(0)
  })
})

describe('grouping lifecycle events', () => {
  it('keeps only lifecycle kinds, one group per run, an id-less event on its own', () => {
    const events = [ev('signal.claimed', 0), ev('run.dispatched', 1, 'x'), ev('run.completed', 2, 'x'), ev('context.failed', 3)]
    const groups = groupRunEvents(events)
    expect(groups.map((g) => [g.runId, g.events.map((e) => e.kind)])).toEqual([
      ['x', ['run.dispatched', 'run.completed']],
      ['', ['context.failed']],
    ])
  })

  it('labels a known kind and passes an unknown one through — never a prototype key', () => {
    expect(runEventLabel('context.checkpoint')).toBe('context checkpoint')
    expect(runEventLabel('toString')).toBe('toString')
    expect(groupRunEvents([ev('toString', 0, 'x')])).toEqual([])
  })

  it('a working run with no id named is the newest group not yet completed', () => {
    const events = [ev('run.dispatched', 0, 'old'), ev('run.completed', 1, 'old'), ev('run.dispatched', 2, 'new')]
    expect(workingRun(conv({ presence: true }), events, T0 + 4000)).toMatchObject({ runId: 'new', elapsed: 2000, count: 1 })
  })

  it('a working run nothing dates has no elapsed time rather than an invented one', () => {
    expect(workingRun(conv({ presence: true }), [], T0)).toEqual({ runId: undefined, elapsed: 0, latest: undefined, count: 0 })
  })
})

describe('useWorkingRun', () => {
  afterEach(() => vi.useRealTimers())

  it('advances elapsed while working and stops once the run ends', () => {
    vi.useFakeTimers()
    vi.setSystemTime(T0 + 5000)
    const working = conv({ presence: true, inflight: { runId: 'r', dispatchedAt: at(0) } })
    const { result, rerender } = renderHook(({ c }) => useWorkingRun(c, []), { initialProps: { c: working } })
    expect(result.current?.elapsed).toBe(5000)
    act(() => vi.advanceTimersByTime(ELAPSED_TICK_MS))
    expect(result.current?.elapsed).toBe(6000)
    act(() => vi.advanceTimersByTime(ELAPSED_TICK_MS))
    expect(result.current?.elapsed).toBe(7000)
    rerender({ c: conv({ phase: 'Idle' }) })
    expect(result.current).toBeUndefined()
    expect(vi.getTimerCount()).toBe(0)
  })
})

describe('a finished run, as rows', () => {
  const ev = (kind: string, ts: string) => ({ cursor: ts, ts, kind, runId: 'r', status: 'ok' }) as ActivityEvent

  it('takes its duration from the run record, else from its events, else says nothing', () => {
    expect(runDuration({ run: { runId: 'r', status: 'ok', startedAt: '2024-01-01T00:00:00Z', finishedAt: '2024-01-01T00:00:25Z' }, events: [] })).toBe(25_000)
    expect(runDuration({ events: [ev('run.dispatched', '2024-01-01T00:00:00Z'), ev('run.completed', '2024-01-01T00:00:09Z')] })).toBe(9_000)
    expect(runDuration({ events: [ev('run.dispatched', '2024-01-01T00:00:00Z')] })).toBeUndefined()
  })

  it('labels its events in order', () => {
    expect(runEventRows({ events: [ev('run.dispatched', 'a'), ev('context.checkpoint', 'b')] })).toEqual([
      { kind: 'run.dispatched', label: 'run dispatched', at: 'a' },
      { kind: 'context.checkpoint', label: 'context checkpoint', at: 'b' },
    ])
  })
})
