import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { createQueryClient } from '../api/queryClient'
import { ACTIVITY_GAP, connectStream, useStream } from '../api/stream'
import { FakeEventSource } from '../test-fixtures/fakeEventSource'

// THE STANDING RULE, one case per page: after a view has painted, an arriving
// event never puts it back into a loading state.
//
// This is a test rather than a convention because conventions of this kind
// decay one call site at a time — which is exactly how twelve queries came to
// differ from the one that had it right. Each page is mounted against its real
// hooks and a real cache, painted, and then sent events through the same
// listeners a browser is wired to.

// ---- the server, as far as these tests are concerned --------------------------

let served: Record<string, unknown>
let calls: Record<string, number>

function serve<T>(key: string): Promise<T> {
  calls[key] = (calls[key] ?? 0) + 1
  return Promise.resolve(structuredClone(served[key]) as T)
}

vi.mock('../api/client', () => ({
  ApiError: class ApiError extends Error {},
  api: {
    session: () => serve('session'),
    overview: () => serve('overview'),
    queues: () => serve('queues'),
    kinds: () => serve('kinds'),
    inventory: (kind: string) => serve(`inventory:${kind}`),
    detail: (kind: string, name: string) => serve(`detail:${kind}:${name}`),
    findings: () => serve('findings'),
    topology: () => serve('topology'),
    conversations: (params: URLSearchParams) =>
      serve(params.get('count') ? 'conversationCount' : 'conversations'),
    conversation: (name: string) => serve(`conversation:${name}`),
    conversationGraph: () => serve('conversationGraph'),
    sources: () => serve('sources'),
    vocabulary: () => serve('vocabulary'),
    charts: () => serve('charts'),
    chart: () => serve('chart'),
    send: () => serve('send'),
    markRead: () => serve('markRead'),
  },
}))

vi.stubGlobal('EventSource', FakeEventSource)

const summary = (over: Record<string, unknown> = {}) => ({
  name: 'conv-1', title: 'disk pressure on node-3', phase: 'Running', profile: 'k8s-engineer',
  pipeline: 'k8s-ops', runCount: 1, queued: 0, joined: true, consoleThread: 'th-1',
  errored: false, unread: false, ageSeconds: 12, deleting: false, threads: [],
  runs: [], ...over,
})

const conversationView = (over: Record<string, unknown> = {}) => ({
  conversation: summary(over),
  object: { kind: 'conversations', metadata: { name: 'conv-1' } },
  yaml: 'kind: Conversation',
  archived: false,
})

beforeEach(() => {
  FakeEventSource.reset()
  useStream.setState({ connected: false, events: [], cursor: '', queues: undefined })
  calls = {}
  served = {
    session: { canWrite: true, writeEnabled: true, configured: true },
    overview: {
      namespace: 'agent-ops', stream: { connected: true, events: 0, resyncs: 0 },
      workloads: [{ name: 'agentops-manager', desired: 1, ready: 1, restarts: 0 }],
      runtimes: [], adapters: [], counts: { channels: 1 }, synced: { channels: true },
      problems: [], manager: { runtimeSlots: { inUse: 1, max: 5, waiting: 0 }, queues: [], cooldowns: [] },
    },
    charts: { available: false, charts: [] },
    queues: {
      capacity: { inUse: 1, max: 5, waiting: 0 },
      work: [{ conversation: 'conv-1', queued: 1, ageSeconds: 4 }],
      delivery: [], cooldowns: [],
    },
    kinds: [{ kind: 'channels', title: 'Channel', count: 1, synced: true }],
    'inventory:channels': [{ name: 'console', health: 'ok', findings: 0, summary: 'adapter console' }],
    'detail:channels:console': {
      object: { kind: 'channels', metadata: { name: 'console' } }, health: 'ok',
      conditions: [{ type: 'Served', status: 'True', reason: 'AdapterReady' }],
      yaml: 'kind: Channel', usedBy: [], findings: [],
    },
    findings: [],
    topology: {
      topology: {
        nodes: [{ id: 'channels/console', kind: 'channels', name: 'console', health: 'ok', active: 0, recent: 0 }],
        edges: [], eventNodeKinds: {},
      },
      consoleChannel: 'console', unjoinedPipelines: null, synced: { channels: true },
      stream: { connected: true, events: 0, resyncs: 0 }, oldestEvent: '', metricsAvailable: false,
    },
    conversations: {
      items: [summary()], total: 1, unreadTotal: 0, offset: 0, limit: 50, facets: {},
    },
    conversationCount: { items: [], total: 1, unreadTotal: 0, offset: 0, limit: 0, facets: {} },
    'conversation:conv-1': {
      ...conversationView(),
      transcript: [{ id: 'm1', thread: 'th-1', kind: 'user', text: 'what happened?', at: '2026-08-22T10:00:00Z' }],
      events: [],
    },
    conversationGraph: { nodes: [], edges: [], eventNodeKinds: {}, events: [], diverged: false },
    vocabulary: { entries: [] },
    sources: { sources: [], canOriginate: false, writeEnabled: true },
  }
})

