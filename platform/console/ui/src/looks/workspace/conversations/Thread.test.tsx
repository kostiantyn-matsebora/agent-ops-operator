import { afterEach, describe, expect, it } from 'vitest'
import { fireEvent, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import type { ConversationSummary, Message, VocabularyEntry } from '../../../api/types'
import { TESTID } from '../../../model/testids'
import { ThreadView, type ThreadViewProps } from './Thread'

const conv = (over: Partial<ConversationSummary> = {}): ConversationSummary => ({
  name: 'c1', title: 'disk pressure', phase: 'Idle', runCount: 1, queued: 0, joined: true, errored: false,
  unread: false, ageSeconds: 0, deleting: false, created: '2025-06-10T22:00:00', ...over,
})
const msg = (id: string, kind: string, at: string, text = id): Message => ({ id, thread: 't', kind, text, at })

function mount(over: Partial<ThreadViewProps> = {}) {
  const props: ThreadViewProps = {
    conversation: conv(), title: 'disk pressure', messages: [], runs: [], runDetails: new Map(),
    startedAt: '2025-06-10T22:00:00', canWrite: true, invoked: new Map(), onSend: () => undefined,
    onVerb: () => undefined, ...over,
  }
  return render(<MemoryRouter><ThreadView {...props} /></MemoryRouter>)
}

afterEach(() => localStorage.clear())

describe('the thread', () => {
  it('states when the conversation started on the line under its tabs', () => {
    mount()
    const tabs = screen.getByRole('tablist')
    const started = screen.getByTestId('thread-started')
    expect(started).toHaveAttribute('dateTime', '2025-06-10T22:00:00')
    expect(started.textContent).toMatch(/·/)
    // Under the tabs: the very next element after the tab bar holds it.
    expect(tabs.nextElementSibling?.contains(started)).toBe(true)
  })

  it('draws no day divider while every message falls on the day it started', () => {
    mount({ messages: [msg('s', 'signal', '2025-06-10T22:00:00'), msg('a', 'agent', '2025-06-10T22:05:00')] })
    expect(screen.queryAllByRole('separator')).toHaveLength(0)
  })

  it('draws one day divider where the day changes, and only there', () => {
    mount({
      messages: [
        msg('s', 'signal', '2025-06-10T22:00:00'),
        msg('a', 'agent', '2025-06-10T23:30:00'),
        msg('u', 'local', '2025-06-11T08:00:00'),
        msg('b', 'agent', '2025-06-11T08:10:00'),
      ],
    })
    const dividers = screen.getAllByRole('separator')
    expect(dividers).toHaveLength(1)
    // It sits between the last message of one day and the first of the next.
    const order = [...document.querySelectorAll(`[data-testid="${TESTID.message}"], [role="separator"]`)].map((n) => n.getAttribute('data-id') ?? 'divider')
    expect(order).toEqual(['s', 'a', 'divider', 'u', 'b'])
  })

  it('shows nothing of an acknowledgement and an empty thread as a state', () => {
    mount({ messages: [msg('k', 'ack', '2025-06-10T22:00:00')] })
    expect(screen.queryAllByTestId(TESTID.message)).toHaveLength(0)
    expect(screen.getByText('No messages yet')).toBeInTheDocument()
  })

  it('confirms a close in a dialog before sending it, and never confirms an exit', async () => {
    const verbs: string[] = []
    mount({ onVerb: (v) => verbs.push(v) })
    await userEvent.click(screen.getByRole('button', { name: 'Close' }))
    expect(verbs).toEqual([])
    expect(screen.getByTestId(TESTID.closeConfirmModal)).toBeInTheDocument()
    await userEvent.click(screen.getByTestId(TESTID.closeConfirmOk))
    expect(verbs).toEqual(['/close'])
    await userEvent.click(screen.getByRole('button', { name: 'Exit runtime' }))
    expect(verbs).toEqual(['/close', '/exit'])
  })

  it('cancels a close without sending it', async () => {
    const verbs: string[] = []
    mount({ onVerb: (v) => verbs.push(v) })
    await userEvent.click(screen.getByRole('button', { name: 'Close' }))
    await userEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(verbs).toEqual([])
    expect(screen.queryByTestId(TESTID.closeConfirmModal)).toBeNull()
  })

  it('stops asking once the viewer opts out, and asks again when the dialog reopens unchecked', async () => {
    const verbs: string[] = []
    mount({ onVerb: (v) => verbs.push(v) })
    await userEvent.click(screen.getByRole('button', { name: 'Close' }))
    expect(screen.getByTestId(TESTID.closeConfirmSkip)).not.toBeChecked()
    await userEvent.click(screen.getByTestId(TESTID.closeConfirmSkip))
    await userEvent.click(screen.getByTestId(TESTID.closeConfirmOk))
    await userEvent.click(screen.getByRole('button', { name: 'Close' }))
    expect(screen.queryByTestId(TESTID.closeConfirmModal)).toBeNull()
    expect(verbs).toEqual(['/close', '/close'])
  })

  it('sends a reply with Shift+Enter and clears the box', async () => {
    const sent: string[] = []
    mount({ onSend: (t) => sent.push(t) })
    const box = screen.getByRole('textbox', { name: 'message' })
    await userEvent.type(box, 'and the others?{Shift>}{Enter}{/Shift}')
    expect(sent).toEqual(['and the others?'])
    expect(box).toHaveValue('')
  })

  it('offers no composer or verbs on a closed conversation, and says its history is kept', () => {
    mount({ conversation: conv({ phase: 'Closed' }) })
    expect(screen.queryByRole('textbox')).toBeNull()
    expect(screen.queryByRole('button', { name: 'Close' })).toBeNull()
    expect(screen.getByText('This conversation is closed. Its history is kept.')).toBeInTheDocument()
  })

  it('sends a member back to its root to reply, through a crumb and a control', () => {
    mount({ conversation: conv({ causedBy: { parent: 'root', entry: 'triage' } }), root: { name: 'root', title: 'Incident' }, memberEntry: 'triage' })
    expect(screen.queryByRole('textbox')).toBeNull()
    expect(screen.getByRole('link', { name: 'Reply in Incident' })).toHaveAttribute('href', '/conversations/root')
    expect(screen.getByRole('link', { name: 'Incident' })).toHaveAttribute('href', '/conversations/root')
  })

  it('shows the runs and the YAML on their own tabs', async () => {
    mount({ runs: [{ runId: 'run-1', status: 'succeeded' }], yaml: 'kind: Conversation' })
    await userEvent.click(screen.getByRole('tab', { name: 'Runs · 1' }))
    expect(screen.getByText('run-1')).toBeInTheDocument()
    expect(screen.queryByTestId('thread-started')).toBeNull()
    await userEvent.click(screen.getByRole('tab', { name: 'YAML' }))
    expect(screen.getByText('kind: Conversation')).toBeInTheDocument()
  })

  it('offers a way back at phone width', async () => {
    let back = false
    mount({ onBack: () => { back = true } })
    await userEvent.click(screen.getByRole('button', { name: 'Back to the list' }))
    expect(back).toBe(true)
  })
})

const VOCAB: VocabularyEntry[] = [
  { kind: 'builtin', name: 'exit', position: 'thread', description: 'release the runtime' },
  { kind: 'builtin', name: 'close', position: 'thread' },
  { kind: 'pipeline', name: 'ops', position: 'general' },
]

describe('the composer', () => {
  it('lists the thread\u2019s commands as the viewer types a slash, and chooses one with the keyboard', async () => {
    mount({ vocabulary: VOCAB })
    const box = screen.getByRole('textbox', { name: 'message' })
    await userEvent.type(box, '/')
    const list = screen.getByTestId(TESTID.commandTypeahead)
    // Position is the filter: a pipeline starts a conversation, it does not act on one.
    expect(within(list).getAllByRole('option').map((o) => o.textContent)).toEqual(['/exitrelease the runtime', '/close'])
    await userEvent.keyboard('{ArrowDown}{Enter}')
    expect(box).toHaveValue('/close')
    expect(screen.queryByTestId(TESTID.commandTypeahead)).toBeNull()
  })

  it('narrows by prefix, steps out of the way on Escape, and stays quiet mid-sentence', async () => {
    mount({ vocabulary: VOCAB })
    const box = screen.getByRole('textbox', { name: 'message' })
    await userEvent.type(box, '/ex')
    expect(screen.getAllByRole('option')).toHaveLength(1)
    await userEvent.keyboard('{Escape}')
    expect(screen.queryByTestId(TESTID.commandTypeahead)).toBeNull()
    await userEvent.clear(box)
    await userEvent.type(box, 'check /var/log')
    expect(screen.queryByTestId(TESTID.commandTypeahead)).toBeNull()
  })

  it('runs a thread command from its chip at once, leaving a draft alone', async () => {
    const verbs: string[] = []
    mount({ vocabulary: VOCAB, onVerb: (v) => verbs.push(v) })
    await userEvent.type(screen.getByRole('textbox', { name: 'message' }), 'a draft')
    const chips = screen.getByTestId(TESTID.quickChips)
    await userEvent.click(within(chips).getByRole('button', { name: '/exit' }))
    expect(verbs).toEqual(['/exit'])
    expect(screen.getByRole('textbox', { name: 'message' })).toHaveValue('a draft')
    // /close goes through the same confirmation as the header's.
    await userEvent.click(within(chips).getByRole('button', { name: '/close' }))
    expect(verbs).toEqual(['/exit'])
    await userEvent.click(screen.getByTestId(TESTID.closeConfirmOk))
    expect(verbs).toEqual(['/exit', '/close'])
  })

  it('shows no chips to a viewer who cannot write', () => {
    mount({ vocabulary: VOCAB, canWrite: false })
    expect(screen.queryByTestId(TESTID.quickChips)).toBeNull()
  })
})

describe('the graph and sequence views', () => {
  it('belong to a root, and a member offers neither', () => {
    const { unmount } = mount()
    expect(screen.getByRole('tab', { name: 'Graph' })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: 'Sequence' })).toBeInTheDocument()
    unmount()
    mount({ conversation: conv({ causedBy: { parent: 'root', entry: 'triage' } }), root: { name: 'root', title: 'Incident' } })
    expect(screen.queryByRole('tab', { name: 'Graph' })).toBeNull()
    expect(screen.queryByRole('tab', { name: 'Sequence' })).toBeNull()
  })

  it('lists the recorded hops with their route, offset and latency', async () => {
    mount({
      events: [
        { cursor: '1', ts: '2025-06-10T22:00:00Z', kind: 'signal', status: 'ok', from: { kind: 'SignalSource', name: 'cron' }, to: { kind: 'Pipeline', name: 'ops' } },
        { cursor: '2', ts: '2025-06-10T22:00:02Z', kind: 'run.completed', status: 'error', latencyMs: 1500 },
      ],
    })
    await userEvent.click(screen.getByRole('tab', { name: 'Sequence' }))
    const table = screen.getByRole('table', { name: 'Sequence' })
    expect(within(table).getByText('SignalSource/cron → Pipeline/ops')).toBeInTheDocument()
    expect(within(table).getByText('+2.0s')).toBeInTheDocument()
    expect(within(table).getByText('1.50s')).toBeInTheDocument()
    expect(within(table).getByText('error')).toBeInTheDocument()
  })

  it('says so when the buffer holds no hop for the conversation', async () => {
    mount({ events: [] })
    await userEvent.click(screen.getByRole('tab', { name: 'Sequence' }))
    expect(screen.getByText('No recorded hops for this conversation')).toBeInTheDocument()
  })
})

