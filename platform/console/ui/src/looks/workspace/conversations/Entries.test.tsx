import { describe, expect, it, vi } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import type { ActivityEvent, ConversationSummary, Message } from '../../../api/types'
import { SEVERITIES } from '../../../model/message'
import type { RunDetails as RunDetailsFact } from '../../../model/run'
import { TESTID } from '../../../model/testids'
import { INVOKED_TESTID, PersonMessage, Reply, SignalCard, WorkingCard } from './Entries'
import { RUN_DETAILS_TESTID } from './RunDetails'

const exchange = vi.hoisted(() => ({ value: { isLoading: false, error: undefined as unknown, loaded: true, conversation: {}, task: { text: 'check the disk' }, result: { text: 'disk is fine' } } }))
vi.mock('../../../model/thread', async (importOriginal) => ({ ...(await importOriginal<typeof import('../../../model/thread')>()), useMemberExchange: () => exchange.value }))

const msg = (over: Partial<Message>): Message => ({ id: 'm1', thread: 't', kind: 'agent', text: '', at: '2025-06-11T09:39:00Z', ...over })
const ev = (kind: string, ts: string): ActivityEvent => ({ cursor: ts, ts, kind, runId: 'run-1', status: 'ok' } as ActivityEvent)
const wrap = (node: React.ReactNode) => render(<MemoryRouter>{node}</MemoryRouter>)

describe('a signal card', () => {
  it.each(SEVERITIES)('draws severity %s as an icon, never a chip', (severity) => {
    wrap(<SignalCard message={msg({ kind: 'signal', text: 'Disk is full.', severity })} />)
    const card = screen.getByTestId(TESTID.message)
    const icon = within(card).getByRole('img', { name: `${severity} signal` })
    expect(icon).toHaveAttribute('data-icon', severity)
    // No chip, no pill, and the word itself is not printed as a label.
    expect(card.querySelector('.wk-chip, .wk-pill')).toBeNull()
    expect(within(card).queryByText(severity)).toBeNull()
  })

  it('shows no severity icon for an unrated signal', () => {
    wrap(<SignalCard message={msg({ kind: 'signal', text: 'Something happened.' })} />)
    const card = screen.getByTestId(TESTID.message)
    expect(within(card).queryByRole('img')).toBeNull()
    expect(card.querySelector('[data-severity]')).toHaveAttribute('data-severity', 'unrated')
    for (const s of SEVERITIES) expect(card.querySelector(`[data-icon="${s}"]`)).toBeNull()
  })

  it('folds its attributes until asked', async () => {
    wrap(<SignalCard message={msg({ kind: 'signal', text: 'Disk is full.', severity: 'warning', payload: '{"node":"n1"}' })} conversation={{ source: 'node-events' }} />)
    expect(screen.queryByText('node-events', { selector: 'dd' })).toBeNull()
    await userEvent.click(screen.getByRole('button', { name: 'Attributes' }))
    expect(screen.getByText('node-events', { selector: 'dd' })).toBeInTheDocument()
    expect(screen.getByText('warning', { selector: 'dd' })).toBeInTheDocument()
    expect(screen.getByText('{"node":"n1"}')).toBeInTheDocument()
  })
})

