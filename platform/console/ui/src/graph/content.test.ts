import { describe, expect, it } from 'vitest'
import { CLAUDE_IMAGE, RESULT, detailWithRun, hop } from '../test-fixtures/topology'
import { hopContent, hopSummary, parseOpId } from './content'

const base = { conversation: 'cluster-events-b7', pipeline: 'k8s-observe' }

describe('what a hop carried', () => {
  it('shows a completion\'s result from the conversation\'s status, not from the event', () => {
    const ev = hop('run.completed', 'runtime/default', 'pipeline/k8s-observe', {
      ...base, runId: 'r7', detail: 'succeeded (exit 0)', data: { excerpt: 'an excerpt the event carried' },
    })
    const c = hopContent(ev, detailWithRun())
    expect(c.body).toEqual({ label: 'Result', text: RESULT })
    expect(c.rows).toContainEqual(['Exit', '0'])
    expect(c.body?.text).not.toContain('excerpt')
  })

  it('says why there is no result rather than inventing one', () => {
    const ev = hop('run.completed', 'runtime/default', 'pipeline/k8s-observe', { ...base, runId: 'gone', detail: 'failed (exit 1)' })
    expect(hopContent(ev, detailWithRun()).body).toBeUndefined()
    expect(hopContent(ev, detailWithRun()).note).toMatch(/not on the conversation/)
    expect(hopContent(ev).note).toMatch(/not loaded/)
  })

  it('shows an input\'s text by its id, recorded or still queued', () => {
    const recorded = hopContent(hop('channel.inbound', 'channel/console', 'conversation/cluster-events-b7', { ...base, inputId: 'in-7' }), detailWithRun())
    expect(recorded.body?.text).toBe('why is checkout restarting?')
    expect(recorded.rows).toContainEqual(['Sender', 'operator'])
    const queued = hopContent(hop('input.queued', 'signal-source/cluster-events', 'conversation/cluster-events-b7', { ...base, inputId: 'in-q' }), detailWithRun())
    expect(queued.body?.text).toBe('queued, not yet run')
  })

  it('shows a work unit\'s tools and context handle', () => {
    const c = hopContent(hop('run.dispatched', 'pipeline/k8s-observe', 'runtime/default', { ...base, runId: 'r7' }), detailWithRun())
    expect(c.rows).toContainEqual(['Toolsets', 'agentops-observe'])
    expect(c.rows).toContainEqual(['Context handle', 'ctx-b7-4'])
    expect(c.body?.text).toContain('checkout')
  })

  it('shows an op\'s message and delivery by its id', () => {
    const opId = 'send:cluster-events-b7:console:r7'
    expect(parseOpId(opId)).toEqual({ kind: 'send', channel: 'console', runId: 'r7' })
    const c = hopContent(hop('channel.op.completed', 'channel-adapter/console', 'channel/console', { ...base, opId, code: 'send' }), detailWithRun())
    expect(c.body?.text).toBe(RESULT)
    expect(c.rows).toContainEqual(['Recorded delivered', 'yes'])
    expect(c.rows).toContainEqual(['Report', 'delivered'])
  })

  it('shows a model call\'s and a tool call\'s own facts', () => {
    const call = hop('model.call', `runtime-image/${CLAUDE_IMAGE}`, 'model/claude-sonnet-5', {
      ...base, data: { model: 'claude-sonnet-5', tokensIn: '1200', tokensOut: '340', stopReason: 'tool_use' },
    })
    expect(hopContent(call).rows).toContainEqual(['Stop reason', 'tool_use'])
    expect(hopSummary(call)).toBe('claude-sonnet-5 · 1200 in · 340 out · tool_use')
    const tool = hop('tool.call', `runtime-image/${CLAUDE_IMAGE}`, null, { ...base, data: { tool: 'Read' }, latencyMs: 40 })
    expect(hopContent(tool).rows).toContainEqual(['Target', 'built-in'])
    expect(hopSummary(tool)).toBe('Read · built-in')
  })
})
