import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { ConversationsPage } from './Conversations'
import type { ConversationSummary } from '../api/types'

// The list's half of the bulk close: what can be picked, and how far a
// select-all reaches. Both are bounded by the rows on screen — the server
// paginates, so `items` is exactly one page and there is no control that goes
// past it.

const mutate = vi.fn()
// The server's own answer to "may this browser write" — flipped by the
// read-only test below.
let canWrite = true

vi.mock('../api/hooks', () => ({
  useConversations: () => ({ data: page(), isLoading: false, error: null }),
  useSession: () => ({ data: { canWrite } }),
  useCloseConversations: () => ({
    mutate,
    data: undefined,
    error: null,
    isPending: false,
    reset: vi.fn(),
  }),
}))

function conv(name: string, over: Partial<ConversationSummary> = {}): ConversationSummary {
  return {
    name, runCount: 0, queued: 0, joined: true, errored: false, ageSeconds: 1, closing: false,
    phase: 'Idle', ...over,
  }
}

// One PAGE of a larger result: 3 shown, 120 matching.
function page() {
  return {
    items: [conv('a'), conv('going', { closing: true }), conv('b')],
    total: 120,
    offset: 0,
    limit: 50,
    facets: {},
  }
}

function renderList() {
  return render(
    <MemoryRouter>
      <ConversationsPage />
    </MemoryRouter>,
  )
}

// PatternFly names its own checkboxes: "Select all rows" in the header and
// "Select row <index>" per row. Row order here is a, going, b.
const SELECT_ALL = 'Select all rows'
const ROW = (i: number) => `Select row ${i}`

describe('closing a selection', () => {
  it('selects only the rows on this page, and only the closeable ones', async () => {
    renderList()
    await userEvent.click(screen.getByLabelText(SELECT_ALL))

    // 120 conversations match; 3 are on screen; one of those is already
    // closing. Select-all means the 2 that are left, never the other 117.
    expect(screen.getByTestId('close-selected')).toHaveTextContent('Close selected (2)')
  })

  it('will not let a closing conversation be selected', () => {
    renderList()
    expect(screen.getByLabelText(ROW(1))).toBeDisabled()
    expect(screen.getByLabelText(ROW(0))).toBeEnabled()
    // and the row says so rather than showing its last phase
    expect(screen.getByText('closing')).toBeInTheDocument()
  })

  it('offers nothing to close until something is selected', () => {
    renderList()
    expect(screen.getByTestId('close-selected')).toBeDisabled()
  })

  it('sends the selected names to one request', async () => {
    renderList()
    await userEvent.click(screen.getByLabelText(ROW(0)))
    await userEvent.click(screen.getByTestId('close-selected'))
    await userEvent.click(screen.getByTestId('close-confirm'))

    expect(mutate).toHaveBeenCalledWith(
      { names: ['a'], includeWorking: false },
      expect.anything(),
    )
  })
})

describe('a read-only console', () => {
  it('renders no close action and nothing to select', () => {
    // canWrite false is the server's own answer — the write gate is off, or
    // nobody forwarded an identity to attribute the close against. The action
    // is hidden rather than disabled, consistent with the other write surfaces.
    canWrite = false
    try {
      renderList()
      expect(screen.queryByTestId('close-selected')).toBeNull()
      expect(screen.queryByLabelText(SELECT_ALL)).toBeNull()
      expect(screen.queryByLabelText(ROW(0))).toBeNull()
    } finally {
      canWrite = true
    }
  })
})
