import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { ThreadPane } from './ThreadPane'
import type { ConversationDetail, ConversationSummary } from '../../api/types'

function conv(name: string, over: Partial<ConversationSummary> = {}): ConversationSummary {
  return {
    name, runCount: 0, queued: 0, joined: true, errored: false, unread: false,
    ageSeconds: 10, deleting: false, phase: 'Idle', threads: [{ channel: 'console', threadId: 't' }],
    ...over,
  }
}

function detailFor(summary: ConversationSummary, over: Partial<ConversationDetail> = {}): ConversationDetail {
  return {
    conversation: summary,
    object: { kind: 'Conversation', metadata: { name: summary.name } },
    yaml: 'kind: Conversation',
    transcript: [],
    archived: false,
    events: [],
    ...over,
  }
}

const ROOT = conv('root-1', {
  coordinator: 'root-1', escalatedAt: '2024-01-01T00:02:00Z',
  budget: { maxTurns: 6, turns: 2 }, runs: [{ runId: 'run-1', status: 'succeeded', startedAt: '2024-01-01T00:00:00Z' }],
})
const MID = conv('mid-1', { causedBy: { parent: 'root-1', entry: 'diagnose' }, coordinator: 'mid-1', created: '2024-01-01T00:00:30Z' })
const LEAF = conv('leaf-1', { causedBy: { parent: 'mid-1', entry: 'logs' }, phase: 'Closed', created: '2024-01-01T00:00:40Z' })
const PLAIN = conv('plain-1', {
  title: 'ordinary conversation',
  runs: [{ runId: 'plain-run-1', status: 'succeeded', finishedAt: '2024-01-01T00:01:00Z' }],
})

const ALL = [ROOT, MID, LEAF, PLAIN]
const DETAILS: Record<string, ConversationDetail> = {
  'root-1': detailFor(ROOT),
  'mid-1': detailFor(MID),
  'leaf-1': detailFor(LEAF),
  'plain-1': detailFor(PLAIN, { transcript: [{ id: 'm1', thread: 't', kind: 'agent', text: 'hello', at: '2024-01-01T00:00:00Z' }] }),
}

vi.mock('../../api/hooks', () => ({
  useConversation: (name: string) => ({ data: DETAILS[name], isLoading: false, error: null }),
  useConversations: () => ({ data: { items: ALL, total: ALL.length, unreadTotal: 0, offset: 0, limit: 200, facets: {} }, isLoading: false, error: null }),
  useConversationGraph: () => ({ data: undefined, isLoading: false, error: 'not needed here' }),
  useMarkRead: () => ({ mutate: vi.fn() }),
  useSession: () => ({ data: { canWrite: true, canOriginate: true } }),
  useSources: () => ({ data: { sources: [] } }),
  useTopology: () => ({ data: undefined, isLoading: false, error: null }),
  useVocabulary: () => ({ data: { entries: [] } }),
}))

vi.mock('../../api/stream', () => ({
  useStream: (selector: (s: { connected: boolean; events: never[] }) => unknown) => selector({ connected: true, events: [] }),
  eventsFor: () => [],
}))

vi.mock('../../graph/Graph', () => ({ Graph: () => <div data-testid="graph-stub" /> }))
vi.mock('../../graph/display', () => ({ useDisplay: () => 60 }))

function renderPane(name: string) {
  return render(
    <MemoryRouter>
      <ThreadPane name={name} />
    </MemoryRouter>,
  )
}

describe('secondary views replace the transcript in place', () => {
  it('switches to Runs and back', async () => {
    renderPane('plain-1')
    expect(screen.getByText('hello')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Runs' }))
    expect(screen.queryByText('hello')).toBeNull()
    expect(screen.getByText('plain-run-1')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Back to transcript' }))
    expect(screen.getByText('hello')).toBeInTheDocument()
  })
})

describe('the incident timeline, two levels deep', () => {
  it('reads as one column — the run, then the member, in time order', () => {
    renderPane('root-1')
    const body = document.body.textContent ?? ''
    expect(body.indexOf('run-1')).toBeLessThan(body.indexOf('mid-1'))
  })

  it('nests a coordinating member one level deeper when expanded', async () => {
    renderPane('root-1')
    await userEvent.click(screen.getByText('mid-1'))
    // leaf-1 is mid-1's own member, found only once mid-1 is expanded
    expect(screen.getAllByText(/leaf-1/).length).toBeGreaterThan(0)
  })
})

describe('a member opened directly', () => {
  it('is read-only and points at its incident rather than showing a composer', () => {
    renderPane('leaf-1')
    expect(screen.getByText(/holds no channel of its own/)).toBeInTheDocument()
    expect(screen.queryByLabelText('message')).toBeNull()
  })
})
