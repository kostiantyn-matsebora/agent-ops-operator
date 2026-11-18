import { describe, expect, it } from 'vitest'
import type { Message } from '../api/types'
import { SEVERITIES, copyText, firstUnreadId, isOwn, replyBlocks, signalAttributes, signalSeverity, speakerOf } from './message'

const msg = (over: Partial<Message>): Message => ({ id: 'm', thread: 't', kind: 'agent', text: '', at: '', ...over })

describe('a message, as every look reads it', () => {
  it('reads each contract severity, and anything else as unrated', () => {
    for (const s of SEVERITIES) expect(signalSeverity({ severity: s })).toBe(s)
    expect(signalSeverity({})).toBeUndefined()
    expect(signalSeverity({ severity: 'sparkly' })).toBeUndefined()
  })

  it('names who speaks, hiding the manager\u2019s own acknowledgement', () => {
    expect(speakerOf({ kind: 'signal' })).toBe('signal')
    expect(speakerOf({ kind: 'agent' })).toBe('agent')
    expect(speakerOf({ kind: 'notice' })).toBe('notice')
    expect(speakerOf({ kind: 'ack' })).toBe('hidden')
    for (const kind of ['local', 'relay', 'user']) expect(speakerOf({ kind })).toBe('person')
    expect(isOwn({ kind: 'local' })).toBe(true)
    expect(isOwn({ kind: 'relay' })).toBe(false)
  })

  it('parses an agent\u2019s text into blocks and never a person\u2019s', () => {
    const text = '<title>\nDone\n</title>\n\n<fix>\nnothing\n</fix>'
    expect(replyBlocks(msg({ kind: 'agent', text }))?.map((b) => b.role)).toEqual(['title', 'section'])
    expect(replyBlocks(msg({ kind: 'local', text }))).toBeUndefined()
  })

  it('copies a reply and a person\u2019s message as the markdown they were written as', () => {
    const text = '<title>\nDone\n</title>\n\nRan `kubectl get pods` and **found** it.'
    expect(copyText(msg({ kind: 'agent', text }))).toBe(text)
    expect(copyText(msg({ kind: 'local', text: 'please **check**' }))).toBe('please **check**')
  })

  it('copies a signal as its body, its attributes and its event document', () => {
    const signal = msg({ kind: 'signal', text: 'Disk is full.', severity: 'warning', sender: 'manager', payload: '{"a":1}' })
    expect(signalAttributes(signal, { source: 'node-events' })).toEqual([
      ['severity', 'warning'], ['source', 'node-events'], ['sender', 'manager'],
    ])
    expect(copyText(signal, { source: 'node-events' })).toBe(
      'Disk is full.\n\nseverity: warning\nsource: node-events\nsender: manager\n\n```json\n{"a":1}\n```',
    )
    expect(copyText(msg({ kind: 'signal', text: 'x' }))).toBe('x\n\nseverity: unrated')
  })
})

describe('firstUnreadId', () => {
  const m = (id: string, kind: string, at: string) => ({ id, thread: 't', kind, text: id, at })
  const all = [m('a', 'agent', '2025-06-10T22:00:00Z'), m('k', 'ack', '2025-06-10T22:03:00Z'), m('b', 'signal', '2025-06-10T22:05:00Z'), m('c', 'local', '2025-06-10T22:06:00Z')]
  it('is the first counted message after the watermark, skipping acknowledgements and a person\u2019s own', () => {
    expect(firstUnreadId(all, '2025-06-10T22:01:00Z')).toBe('b')
  })
  it('is absent without a watermark, and when everything was read', () => {
    expect(firstUnreadId(all, undefined)).toBeUndefined()
    expect(firstUnreadId(all, '2025-06-10T23:00:00Z')).toBeUndefined()
  })
})
