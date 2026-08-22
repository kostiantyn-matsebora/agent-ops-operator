import { describe, expect, it, vi } from 'vitest'
import { QueryClient } from '@tanstack/react-query'
import { applyDelta, applyMessage } from './apply'
import type {
  ConversationDetail, ConversationPage, ConversationSummary, Detail, InventoryRow,
} from './types'

// The applier: one event, every view holding that object.
//
// Views read the cache and never the stream, so this module is the ONLY place
// an event becomes a change on screen. Two views implementing the same update
// would eventually disagree, and the disagreement would show as a list and a
// detail saying different things about one object.

function newClient() {
  return new QueryClient({ defaultOptions: { queries: { retry: false } } })
}

const summary = (over: Partial<ConversationSummary> = {}): ConversationSummary => ({
  name: 'conv-1', phase: 'Running', runCount: 0, queued: 0, joined: true,
  consoleThread: 'th-1', errored: false, unread: false, ageSeconds: 0, deleting: false,
  ...over,
})

const page = (items: ConversationSummary[], params = 'limit=50&offset=0'): [string, ConversationPage] => [
  params,
  { items, total: items.length, unreadTotal: 0, offset: 0, limit: 50, facets: {} },
]

const detail = (over: Partial<ConversationDetail> = {}): ConversationDetail =>
  ({
    conversation: summary(), object: { kind: 'conversations', metadata: { name: 'conv-1' } },
    yaml: 'a', transcript: [], archived: false, events: [], ...over,
  }) as ConversationDetail

describe('one event, every view holding that object', () => {
  it('updates the listing and the detail of the same object at once', () => {
    const client = newClient()
    client.setQueryData<InventoryRow[]>(['inventory', 'pipelines'], [
      { name: 'k8s-ops', health: 'ok', findings: 0 },
    ])
    client.setQueryData<Detail>(['detail', 'channels', 'console'], {
      object: { kind: 'channels', metadata: { name: 'console' } }, health: 'ok',
      yaml: 'old', usedBy: [], findings: [],
    } as Detail)

    applyDelta(client, {
      type: 'MODIFIED', kind: 'channels', name: 'console',
      row: { name: 'console', health: 'bad', findings: 1 },
      detail: {
        object: { kind: 'channels', metadata: { name: 'console' } }, health: 'bad',
        yaml: 'new', usedBy: [], findings: [],
      } as Detail,
    })

    expect(client.getQueryData<Detail>(['detail', 'channels', 'console'])!.yaml).toBe('new')
    // A view nobody opened is left alone rather than invented: the channels
    // listing was never loaded here, and writing it would make every browser
    // accumulate every object that ever changed.
    expect(client.getQueryData(['inventory', 'channels'])).toBeUndefined()
  })

  it('updates a conversation in its list and on its page from one event', () => {
    const client = newClient()
    const [params, p] = page([summary()])
    client.setQueryData(['conversations', params], p)
    client.setQueryData(['conversation', 'conv-1'], detail())

    applyDelta(client, {
      type: 'MODIFIED', kind: 'conversations', name: 'conv-1',
      conversationRow: summary({ phase: 'Closed' }),
      conversationView: {
        conversation: summary({ phase: 'Closed' }),
        object: { kind: 'conversations', metadata: { name: 'conv-1' } },
        yaml: 'b', archived: true,
      } as Omit<ConversationDetail, 'transcript' | 'events'>,
    })

    expect(client.getQueryData<ConversationPage>(['conversations', params])!.items[0].phase).toBe('Closed')
    expect(client.getQueryData<ConversationDetail>(['conversation', 'conv-1'])!.conversation.phase).toBe('Closed')
  })

  it('inserts a new conversation where its place is unambiguous', () => {
    const client = newClient()
    const [params, p] = page([summary({ name: 'older' })])
    client.setQueryData(['conversations', params], p)

    applyDelta(client, {
      type: 'ADDED', kind: 'conversations', name: 'fresh',
      conversationRow: summary({ name: 'fresh' }),
    })

    const after = client.getQueryData<ConversationPage>(['conversations', params])!
    // Newest first, and no filter to satisfy — so the row's place is the top.
    expect(after.items.map((c) => c.name)).toEqual(['fresh', 'older'])
    expect(after.total).toBe(2)
  })

  it('re-reads a filtered page instead of guessing whether a new row belongs', () => {
    const client = newClient()
    const [params, p] = page([summary({ name: 'older' })], 'phase=Running&limit=50&offset=0')
    client.setQueryData(['conversations', params], p)
    const invalidate = vi.spyOn(client, 'invalidateQueries').mockResolvedValue()

    applyDelta(client, {
      type: 'ADDED', kind: 'conversations', name: 'fresh',
      conversationRow: summary({ name: 'fresh', phase: 'Closed' }),
    })

    // Membership is the server's decision. The key is stable, so the re-read
    // swaps the rows under a table that never blanks.
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ['conversations', params] })
    expect(client.getQueryData<ConversationPage>(['conversations', params])!.items).toHaveLength(1)
  })

  it('removes a deleted conversation from the list and drops its page', () => {
    const client = newClient()
    const [params, p] = page([summary(), summary({ name: 'other' })])
    client.setQueryData(['conversations', params], p)
    client.setQueryData(['conversation', 'conv-1'], detail())

    applyDelta(client, { type: 'DELETED', kind: 'conversations', name: 'conv-1' })

    const after = client.getQueryData<ConversationPage>(['conversations', params])!
    expect(after.items.map((c) => c.name)).toEqual(['other'])
    expect(after.total).toBe(1)
    expect(client.getQueryData(['conversation', 'conv-1'])).toBeUndefined()
  })
})

