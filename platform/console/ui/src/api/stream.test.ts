import { afterEach, describe, expect, it, vi } from 'vitest'
import { QueryClient } from '@tanstack/react-query'
import { ACTIVITY_GAP, connectStream, noteDerived, useStream } from './stream'
import type { ConversationDetail, ConversationPage, InventoryRow } from './types'
import { FakeEventSource } from '../test-fixtures/fakeEventSource'

// Two things live here. WHO OWNS RECONNECTION — the masthead chip used to be
// able to stick on "stream disconnected" until the page was reloaded, because
// EventSource retries a dropped connection by itself but gives up PERMANENTLY
// on an HTTP error (a 502 while the console rolls, a 401 when the session
// expires): readyState goes CLOSED and nothing reopened it. And WHAT EACH EVENT
// DOES, one case per kind the stream carries, because an event that is merely
// parsed and thrown away is how this surface came to refetch what it had just
// been told.

vi.stubGlobal('EventSource', FakeEventSource)

function reset() {
  FakeEventSource.instances = []
  useStream.setState({ connected: false, events: [], cursor: '', queues: undefined })
}

function newClient() {
  return new QueryClient({ defaultOptions: { queries: { retry: false } } })
}

afterEach(() => {
  vi.useRealTimers()
})

const row = (name: string): InventoryRow => ({ name, health: 'ok', findings: 0 })

// Derived views are the stated exception to applying: a count across every
// kind, a traffic graph, a cross-object finding. They are re-read, and a burst
// of deltas must not be a burst of re-reads — closing fifty conversations
// writes fifty statuses and fifty deletions within a couple of seconds, and
// asking the overview fifty times what it now counts is a hundred requests for
// one answer.
describe('derived-view coalescing', () => {
  it('re-reads once for a burst, not once per delta', () => {
    vi.useFakeTimers()
    const client = newClient()
    const invalidate = vi.spyOn(client, 'invalidateQueries').mockResolvedValue()

    for (let i = 0; i < 50; i++) noteDerived(client, 'conversations')
    // nothing yet: the window is still open
    expect(invalidate).not.toHaveBeenCalled()

    vi.advanceTimersByTime(250)
    // One per derived view that conversations move — never fifty of each.
    const overviewCalls = invalidate.mock.calls.filter(
      ([arg]) => JSON.stringify((arg as { queryKey: unknown })?.queryKey) === '["overview"]',
    )
    expect(overviewCalls).toHaveLength(1)
  })

  it('re-reads only the views the moved kinds touch', () => {
    vi.useFakeTimers()
    const client = newClient()
    const invalidate = vi.spyOn(client, 'invalidateQueries').mockResolvedValue()

    noteDerived(client, 'conversations')
    vi.advanceTimersByTime(250)

    const keys = invalidate.mock.calls.map(([arg]) =>
      JSON.stringify((arg as { queryKey: unknown })?.queryKey),
    )
    // The addressable-pipeline vocabulary cannot move because a conversation did.
    expect(keys).not.toContain('["vocabulary"]')
  })

  it('opens a fresh window for the next burst', () => {
    vi.useFakeTimers()
    const client = newClient()
    const invalidate = vi.spyOn(client, 'invalidateQueries').mockResolvedValue()

    noteDerived(client, 'pipelines')
    vi.advanceTimersByTime(250)
    noteDerived(client, 'pipelines')
    vi.advanceTimersByTime(250)

    const vocab = invalidate.mock.calls.filter(
      ([arg]) => JSON.stringify((arg as { queryKey: unknown })?.queryKey) === '["vocabulary"]',
    )
    // Coalescing delays a re-read, never drops one.
    expect(vocab).toHaveLength(2)
  })
})

