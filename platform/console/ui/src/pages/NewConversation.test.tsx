import { afterEach, describe, expect, it, vi } from 'vitest'
import { act, fireEvent, render, screen } from '@testing-library/react'
import { matchEntries, NewConversation } from './NewConversation'
import type { VocabularyEntry } from '../api/types'

// The typeahead exists because a source is shareable: with several Pipelines
// serving one surface, an unaddressed task is refused, so the composer has to
// offer what can be addressed rather than expect a name to be recalled.

const entries: VocabularyEntry[] = [
  { kind: 'pipeline', name: 'ha-control', position: 'general', profile: 'ha-user' },
  { kind: 'pipeline', name: 'ha-ops', position: 'general', profile: 'ha-admin' },
  { kind: 'pipeline', name: 'k8s-ops', position: 'general', profile: 'k8s-engineer' },
  // Valid only inside a conversation. This composer starts one, so neither of
  // these may ever be offered here.
  { kind: 'builtin', name: 'exit', position: 'thread', description: 'Release the runtime' },
  { kind: 'builtin', name: 'close', position: 'thread', description: 'End the conversation' },
]

describe('matchEntries', () => {
  it('offers every pipeline on the bare prefix', () => {
    expect(matchEntries('/', entries, 'general')).toHaveLength(3)
  })

  it('narrows as the name is typed', () => {
    expect(matchEntries('/ha', entries, 'general')?.map((a) => a.name)).toEqual(['ha-control', 'ha-ops'])
    expect(matchEntries('/ha-o', entries, 'general')?.map((a) => a.name)).toEqual(['ha-ops'])
  })

  it('is case-insensitive, because a name is not a password', () => {
    expect(matchEntries('/HA-O', entries, 'general')?.map((a) => a.name)).toEqual(['ha-ops'])
  })

  it('shows NOTHING rather than an empty box when no name matches', () => {
    // An empty popup is worse than no popup: it covers the field and says
    // nothing. Same answer for a surface with no Ready pipelines at all.
    expect(matchEntries('/nope', entries, 'general')).toBeNull()
    expect(matchEntries('/', [], 'general')).toBeNull()
  })

  it('ignores a slash that is not addressing anyone', () => {
    // Mid-sentence slashes are paths and dates. Only the start of the message
    // addresses a pipeline, so only that position opens the menu.
    expect(matchEntries('check /var/log on node-1', entries, 'general')).toBeNull()
    expect(matchEntries('is 3/4 of the disk used?', entries, 'general')).toBeNull()
  })

  it('closes once the name is finished and the task has begun', () => {
    expect(matchEntries('/ha-ops ', entries, 'general')).toBeNull()
    expect(matchEntries('/ha-ops restart the api', entries, 'general')).toBeNull()
  })

  it('offers only what the server returned, which is Ready-filtered', () => {
    // The filtering itself is the BFF's job (handleVocabulary), so that the
    // typeahead and the listing command can never give different answers. Here we only
    // pin that nothing is invented client-side.
    expect(
      matchEntries('/', [{ kind: 'pipeline', name: 'only-ready', position: 'general' }], 'general')
        ?.map((e) => e.name),
    ).toEqual(['only-ready'])
  })

  it('offers only what is valid WHERE the person is typing', () => {
    // The two composers take disjoint sets. Addressing a Pipeline inside a
    // thread is input for the agent, and the commands that end or release a
    // conversation have nothing to act on outside one — so offering the wrong
    // half puts a command in front of somebody at the one place it does
    // nothing.
    expect(matchEntries('/', entries, 'general')?.map((e) => e.name)).toEqual([
      'ha-control',
      'ha-ops',
      'k8s-ops',
    ])
    expect(matchEntries('/', entries, 'thread')?.map((e) => e.name)).toEqual(['exit', 'close'])
  })

  it('never offers a pipeline inside a thread', () => {
    expect(matchEntries('/ha', entries, 'thread')).toBeNull()
  })
})

