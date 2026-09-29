import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { QueryClientProvider } from '@tanstack/react-query'
import { createQueryClient } from '../api/queryClient'
import { ConversationPage } from './Conversation'
import type { ConversationSummary } from '../api/types'

// The Incident tab (design D-G, tasks.md 5.3): a root's interleaved timeline,
// a member's own accordion expanding lazily, a nested sub-coordinator
// recursing in place, and the un-escalated-closure chip on the ordinary
// header every conversation carries.
//
// Real hooks against a mocked `api/client`, the same recipe live.test.tsx
// uses for this same page — a fake DOM and a real cache is what catches a
// hook wired to the wrong query key, which a shallow render cannot.

let served: Record<string, unknown>

function serve<T>(key: string): Promise<T> {
  const v = served[key]
  if (v === undefined) throw new Error(`test fixture asked for unserved key: ${key}`)
  return Promise.resolve(structuredClone(v) as T)
}

vi.mock('../api/client', () => ({
  ApiError: class ApiError extends Error {},
  api: {
    session: () => serve('session'),
    conversation: (name: string) => serve(`conversation:${name}`),
    conversations: () => serve('conversations'),
    conversationGraph: () => serve('conversationGraph'),
    topology: () => serve('topology'),
    vocabulary: () => serve('vocabulary'),
  },
}))

function summary(over: Partial<ConversationSummary> & { name: string }): ConversationSummary {
  return {
    title: over.name, phase: 'Idle', runCount: 0, queued: 0, joined: false,
    errored: false, unread: false, ageSeconds: 60, deleting: false, ...over,
  }
}

function conversationView(over: Partial<ConversationSummary> & { name: string }) {
  return {
    conversation: summary(over),
    object: { kind: 'conversations', metadata: { name: over.name } },
    yaml: 'kind: Conversation',
    transcript: [],
    events: [],
    archived: false,
  }
}

beforeEach(() => {
  served = {
      session: { canWrite: false, writeEnabled: false, configured: true, authMode: 'token', authenticated: true, identity: '', identitySource: '', externalAuthenticator: '', canOriginate: false, metrics: false },
      vocabulary: { entries: [] },
      conversationGraph: { nodes: [], edges: [], eventNodeKinds: {}, events: [], diverged: false },
      topology: {
        topology: { nodes: [], edges: [], eventNodeKinds: {} },
        consoleChannel: 'console', unjoinedPipelines: null, synced: {},
        stream: { connected: true, events: 0, resyncs: 0 }, oldestEvent: '', metricsAvailable: false,
      },
      // ONE list serves every "who are my members" lookup: CoordinatorIncident
      // fetches with no filter beyond `limit`, so one fixed page covers the
      // whole tree under test.
      conversations: {
        items: [
          summary({ name: 'root-1', coordinator: 'root-1', phase: 'Running' }),
          summary({
            name: 'member-1', phase: 'Idle', brief: 'Checked node-7 for disk pressure.',
            causedBy: { parent: 'root-1', entry: 'triage' }, created: '2026-01-01T00:00:30Z',
          }),
          summary({
            name: 'member-2', phase: 'Running', coordinator: 'member-2', brief: 'Coordinating the mitigation.',
            causedBy: { parent: 'root-1', entry: 'mitigate' }, created: '2026-01-01T00:00:45Z',
          }),
          summary({
            name: 'member-2a', phase: 'Idle', brief: 'Drained node-7.',
            causedBy: { parent: 'member-2', entry: 'drain' }, created: '2026-01-01T00:01:00Z',
          }),
        ],
        total: 4, unreadTotal: 0, offset: 0, limit: 200, facets: {},
      },
      'conversation:root-1': conversationView({
        name: 'root-1', coordinator: 'root-1', phase: 'Running', profile: 'incident-lead',
        runs: [{ runId: 'r-root', status: 'succeeded', startedAt: '2026-01-01T00:00:00Z', finishedAt: '2026-01-01T00:00:10Z' }],
        budget: { maxAgents: 5, agentsInvoked: 2, maxTurns: 20, turns: 4 },
      }),
      'conversation:member-1': conversationView({
        name: 'member-1', phase: 'Idle', profile: 'k8s-engineer', brief: 'Checked node-7 for disk pressure.',
        causedBy: { parent: 'root-1', entry: 'triage' },
        runs: [{ runId: 'r-m1', status: 'succeeded', startedAt: '2026-01-01T00:00:31Z', finishedAt: '2026-01-01T00:00:40Z' }],
      }),
      'conversation:member-2': conversationView({
        name: 'member-2', phase: 'Running', coordinator: 'member-2', profile: 'incident-lead',
        brief: 'Coordinating the mitigation.', causedBy: { parent: 'root-1', entry: 'mitigate' },
        runs: [{ runId: 'r-m2', status: 'succeeded', startedAt: '2026-01-01T00:00:46Z', finishedAt: '2026-01-01T00:00:50Z' }],
      }),
      'conversation:member-2a': conversationView({
        name: 'member-2a', phase: 'Idle', profile: 'k8s-engineer', brief: 'Drained node-7.',
        causedBy: { parent: 'member-2', entry: 'drain' },
        runs: [{ runId: 'r-m2a', status: 'succeeded', startedAt: '2026-01-01T00:01:01Z', finishedAt: '2026-01-01T00:01:05Z' }],
      }),
  }
})