// One case per event the stream carries. Each is APPLIED — the payload is the
// answer, not a hint that an answer exists somewhere.
describe('what each event does', () => {
  it('reloads on resync, because the client cannot know what it missed', () => {
    reset()
    const client = newClient()
    const invalidate = vi.spyOn(client, 'invalidateQueries').mockResolvedValue()
    const stop = connectStream(client)

    FakeEventSource.instances[0].emit('resync', { reason: 'connected', cursor: '1' })

    expect(invalidate).toHaveBeenCalledWith()
    stop()
  })

  it('writes a created and an updated object into the listing it belongs to', () => {
    reset()
    const client = newClient()
    client.setQueryData(['inventory', 'channels'], [row('slack')])
    const stop = connectStream(client)
    const es = FakeEventSource.instances[0]

    es.emit('delta', { type: 'ADDED', kind: 'channels', name: 'console', row: row('console') })
    es.emit('delta', {
      type: 'MODIFIED', kind: 'channels', name: 'slack',
      row: { ...row('slack'), health: 'bad' },
    })

    const rows = client.getQueryData<InventoryRow[]>(['inventory', 'channels'])!
    // Name order, because that is the order the server lists in — an applied
    // row lands where a re-fetch would have put it.
    expect(rows.map((r) => r.name)).toEqual(['console', 'slack'])
    expect(rows.find((r) => r.name === 'slack')!.health).toBe('bad')
    stop()
  })

  it('drops what was deleted, from the row and from its detail', () => {
    reset()
    const client = newClient()
    client.setQueryData(['inventory', 'channels'], [row('slack')])
    client.setQueryData(['detail', 'channels', 'slack'], { yaml: 'x' })
    const stop = connectStream(client)

    FakeEventSource.instances[0].emit('delta', { type: 'DELETED', kind: 'channels', name: 'slack' })

    expect(client.getQueryData<InventoryRow[]>(['inventory', 'channels'])).toEqual([])
    expect(client.getQueryData(['detail', 'channels', 'slack'])).toBeUndefined()
    stop()
  })

  it('keeps activity events whole, and tracks the cursor', () => {
    reset()
    const client = newClient()
    const stop = connectStream(client)

    FakeEventSource.instances[0].emit('activity', {
      cursor: '0000000000000007', ts: 't', kind: 'run.dispatched', status: 'ok',
    })

    expect(useStream.getState().events).toHaveLength(1)
    expect(useStream.getState().cursor).toBe('0000000000000007')
    stop()
  })

  it('treats a history gap as a resync, because history the client missed is gone', () => {
    reset()
    const client = newClient()
    const invalidate = vi.spyOn(client, 'invalidateQueries').mockResolvedValue()
    const stop = connectStream(client)

    FakeEventSource.instances[0].emit('activity', {
      cursor: '', ts: 't', kind: ACTIVITY_GAP, status: 'ok',
    })

    expect(invalidate).toHaveBeenCalledWith()
    stop()
  })

  it('applies pushed queue state', () => {
    reset()
    const client = newClient()
    const stop = connectStream(client)

    FakeEventSource.instances[0].emit('queues', {
      capacity: { inUse: 1, max: 5, waiting: 0 }, work: [], delivery: [], cooldowns: [],
    })

    expect(useStream.getState().queues?.capacity.inUse).toBe(1)
    stop()
  })

  it('puts a message in the conversation holding its thread, asking nothing', () => {
    reset()
    const client = newClient()
    const detail = {
      conversation: { name: 'conv-1', consoleThread: 'th-1', runCount: 0, queued: 0, joined: true, errored: false, unread: false, ageSeconds: 0, deleting: false },
      object: { kind: 'conversations', metadata: { name: 'conv-1' } },
      yaml: '', transcript: [], archived: false, events: [],
    } as unknown as ConversationDetail
    client.setQueryData(['conversation', 'conv-1'], detail)
    const stop = connectStream(client)

    FakeEventSource.instances[0].emit('message', {
      id: 'm1', thread: 'th-1', kind: 'agent', text: 'done', at: 't',
    })

    const after = client.getQueryData<ConversationDetail>(['conversation', 'conv-1'])!
    expect(after.transcript).toHaveLength(1)
    expect(after.transcript![0].text).toBe('done')
    stop()
  })

  it('applies a conversation delta to the list and the page it is open on', () => {
    reset()
    const client = newClient()
    const summary = {
      name: 'conv-1', phase: 'Running', runCount: 0, queued: 0, joined: true,
      errored: false, unread: false, ageSeconds: 0, deleting: false,
    }
    client.setQueryData<ConversationPage>(['conversations', 'limit=50&offset=0'], {
      items: [summary as ConversationPage['items'][number]],
      total: 1, unreadTotal: 0, offset: 0, limit: 50, facets: {},
    })
    client.setQueryData(['conversation', 'conv-1'], {
      conversation: summary, object: {}, yaml: '', transcript: [{ id: 'm1' }], archived: false, events: [],
    } as unknown as ConversationDetail)
    const stop = connectStream(client)

    FakeEventSource.instances[0].emit('delta', {
      type: 'MODIFIED', kind: 'conversations', name: 'conv-1',
      conversationRow: { ...summary, phase: 'Closed' },
      conversationView: {
        conversation: { ...summary, phase: 'Closed' }, object: {}, yaml: 'y', archived: true,
      },
    })

    const page = client.getQueryData<ConversationPage>(['conversations', 'limit=50&offset=0'])!
    expect(page.items[0].phase).toBe('Closed')
    const open = client.getQueryData<ConversationDetail>(['conversation', 'conv-1'])!
    expect(open.conversation.phase).toBe('Closed')
    expect(open.archived).toBe(true)
    // The transcript arrives on its own stream and is NOT in a delta — a page
    // that emptied its thread whenever the object changed would be worse than
    // the refetch this replaces.
    expect(open.transcript).toHaveLength(1)
    stop()
  })
})

