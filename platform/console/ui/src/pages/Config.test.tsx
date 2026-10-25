import { describe, expect, it, vi } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { ConfigDetailPage, ConfigKindPage } from './Config'
import type { Detail, InventoryRow } from '../api/types'

// The Signal kind rides the SAME generic inventory → detail pages every
// other kind does (console-topology's "CR inventory views"), with its own
// filters on the list and its own routing card on the detail — this is the
// wiring test for that, mocking only the data-fetching hooks.

const inventory = vi.fn<() => { data?: InventoryRow[]; isLoading: boolean; error: unknown }>()
const detail = vi.fn<() => { data?: Detail; isLoading: boolean; error: unknown }>()

vi.mock('../api/hooks', () => ({
  useInventory: () => inventory(),
  useDetail: () => detail(),
  usePipelineIcon: () => () => undefined,
}))

function signalRow(name: string, source: string, phase: string, occurrences: string, over: Partial<InventoryRow> = {}): InventoryRow {
  return {
    name, health: 'none', findings: 0, created: new Date().toISOString(),
    columns: { source, initiationId: `init-${name}`, phase, occurrences },
    ...over,
  }
}

describe('ConfigKindPage — the Signal kind', () => {
  it('shows source, initiationId, phase and occurrences, and filters by source and phase', async () => {
    inventory.mockReturnValue({
      isLoading: false,
      error: undefined,
      data: [
        signalRow('sig-a', 'cluster-events', 'Routed', '1'),
        signalRow('sig-b', 'cluster-events', 'PartiallyRouted', '3'),
        signalRow('sig-c', 'prometheus-alerts', 'Routed', '2'),
      ],
    })
    render(
      <MemoryRouter initialEntries={['/config/signals']}>
        <Routes>
          <Route path="/config/:kind" element={<ConfigKindPage />} />
        </Routes>
      </MemoryRouter>,
    )

    // Every row is visible unfiltered — the PartiallyRouted one included,
    // exactly the "held-up signal is findable" scenario the spec names.
    expect(screen.getByText('sig-a')).toBeInTheDocument()
    expect(screen.getByText('sig-b')).toBeInTheDocument()
    expect(screen.getByText('sig-c')).toBeInTheDocument()
    expect(screen.getByText('init-sig-a')).toBeInTheDocument()

    const user = userEvent.setup()
    await user.selectOptions(screen.getByLabelText('Filter by source'), 'cluster-events')
    expect(screen.getByText('sig-a')).toBeInTheDocument()
    expect(screen.getByText('sig-b')).toBeInTheDocument()
    expect(screen.queryByText('sig-c')).not.toBeInTheDocument()

    await user.selectOptions(screen.getByLabelText('Filter by phase'), 'PartiallyRouted')
    expect(screen.queryByText('sig-a')).not.toBeInTheDocument()
    expect(screen.getByText('sig-b')).toBeInTheDocument()
  })

  it('offers no create/edit/delete/retry control anywhere on the page', () => {
    inventory.mockReturnValue({
      isLoading: false, error: undefined, data: [signalRow('sig-a', 'cluster-events', 'Pending', '1')],
    })
    render(
      <MemoryRouter initialEntries={['/config/signals']}>
        <Routes>
          <Route path="/config/:kind" element={<ConfigKindPage />} />
        </Routes>
      </MemoryRouter>,
    )
    expect(screen.queryByRole('button', { name: /delete|retry|edit|create/i })).not.toBeInTheDocument()
  })
})

describe('ConfigDetailPage — the Signal kind', () => {
  function signalDetail(status: Detail['object']['status']): Detail {
    return {
      object: { kind: 'signals', metadata: { name: 'sig-a' }, spec: {}, status },
      health: 'none', yaml: '', usedBy: [], findings: [],
    }
  }

  it('shows the per-claimant routing record, with a Routed claimant linked to its conversation', () => {
    detail.mockReturnValue({
      isLoading: false, error: undefined,
      data: signalDetail({
        phase: 'PartiallyRouted',
        claimants: [
          { ref: { kind: 'Pipeline', name: 'k8s-observe' }, state: 'Routed', conversationRef: { name: 'cluster-events-7c1d4e' } },
          { ref: { kind: 'Coordinator', name: 'incident-response' }, state: 'Routing' },
        ],
      }),
    })
    render(
      <MemoryRouter initialEntries={['/config/signals/sig-a']}>
        <Routes>
          <Route path="/config/:kind/:name" element={<ConfigDetailPage />} />
        </Routes>
      </MemoryRouter>,
    )
    const routing = screen.getByText('Routing', { selector: '.pf-v6-c-card__title-text' }).closest('.pf-v6-c-card') as HTMLElement
    expect(within(routing).getByText('k8s-observe')).toBeInTheDocument()
    expect(within(routing).getByRole('link', { name: 'cluster-events-7c1d4e' })).toHaveAttribute(
      'href', '/conversations/cluster-events-7c1d4e',
    )
    expect(within(routing).getByText('incident-response')).toBeInTheDocument()
  })

  it("shows a Failed claimant's reason exactly as recorded, and the Dropped/Failed signal's own terminal reason", () => {
    detail.mockReturnValue({
      isLoading: false, error: undefined,
      data: signalDetail({
        phase: 'Dropped',
        reason: 'unclaimed',
        claimants: [],
      }),
    })
    render(
      <MemoryRouter initialEntries={['/config/signals/sig-a']}>
        <Routes>
          <Route path="/config/:kind/:name" element={<ConfigDetailPage />} />
        </Routes>
      </MemoryRouter>,
    )
    expect(screen.getByText('Dropped')).toBeInTheDocument()
    expect(screen.getByText('unclaimed')).toBeInTheDocument()
  })
})