// A leaked connection would make every LATER test silently eventless:
// connectStream keeps one per module, so a test that threw before its stop()
// leaves the next one wired to a stream nobody can drive.
let stopStream: (() => void) | undefined

afterEach(() => {
  stopStream?.()
  stopStream = undefined
  vi.useRealTimers()
})

function mount(ui: ReactNode, path = '/') {
  const client = createQueryClient()
  const stop = connectStream(client)
  stopStream = stop
  render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[path]}>{ui}</MemoryRouter>
    </QueryClientProvider>,
  )
  return { client, stop }
}

/** The spinner every page renders while it has nothing. */
function loading() {
  return screen.queryByLabelText('loading')
}

/** Deliver one event the way the server would. */
function emit(name: string, payload: unknown) {
  FakeEventSource.latest().emit(name, payload)
}

// ---- one page at a time -------------------------------------------------------

describe('Overview', () => {
  // 20s, and ONLY this test. Its neighbours assert a delta causes NO refetch, so
  // a frozen clock proves the absence and returns immediately. This one waits
  // for a refetch to LAND while `shouldAdvanceTime` creeps the fake clock at
  // real-time pace — so the wait is real seconds, and the default 5s is a
  // budget, not a bug. It failed on Node 22 and passed on 24, which is the shape
  // of a timeout that is simply too tight rather than a defect either version
  // owns.
  it('updates from a delta without ever showing a spinner', async () => {
    const { OverviewPage } = await import('./Overview')
    const { stop } = mount(<OverviewPage />)
    await screen.findByText('Installation')

    vi.useFakeTimers({ shouldAdvanceTime: true })
    // The install grew a channel. Counts are an aggregate the browser never
    // holds whole, so this view is RE-READ — on a stable key, which is what
    // keeps the painted page on screen while the read lands.
    served.overview = { ...(served.overview as object), counts: { channels: 2 } }
    emit('delta', {
      type: 'ADDED', kind: 'channels', name: 'second',
      row: { name: 'second', health: 'ok', findings: 0 },
    })
    vi.advanceTimersByTime(300)

    await waitFor(() => expect(calls.overview).toBe(2))
    expect(loading()).toBeNull()
    expect(screen.getByText('Installation')).toBeInTheDocument()
    stop()
  }, 20_000)
})

describe('Queues', () => {
  it('applies pushed queue state, with no request and no spinner', async () => {
    const { QueuesPage } = await import('./Queues')
    const { stop } = mount(<QueuesPage />)
    await screen.findByText('conv-1')
    const before = calls.queues

    emit('queues', {
      capacity: { inUse: 3, max: 5, waiting: 1 },
      work: [{ conversation: 'conv-1', queued: 4, ageSeconds: 9 }],
      delivery: [], cooldowns: [],
    })

    await screen.findByText('4')
    expect(loading()).toBeNull()
    // The queue state IS the event. Asking the manager again for what it just
    // sent is the pattern this whole change removes.
    expect(calls.queues).toBe(before)
    stop()
  })
})

describe('Topology', () => {
  it('updates from a delta without a spinner, and keeps its timer for rates only', async () => {
    const { TopologyPage } = await import('./Topology')
    const { stop } = mount(<TopologyPage />)
    await screen.findByText('console')
    expect(loading()).toBeNull()

    vi.useFakeTimers({ shouldAdvanceTime: true })
    emit('delta', {
      type: 'MODIFIED', kind: 'channels', name: 'console',
      row: { name: 'console', health: 'bad', findings: 0 },
    })
    vi.advanceTimersByTime(300)
    await waitFor(() => expect(calls.topology).toBe(2))
    expect(loading()).toBeNull()

    // The one timed refresh it keeps is for RATES: traffic per minute is not
    // wrong because something changed, it is wrong because time passed.
    vi.advanceTimersByTime(10_000)
    await waitFor(() => expect(calls.topology).toBe(3))
    expect(loading()).toBeNull()
    stop()
  })
})