describe('a coordinator root', () => {
  it('states its budget per level, and the moment it escalated among the messages', () => {
    mount({
      conversation: conv({
        coordinator: 'incident',
        budget: { agentsInvoked: 2, maxAgents: 4, turns: 3 },
        escalatedAt: '2025-06-10T22:10:00Z',
      }),
      messages: [msg('a', 'agent', '2025-06-10T22:05:00Z'), msg('b', 'agent', '2025-06-10T22:20:00Z')],
    })
    const budget = screen.getByTestId('budget')
    expect(budget).toHaveTextContent('Agents invoked 2 of 4')
    expect(budget).toHaveTextContent('Turns 3')
    const order = [...document.querySelectorAll(`[data-testid="${TESTID.message}"], [data-testid="escalated"]`)].map((n) => n.getAttribute('data-id') ?? 'escalated')
    expect(order).toEqual(['a', 'escalated', 'b'])
  })

  it('draws neither where nothing was budgeted or escalated', () => {
    mount({ messages: [msg('a', 'agent', '2025-06-10T22:05:00Z')] })
    expect(screen.queryByTestId('budget')).toBeNull()
    expect(screen.queryByTestId('escalated')).toBeNull()
  })
})

describe('arrivals while reading higher up', () => {
  function scrolledUp() {
    const body = document.getElementById('wk-thread-body') as HTMLElement
    Object.defineProperty(body, 'scrollHeight', { configurable: true, value: 1000 })
    Object.defineProperty(body, 'clientHeight', { configurable: true, value: 100 })
    fireEvent.scroll(body, { target: { scrollTop: 0 } })
    return body
  }
  const props = (n: number) => ({
    conversation: conv(), title: 'disk pressure', messages: Array.from({ length: n }, (_, i) => msg(`m${i}`, 'agent', '2025-06-10T22:05:00Z')),
    runs: [], runDetails: new Map(), invoked: new Map(), canWrite: true,
  }) as ThreadViewProps

  it('counts them on a pill instead of moving the viewer, and jumps on request', async () => {
    const { rerender } = render(<MemoryRouter><ThreadView {...props(2)} /></MemoryRouter>)
    expect(screen.queryByTestId(TESTID.jumpPill)).toBeNull()
    const body = scrolledUp()
    rerender(<MemoryRouter><ThreadView {...props(3)} /></MemoryRouter>)
    expect(screen.getByTestId(TESTID.jumpPill)).toHaveTextContent('↓ 1 new message')
    expect(body.scrollTop).toBe(0)
    rerender(<MemoryRouter><ThreadView {...props(5)} /></MemoryRouter>)
    expect(screen.getByTestId(TESTID.jumpPill)).toHaveTextContent('↓ 3 new messages')
    await userEvent.click(screen.getByTestId(TESTID.jumpPill))
    expect(screen.queryByTestId(TESTID.jumpPill)).toBeNull()
    expect(body.scrollTop).toBe(1000)
  })

  it('shows no pill to a viewer who is at the bottom', () => {
    const { rerender } = render(<MemoryRouter><ThreadView {...props(2)} /></MemoryRouter>)
    rerender(<MemoryRouter><ThreadView {...props(3)} /></MemoryRouter>)
    expect(screen.queryByTestId(TESTID.jumpPill)).toBeNull()
  })
})