describe('a message', () => {
  it('appears in its conversation with no request issued', () => {
    const fetchSpy = vi.spyOn(globalThis, 'fetch')
    const client = newClient()
    client.setQueryData(['conversation', 'conv-1'], detail())

    applyMessage(client, { id: 'm1', thread: 'th-1', kind: 'agent', text: 'done', at: 't' })

    const after = client.getQueryData<ConversationDetail>(['conversation', 'conv-1'])!
    expect(after.transcript!.map((m) => m.text)).toEqual(['done'])
    // The whole point: the payload IS the answer. The console used to parse
    // exactly this and then ask the server what had just arrived.
    expect(fetchSpy).not.toHaveBeenCalled()
    fetchSpy.mockRestore()
  })

  it('replaces a locally-typed message rather than showing it twice', () => {
    const client = newClient()
    client.setQueryData(['conversation', 'conv-1'], detail({
      transcript: [{ id: 'm1', thread: 'th-1', kind: 'user', text: 'hello', at: 't', pending: true }],
    }))

    applyMessage(client, { id: 'm1', thread: 'th-1', kind: 'user', text: 'hello', at: 't' })

    const after = client.getQueryData<ConversationDetail>(['conversation', 'conv-1'])!
    expect(after.transcript).toHaveLength(1)
    expect(after.transcript![0].pending).toBeUndefined()
  })

  it('goes to the conversation holding that thread, and to no other', () => {
    const client = newClient()
    client.setQueryData(['conversation', 'conv-1'], detail())
    client.setQueryData(['conversation', 'conv-2'], detail({
      conversation: summary({ name: 'conv-2', consoleThread: 'th-2' }),
    }))

    applyMessage(client, { id: 'm1', thread: 'th-1', kind: 'agent', text: 'done', at: 't' })

    expect(client.getQueryData<ConversationDetail>(['conversation', 'conv-2'])!.transcript).toHaveLength(0)
  })
})

// An applied entry must be a shape the snapshot would have produced. The row
// and the detail on a delta come from the same server functions the endpoints
// use — pinned on the Go side by TestDeltaShapesEqualTheSnapshots — so what is
// pinned HERE is that the applier writes them through unaltered, and touches
// nothing the event does not carry.
describe('an applied entry equals a fetched one', () => {
  it('writes the server\'s shape through, field for field', () => {
    const client = newClient()
    const fetched: Detail = {
      object: { kind: 'channels', metadata: { name: 'console' } }, health: 'ok',
      conditions: [{ type: 'Served', status: 'True' }],
      yaml: 'apiVersion: v1', usedBy: [], findings: [],
    }
    client.setQueryData<Detail>(['detail', 'channels', 'console'], {
      ...fetched, yaml: 'stale',
    })

    applyDelta(client, { type: 'MODIFIED', kind: 'channels', name: 'console', detail: fetched })

    expect(client.getQueryData<Detail>(['detail', 'channels', 'console'])).toEqual(fetched)
  })

  it('keeps the parts a delta does not carry, rather than emptying them', () => {
    const client = newClient()
    const transcript = [{ id: 'm1', thread: 'th-1', kind: 'agent', text: 'hi', at: 't' }]
    client.setQueryData(['conversation', 'conv-1'], detail({ transcript }))

    applyDelta(client, {
      type: 'MODIFIED', kind: 'conversations', name: 'conv-1',
      conversationView: {
        conversation: summary({ phase: 'Closed' }),
        object: { kind: 'conversations', metadata: { name: 'conv-1' } },
        yaml: 'b', archived: false,
      } as Omit<ConversationDetail, 'transcript' | 'events'>,
    })

    // The transcript and the events arrive on streams of their own. A page that
    // emptied its thread whenever the object changed would be worse than the
    // refetch this replaces.
    expect(client.getQueryData<ConversationDetail>(['conversation', 'conv-1'])!.transcript).toEqual(transcript)
  })
})
