import { beforeEach, describe, expect, it, vi } from 'vitest'
import { renderHook } from '@testing-library/react'
import { curatedConversations, curatedDetail } from '../test-fixtures/conversations'
import type { ConversationDetail, ConversationSummary, Message } from '../api/types'
import { resetCommands, runCommand, useCommandStore } from './commands'
import {
  chainStepLabel, firstTaskInput, invokeCardAt, lastResult, memberChain, memberTranscript, messageSource,
  useMemberChain, useMemberExchange, useThread, useThreadMembers,
} from './thread'

// The thread model over the curated install's featured conversation — a
// signal-started run that replied, a relayed question, and a second run
// working — plus the coordination tree the same install carries.

let detail: ConversationDetail | undefined = curatedDetail
let canWrite = true
const markRead = vi.fn()

vi.mock('../api/hooks', () => ({
  useConversation: (name: string) => ({
    data: name === curatedDetail.conversation.name ? detail : MEMBERS[name], isLoading: false, error: null, refetch: vi.fn(),
  }),
  useConversations: () => ({ data: curatedConversations, isLoading: false, error: null }),
  useMarkRead: () => ({ mutate: markRead }),
  useSession: () => ({ data: { canWrite } }),
  useVocabulary: () => ({ data: { entries: [{ kind: 'pipeline', name: 'k8s-observe', position: 'general', icon: 'aops:k8s' }] } }),
}))
vi.mock('../api/stream', () => ({
  useStream: (select: (s: { connected: boolean; events: never[] }) => unknown) => select({ connected: true, events: [] }),
  eventsFor: () => [],
}))

const ROOT = 'incident-response-4a8f21'
const item = (name: string) => curatedConversations.items.find((c) => c.name === name) as ConversationSummary
const MEMBERS: Record<string, ConversationDetail> = {
  [`${ROOT}-triage`]: {
    ...curatedDetail,
    conversation: {
      ...item(`${ROOT}-triage`),
      runs: [{
        runId: 'r', status: 'succeeded', startedAt: '2025-06-11T09:35:00Z', finishedAt: '2025-06-11T09:35:40Z',
        inputs: [{ id: 'i', text: 'triage checkout-api', receivedAt: '2025-06-11T09:35:00Z' }],
        result: 'OOMKilled restarts confirmed',
      }],
    },
  },
}

beforeEach(() => {
  detail = curatedDetail
  canWrite = true
  markRead.mockClear()
  resetCommands()
})

describe('useThread', () => {
  it('carries each message with its SOURCE markdown, signals apart, and the start time', () => {
    const { result } = renderHook(() => useThread('cluster-events-7c1d4e'))
    const t = result.current
    expect(t.messages.map((m) => [m.id, m.kind])).toEqual([['m1', 'signal'], ['m2', 'agent'], ['m3', 'relay']])
    // The agent's reply keeps its raw block grammar — the markdown, not HTML.
    expect(t.messages[1].source).toContain('<root-cause>')
    expect(t.messages[1].source).toBe(curatedDetail.transcript?.[1].text)
    expect(t.signals.map((m) => m.id)).toEqual(['m1'])
    expect(t.startedAt).toBe(curatedDetail.conversation.created)
    expect(t.title).toBe('checkout-api is restarting')
    expect(t.pipelineIcon).toBe('aops:k8s')
    expect([t.isRoot, t.isMember]).toEqual([false, false])
  })

  it('shapes the runs: the finished one attached to its reply, the inflight one as one fact', () => {
    const { result } = renderHook(() => useThread('cluster-events-7c1d4e'))
    expect([...result.current.runDetails.keys()]).toEqual(['m2'])
    expect(result.current.working).toMatchObject({ runId: 'run-4', count: 0 })
    expect(result.current.events.map((e) => e.cursor)).toEqual(['18240', '18241', '18242', '18243'])
  })

  it('may write only with the grant, a console thread, and an unarchived thread', () => {
    expect(renderHook(() => useThread('cluster-events-7c1d4e')).result.current.canWrite).toBe(true)
    canWrite = false
    expect(renderHook(() => useThread('cluster-events-7c1d4e')).result.current.canWrite).toBe(false)
    canWrite = true
    detail = { ...curatedDetail, archived: true }
    expect(renderHook(() => useThread('cluster-events-7c1d4e')).result.current.canWrite).toBe(false)
  })

  it('marks a joined, unread conversation read once per new activity', () => {
    const { rerender } = renderHook(() => useThread('cluster-events-7c1d4e'))
    rerender()
    expect(markRead).toHaveBeenCalledTimes(1)
    expect(markRead).toHaveBeenCalledWith({ names: ['cluster-events-7c1d4e'] })
  })

  it('settles the viewer’s accepted reply once its message turns durable', async () => {
    await runCommand({ target: 'cluster-events-7c1d4e', verb: 'send', perform: async () => 1, accepted: () => 'local:9' })
    const mine: Message = { id: 'local:9', thread: 't', kind: 'local', text: 'thanks', at: '2025-06-11T09:39:59Z', pending: true }
    detail = { ...curatedDetail, transcript: [...(curatedDetail.transcript ?? []), mine] }
    const { rerender } = renderHook(() => useThread('cluster-events-7c1d4e'))
    expect(useCommandStore.getState().commands[0].state).toBe('accepted')
    detail = { ...curatedDetail, transcript: [...(curatedDetail.transcript ?? []), { ...mine, pending: false }] }
    rerender()
    expect(useCommandStore.getState().commands[0].state).toBe('done')
  })

  it('an empty conversation is an empty thread, not a broken one', () => {
    detail = { ...curatedDetail, transcript: null as unknown as Message[], events: null, conversation: { ...curatedDetail.conversation, runs: undefined, presence: false } }
    const { result } = renderHook(() => useThread('cluster-events-7c1d4e'))
    expect(result.current.messages).toEqual([])
    expect(result.current.signals).toEqual([])
    expect(result.current.runs).toEqual([])
    expect(result.current.working).toBeUndefined()
    expect(result.current.runDetails.size).toBe(0)
  })

  it('before the conversation loads, it has nothing and claims nothing', () => {
    detail = undefined
    const { result } = renderHook(() => useThread('cluster-events-7c1d4e'))
    expect(result.current.conversation).toBeUndefined()
    expect(result.current.messages).toEqual([])
    expect(result.current.canWrite).toBe(false)
  })
})