describe('the runs tab', () => {
  it('shows a run\u2019s token statistics, its result and its tool calls', async () => {
    mount({
      runs: [{
        runId: 'run-1', status: 'succeeded', exitCode: 0, result: 'the raw result',
        turns: [{ model: 'sonnet', tokensIn: 1000, tokensOut: 50, cacheReadTokens: 600, cacheCreationTokens: 100, stopReason: 'end_turn' }],
        toolCalls: [{ tool: 'mcp__aops__invoke', durationMs: 154, resultBytes: 300 }],
      }],
    })
    await userEvent.click(screen.getByRole('tab', { name: 'Runs · 1' }))
    expect(screen.getByTestId('run-tokens')).toHaveTextContent('1000 in · 50 out')
    const turns = screen.getByRole('table', { name: 'Turns' })
    expect(within(turns).getByText('300')).toBeInTheDocument() // fresh
    expect(within(turns).getByText('end_turn')).toBeInTheDocument()
    const tools = screen.getByRole('table', { name: 'Tool calls' })
    expect(within(tools).getByText('invoke')).toBeInTheDocument()
    expect(within(tools).getByText('aops')).toBeInTheDocument()
    expect(screen.getByText('the raw result')).toBeInTheDocument()
    expect(screen.getByText('Bindings and runtime')).toBeInTheDocument()
  })

  it('draws no turns or tools table for a run that reported none', async () => {
    mount({ runs: [{ runId: 'run-1', status: 'succeeded' }] })
    await userEvent.click(screen.getByRole('tab', { name: 'Runs · 1' }))
    expect(screen.queryByRole('table', { name: 'Turns' })).toBeNull()
    expect(screen.queryByTestId('run-tokens')).toBeNull()
  })
})

