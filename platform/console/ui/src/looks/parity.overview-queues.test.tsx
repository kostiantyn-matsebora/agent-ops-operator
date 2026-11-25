import { describe, expect, it, vi } from 'vitest'
import { render } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import type { Overview, Queues } from '../api/types'
import { OverviewPage as ClassicOverview } from './classic/pages/Overview'
import { QueuesPage as ClassicQueues } from './classic/pages/Queues'
import { OverviewPage } from './workspace/pages/Overview'
import { QueuesPage } from './workspace/pages/Queues'

// TASKS 4.1 / 4.2: both looks draw the same cards, in the same order, from the
// same data. This file sits at the seam (looks/), the one place that may name
// two looks.

const overview: Overview = {
  namespace: 'agent-ops',
  manager: { version: '1', leader: 'm', runtimeSlots: { inUse: 1, max: 5, waiting: 0 }, queues: [], cooldowns: [] },
  stream: { connected: true, events: 1, resyncs: 0 },
  workloads: [], runtimes: [], adapters: [], counts: {}, synced: {},
  problems: [{ kind: 'pods', name: 'p', type: 'Failing', source: 'pod' }],
}
const queues: Queues = {
  capacity: { inUse: 1, max: 5, waiting: 0 },
  members: { inUse: 5, max: 5, waiting: 2 },
  work: [], delivery: [], cooldowns: [],
}

vi.mock('../model/overview', async (orig) => ({
  ...(await orig<typeof import('../model/overview')>()),
  useOverviewPage: () => ({ data: overview }),
  useMetricsBackend: () => false,
}))
vi.mock('../model/queues', async (orig) => ({
  ...(await orig<typeof import('../model/queues')>()),
  useQueuesPage: () => ({ data: queues }),
}))

const titles = (ui: React.ReactElement, selector: string) => {
  const r = render(<MemoryRouter>{ui}</MemoryRouter>)
  const out = [...r.container.querySelectorAll(selector)].map((e) => e.textContent)
  r.unmount()
  return out
}

describe('classic and workspace draw the same cards', () => {
  it('Overview', () => {
    const mine = titles(<OverviewPage />, '.wk-card > h2')
    expect(mine).toEqual(['Manager', 'Capacity', 'Runtimes', 'Telemetry', 'Workloads', 'Adapters', 'Problems (1)'])
    expect(mine).toEqual(titles(<ClassicOverview />, '.pf-v6-c-card__title-text'))
  })

  it('Queues', () => {
    const mine = titles(<QueuesPage />, '.wk-card > h2')
    expect(mine).toEqual([
      'Work queue — Conversations 1/5 slots, 0 waiting · Members 5/5 slots, 2 waiting — at ceiling',
      'Delivery queue',
      'Cooldowns',
    ])
    expect(mine).toEqual(titles(<ClassicQueues />, '.pf-v6-c-card__title-text'))
  })
})
