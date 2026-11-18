import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import type { Overview } from '../../../api/types'
import { OverviewPage } from './Overview'

const state = vi.hoisted(() => ({ value: {} as { data?: unknown; isLoading?: boolean; error?: unknown }, backend: false }))

vi.mock('../../../model/overview', async (orig) => ({
  ...(await orig<typeof import('../../../model/overview')>()),
  useOverviewPage: () => state.value,
  useMetricsBackend: () => state.backend,
}))

const since = new Date(Date.now() - 2 * 3600 * 1000).toISOString()

function overview(over: Partial<Overview> = {}): Overview {
  return {
    namespace: 'agent-ops',
    manager: {
      version: '0.61.0', leader: 'mgr-0', runtimeSlots: { inUse: 2, max: 5, waiting: 1 }, queues: [], cooldowns: [],
    },
    stream: { connected: true, events: 150, resyncs: 0 },
    workloads: [
      { name: 'agentops-manager', image: 'ghcr.io/x/manager:1', digest: 'sha256:abc', desired: 1, ready: 1, restarts: 0 },
      { name: 'agentops-bad', image: 'img', desired: 1, ready: 0, restarts: 3, problem: 'CrashLoopBackOff' },
    ],
    runtimes: [{ name: 'claude', image: 'ghcr.io/x/claude:1' }],
    adapters: [
      { kind: 'channeladapters', name: 'console', image: 'img-console', health: 'ok', serves: 1 },
      { kind: 'signaladapters', name: 'console', servedBy: 'ChannelAdapter/console', health: 'ok', serves: 1 },
    ],
    counts: {}, synced: {},
    problems: [
      { kind: 'conversations', name: 'c1', type: 'Ready', reason: 'Because', message: 'it broke', source: 'reported', since },
      { kind: 'pods', name: 'pod-1', type: 'Failing', source: 'pod' },
    ],
    ...over,
  }
}

const page = (ui: React.ReactElement) => render(<MemoryRouter>{ui}</MemoryRouter>)

beforeEach(() => {
  state.value = { data: overview() }
  state.backend = false
})

describe('workspace OverviewPage', () => {
  it('draws every card, field and link', () => {
    page(<OverviewPage />)
    expect(screen.getByRole('main', { name: 'Overview' })).toHaveClass('wk-pane', 'wk-page')
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('Installation')
    expect(screen.getByText('0.61.0')).toBeInTheDocument()
    expect(screen.getByText('mgr-0')).toBeInTheDocument()
    expect(screen.getByText('agent-ops')).toBeInTheDocument()
    expect(screen.getByText('2 of 5 runtime slots')).toBeInTheDocument()
    expect(screen.getByRole('progressbar')).toHaveAttribute('aria-valuenow', '40')
    expect(screen.getByText(/1 conversation\(s\) waiting/)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Queues' })).toHaveAttribute('href', '/queues')
    expect(screen.getByText('ghcr.io/x/claude:1')).toBeInTheDocument()
    expect(screen.getByText('150 event(s) held')).toBeInTheDocument()
    expect(screen.getByText('not configured')).toBeInTheDocument()
    expect(screen.getByText('console.metrics.url')).toBeInTheDocument()
    expect(screen.getByText('sha256:abc')).toBeInTheDocument()
    expect(screen.getByText('CrashLoopBackOff')).toHaveClass('wk-pill')
    const adapters = within(screen.getByRole('table', { name: 'adapters' }))
    expect(adapters.getAllByRole('link')[0]).toHaveAttribute('href', '/config/channeladapters/console')
    expect(adapters.getByText('served by ChannelAdapter/console')).toBeInTheDocument()
    const problems = within(screen.getByRole('table', { name: 'problems' }))
    expect(problems.getByRole('link', { name: 'conversations/c1' })).toHaveAttribute('href', '/config/conversations/c1')
    expect(problems.getByText('pod-1')).toBeInTheDocument()
    expect(problems.getByText(/ago/)).toBeInTheDocument()
    expect(problems.getByText('—')).toBeInTheDocument()
    expect(screen.queryByRole('alert')).toBeNull()
  })

  it('links the charts when a metrics backend answers', () => {
    state.backend = true
    page(<OverviewPage />)
    expect(screen.getByRole('link', { name: 'See the charts' })).toHaveAttribute('href', '/topology')
  })

  it('raises the storage outage, manager error and stream gap', () => {
    state.value = {
      data: overview({
        manager: { runtimeSlots: { inUse: 5, max: 5, waiting: 0 }, queues: [], cooldowns: [], storageOutage: { forSeconds: 600 } },
        managerError: 'dial tcp refused',
        stream: { connected: false, events: 3, resyncs: 2, error: 'eof', lastGap: { ts: since, detail: 'restart' } },
      }),
    }
    page(<OverviewPage />)
    expect(screen.getByText('Context storage is unavailable — work is being held')).toBeInTheDocument()
    expect(screen.getByText('10 minutes')).toBeInTheDocument()
    expect(screen.getByText("The manager's status surface is unreachable")).toBeInTheDocument()
    expect(screen.getByText('The activity stream is disconnected')).toBeInTheDocument()
    expect(screen.getByText(/2 resync\(s\)/)).toBeInTheDocument()
    expect(screen.getByText(/is unavailable — restart/)).toBeInTheDocument()
    expect(screen.getByRole('progressbar').parentElement?.querySelector('.full')).not.toBeNull()
  })

  it('states unavailable capacity and an empty problems list instead of zero', () => {
    state.value = { data: overview({ manager: undefined, problems: null, workloads: [], adapters: [], runtimes: [] }) }
    page(<OverviewPage />)
    expect(screen.getByText('Unavailable')).toBeInTheDocument()
    expect(screen.getAllByText('unknown')).toHaveLength(2)
    expect(screen.getByRole('heading', { name: 'Problems (0)' })).toBeInTheDocument()
    expect(screen.getByText('Nothing is reporting a problem')).toBeInTheDocument()
  })

  it('has loading and error states', () => {
    state.value = { isLoading: true }
    const { unmount } = page(<OverviewPage />)
    expect(screen.getByRole('status', { name: 'loading' })).toBeInTheDocument()
    unmount()
    state.value = { error: new Error('boom') }
    page(<OverviewPage />)
    expect(screen.getByText('Could not load the overview')).toBeInTheDocument()
  })
})