describe('what the classic thread shows that the workspace thread must too', () => {
  it('offers the latest message\u2019s choices as buttons that fill the reply box, and shows older ones as spent', async () => {
    mount({
      messages: [
        { ...msg('a', 'agent', '2025-06-10T22:05:00Z'), choices: [{ label: 'Old', command: '/old' }] },
        { ...msg('b', 'agent', '2025-06-10T22:06:00Z'), choices: [{ label: 'Reload', command: '/reload' }] },
      ],
    })
    expect(screen.getAllByTestId('choices')).toHaveLength(1)
    expect(screen.getByRole('list', { name: /no longer actionable/ })).toHaveTextContent('Old')
    await userEvent.click(screen.getByRole('button', { name: 'Reload' }))
    expect(screen.getByRole('textbox', { name: 'message' })).toHaveValue('/reload ')
  })

  it('shows no actionable choices to a viewer who cannot write', () => {
    mount({ canWrite: false, messages: [{ ...msg('b', 'agent', '2025-06-10T22:06:00Z'), choices: [{ label: 'Reload', command: '/reload' }] }] })
    expect(screen.queryByTestId('choices')).toBeNull()
  })

  it('divides what the viewer had not read from what they had', () => {
    mount({ unreadFrom: 'b', messages: [msg('a', 'agent', '2025-06-10T22:05:00Z'), msg('b', 'agent', '2025-06-10T22:06:00Z')] })
    const order = [...document.querySelectorAll(`[data-testid="${TESTID.message}"], [data-testid="unread-divider"]`)].map((n) => n.getAttribute('data-id') ?? 'divider')
    expect(order).toEqual(['a', 'divider', 'b'])
  })

  it('says a conversation has no console thread, with the command that joins one', () => {
    mount({ joinHint: { reason: 'no binding', fix: 'kubectl patch x' } })
    expect(screen.getByTestId(TESTID.joinHintFix)).toHaveTextContent('kubectl patch x')
    expect(screen.getByText('This conversation has no console thread')).toBeInTheDocument()
  })

  it('says a thread was archived', () => {
    mount({ archived: true })
    expect(screen.getByTestId('archived')).toBeInTheDocument()
  })

  it('names the channels a reply also reaches, and draws the pipeline\u2019s icon', () => {
    mount({ conversation: conv({ threads: [{ channel: 'console' }, { channel: 'home-telegram' }] } as Partial<ConversationSummary>), pipelineIcon: '🛠' })
    expect(screen.getByTestId('bound-channels')).toHaveTextContent('console · home-telegram')
    expect(screen.getByRole('heading', { name: /disk pressure/ }).querySelector('.wk-pmark')).not.toBeNull()
  })

  it('says why a reply was not sent, until it is dismissed', async () => {
    let dismissed = false
    mount({ sendFailure: { error: 'manager unreachable', onDismiss: () => { dismissed = true } } })
    expect(screen.getByRole('alert')).toHaveTextContent('Not sent — manager unreachable')
    await userEvent.click(screen.getByRole('button', { name: 'Dismiss' }))
    expect(dismissed).toBe(true)
  })
})
