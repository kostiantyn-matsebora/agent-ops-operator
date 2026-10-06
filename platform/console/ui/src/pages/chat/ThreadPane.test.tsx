import { beforeEach, describe, expect, it, vi } from 'vitest'
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

// No bound channel — `joined` explicit rather than the helper's default
// `true`, since the two are independent fields here and a root with no
// console thread is exactly the "genuinely no bound channel" case (item 6).
const ROOT = conv('root-1', {
  coordinator: 'root-1', budget: { maxTurns: 6, turns: 2 }, threads: [], joined: false,
})
// A direct member of the root, invoked as "diagnose" — its own task and
// result live on ITS OWN runs[], never on the root's.
const MID = conv('mid-1', {
  causedBy: { parent: 'root-1', entry: 'diagnose' }, created: '2024-01-01T00:00:30Z', phase: 'Closed', threads: [],
  runs: [{
    runId: 'run-mid', status: 'succeeded', finishedAt: '2024-01-01T00:01:00Z',
    inputs: [{ id: 'in-1', text: 'investigate the disk pressure on node-3', receivedAt: '2024-01-01T00:00:31Z' }],
    result: 'disk pressure cleared on node-3',
    // A member's own turns/tool calls — item 16 QA: the backend always
    // recorded these for a member exactly as for a root, but no tab ever
    // showed them. Shaped on the real data measured live on member-xw4h5.
    turns: [{ model: 'claude-5', tokensIn: 900, tokensOut: 120, stopReason: 'end_turn' }],
    toolCalls: [{ tool: 'mcp__ha__get_states', server: 'ha', durationMs: 220, resultBytes: 4096 }],
  }],
})
const PLAIN = conv('plain-1', {
  title: 'ordinary conversation',
  runs: [{
    runId: 'run-plain', status: 'succeeded', finishedAt: '2024-01-01T00:02:00Z',
    result: 'plain result text with no turns or tool calls',
  }],
})
// A run carrying both arrays the backend may now report (item 16) — one
// turn, and two tool calls, one of them a built-in (empty `server`).
const RICH = conv('rich-1', {
  title: 'rich conversation',
  runs: [{
    runId: 'run-rich', status: 'succeeded', finishedAt: '2024-01-01T00:03:00Z',
    result: 'rich result text',
    turns: [
      { model: 'claude-5', tokensIn: 1200, tokensOut: 340, cacheReadTokens: 800, stopReason: 'end_turn' },
    ],
    toolCalls: [
      { tool: 'Read', server: '', durationMs: 410, resultBytes: 1234 },
      { tool: 'mcp__k8s__get_pods', server: 'kubernetes', durationMs: 1500, resultBytes: 2_500_000 },
    ],
  }],
})

// A root whose Coordinator's channelRefs are already bound (item 6):
// `joined: true` is exactly what `coordination-escalation`'s unconditional
// creation-time binding produces, with no escalation needed.
const ROOT_JOINED = conv('root-joined', {
  coordinator: 'root-joined', budget: { maxTurns: 4, turns: 1 },
  threads: [{ channel: 'console', threadId: 't-root' }], joined: true,
})

// Item #15 QA, bug 1: a member created 35 SECONDS into its invoking run,
// which does not finish — and record its own reasoning — for almost two more
// minutes. Shaped on the real timestamps measured live on task-mcvfd: a
// member created at 19:05:33, its invoking run started 19:04:58 and finished
// 19:07:04. Sorting the invoke card by `member.created` alone would place it
// BEFORE its own run's reasoning text, reversing decide-then-act.
const ROOT_INVOKE_ORDER = conv('root-invoke-order', {
  coordinator: 'root-invoke-order', threads: [], joined: false,
  runs: [{
    runId: 'r0', status: 'succeeded',
    startedAt: '2024-01-01T19:04:58Z', finishedAt: '2024-01-01T19:07:04Z',
    result: 'deciding to check on ha',
  }],
})
const MID_INVOKE_ORDER = conv('mid-invoke-order', {
  causedBy: { parent: 'root-invoke-order', entry: 'ha-ops' },
  created: '2024-01-01T19:05:33Z', phase: 'Closed', threads: [],
  runs: [{
    runId: 'run-ha', status: 'succeeded', finishedAt: '2024-01-01T19:06:00Z',
    inputs: [{ id: 'in-ha', text: 'what is the status of ha?', receivedAt: '2024-01-01T19:05:34Z' }],
    result: 'ha is healthy',
  }],
})

