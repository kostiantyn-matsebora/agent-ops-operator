import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { useState } from 'react'
import { MemoryRouter, useLocation } from 'react-router-dom'
import { DraftRow, EMPTY_DRAFT, NewConversationPane, type Draft } from './NewConversation'
import { TESTID, testid } from '../../../model/testids'

type Hook = ReturnType<typeof import('../../../model/newConversation').useNewConversation>
const entries = [
  { kind: 'pipeline', name: 'k8s-ops', position: 'general', profile: 'k8s-engineer', icon: 'aops:bot' },
  { kind: 'coordinator', name: 'incident', position: 'general', description: 'Coordinates', icon: '🔥' },
] as Hook['startable']

let state: Hook
const start = vi.fn()
function hook(over: Partial<Hook> = {}): Hook {
  return {
    availability: 'ready', externalAuthenticator: '',
    wired: [{ name: 'console', wired: true, pipeline: 'k8s-ops', profile: '' }] as Hook['wired'],
    unwired: [], ambiguous: true, vocabularyLoading: false, startable: entries, busy: false, start, ...over,
  }
}
vi.mock('../../../model/newConversation', async (orig) => ({
  ...(await orig<typeof import('../../../model/newConversation')>()),
  useNewConversation: () => state,
}))

afterEach(() => start.mockReset())

const Where = () => <div data-testid="where">{useLocation().pathname}</div>
function Harness() {
  const [draft, setDraft] = useState<Draft>(EMPTY_DRAFT)
  return (
    <>
      <ul><DraftRow draft={draft} current /></ul>
      <NewConversationPane draft={draft} onDraft={setDraft} />
    </>
  )
}
function show(over: Partial<Hook> = {}) {
  state = hook(over)
  render(<MemoryRouter initialEntries={['/conversations?new=1']}><Harness /><Where /></MemoryRouter>)
}
const task = () => screen.getByLabelText('task', { selector: 'textarea' })

describe('the new-conversation draft', () => {
  it('lists every destination with its kind and filters by substring', () => {
    show()
    expect(screen.getByTestId(testid.destination('k8s-ops'))).toBeInTheDocument()
    expect(screen.getByTestId(testid.destination('incident'))).toHaveTextContent('coordinator')
    expect(screen.getByText('🔥')).toBeInTheDocument()
    fireEvent.change(screen.getByTestId(TESTID.destinationFilter), { target: { value: 'INC' } })
    expect(screen.queryByTestId(testid.destination('k8s-ops'))).toBeNull()
    fireEvent.change(screen.getByTestId(TESTID.destinationFilter), { target: { value: 'zzz' } })
    expect(screen.getByTestId('destination-no-matches')).toBeInTheDocument()
  })

  it('says so when nothing is addressable, and while loading', () => {
    show({ startable: [] })
    expect(screen.getByTestId('destination-empty')).toBeInTheDocument()
  })
  it('shows the loading line', () => {
    show({ vocabularyLoading: true })
    expect(screen.getByText(/Loading what can be addressed/)).toBeInTheDocument()
  })

  it('warns that an unaddressed task is refused when several pipelines serve the source', () => {
    show()
    expect(screen.getByText(/Several Pipelines serve this source/)).toBeInTheDocument()
  })

  it('names the sole answering pipeline otherwise', () => {
    show({ ambiguous: false, wired: [{ name: 'console', wired: true, pipeline: 'k8s-ops', profile: 'eng' }] as Hook['wired'] })
    expect(screen.getByText(/The Pipeline serving this source answers/)).toBeInTheDocument()
    expect(screen.getByText('eng')).toBeInTheDocument()
  })

  it('selecting a card prepares the addressed form and moves with arrow keys', async () => {
    start.mockResolvedValue({ ok: true })
    show()
    const card = screen.getByTestId(testid.destination('k8s-ops'))
    fireEvent.click(card)
    expect(card).toHaveAttribute('aria-checked', 'true')
    fireEvent.keyDown(card, { key: 'ArrowDown' })
    expect(screen.getByTestId(testid.destination('incident'))).toHaveAttribute('aria-checked', 'true')
    fireEvent.change(task(), { target: { value: 'look' } })
    expect(screen.getByTestId(TESTID.startConversation)).toHaveTextContent('Start with incident')
    fireEvent.click(screen.getByTestId(TESTID.startConversation))
    await waitFor(() => expect(start).toHaveBeenCalledWith('look', 'incident'))
  })

  it('disables Start until there is a task, and Shift+Enter sends, then navigates', async () => {
    start.mockResolvedValue({ ok: true, answering: 'k8s-ops' })
    show()
    expect(screen.getByTestId(TESTID.startConversation)).toBeDisabled()
    fireEvent.change(task(), { target: { value: 'go' } })
    fireEvent.keyDown(task(), { key: 'Enter', shiftKey: true })
    await waitFor(() => expect(start).toHaveBeenCalledWith('go', null))
    expect(screen.getByTestId('where')).toHaveTextContent('/conversations')
  })

  it('plain Enter does not send', () => {
    show()
    fireEvent.change(task(), { target: { value: 'go' } })
    fireEvent.keyDown(task(), { key: 'Enter' })
    expect(start).not.toHaveBeenCalled()
  })

  it('shows the server error and stays open on failure', async () => {
    start.mockResolvedValue({ ok: false, error: 'no Ready Pipeline serves it' })
    show()
    fireEvent.change(task(), { target: { value: 'go' } })
    fireEvent.click(screen.getByTestId(TESTID.startConversation))
    expect(await screen.findByRole('alert')).toHaveTextContent('no Ready Pipeline serves it')
    expect(screen.getByTestId('where')).toHaveTextContent('/conversations')
  })

  it('mirrors the task into the list\u2019s Draft row', () => {
    show()
    expect(screen.getByTestId('draft-row')).toHaveTextContent('Choose a destination and describe the task')
    fireEvent.change(task(), { target: { value: 'check the nodes' } })
    expect(screen.getByTestId('draft-row')).toHaveTextContent('check the nodes')
    expect(screen.getByTestId('draft-row')).toHaveTextContent('Draft')
  })

  it('disables Start while the command is pending', () => {
    show({ busy: true })
    fireEvent.change(task(), { target: { value: 'go' } })
    expect(screen.getByTestId(TESTID.startConversation)).toBeDisabled()
  })

  it.each([
    ['read-only', /read-only/],
    ['no-identity', /Nobody said who you are/],
    ['not-originating', /no signal identity/],
    ['loading', /Checking/],
  ] as const)('explains the %s state instead of offering a form', (availability, text) => {
    show({ availability, externalAuthenticator: 'oauth2-proxy' })
    expect(screen.getByText(text)).toBeInTheDocument()
    expect(screen.queryByTestId(TESTID.startConversation)).toBeNull()
  })

  it('names the proxy in the no-identity reason', () => {
    show({ availability: 'no-identity', externalAuthenticator: 'oauth2-proxy' })
    expect(screen.getByText(/oauth2-proxy in front/)).toBeInTheDocument()
  })

  it('shows the claim patch when unwired', () => {
    show({
      availability: 'unwired', wired: [],
      unwired: [{ name: 'console', wired: false, message: '', patch: 'kubectl patch pipeline x' }] as Hook['unwired'],
    })
    const box = screen.getByTestId(TESTID.newConversationUnavailable)
    expect(box).toHaveTextContent('no Ready Pipeline claims console')
    expect(box).toHaveTextContent('kubectl patch pipeline x')
  })
})
