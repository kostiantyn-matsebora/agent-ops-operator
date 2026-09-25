import { describe, expect, it } from 'vitest'
import type { ActivityEvent } from '../api/types'
import { CLAUDE_IMAGE, fixtureTopology, hop } from '../test-fixtures/topology'
import { hopIndex } from './hops'
import {
  FRAME_MS, activeConversations, conversationRoute, describeNotHeld, frameHeld, frameHops, frameTime,
  latestRun, nextFrame, playDelay, replayWindow, steps, SPEEDS,
} from './replay'
import { buildView } from './views'

const NOW = Date.UTC(2026, 8, 23, 12, 0, 0)
const at = (ms: number, e: ActivityEvent): ActivityEvent => ({ ...e, ts: new Date(ms).toISOString() })

describe('window replay', () => {
  it('divides the interval into ten second frames', () => {
    const w = replayWindow(NOW, 300, NOW - 3_600_000)
    expect(w.frames).toBe(30)
    expect(frameTime(w, 0)).toBe(NOW - 300_000)
    expect(frameTime(w, 30)).toBe(NOW)
    expect(frameTime(w, 99)).toBe(NOW)
    expect(w.notHeldMs).toBe(0)
  })

  it('pulses the hops of the frame and nothing else', () => {
    const w = replayWindow(NOW, 60, NOW - 3_600_000)
    const t = frameTime(w, 3)
    const evs = [at(t - 1, hop('a', null, null)), at(t - FRAME_MS - 1, hop('b', null, null)), at(t + 1, hop('c', null, null))]
    expect(frameHops(evs, t).map((e) => e.kind)).toEqual(['a'])
  })

  it('advances frame by frame and stops at the end', () => {
    const w = replayWindow(NOW, 60, NOW - 3_600_000)
    expect(nextFrame(w, 0)).toBe(1)
    expect(nextFrame(w, w.frames)).toBeNull()
  })

  it('advances once a second at the fastest speed', () => {
    expect(SPEEDS.map((s) => s.label)).toEqual(['slow', 'medium', 'fast'])
    expect(Math.min(...SPEEDS.map((s) => s.ms))).toBe(1000)
  })

  it('admits the buffer\'s edge rather than showing it as quiet', () => {
    // a thirty minute interval over a buffer holding twelve
    const w = replayWindow(NOW, 1800, NOW - 12 * 60_000)
    expect(describeNotHeld(w.notHeldMs)).toBe('the first 18 minutes')
    expect(w.firstHeldFrame).toBe(108)
    expect(frameHeld(w, 107)).toBe(false)
    expect(frameHeld(w, 108)).toBe(true)
  })

  it('holds nothing when the buffer holds nothing', () => {
    const w = replayWindow(NOW, 60)
    expect(w.notHeldMs).toBe(60_000)
    expect(frameHeld(w, w.frames)).toBe(true)
    expect(frameHeld(w, 0)).toBe(false)
  })
})

describe('conversation replay', () => {
  const conv = 'prometheus-alerts-a1'
  const base = { conversation: conv, pipeline: 'alert-triage' }
  const first = [
    at(NOW, hop('input.queued', 'signal-source/prometheus-alerts', `conversation/${conv}`, base)),
    at(NOW + 1_000, hop('run.dispatched', 'pipeline/alert-triage', 'runtime/default', { ...base, runId: 'r1' })),
    at(NOW + 9_000, hop('run.completed', 'runtime/default', 'pipeline/alert-triage', { ...base, runId: 'r1' })),
  ]
  const second = [
    at(NOW + 60_000, hop('input.queued', 'signal-source/prometheus-alerts', `conversation/${conv}`, base)),
    at(NOW + 60_100, hop('runtime.starting', `conversation/${conv}`, 'runtime/agentops-conv-a1', { ...base, latencyMs: 30_000 })),
    at(NOW + 90_100, hop('run.dispatched', 'pipeline/alert-triage', 'runtime/default', { ...base, runId: 'r2' })),
    at(NOW + 95_000, hop('model.call', `runtime-image/${CLAUDE_IMAGE}`, 'model/claude-sonnet-5', { ...base, runId: 'r2' })),
    at(NOW + 99_000, hop('run.completed', 'runtime/default', 'pipeline/alert-triage', { ...base, runId: 'r2' })),
    at(NOW + 99_200, hop('channel.op.enqueued', `conversation/${conv}`, 'channel/console', { ...base, opId: `send:${conv}:console:r2` })),
  ]
  const events = [...second, ...first, at(NOW + 70_000, hop('input.queued', 'x/y', 'conversation/other', { conversation: 'other' }))]

  it('lists the latest run in order with offsets and latencies', () => {
    const run = latestRun(events, conv)
    expect(run.map((e) => e.kind)).toEqual([
      'input.queued', 'runtime.starting', 'run.dispatched', 'model.call', 'run.completed', 'channel.op.enqueued',
    ])
    const s = steps(run)
    // a slow start is attributable: the pod start's latency and the step to dispatch agree
    expect(s[1].ev.latencyMs).toBe(30_000)
    expect(s[2].offsetMs - s[1].offsetMs).toBe(30_000)
  })

  it('compresses gaps so a long one reads as long, not as forever', () => {
    expect(playDelay(0)).toBe(900)
    expect(playDelay(30_000)).toBeGreaterThan(playDelay(1_000))
    expect(playDelay(3_600_000)).toBe(3100)
  })

  it('lists the conversations active in the window, most recent first', () => {
    expect(activeConversations(events, NOW + 120_000, 300_000)).toEqual([conv, 'other'])
  })

  it('dims everything off the conversation\'s route on each view', () => {
    const topo = fixtureTopology()
    const run = latestRun(events, conv)
    for (const view of ['model', 'components', 'infrastructure'] as const) {
      const g = buildView(view, topo)
      const route = conversationRoute(g, hopIndex(g), run, conv)
      const lit = (id: string) => route.nodes.has(id)
      if (view === 'model') {
        expect(lit('pipelines/alert-triage')).toBe(true)
        expect(lit('agentruntimes/default')).toBe(true)
        expect(lit(`conversations/${conv}`)).toBe(true)
        expect(lit('pipelines/nightly-report')).toBe(false)
      }
      if (view === 'components') {
        for (const id of ['manager/manager', 'context-sync/context-sync', `runtime-image/${CLAUDE_IMAGE}`, 'egress-proxy/egress-proxy', 'model/claude-sonnet-5']) {
          expect(lit(id), id).toBe(true)
        }
        expect(lit('mcp-server/kubernetes')).toBe(false)
        expect(lit('signal-adapter/cron')).toBe(false)
      }
      if (view === 'infrastructure') {
        expect(lit('pods/agentops-manager-7c9d')).toBe(true)
        expect(lit(`pods/agentops-conv-${conv}`)).toBe(true)
        expect(lit('pods/agentops-conv-nightly-d2')).toBe(false)
      }
    }
  })
})
