import { beforeEach, describe, expect, it, vi } from 'vitest'
import { act, renderHook } from '@testing-library/react'
import { useStream } from '../api/stream'
import type { ActivityEvent } from '../api/types'
import {
  RETRY_SAFE, rowCommand, confirmCommand, confirmRef, describeError, dismissCommand, errorMessage, observeEvents, resetCommands,
  retryCommand, runCommand, useCommandStore, useCommands, type Verb,
} from './commands'

// design D7: each of the viewer's actions goes sending → accepted → done, or
// failed, DERIVED from the response and the event that confirms it — and
// `accepted` is optional, so a synchronous verb goes straight to done.

beforeEach(() => {
  resetCommands()
  useStream.setState({ connected: true, events: [], cursor: '' })
})

const commands = () => useCommandStore.getState().commands
const only = () => {
  expect(commands()).toHaveLength(1)
  return commands()[0]
}

/** A request the test settles by hand, so the in-between state can be read. */
function deferred<T>() {
  let resolve!: (v: T) => void
  let reject!: (e: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

const event = (cursor: string, kind: string, conversation = 'c'): ActivityEvent => ({
  cursor, kind, conversation, ts: '2026-01-01T00:00:00Z', status: 'ok',
})

describe('each transition', () => {
  it('starts as sending, before anything answers', () => {
    const d = deferred<unknown>()
    void runCommand({ target: 'c', verb: 'close', perform: () => d.promise })
    expect(only()).toMatchObject({ target: 'c', verb: 'close', state: 'sending', retryable: false })
  })

  it('a synchronous verb goes from sending straight to done — accepted is optional (2.5)', async () => {
    const d = deferred<{ outcome: string }>()
    const run = runCommand({ target: 'c', verb: 'reopen', perform: () => d.promise })
    expect(only().state).toBe('sending')
    d.resolve({ outcome: 'reopened' })
    const outcome = await run
    expect(outcome).toMatchObject({ ok: true, response: { outcome: 'reopened' } })
    expect(only()).toMatchObject({ state: 'done', retryable: false })
    expect(only().settledAt).toBeTypeOf('number')
    // Never passed through accepted on the way.
    expect(only().ref).toBeUndefined()
  })

  it('sending → accepted when the response says the work happens later, holding what to wait for', async () => {
    await runCommand({
      target: 'c', verb: 'send', perform: async () => ({ id: 'local:1', pending: true }),
      accepted: (r) => (r.pending ? r.id : undefined),
    })
    expect(only()).toMatchObject({ state: 'accepted', ref: 'local:1' })
  })

  it('accepted → done when the confirming message arrives, and only that command', async () => {
    const accepted = (r: { id: string }) => r.id
    await runCommand({ target: 'c', verb: 'send', perform: async () => ({ id: 'local:1' }), accepted })
    await runCommand({ target: 'c', verb: 'send', perform: async () => ({ id: 'local:2' }), accepted })
    confirmRef('c', 'local:2')
    expect(commands().map((c) => [c.ref, c.state])).toEqual([['local:1', 'accepted'], ['local:2', 'done']])
    confirmRef('other', 'local:1')
    expect(commands()[0].state).toBe('accepted')
  })

  it('accepted → done when the confirming activity event arrives', async () => {
    await runCommand({
      target: 'c', verb: 'close', perform: async () => ({ accepted: true }), accepted: () => 'close',
      confirmedBy: (e, cmd) => e.kind === 'conversation.closed' && e.conversation === cmd.target,
    })
    observeEvents([event('1', 'conversation.closed', 'other'), event('2', 'run.dispatched')])
    expect(only().state).toBe('accepted')
    observeEvents([event('3', 'conversation.closed')])
    expect(only().state).toBe('done')
  })

  it('an event already checked is not checked again', async () => {
    const confirmedBy = vi.fn(() => false)
    await runCommand({ target: 'c', verb: 'close', perform: async () => 1, accepted: () => 'x', confirmedBy })
    const seen = [event('5', 'k')]
    observeEvents(seen)
    observeEvents(seen)
    expect(confirmedBy).toHaveBeenCalledTimes(1)
  })

  it('confirmCommand settles every accepted command of a target and verb', async () => {
    await runCommand({ target: 'c', verb: 'exit', perform: async () => 1, accepted: () => 'a' })
    await runCommand({ target: 'c', verb: 'close', perform: async () => 1, accepted: () => 'b' })
    confirmCommand('c', 'exit')
    expect(commands().map((c) => [c.verb, c.state])).toEqual([['exit', 'done'], ['close', 'accepted']])
  })

  it('sending → failed, saying what did not happen in the server’s words', async () => {
    const refusal = Object.assign(new Error('source is not wired'), {
      status: 409, body: { message: 'no Ready Pipeline claims chat', fix: 'kubectl patch …' },
    })
    const outcome = await runCommand({ target: 'new', verb: 'start', perform: () => Promise.reject(refusal) })
    expect(outcome).toMatchObject({ ok: false, error: 'source is not wired — no Ready Pipeline claims chat — kubectl patch …' })
    expect(only()).toMatchObject({ state: 'failed', error: 'source is not wired — no Ready Pipeline claims chat — kubectl patch …' })
  })
})

describe('retry is offered only where repeating is safe', () => {
  it('names every verb, and only send and start are unsafe', () => {
    const unsafe = (Object.keys(RETRY_SAFE) as Verb[]).filter((v) => !RETRY_SAFE[v])
    expect(unsafe.sort()).toEqual(['send', 'start'])
  })

  it('a failed close is retryable, and a retry runs it again as the same command until it is done', async () => {
    const perform = vi.fn().mockRejectedValueOnce(new Error('refused: still working')).mockResolvedValueOnce({ closed: 1 })
    await runCommand({ target: 'c', verb: 'close', perform })
    expect(only()).toMatchObject({ state: 'failed', error: 'refused: still working', retryable: true })
    const id = only().id
    const again = retryCommand(id)
    expect(only()).toMatchObject({ id, state: 'sending', retryable: false, error: undefined })
    await again
    expect(only()).toMatchObject({ id, state: 'done' })
    expect(perform).toHaveBeenCalledTimes(2)
  })

  it('a failed reply is NOT retryable, and asking to retry sends nothing', async () => {
    const perform = vi.fn().mockRejectedValue(new Error('sending failed'))
    await runCommand({ target: 'c', verb: 'send', perform })
    expect(only()).toMatchObject({ state: 'failed', retryable: false })
    expect(retryCommand(only().id)).toBeUndefined()
    expect(perform).toHaveBeenCalledTimes(1)
    expect(only().state).toBe('failed')
  })

  it('a failed start is NOT retryable either', async () => {
    await runCommand({ target: 'new', verb: 'start', perform: () => Promise.reject(new Error('x')) })
    expect(only().retryable).toBe(false)
  })

  it('a command that did not fail cannot be retried', async () => {
    await runCommand({ target: 'c', verb: 'close', perform: async () => 1 })
    expect(retryCommand(only().id)).toBeUndefined()
  })
})

describe('useCommands', () => {
  it('counts what is in flight and clears as each settles', async () => {
    const [a, b, c] = [deferred<unknown>(), deferred<unknown>(), deferred<unknown>()]
    const { result } = renderHook(() => useCommands())
    expect(result.current.inFlight).toBe(0)
    let runs: Promise<unknown>[] = []
    act(() => {
      runs = [
        runCommand({ target: 'x', verb: 'close', perform: () => a.promise }),
        runCommand({ target: 'y', verb: 'send', perform: () => b.promise, accepted: () => 'm1' }),
        runCommand({ target: 'z', verb: 'reopen', perform: () => c.promise }),
      ]
    })
    expect(result.current.inFlight).toBe(3)
    await act(async () => {
      a.resolve(1)
      await runs[0]
    })
    expect(result.current.inFlight).toBe(2)
    await act(async () => {
      b.resolve(1) // accepted: still awaiting its event
      await runs[1]
    })
    expect(result.current.inFlight).toBe(2)
    await act(async () => {
      c.reject(new Error('no'))
      await runs[2]
    })
    expect(result.current.inFlight).toBe(1)
    act(() => confirmRef('y', 'm1'))
    expect(result.current.inFlight).toBe(0)
  })

  it('states the connection from the stream', () => {
    const { result } = renderHook(() => useCommands())
    expect(result.current.connection).toBe('connected')
    act(() => useStream.setState({ connected: false }))
    expect(result.current.connection).toBe('disconnected')
  })

  it('a disconnect holds an accepted command where it was, and the returning stream settles it', async () => {
    const { result } = renderHook(() => useCommands())
    await act(async () => {
      await runCommand({
        target: 'c', verb: 'exit', perform: async () => 1, accepted: () => 'exit',
        confirmedBy: (e) => e.kind === 'runtime.released',
      })
    })
    act(() => useStream.setState({ connected: false }))
    expect(result.current.latest('c')?.state).toBe('accepted')
    act(() => useStream.setState({ connected: true, events: [event('9', 'runtime.released')] }))
    expect(result.current.latest('c')?.state).toBe('done')
  })

  it('reads the newest command per target, and whether anything there is still sending', async () => {
    const d = deferred<unknown>()
    const { result } = renderHook(() => useCommands())
    await act(async () => {
      await runCommand({ target: 'c', verb: 'markRead', perform: async () => 1 })
    })
    act(() => {
      void runCommand({ target: 'c', verb: 'close', perform: () => d.promise })
    })
    expect(result.current.latest('c')?.verb).toBe('close')
    expect(result.current.latest('c', 'markRead')?.state).toBe('done')
    expect(result.current.latest('nobody')).toBeUndefined()
    expect(result.current.sending('c')).toBe(true)
    expect(result.current.sending('d')).toBe(false)
    await act(async () => d.resolve(1))
    expect(result.current.sending('c')).toBe(false)
  })

  it('a dismissed result is forgotten', async () => {
    await runCommand({ target: 'c', verb: 'close', perform: async () => 1 })
    dismissCommand(only().id)
    expect(commands()).toEqual([])
  })

  it('keeps a bounded number of settled results', async () => {
    for (let i = 0; i < 105; i++) await runCommand({ target: `t${i}`, verb: 'markRead', perform: async () => i })
    expect(commands()).toHaveLength(100)
    expect(commands()[0].target).toBe('t5')
  })
})

describe('explaining a failure', () => {
  it('reads a message off anything that carries one', () => {
    expect(errorMessage(new Error('boom'))).toBe('boom')
    expect(errorMessage({ message: 'plain object' })).toBe('plain object')
    expect(errorMessage('a string')).toBe('a string')
    expect(errorMessage(undefined)).toBe('the request failed')
  })

  it('adds the body’s message and fix only where present', () => {
    expect(describeError(new Error('boom'))).toBe('boom')
    expect(describeError(Object.assign(new Error('boom'), { body: { fix: 'do x', message: '' } }))).toBe('boom — do x')
  })
})

describe('rowCommand', () => {
  const base = { id: 1, target: 'c', retryable: false, startedAt: 0 } as const
  it('names the verb in progress, and says nothing of a reply or a finished action', () => {
    expect(rowCommand({ ...base, verb: 'close', state: 'accepted' })).toMatchObject({ kind: 'progress', label: 'closing…' })
    expect(rowCommand({ ...base, verb: 'send', state: 'sending' })).toBeUndefined()
    expect(rowCommand({ ...base, verb: 'close', state: 'done' })).toBeUndefined()
    expect(rowCommand(undefined)).toBeUndefined()
  })
  it('says what did not happen, with the server\u2019s words and whether a retry is safe', () => {
    expect(rowCommand({ ...base, verb: 'reopen', state: 'failed', error: 'nope', retryable: true })).toEqual({
      id: 1, kind: 'failed', label: 'Not reopened', error: 'nope', retryable: true,
    })
  })
})