describe('Configuration', () => {
  it('applies a delta into a kind listing with no request and no spinner', async () => {
    const { ConfigKindPage } = await import('./Config')
    const { stop } = mount(
      <Routes>
        <Route path="/config/:kind" element={<ConfigKindPage />} />
      </Routes>,
      '/config/channels',
    )
    await screen.findByText('console')
    const before = calls['inventory:channels']

    emit('delta', {
      type: 'ADDED', kind: 'channels', name: 'slack',
      row: { name: 'slack', health: 'bad', findings: 1, summary: 'adapter slack' },
    })

    await screen.findByText('slack')
    expect(loading()).toBeNull()
    expect(calls['inventory:channels']).toBe(before)
    stop()
  })

  it('applies a delta into a kind DETAIL with no request and no spinner', async () => {
    const { ConfigDetailPage } = await import('./Config')
    const { stop } = mount(
      <Routes>
        <Route path="/config/:kind/:name" element={<ConfigDetailPage />} />
      </Routes>,
      '/config/channels/console',
    )
    await screen.findByText('AdapterReady')
    const before = calls['detail:channels:console']

    emit('delta', {
      type: 'MODIFIED', kind: 'channels', name: 'console',
      row: { name: 'console', health: 'bad', findings: 0 },
      detail: {
        object: { kind: 'channels', metadata: { name: 'console' } }, health: 'bad',
        conditions: [{ type: 'Served', status: 'False', reason: 'NoServingImplementation' }],
        yaml: 'kind: Channel', usedBy: [], findings: [],
      },
    })

    await screen.findByText('NoServingImplementation')
    expect(loading()).toBeNull()
    expect(calls['detail:channels:console']).toBe(before)
    stop()
  })
})

describe('the Conversations list', () => {
  it('shows a conversation appearing, changing phase and being deleted, in place', async () => {
    const { ConversationsPage } = await import('./Conversations')
    const { stop } = mount(<ConversationsPage />)
    await screen.findByText('disk pressure on node-3')
    const before = calls.conversations

    // APPEARING: newest first, no filter — its place is unambiguous.
    emit('delta', {
      type: 'ADDED', kind: 'conversations', name: 'conv-2',
      conversationRow: summary({ name: 'conv-2', title: 'certificate expiring', runs: undefined }),
    })
    await screen.findByText('certificate expiring')
    expect(loading()).toBeNull()

    // CHANGING PHASE.
    emit('delta', {
      type: 'MODIFIED', kind: 'conversations', name: 'conv-2',
      conversationRow: summary({ name: 'conv-2', title: 'certificate expiring', phase: 'Closed', runs: undefined }),
    })
    await screen.findByText('Closed')
    expect(loading()).toBeNull()

    // BEING DELETED.
    emit('delta', { type: 'DELETED', kind: 'conversations', name: 'conv-2' })
    await waitFor(() => expect(screen.queryByText('certificate expiring')).toBeNull())
    expect(loading()).toBeNull()
    expect(screen.getByText('disk pressure on node-3')).toBeInTheDocument()
    expect(calls.conversations).toBe(before)
    stop()
  })
})

describe('one Conversation', () => {
  it('takes a message and a run advancing without blanking, and asks nothing', async () => {
    const { ConversationPage } = await import('./Conversation')
    const { stop } = mount(
      <Routes>
        <Route path="/conversations/:name" element={<ConversationPage />} />
      </Routes>,
      '/conversations/conv-1',
    )
    await screen.findByText('what happened?')
    await waitFor(() => expect(loading()).toBeNull())
    const before = calls['conversation:conv-1']

    // A MESSAGE: the payload is the answer, so nothing is asked.
    emit('message', {
      id: 'm2', thread: 'th-1', kind: 'agent', text: 'node-3 is out of disk', at: '2026-08-22T10:00:05Z',
    })
    await screen.findByText('node-3 is out of disk')
    expect(loading()).toBeNull()

    // A RUN ADVANCING: the conversation object changed, and the page takes the
    // new phase while keeping the thread it is showing.
    emit('delta', {
      type: 'MODIFIED', kind: 'conversations', name: 'conv-1',
      conversationRow: summary({ phase: 'Idle', runs: undefined }),
      conversationView: conversationView({ phase: 'Idle' }),
    })
    await waitFor(() => expect(screen.getByText('node-3 is out of disk')).toBeInTheDocument())
    expect(loading()).toBeNull()
    expect(calls['conversation:conv-1']).toBe(before)
    stop()
  })
})