describe('connectStream', () => {
  it('reopens after the browser gives up, so the chip recovers without a reload', () => {
    vi.useFakeTimers()
    reset()
    const stop = connectStream(newClient())

    const first = FakeEventSource.instances[0]
    first.open()
    expect(useStream.getState().connected).toBe(true)

    first.failPermanently()
    expect(useStream.getState().connected).toBe(false)
    expect(first.closed).toBe(true)

    // Nothing reopened it before — this is the bug.
    vi.advanceTimersByTime(1_000)
    expect(FakeEventSource.instances).toHaveLength(2)

    FakeEventSource.instances[1].open()
    expect(useStream.getState().connected).toBe(true)

    stop()
  })

  it('leaves a transport blip to the browser', () => {
    vi.useFakeTimers()
    reset()
    const stop = connectStream(newClient())

    const es = FakeEventSource.instances[0]
    es.open()
    es.blip() // still CONNECTING: EventSource is retrying by itself

    expect(useStream.getState().connected).toBe(false)
    vi.advanceTimersByTime(30_000)
    // Opening a second connection here would double every browser's stream.
    expect(FakeEventSource.instances).toHaveLength(1)
    expect(es.closed).toBe(false)

    stop()
  })

  it('backs off instead of hammering a console that is down', () => {
    vi.useFakeTimers()
    reset()
    const stop = connectStream(newClient())

    FakeEventSource.instances[0].failPermanently()
    vi.advanceTimersByTime(1_000)
    expect(FakeEventSource.instances).toHaveLength(2)

    FakeEventSource.instances[1].failPermanently()
    vi.advanceTimersByTime(1_000) // too early: the wait has doubled
    expect(FakeEventSource.instances).toHaveLength(2)
    vi.advanceTimersByTime(1_000)
    expect(FakeEventSource.instances).toHaveLength(3)

    stop()
  })

  it('stops retrying once the caller unmounts', () => {
    vi.useFakeTimers()
    reset()
    const stop = connectStream(newClient())

    FakeEventSource.instances[0].failPermanently()
    stop()

    vi.advanceTimersByTime(60_000)
    expect(FakeEventSource.instances).toHaveLength(1)
    expect(useStream.getState().connected).toBe(false)
  })
})