describe('members and the chain above them', () => {
  it('a root’s direct members, each placed by its invocation time', () => {
    const root = item(ROOT)
    const { result } = renderHook(() => useThreadMembers(root, []))
    expect(result.current.members.map((m) => m.member.name)).toEqual([`${ROOT}-triage`, `${ROOT}-mitigate`, `${ROOT}-notify`])
    expect(result.current.members[0].at).toBe(Date.parse(item(`${ROOT}-triage`).created as string))
  })

  it('a nested member’s chain runs from the root, the Coordinator named by the root', () => {
    const { result } = renderHook(() => useMemberChain(item(`${ROOT}-mitigate-drain`)))
    expect(result.current.coordinator).toBe('incident-response')
    expect(result.current.ancestors.map((a) => a.name)).toEqual([ROOT, `${ROOT}-mitigate`])
    expect(result.current.ancestors.map(chainStepLabel)).toEqual(['Elevated 5xx rate on checkout-api', 'mitigate'])
  })

  it('a member’s exchange is its first task and its last result', () => {
    const { result } = renderHook(() => useMemberExchange(`${ROOT}-triage`))
    expect(result.current.task?.text).toBe('triage checkout-api')
    expect(result.current.result?.text).toBe('OOMKilled restarts confirmed')
    expect(memberTranscript(item(`${ROOT}-triage`), MEMBERS[`${ROOT}-triage`].conversation.runs).map((m) => [m.kind, m.text]))
      .toEqual([['signal', 'triage checkout-api'], ['agent', 'OOMKilled restarts confirmed']])
  })

  it('a member with no record yet has no task and no result, never a blank line', () => {
    expect(firstTaskInput(undefined)).toBeUndefined()
    expect(lastResult([])).toBeUndefined()
    expect(memberTranscript(item(`${ROOT}-mitigate`), undefined)).toEqual([])
    expect(memberChain(item(ROOT), []).ancestors).toEqual([])
  })

  it('an invoke card sits at the reasoning of the run that invoked it', () => {
    const runs = [{ runId: 'r0', status: 'succeeded', startedAt: '2024-01-01T19:04:58Z', finishedAt: '2024-01-01T19:07:04Z' }]
    const member = { ...item(`${ROOT}-triage`), created: '2024-01-01T19:05:33Z' }
    const reasoning: Message = { id: 'run:r0', thread: 't', kind: 'agent', text: 'x', at: '2024-01-01T19:07:06Z' }
    expect(invokeCardAt(runs, member, [reasoning])).toBe(Date.parse('2024-01-01T19:07:06Z'))
    expect(invokeCardAt(runs, member, [])).toBe(Date.parse('2024-01-01T19:07:04Z'))
    expect(invokeCardAt([], member, [])).toBe(Date.parse('2024-01-01T19:05:33Z'))
  })
})

describe('a message’s source', () => {
  it('is its text, with a signal’s raw document fenced after it', () => {
    expect(messageSource({ id: 'a', thread: 't', kind: 'agent', text: '**bold**', at: '' })).toBe('**bold**')
    expect(messageSource({ id: 's', thread: 't', kind: 'signal', text: 'disk', at: '', payload: '{"a":1}' }))
      .toBe('disk\n\n```json\n{"a":1}\n```')
  })
})