describe('the composer', () => {
  it('sends and asks for nothing — the echo arrives on the stream', async () => {
    const { ConversationPage } = await import('./Conversation')
    const { stop } = mount(
      <Routes>
        <Route path="/conversations/:name" element={<ConversationPage />} />
      </Routes>,
      '/conversations/conv-1',
    )
    await screen.findByText('what happened?')
    await waitFor(() => expect(loading()).toBeNull())
    useStream.setState({ connected: true })
    const before = calls['conversation:conv-1']

    await userEvent.type(screen.getByLabelText('message'), 'restart it')
    await userEvent.click(screen.getByRole('button', { name: 'Send' }))
    await waitFor(() => expect(calls.send).toBe(1))

    // The manager delivers the message back to this channel, so the bubble
    // arrives like any other. Re-reading the conversation here asked for the
    // heaviest payload on the page — and asked for what was already on its way.
    emit('message', { id: 'm2', thread: 'th-1', kind: 'local', text: 'restart it', at: 't' })
    await screen.findByText('restart it')
    expect(calls['conversation:conv-1']).toBe(before)
    expect(loading()).toBeNull()
    stop()
  })

  // The confirmation that clears `sending…` IS a stream event. With the stream
  // down nothing delivers it, and the bubble would sit unconfirmed until the
  // page was reloaded — so the read is conditioned, not removed.
  it('reads once after a send when the stream is down', async () => {
    const { ConversationPage } = await import('./Conversation')
    const { stop } = mount(
      <Routes>
        <Route path="/conversations/:name" element={<ConversationPage />} />
      </Routes>,
      '/conversations/conv-1',
    )
    await screen.findByText('what happened?')
    await waitFor(() => expect(loading()).toBeNull())
    const before = calls['conversation:conv-1']

    useStream.setState({ connected: false })
    await userEvent.type(screen.getByLabelText('message'), 'is anyone there')
    await userEvent.click(screen.getByRole('button', { name: 'Send' }))

    await waitFor(() => expect(calls['conversation:conv-1']).toBe(before + 1))
    expect(loading()).toBeNull()
    stop()
  })
})

// ---- what applying does NOT weaken --------------------------------------------

describe('a resync still replaces applied state wholesale', () => {
  it('converges to a cold load, so an applier that was wrong is corrected', async () => {
    const { ConfigKindPage } = await import('./Config')
    const { stop } = mount(
      <Routes>
        <Route path="/config/:kind" element={<ConfigKindPage />} />
      </Routes>,
      '/config/channels',
    )
    await screen.findByText('console')
    await waitFor(() => expect(loading()).toBeNull())

    // A delta the browser applies, for a row the SERVER does not have — which
    // is what a wrong applier looks like from the outside.
    emit('delta', {
      type: 'ADDED', kind: 'channels', name: 'ghost',
      row: { name: 'ghost', health: 'ok', findings: 0 },
    })
    await screen.findByText('ghost')

    // A reconnect: the client cannot know what it missed, so it reloads.
    emit('resync', { reason: 'connected', cursor: '0000000000000001' })

    await waitFor(() => expect(screen.queryByText('ghost')).toBeNull())
    // Snapshots stay AUTHORITATIVE. That is the property that makes applying an
    // optimisation over re-reading rather than a second source of truth.
    expect(screen.getByText('console')).toBeInTheDocument()
    expect(loading()).toBeNull()
    stop()
  })

  it('treats a reported history gap the same way, and keeps the gap visible', async () => {
    const { TopologyPage } = await import('./Topology')
    const { stop } = mount(<TopologyPage />)
    await screen.findByText('console')
    await waitFor(() => expect(loading()).toBeNull())
    const before = calls.topology ?? 0

    // The manager could not serve the cursor: history either side of this is
    // not consecutive.
    served.topology = {
      ...(served.topology as Record<string, unknown>),
      stream: {
        connected: true, events: 0, resyncs: 1,
        lastGap: { ts: '2026-08-22T10:00:00Z', detail: 'manager restarted' },
      },
    }
    emit('activity', { cursor: '', ts: 't', kind: ACTIVITY_GAP, status: 'ok' })

    // A reload, because the gap means events were missed — and the gap itself
    // lives on the stream HEALTH, so a browser opened afterwards still sees it.
    await waitFor(() => expect(calls.topology).toBeGreaterThan(before))
    expect(loading()).toBeNull()
    stop()
  })
})

