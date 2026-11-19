import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import type { Queues } from '../../../api/types'
import { QueuesPage } from './Queues'

const state = vi.hoisted(() => ({ value: {} as { data?: unknown; isLoading?: boolean; error?: unknown } }))

vi.mock('../../../model/queues', async (orig) => ({
  ...(await orig<typeof import('../../../model/queues')>()),
  useQueuesPage: () => state.value,
}))
vi.mock('../../../api/hooks', async (orig) => ({
  ...(await orig<typeof import('../../../api/hooks')>()),
  usePipelineIcon: () => () => 'aops:bot',
}))

function queues(over: Partial<Queues> = {}): Queues {
  return {
    capacity: { inUse: 2, max: 5, waiting: 1 },
    work: [
      { conversation: 'c-1', title: 'Fix the build', pipeline: 'ops', phase: 'Running', queued: 2, ageSeconds: 30, inflight: 'run-1', inflightAgeSeconds: 7200, stuck: 'runtime-hung' },
      { conversation: 'c-2', phase: 'Pending', queued: 1, ageSeconds: 90 },
    ],
    delivery: [
      { adapter: 'console', queued: 1, claimed: 1, oldestClaimedOpId: 'op-9', oldestClaimedAgeSeconds: 120, stuck: 'adapter-wedged' },
      { adapter: 'tg', queued: 2, claimed: 0, oldestQueuedOpId: 'op-3', oldestQueuedAgeSeconds: 10 },
      { adapter: 'idle', queued: 0, claimed: 0 },
    ],
    cooldowns: [{ source: 'cron', suppressed: 4, windowSeconds: 600 }],
    ...over,
  } as Queues
}

const page = (ui: React.ReactElement) => render(<MemoryRouter>{ui}</MemoryRouter>)

beforeEach(() => {
  state.value = { data: queues() }
})

describe('workspace QueuesPage', () => {
  it('draws every table, field, flag and link', () => {
    page(<QueuesPage />)
    expect(screen.getByRole('main', { name: 'Queues and capacity' })).toHaveClass('wk-pane', 'wk-page')
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('Queues and capacity')
    expect(screen.getByRole('heading', { name: 'Work queue — 2/5 slots, 1 waiting' })).toBeInTheDocument()
    const work = within(screen.getByRole('table', { name: 'work queue' }))
    expect(work.getByRole('link', { name: 'Fix the build' })).toHaveAttribute('href', '/conversations/c-1')
    expect(work.getByRole('link', { name: 'c-2' })).toBeInTheDocument()
    expect(work.getByText('ops')).toBeInTheDocument()
    expect(work.getByText('2.0h')).toBeInTheDocument() // inflight age wins
    expect(work.getByText('2m')).toBeInTheDocument() // 90s waiting
    expect(work.getByText('runtime hung')).toBeInTheDocument()
    const delivery = within(screen.getByRole('table', { name: 'delivery queue' }))
    expect(delivery.getByRole('link', { name: 'console' })).toHaveAttribute('href', '/config/channeladapters/console')
    expect(delivery.getByText('op-9')).toBeInTheDocument()
    expect(delivery.getByText('claimed 2m')).toBeInTheDocument()
    expect(delivery.getByText('op-3')).toBeInTheDocument()
    expect(delivery.getByText('queued 10s')).toBeInTheDocument()
    expect(delivery.getByText('—')).toBeInTheDocument()
    expect(delivery.getByText('adapter wedged')).toBeInTheDocument()
    const cool = within(screen.getByRole('table', { name: 'cooldowns' }))
    expect(cool.getByRole('link', { name: 'cron' })).toHaveAttribute('href', '/config/signalsources/cron')
    expect(cool.getByText('4')).toBeInTheDocument()
    expect(cool.getByText('10m')).toBeInTheDocument()
  })

  it('states each empty queue and the unavailable delivery queue', () => {
    state.value = { data: queues({ work: [], delivery: [], cooldowns: [], error: 'manager down' }) }
    page(<QueuesPage />)
    expect(screen.getByText('Nothing is waiting')).toBeInTheDocument()
    expect(screen.getByText('No outstanding channel ops')).toBeInTheDocument()
    expect(screen.getByText('No signals are being suppressed')).toBeInTheDocument()
    expect(screen.getByText('Delivery queue unavailable')).toBeInTheDocument()
    expect(screen.getByText('manager down')).toBeInTheDocument()
  })

  it('has loading and error states', () => {
    state.value = { isLoading: true }
    const { unmount } = page(<QueuesPage />)
    expect(screen.getByRole('status', { name: 'loading' })).toBeInTheDocument()
    unmount()
    state.value = { error: new Error('boom') }
    page(<QueuesPage />)
    expect(screen.getByText('Could not load queues')).toBeInTheDocument()
  })
})
