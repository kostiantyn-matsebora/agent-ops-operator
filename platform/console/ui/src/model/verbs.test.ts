import { beforeEach, describe, expect, it, vi } from 'vitest'
import { resetCommands, useCommandStore } from './commands'
import { NEW_CONVERSATION, exitRuntime, pendingMessage, sendText, startConversation, verbOfText } from './verbs'
import { signIn, signOut } from './session'

const api = vi.hoisted(() => ({ send: vi.fn(), start: vi.fn(), login: vi.fn(), logout: vi.fn() }))
vi.mock('../api/client', () => ({ api }))

beforeEach(() => {
  resetCommands()
  for (const f of Object.values(api)) f.mockReset()
})

const latest = () => useCommandStore.getState().commands.at(-1)

describe('a message post', () => {
  it('is accepted while the console holds it pending, waiting on that message', async () => {
    api.send.mockResolvedValue({ id: 'local:2026', kind: 'local', pending: true })
    const outcome = await sendText('c', 'is the pod healthy?')
    expect(api.send).toHaveBeenCalledWith('c', 'is the pod healthy?')
    expect(outcome.ok).toBe(true)
    expect(latest()).toMatchObject({ target: 'c', verb: 'send', state: 'accepted', ref: 'local:2026' })
  })

  it('is done at once when the response is not a pending message', async () => {
    api.send.mockResolvedValue({ id: 'sent' })
    await sendText('c', 'hello')
    expect(latest()).toMatchObject({ verb: 'send', state: 'done' })
  })

  it('keeps a thread command’s own verb, so retry follows the verb', async () => {
    api.send.mockRejectedValue(new Error('sending failed'))
    await sendText('c', '/close now')
    expect(latest()).toMatchObject({ verb: 'close', state: 'failed', retryable: true })
    await sendText('c', 'a reply')
    expect(latest()).toMatchObject({ verb: 'send', state: 'failed', retryable: false })
  })

  it('reads which verb a typed message is', () => {
    expect(verbOfText('/exit')).toBe('exit')
    expect(verbOfText('  /close the incident')).toBe('close')
    expect(verbOfText('/closer')).toBe('send')
    expect(verbOfText('please /exit')).toBe('send')
  })

  it('reads only a pending message as accepted', () => {
    expect(pendingMessage({ id: 'a', pending: true })).toBe('a')
    expect(pendingMessage({ id: 'a' })).toBeUndefined()
    expect(pendingMessage(undefined)).toBeUndefined()
  })
})

describe('the row’s exit', () => {
  it('posts /exit and settles on the response', async () => {
    api.send.mockResolvedValue({ id: 'local:1', pending: true })
    await exitRuntime('c')
    expect(api.send).toHaveBeenCalledWith('c', '/exit')
    expect(latest()).toMatchObject({ target: 'c', verb: 'exit', state: 'done' })
  })
})

describe('starting a conversation', () => {
  it('is tracked under the new-conversation target and says why it was refused', async () => {
    api.start.mockRejectedValue(Object.assign(new Error('not wired'), { body: { fix: 'add a Pipeline' } }))
    const outcome = await startConversation('/ha-ops restart', 'chat')
    expect(api.start).toHaveBeenCalledWith('/ha-ops restart', 'chat')
    expect(outcome).toMatchObject({ ok: false, error: 'not wired — add a Pipeline' })
    expect(latest()).toMatchObject({ target: NEW_CONVERSATION, verb: 'start', state: 'failed', retryable: false })
  })
})

describe('the session', () => {
  it('signs in with a token, saying only whether it was accepted', async () => {
    api.login.mockResolvedValueOnce(undefined).mockRejectedValueOnce(new Error('401'))
    expect(await signIn('good')).toBe(true)
    expect(await signIn('bad')).toBe(false)
    expect(api.login).toHaveBeenCalledWith('good')
  })

  it('signs out', async () => {
    api.logout.mockResolvedValue(undefined)
    await signOut()
    expect(api.logout).toHaveBeenCalled()
  })
})