// ---- the rule itself ----------------------------------------------------------

describe('after first paint, no event puts a page back into loading', () => {
  // `painted` marks first paint; `stays` is the page's own heading, which no
  // event may take away. They differ because an event legitimately CHANGES
  // content — a pushed queue state can empty the work table — and what this
  // rule is about is the page never going back to a spinner.
  const pages: [string, () => Promise<{ ui: ReactNode; path: string; painted: string; stays: string }>][] = [
    [
      'Overview',
      async () => {
        const { OverviewPage } = await import('./Overview')
        return { ui: <OverviewPage />, path: '/', painted: 'Installation', stays: 'Installation' }
      },
    ],
    [
      'Queues',
      async () => {
        const { QueuesPage } = await import('./Queues')
        return { ui: <QueuesPage />, path: '/', painted: 'conv-1', stays: 'Queues and capacity' }
      },
    ],
    [
      'Topology',
      async () => {
        const { TopologyPage } = await import('./Topology')
        return { ui: <TopologyPage />, path: '/', painted: 'console', stays: 'Topology' }
      },
    ],
    [
      'Configuration',
      async () => {
        const { ConfigPage } = await import('./Config')
        return { ui: <ConfigPage />, path: '/config', painted: 'Channel', stays: 'Configuration' }
      },
    ],
    [
      'Conversations',
      async () => {
        const { ConversationsPage } = await import('./Conversations')
        return { ui: <ConversationsPage />, path: '/conversations', painted: 'disk pressure on node-3', stays: 'Conversations' }
      },
    ],
    [
      'Conversation',
      async () => {
        const { ConversationPage } = await import('./Conversation')
        return {
          ui: (
            <Routes>
              <Route path="/conversations/:name" element={<ConversationPage />} />
            </Routes>
          ),
          path: '/conversations/conv-1',
          painted: 'what happened?',
          stays: 'disk pressure on node-3',
        }
      },
    ],
  ]

  for (const [name, load] of pages) {
    it(`holds its content on ${name}`, async () => {
      const { ui, path, painted, stays } = await load()
      const { stop } = mount(ui, path)
      await screen.findByText(painted)
      // Settle FIRST: every query this page opens has answered, so any spinner
      // after this point is a view going BACK to loading rather than one still
      // arriving. First load is an allowed reason; this rule is about what
      // happens after it.
      await waitFor(() => expect(loading()).toBeNull())

      vi.useFakeTimers({ shouldAdvanceTime: true })
      // Every event kind, one after another, including the ones this page has
      // no interest in — a burst is the normal shape, and a burst is what used
      // to strobe.
      emit('delta', {
        type: 'MODIFIED', kind: 'channels', name: 'console',
        row: { name: 'console', health: 'bad', findings: 0 },
      })
      emit('delta', {
        type: 'MODIFIED', kind: 'conversations', name: 'conv-1',
        conversationRow: summary({ phase: 'Idle', runs: undefined }),
        conversationView: conversationView({ phase: 'Idle' }),
      })
      emit('delta', { type: 'DELETED', kind: 'channels', name: 'slack' })
      emit('activity', { cursor: '0000000000000009', ts: 't', kind: 'run.dispatched', status: 'ok' })
      emit('queues', { capacity: { inUse: 1, max: 5, waiting: 0 }, work: [], delivery: [], cooldowns: [] })
      emit('message', { id: 'm9', thread: 'th-1', kind: 'agent', text: 'still here', at: 't' })
      vi.advanceTimersByTime(1_000)

      expect(loading()).toBeNull()
      // And it stays gone once every re-read those events triggered has landed.
      await waitFor(() => expect(screen.getAllByText(stays).length).toBeGreaterThan(0))
      expect(loading()).toBeNull()
      stop()
    })
  }
})

// DELIBERATE FAILURE for sdlc-setup 2.8. Reverted on the next commit.
it('deliberate failure: proving CI attributes a UI test failure to console-ui', () => {
  expect(1).toBe(2)
})