describe('an agent reply', () => {
  const REPLY = msg({
    id: 'run:run-1',
    text: '<title>\nPods restarted\n</title>\n\n<root-cause>\nOOM, `exit 137`.\n</root-cause>\n\n<details>\nline one\nline two\n</details>',
  })
  const details: RunDetailsFact = {
    runId: 'run-1',
    run: { runId: 'run-1', status: 'succeeded', startedAt: '2025-06-11T09:38:00Z', finishedAt: '2025-06-11T09:38:25Z' },
    tools: [{ tool: 'mcp__k8s__get_pods', durationMs: 1500 }, { tool: 'Read', durationMs: 410 }],
    events: [ev('run.dispatched', '2025-06-11T09:38:00Z'), ev('context.checkpoint', '2025-06-11T09:38:26Z')],
  }
  const member: ConversationSummary = {
    name: 'root-triage', phase: 'Idle', runCount: 1, queued: 0, joined: false, errored: false, unread: false,
    ageSeconds: 0, deleting: false, causedBy: { parent: 'root', entry: 'triage' },
  }

  it('shows only the reply, its Invoked rows and a collapsed Run details line when finished', () => {
    wrap(<Reply message={REPLY} members={[member]} details={details} />)
    expect(screen.getByText('Pods restarted')).toHaveClass('wk-ttl')
    expect(screen.getByText('Root cause')).toBeInTheDocument()
    const invoked = screen.getByTestId(INVOKED_TESTID('root-triage'))
    expect(invoked).toHaveTextContent('Invoked triage · idle')
    expect(within(invoked).getByRole('link', { name: 'open conversation →' })).toHaveAttribute('href', '/conversations/root-triage')
    const run = screen.getByTestId(RUN_DETAILS_TESTID)
    expect(run).toHaveAttribute('data-expanded', 'false')
    expect(run).toHaveTextContent('Run details · 25s')
    // Nothing else of the run is visible: no tool row, no event line.
    expect(screen.queryByText(/^Ran \d+ tool/)).toBeNull()
    expect(document.querySelector('[data-event]')).toBeNull()
    expect(screen.queryByTestId(TESTID.runEvents)).toBeNull()
    // The fold is closed too.
    expect(screen.getByText('Details (2 lines)')).toBeInTheDocument()
    expect(screen.queryByText('line one')).toBeNull()
  })

  it('expands an Invoked row to the task sent and the answer, as classic did', async () => {
    wrap(<Reply message={REPLY} members={[member]} details={details} />)
    expect(screen.queryByText('Task sent')).toBeNull()
    await userEvent.click(screen.getByTestId(TESTID.memberInvocationToggle))
    expect(screen.getByText('Task sent')).toBeInTheDocument()
    expect(screen.getByText('check the disk')).toBeInTheDocument()
    expect(screen.getByText('triage responded')).toBeInTheDocument()
    expect(screen.getByText('disk is fine')).toBeInTheDocument()
  })

  it('opens to the tools as one expandable row, the events and the checkpoint, stating no fact twice', async () => {
    wrap(<Reply message={REPLY} details={details} />)
    await userEvent.click(within(screen.getByTestId(RUN_DETAILS_TESTID)).getByRole('button', { name: 'show' }))
    const run = screen.getByTestId(RUN_DETAILS_TESTID)
    expect(run).toHaveAttribute('data-expanded', 'true')
    expect(within(run).getByText('Ran 2 tools')).toBeInTheDocument()
    expect(within(run).queryByText('mcp__k8s__get_pods')).toBeNull()
    expect(run.querySelector('[data-event="run.dispatched"]')).toHaveTextContent('run dispatched')
    expect(run.querySelector('[data-event="context.checkpoint"]')).toHaveTextContent('context checkpoint')
    // The duration is said once, on the line itself.
    expect(run.textContent?.match(/25s/g)).toHaveLength(1)
    await userEvent.click(within(run).getAllByRole('button', { name: 'show' })[0])
    expect(within(run).getByText('mcp__k8s__get_pods')).toBeInTheDocument()
    expect(within(run).getByText('1.5s')).toBeInTheDocument()
  })

  it('draws no Run details line when nothing is known about the run', () => {
    wrap(<Reply message={REPLY} details={{ runId: 'run-1', tools: [], events: [] }} />)
    expect(screen.queryByTestId(RUN_DETAILS_TESTID)).toBeNull()
  })

  it('states a duration-only run without a control that opens nothing', () => {
    wrap(<Reply message={REPLY} details={{ ...details, tools: [], events: [] }} />)
    const run = screen.getByTestId(RUN_DETAILS_TESTID)
    expect(run).toHaveTextContent('Run details · 25s')
    expect(within(run).queryByRole('button')).toBeNull()
  })

  it('sets short inline code in the line and a long command as its own block', () => {
    wrap(<Reply message={msg({ text: 'Run `kubectl get pods` then\n`kubectl -n storefront rollout undo deploy/checkout-api`' })} />)
    expect(screen.getByText('kubectl get pods').tagName).toBe('CODE')
    expect(screen.getByText('kubectl get pods')).not.toHaveClass('cmd')
    expect(screen.getByText('kubectl -n storefront rollout undo deploy/checkout-api')).toHaveClass('cmd')
  })
})

describe("a person's message", () => {
  it('sits on the viewer\u2019s side while sending, then says it was delivered', () => {
    const { rerender } = wrap(<PersonMessage message={msg({ kind: 'local', text: 'and the others?', pending: true })} />)
    expect(screen.getByText('sending…')).toBeInTheDocument()
    expect(document.querySelector('.wk-msg.own.pending')).not.toBeNull()
    rerender(<MemoryRouter><PersonMessage message={msg({ kind: 'local', text: 'and the others?' })} /></MemoryRouter>)
    expect(screen.getByText(/delivered ✓/)).toBeInTheDocument()
  })

  it('names a relayed sender and keeps their words unparsed', () => {
    wrap(<PersonMessage message={msg({ kind: 'relay', sender: 'ops-chat/dana', text: '<title>\nnot a title\n</title>' })} />)
    expect(screen.getByText('ops-chat/dana')).toBeInTheDocument()
    expect(document.querySelector('.wk-ttl')).toBeNull()
  })
})

describe('every message', () => {
  it('carries a copy control: a reply, a person\u2019s message and a signal', () => {
    wrap(
      <>
        <Reply message={msg({ id: 'a', text: 'reply' })} />
        <PersonMessage message={msg({ id: 'b', kind: 'local', text: 'mine' })} />
        <SignalCard message={msg({ id: 'c', kind: 'signal', text: 'signal' })} />
      </>,
    )
    const messages = screen.getAllByTestId(TESTID.message)
    expect(messages.map((m) => m.dataset.kind)).toEqual(['agent', 'local', 'signal'])
    for (const m of messages) expect(within(m).getByRole('button', { name: 'Copy message' })).toBeInTheDocument()
  })
})

describe('the working card', () => {
  it('is one card with the elapsed time, the latest event and a count', () => {
    wrap(<WorkingCard working={{ elapsed: 42_000, count: 3, latest: { kind: 'context.restored', label: 'context restored', at: '' } }} />)
    const card = screen.getByTestId(TESTID.typing)
    expect(card).toHaveTextContent('0:42')
    expect(card).toHaveTextContent('context restored · 3 events')
  })
})