// The modal itself: direct-pick cards replace the old "type / to see what
// exists" flow. See the mockup behind GitHub issue #250's QA pass — every
// Ready pipeline and coordinator is its own card, visible with no typing, and
// a filter box narrows what is already on screen rather than replacing it
// with a blank field.

const sessionData = { writeEnabled: true, canWrite: true, canOriginate: true, externalAuthenticator: '' }
const sourcesData = {
  canOriginate: true,
  sources: [{ name: 'k8s-events', wired: true, pipeline: 'k8s-observe', profile: 'k8s-engineer' }],
}
const cardEntries: VocabularyEntry[] = [
  { kind: 'pipeline', name: 'k8s-observe', position: 'general', description: 'read-only cluster triage' },
  { kind: 'pipeline', name: 'k8s-operate', position: 'general', description: 'cluster changes, write access' },
  {
    kind: 'coordinator',
    name: 'agentops-coordinator',
    position: 'general',
    description: 'routes to ha-control, ha-ops, k8s-observe',
  },
]

const startMock = vi.fn().mockResolvedValue({ source: 'k8s-events', note: 'started' })

vi.mock('../api/hooks', () => ({
  useSession: () => ({ data: sessionData }),
  useSources: () => ({ data: sourcesData }),
  useVocabulary: () => ({ data: { entries: cardEntries }, isLoading: false }),
}))

vi.mock('../api/client', () => ({
  api: { start: (task: string, source?: string) => startMock(task, source) },
  ApiError: class ApiError extends Error {
    status: number
    body: Record<string, unknown>
    constructor(status: number, message: string, body: Record<string, unknown> = {}) {
      super(message)
      this.status = status
      this.body = body
    }
  },
}))

function openModal() {
  render(<NewConversation />)
  fireEvent.click(screen.getByTestId('new-conversation'))
}

describe('the destination picker', () => {
  afterEach(() => {
    startMock.mockClear()
  })

  it('lists every Ready pipeline and coordinator as its own card, with no typing', () => {
    openModal()
    expect(screen.getByTestId('destination-k8s-observe')).toBeInTheDocument()
    expect(screen.getByTestId('destination-k8s-operate')).toBeInTheDocument()
    expect(screen.getByTestId('destination-agentops-coordinator')).toBeInTheDocument()
  })

  it('selecting a card highlights it and relabels Start', () => {
    openModal()
    const card = screen.getByTestId('destination-k8s-operate')
    fireEvent.click(card)
    expect(card).toHaveAttribute('aria-checked', 'true')
    expect(screen.getByTestId('start-conversation')).toHaveTextContent('Start with k8s-operate')
  })

  it('the filter box narrows the visible list by name, never clearing it to blank', () => {
    openModal()
    fireEvent.change(screen.getByTestId('destination-filter'), { target: { value: 'coord' } })
    expect(screen.queryByTestId('destination-k8s-observe')).toBeNull()
    expect(screen.queryByTestId('destination-k8s-operate')).toBeNull()
    expect(screen.getByTestId('destination-agentops-coordinator')).toBeInTheDocument()
  })

  it('sends the addressed task once a card is picked and Start is clicked', async () => {
    openModal()
    fireEvent.click(screen.getByTestId('destination-k8s-observe'))
    fireEvent.change(screen.getByLabelText('task'), { target: { value: 'check cluster health' } })
    await act(async () => {
      fireEvent.click(screen.getByTestId('start-conversation'))
      await Promise.resolve()
    })
    expect(startMock).toHaveBeenCalledWith('/k8s-observe check cluster health', 'k8s-events')
  })

  it('keeps Start disabled while the task is empty, picked card or not', () => {
    openModal()
    fireEvent.click(screen.getByTestId('destination-k8s-observe'))
    expect(screen.getByTestId('start-conversation')).toBeDisabled()
  })
})