function mount(name: string, query = '') {
  const client = createQueryClient()
  render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[`/conversations/${name}${query}`]}>
        <Routes>
          <Route path="/conversations/:name" element={<ConversationPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('the Incident tab', () => {
  it('is absent from a plain conversation', async () => {
    served['conversation:plain'] = conversationView({ name: 'plain', phase: 'Idle' })
    mount('plain')
    await screen.findAllByText('plain')
    expect(screen.queryByRole('tab', { name: 'Incident' })).toBeNull()
  })

  it('shows a banner on a member, linking to its root', async () => {
    mount('member-1', '?tab=incident')
    // The banner is the whole tab body for a plain member — no timeline of
    // its own, since the real one lives on the root.
    await screen.findByText(/This is a member conversation/)
    expect(screen.getByText('triage')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'root-1' })).toHaveAttribute(
      'href',
      '/conversations/root-1?tab=incident',
    )
  })

  it('interleaves the root\'s own runs with its members, collapsed by default', async () => {
    mount('root-1', '?tab=incident')
    await screen.findByText('r-root')
    // The budget is a fact about the ROOT, shown once. Queried against the
    // page's text rather than one element: PatternFly's DescriptionList marks
    // up the term and the value as siblings, so nothing on the page carries
    // "Agents invoked" and "2 of 5" as one node's own text.
    await screen.findByText('Agents invoked')
    expect(document.body.textContent).toContain('2 of 5')
    // Both direct members are on the timeline as collapsed rows...
    expect(screen.getByText('Checked node-7 for disk pressure.')).toBeInTheDocument()
    expect(screen.getByText('Coordinating the mitigation.')).toBeInTheDocument()
    expect(screen.getByText('triage')).toBeInTheDocument()
    expect(screen.getByText('mitigate')).toBeInTheDocument()
    // ...and a member's OWN runs are not fetched until it is expanded.
    expect(screen.queryByText('r-m1')).toBeNull()
  })

  it('expands a member to show its own run history, lazily', async () => {
    mount('root-1', '?tab=incident')
    await screen.findByText('Checked node-7 for disk pressure.')
    await userEvent.click(screen.getByText('Checked node-7 for disk pressure.'))
    await screen.findByText('r-m1')
  })

  it('recurses into a member that is itself a nested Coordinator\'s root', async () => {
    mount('root-1', '?tab=incident')
    await screen.findByText('Coordinating the mitigation.')
    await userEvent.click(screen.getByText('Coordinating the mitigation.'))
    // member-2's OWN run, and its OWN member (member-2a) nested one level
    // further in, in place — never a second navigation.
    await screen.findByText('r-m2')
    await screen.findByText('Drained node-7.')
    expect(screen.getByText('drain')).toBeInTheDocument()
  })
})

describe('an un-escalated closure', () => {
  it('marks the closeReason distinctly from an escalated close', async () => {
    served['conversation:closed-quiet'] = conversationView({
      name: 'closed-quiet', phase: 'Closed', closeReason: 'budget exceeded',
    })
    mount('closed-quiet')
    await within(await screen.findByRole('heading', { level: 1 })).findByText(/closed-quiet/)
    expect(screen.getByText('closed: budget exceeded')).toBeInTheDocument()
  })

  it('says nothing when the closure escalated a thread', async () => {
    served['conversation:closed-loud'] = conversationView({
      name: 'closed-loud', phase: 'Closed', closeReason: 'budget exceeded', escalatedAt: '2026-01-01T00:02:00Z',
    })
    mount('closed-loud')
    await within(await screen.findByRole('heading', { level: 1 })).findByText(/closed-loud/)
    expect(screen.queryByText(/closed: budget exceeded/)).toBeNull()
  })
})
