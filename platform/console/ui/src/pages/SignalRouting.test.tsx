import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { SignalFilters, SignalPhaseLabel, SignalRoutingCard } from './SignalRouting'
import type { SignalStatusWire } from '../api/types'

describe('SignalPhaseLabel', () => {
  it('renders the phase text', () => {
    render(<SignalPhaseLabel phase="PartiallyRouted" />)
    expect(screen.getByText('PartiallyRouted')).toBeInTheDocument()
  })

  it('renders an em dash with no phase at all', () => {
    render(<SignalPhaseLabel />)
    expect(screen.getByText('—')).toBeInTheDocument()
  })
})

describe('SignalFilters', () => {
  it('lists every source and phase option, and reports a change of either', async () => {
    const onSourceChange = vi.fn()
    const onPhaseChange = vi.fn()
    render(
      <SignalFilters
        sources={['cluster-events', 'prometheus-alerts']}
        phases={['Pending', 'Routed']}
        source="all"
        phase="all"
        onSourceChange={onSourceChange}
        onPhaseChange={onPhaseChange}
      />,
    )
    const user = userEvent.setup()
    await user.selectOptions(screen.getByLabelText('Filter by source'), 'cluster-events')
    expect(onSourceChange).toHaveBeenCalledWith('cluster-events')

    await user.selectOptions(screen.getByLabelText('Filter by phase'), 'Routed')
    expect(onPhaseChange).toHaveBeenCalledWith('Routed')
  })
})

describe('SignalRoutingCard', () => {
  it("names every claimant's kind, name and state", () => {
    const status: SignalStatusWire = {
      phase: 'PartiallyRouted',
      claimants: [
        { ref: { kind: 'Pipeline', name: 'k8s-observe' }, state: 'Routed', conversationRef: { name: 'cluster-events-7c1d4e' } },
        { ref: { kind: 'Coordinator', name: 'incident-response' }, state: 'Routing' },
      ],
    }
    render(
      <MemoryRouter>
        <SignalRoutingCard status={status} />
      </MemoryRouter>,
    )
    expect(screen.getByText('k8s-observe')).toBeInTheDocument()
    expect(screen.getByText('Pipeline')).toBeInTheDocument()
    expect(screen.getByText('incident-response')).toBeInTheDocument()
    expect(screen.getByText('Coordinator')).toBeInTheDocument()
    expect(screen.getByRole('cell', { name: 'Routing' })).toBeInTheDocument()
  })

  it('links a Routed claimant to its conversation, by the console\'s own conversation route', () => {
    const status: SignalStatusWire = {
      phase: 'Routed',
      claimants: [
        { ref: { kind: 'Pipeline', name: 'k8s-observe' }, state: 'Routed', conversationRef: { name: 'cluster-events-7c1d4e' } },
      ],
    }
    render(
      <MemoryRouter>
        <SignalRoutingCard status={status} />
      </MemoryRouter>,
    )
    const link = screen.getByRole('link', { name: 'cluster-events-7c1d4e' })
    expect(link).toHaveAttribute('href', '/conversations/cluster-events-7c1d4e')
  })

  it('shows a Failed claimant\'s reason verbatim, never paraphrased', () => {
    const status: SignalStatusWire = {
      phase: 'Failed',
      reason: undefined,
      claimants: [
        {
          ref: { kind: 'Pipeline', name: 'alert-triage' }, state: 'Failed',
          reason: 'retry deadline exceeded: context storage unavailable (3 attempts)',
        },
      ],
    }
    render(
      <MemoryRouter>
        <SignalRoutingCard status={status} />
      </MemoryRouter>,
    )
    expect(
      screen.getByText('retry deadline exceeded: context storage unavailable (3 attempts)'),
    ).toBeInTheDocument()
  })

  it("shows a Dropped signal's own terminal reason as prominently as its phase", () => {
    render(
      <MemoryRouter>
        <SignalRoutingCard status={{ phase: 'Dropped', reason: 'cooldown' }} />
      </MemoryRouter>,
    )
    expect(screen.getByText('Dropped')).toBeInTheDocument()
    expect(screen.getByText('cooldown')).toBeInTheDocument()
  })

  it('renders an empty state rather than a blank table when no claimant was ever snapshotted', () => {
    render(
      <MemoryRouter>
        <SignalRoutingCard status={{ phase: 'Pending' }} />
      </MemoryRouter>,
    )
    expect(screen.getByText('No claimant was ever snapshotted')).toBeInTheDocument()
  })

  it('lists the consumer and delivery history the manager recorded, and omits what it did not', () => {
    render(
      <MemoryRouter>
        <SignalRoutingCard
          status={{
            phase: 'Routed', attempts: 2, firstAttemptAt: '2026-01-01T00:00:00Z',
            consumedBy: 'agentops-manager-0_ab12', consumedAt: '2026-01-01T00:00:08Z',
          }}
        />
      </MemoryRouter>,
    )
    expect(screen.getByText('agentops-manager-0_ab12')).toBeInTheDocument()
    expect(screen.getByText('Attempts')).toBeInTheDocument()
    expect(screen.getByText('2')).toBeInTheDocument()
    expect(screen.queryByText('Completed at')).not.toBeInTheDocument()
  })
})
