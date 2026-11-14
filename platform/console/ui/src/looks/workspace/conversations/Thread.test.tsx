import { describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import type { ConversationSummary, Message } from '../../../api/types'
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

  it('confirms a close before sending it', async () => {
    const verbs: string[] = []
    mount({ onVerb: (v) => verbs.push(v) })
    await userEvent.click(screen.getByRole('button', { name: 'Close' }))
    expect(verbs).toEqual([])
    await userEvent.click(screen.getByRole('button', { name: 'Confirm close' }))
    expect(verbs).toEqual(['/close'])
    await userEvent.click(screen.getByRole('button', { name: 'Exit runtime' }))
    expect(verbs).toEqual(['/close', '/exit'])
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