// Measured live on task-58qx8 (2026-10-05): `run.finishedAt` (the manager's
// own recorded stamp, 21:19:10) and the reasoning message's own `at` (the
// console's live buffer, timestamped at RECEIPT — 21:19:12, two real seconds
// later) are two different clocks. Anchoring the invoke card to
// `run.finishedAt` therefore still sorted it BEFORE the reasoning even once
// both were populated — this fixture pins that exact mismatch rather than
// the coincidental case above where the two happened to already agree.
const ROOT_CLOCK_SKEW = conv('root-clock-skew', {
  coordinator: 'root-clock-skew', threads: [], joined: false,
  runs: [{
    runId: 'r0', status: 'succeeded',
    startedAt: '2026-10-05T21:18:38Z', finishedAt: '2026-10-05T21:19:10Z',
    result: 'request sent to ha-control',
  }],
})
const MID_CLOCK_SKEW = conv('mid-clock-skew', {
  causedBy: { parent: 'root-clock-skew', entry: 'ha-control' },
  created: '2026-10-05T21:19:04Z', threads: [],
})

const ALL = [ROOT, MID, PLAIN, RICH, ROOT_JOINED, ROOT_INVOKE_ORDER, MID_INVOKE_ORDER, ROOT_CLOCK_SKEW, MID_CLOCK_SKEW]
const DETAILS: Record<string, ConversationDetail> = {
  'root-1': detailFor(ROOT, {
    transcript: [
      { id: 'sig-1', thread: 'root-1', kind: 'signal', text: 'Disk pressure on node-3', at: '2024-01-01T00:00:00Z' },
      { id: 'reason-1', thread: 'root-1', kind: 'agent', text: 'Investigating node-3 for disk pressure causes', at: '2024-01-01T00:00:10Z' },
      { id: 'reason-2', thread: 'root-1', kind: 'agent', text: 'Node-3 disk pressure resolved after eviction', at: '2024-01-01T00:00:50Z' },
    ],
  }),
  'mid-1': detailFor(MID),
  'plain-1': detailFor(PLAIN, { transcript: [{ id: 'm1', thread: 't', kind: 'agent', text: 'hello', at: '2024-01-01T00:00:00Z' }] }),
  'rich-1': detailFor(RICH),
  'root-joined': detailFor(ROOT_JOINED),
  'root-invoke-order': detailFor(ROOT_INVOKE_ORDER, {
    transcript: [
      { id: 'run:r0', thread: 'root-invoke-order', kind: 'agent', text: 'deciding to check on ha', at: '2024-01-01T19:07:04Z' },
    ],
  }),
  'mid-invoke-order': detailFor(MID_INVOKE_ORDER),
  'root-clock-skew': detailFor(ROOT_CLOCK_SKEW, {
    transcript: [
      { id: 'run:r0', thread: 'root-clock-skew', kind: 'agent', text: 'request sent to ha-control', at: '2026-10-05T21:19:12Z' },
    ],
  }),
  'mid-clock-skew': detailFor(MID_CLOCK_SKEW),
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

const sendSpy = vi.fn().mockResolvedValue({ id: 'sent' })
vi.mock('../../api/client', () => ({
  api: { send: (...args: unknown[]) => sendSpy(...args) },
  ApiError: class ApiError extends Error {},
}))

beforeEach(() => {
  sendSpy.mockClear()
  localStorage.clear()
})

function renderPane(name: string) {
  return render(
    <MemoryRouter>
      <ThreadPane name={name} />
    </MemoryRouter>,
  )
}

describe('secondary views replace the transcript in place', () => {
  it('switches to Runs and back — a plain pipeline conversation is unchanged', async () => {
    renderPane('plain-1')
    expect(screen.getByText('hello')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Runs' }))
    expect(screen.queryByText('hello')).toBeNull()
    await userEvent.click(screen.getByRole('button', { name: 'Back to transcript' }))
    expect(screen.getByText('hello')).toBeInTheDocument()
  })
})

describe('a coordinator root gets a real transcript, not a structural view (item 15)', () => {
  it('shows the budget summary and an invocation collapsed by default', () => {
    renderPane('root-1')
    expect(screen.getByText('Agents invoked')).toBeInTheDocument()
    expect(screen.getByText('2 of 6')).toBeInTheDocument()
    expect(screen.getByText('diagnose')).toBeInTheDocument()
    expect(screen.getByText('mid-1')).toBeInTheDocument()
    // Collapsed: neither the task nor the result is on screen yet.
    expect(screen.queryByText(/investigate the disk pressure/)).toBeNull()
    expect(screen.queryByText(/disk pressure cleared/)).toBeNull()
  })

  it('expanding the invocation reveals both the task sent and the response', async () => {
    renderPane('root-1')
    await userEvent.click(screen.getByText('diagnose'))
    expect(screen.getByText('Task sent')).toBeInTheDocument()
    expect(screen.getByText(/investigate the disk pressure on node-3/)).toBeInTheDocument()
    expect(screen.getByText(/responded/)).toBeInTheDocument()
    expect(screen.getByText(/disk pressure cleared on node-3/)).toBeInTheDocument()
  })

  it('interleaves its own signal and reasoning turns with the invocation card, in time order', () => {
    // root-1's transcript carries a signal at :00, a reasoning turn at :10,
    // the diagnose invocation at :30 (MID's `created`), then a second
    // reasoning turn at :50 — ONE column, not a status log beside it.
    renderPane('root-1')
    const timeline = screen.getByTestId('timeline')
    const text = timeline.textContent ?? ''
    expect(text.indexOf('Disk pressure on node-3')).toBeLessThan(text.indexOf('Investigating node-3'))
    expect(text.indexOf('Investigating node-3')).toBeLessThan(text.indexOf('invoked'))
    expect(text.indexOf('invoked')).toBeLessThan(text.indexOf('resolved after eviction'))
  })

  it('places the invoke card AFTER the reasoning of the run that invoked it, not before (item 15, bug 1)', () => {
    // The member was created 35s into its invoking run, which does not
    // record its own reasoning until it finishes almost two minutes later —
    // sorting by `member.created` alone would put the card first.
    renderPane('root-invoke-order')
    const timeline = screen.getByTestId('timeline')
    const text = timeline.textContent ?? ''
    expect(text).toContain('deciding to check on ha')
    expect(text).toContain('ha-ops')
    expect(text.indexOf('deciding to check on ha')).toBeLessThan(text.indexOf('ha-ops'))
  })

  it('still orders reasoning before the invoke card when run.finishedAt does not match the reasoning message\'s own timestamp', () => {
    // root-clock-skew: run.finishedAt is 21:19:10, but the reasoning message
    // that run actually produced renders at 21:19:12 (the console's live
    // buffer stamp). Anchoring to run.finishedAt directly would place the
    // invoke card BETWEEN those two times — still ahead of the reasoning.
    renderPane('root-clock-skew')
    const timeline = screen.getByTestId('timeline')
    const text = timeline.textContent ?? ''
    expect(text).toContain('request sent to ha-control')
    expect(text).toContain('invoked')
    expect(text.indexOf('request sent to ha-control')).toBeLessThan(text.indexOf('invoked'))
  })
})

describe('a member opened directly (item 15)', () => {
  it('shows its task and result as read-only messages, with no composer', () => {
    renderPane('mid-1')
    expect(screen.getByText(/holds no channel of its own/)).toBeInTheDocument()
    expect(screen.getByText(/investigate the disk pressure on node-3/)).toBeInTheDocument()
    expect(screen.getByText(/disk pressure cleared on node-3/)).toBeInTheDocument()
    expect(screen.queryByLabelText('message')).toBeNull()
  })

  it('names the coordinator it belongs to in the breadcrumb, never an "Incident" noun (item 14, item 26)', () => {
    renderPane('mid-1')
    expect(screen.queryByText('Incident')).not.toBeInTheDocument()
    // The uncaused root's own coordinator name leads the crumb, and the
    // parent trail names the same root — both read "root-1" in this fixture.
    expect(screen.getAllByText('root-1').length).toBeGreaterThan(0)
  })

  it('has its own Runs tab, showing its own turns and tool calls (item 16 QA)', async () => {
    renderPane('mid-1')
    await userEvent.click(screen.getByRole('button', { name: 'Runs' }))
    expect(screen.getByTestId('runs-view')).toBeInTheDocument()
    expect(screen.getByText('claude-5')).toBeInTheDocument()
    expect(screen.getByText('end_turn')).toBeInTheDocument()
    expect(screen.getByText('mcp__ha__get_states')).toBeInTheDocument()
  })

  it('offers no Graph or Sequence tab — those read a conversation\'s own topology, which for a member is one node its parent already shows in context', () => {
    renderPane('mid-1')
    expect(screen.queryByRole('button', { name: 'Graph' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Sequence' })).not.toBeInTheDocument()
  })
})

describe('a run\'s turns and tool calls, in the Runs view (item 16)', () => {
  it('shows the raw text and no tables when a run reports neither', async () => {
    renderPane('plain-1')
    await userEvent.click(screen.getByRole('button', { name: 'Runs' }))
    expect(screen.getByText('plain result text with no turns or tool calls')).toBeInTheDocument()
    expect(screen.queryByText(/turns —/)).toBeNull()
    expect(screen.queryByText(/tool calls —/)).toBeNull()
  })

  it('renders both tables with the reported values when a run has both', async () => {
    renderPane('rich-1')
    await userEvent.click(screen.getByRole('button', { name: 'Runs' }))
    expect(screen.getByText('rich result text')).toBeInTheDocument()

    expect(screen.getByText('turns — 1 model call')).toBeInTheDocument()
    expect(screen.getByText('claude-5')).toBeInTheDocument()
    expect(screen.getByText('1200')).toBeInTheDocument()
    expect(screen.getByText(/800 cached/)).toBeInTheDocument()
    expect(screen.getByText('340')).toBeInTheDocument()
    expect(screen.getByText('end_turn')).toBeInTheDocument()

    expect(screen.getByText('tool calls — 2')).toBeInTheDocument()
    expect(screen.getByText('Read')).toBeInTheDocument()
    expect(screen.getByText('410ms')).toBeInTheDocument()
    expect(screen.getByText('1.2 KB')).toBeInTheDocument()
    expect(screen.getByText('mcp__k8s__get_pods')).toBeInTheDocument()
    expect(screen.getByText('kubernetes')).toBeInTheDocument()
    expect(screen.getByText('1.5s')).toBeInTheDocument()
    expect(screen.getByText('2.4 MB')).toBeInTheDocument()
  })

  it('names an empty server "built-in" rather than leaving a blank cell', async () => {
    renderPane('rich-1')
    await userEvent.click(screen.getByRole('button', { name: 'Runs' }))
    expect(screen.getByText('built-in')).toBeInTheDocument()
  })
})

describe('the composer follows channel binding, not escalation (item 6)', () => {
  it('is live on a coordinator root that has never escalated, once its channel is bound', () => {
    renderPane('root-joined')
    expect(screen.getByLabelText('message')).toBeInTheDocument()
  })

  it('stays read-only on a root with no bound channel — the spec\'s other permitted case', () => {
    renderPane('root-1')
    expect(screen.queryByLabelText('message')).toBeNull()
  })
})

describe('closing from the composer asks first (item 22)', () => {
  it('typing /close and sending opens a confirm dialog instead of sending at once', async () => {
    renderPane('plain-1')
    await userEvent.type(screen.getByLabelText('message'), '/close')
    await userEvent.click(screen.getByRole('button', { name: 'Send' }))
    expect(screen.getByTestId('close-confirm-modal')).toBeInTheDocument()
    expect(sendSpy).not.toHaveBeenCalled()
  })

  it('cancelling sends nothing and leaves the composer as it was', async () => {
    renderPane('plain-1')
    await userEvent.type(screen.getByLabelText('message'), '/close')
    await userEvent.click(screen.getByRole('button', { name: 'Send' }))
    await userEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(screen.queryByTestId('close-confirm-modal')).toBeNull()
    expect(sendSpy).not.toHaveBeenCalled()
  })

  it('confirming sends the command', async () => {
    renderPane('plain-1')
    await userEvent.type(screen.getByLabelText('message'), '/close')
    await userEvent.click(screen.getByRole('button', { name: 'Send' }))
    await userEvent.click(screen.getByTestId('close-confirm-ok'))
    expect(sendSpy).toHaveBeenCalledWith('plain-1', '/close')
  })

  it('"don\'t ask again" is remembered — a later /close sends straight away', async () => {
    renderPane('plain-1')
    await userEvent.type(screen.getByLabelText('message'), '/close')
    await userEvent.click(screen.getByRole('button', { name: 'Send' }))
    await userEvent.click(screen.getByLabelText('Don\'t ask again'))
    await userEvent.click(screen.getByTestId('close-confirm-ok'))
    expect(sendSpy).toHaveBeenCalledTimes(1)

    sendSpy.mockClear()
    await userEvent.type(screen.getByLabelText('message'), '/close now')
    await userEvent.click(screen.getByRole('button', { name: 'Send' }))
    expect(screen.queryByTestId('close-confirm-modal')).toBeNull()
    expect(sendSpy).toHaveBeenCalledWith('plain-1', '/close now')
  })

  it('/exit needs no confirmation at all — it only releases the runtime', async () => {
    renderPane('plain-1')
    await userEvent.type(screen.getByLabelText('message'), '/exit')
    await userEvent.click(screen.getByRole('button', { name: 'Send' }))
    expect(screen.queryByTestId('close-confirm-modal')).toBeNull()
    expect(sendSpy).toHaveBeenCalledWith('plain-1', '/exit')
  })
})
