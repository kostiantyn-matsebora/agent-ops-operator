import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import type { Detail, InventoryRow, KindInfo } from '../../../../api/types'
import { ConfigDetailPage, ConfigKindPage, ConfigPage } from './index'
import { tokenizeLine } from './yaml'

type Q<T> = { data?: T; isLoading: boolean; error: unknown }
const kinds = vi.fn<() => Q<KindInfo[]>>()
const inventory = vi.fn<() => Q<InventoryRow[]>>()
const detail = vi.fn<() => Q<Detail>>()

vi.mock('../../../../api/hooks', async (orig) => ({
  ...(await orig<object>()),
  useKinds: () => kinds(),
  useInventory: () => inventory(),
  useDetail: () => detail(),
  usePipelineIcon: () => (n?: string) => (n === 'triage' ? '🛠' : undefined),
}))

const ok = <T,>(data: T): Q<T> => ({ data, isLoading: false, error: undefined })

const KINDS: KindInfo[] = [
  { kind: 'pipelines', title: 'Pipelines', count: 2, synced: true },
  { kind: 'channels', title: 'Channels', count: 1, synced: true },
  { kind: 'signals', title: 'Signals', count: 0, synced: false },
]

const ROWS: InventoryRow[] = [
  {
    name: 'triage', icon: '🛠', health: 'ok', findings: 2, created: new Date(Date.now() - 3 * 3600e3).toISOString(),
    labels: { app: 'x', tier: 'y', extra: 'z' },
    conditions: [{ type: 'Ready', status: 'True', reason: 'AllGood' }, { type: 'Wired', status: 'False', reason: 'NoSource', message: 'nothing' }],
    columns: { profile: 'p1', sources: 'a, b', channels: 'tg', toolsMode: 'overwrite' },
  },
  { name: 'bare', health: 'none', findings: 0, columns: {} },
]

const DETAIL: Detail = {
  object: {
    kind: 'Pipeline',
    metadata: {
      name: 'triage', namespace: 'agent-ops', uid: 'u-1', resourceVersion: '77',
      creationTimestamp: new Date(Date.now() - 86400e3).toISOString(),
      labels: { app: 'x' }, annotations: { note: 'hi' },
    },
    spec: { icon: '🛠' },
  },
  health: 'ok',
  conditions: [{ type: 'Ready', status: 'True', reason: 'AllGood', message: 'fine', lastTransitionTime: new Date().toISOString() }],
  yaml: 'kind: Pipeline\nspec:\n  n: 3\n  on: true\n# note\n',
  usedBy: [{ kind: 'channels', name: 'tg', field: 'spec.pipelineRef' }],
  findings: [{ kind: 'pipelines', name: 'triage', check: 'dangling-ref', reason: 'Missing', message: 'no such channel' }],
  resolved: {
    pipeline: 'triage', profile: 'p', runtime: 'claude', allowedTools: ['Read', 'Bash(kubectl:*)'], toolsMode: 'overwrite',
    toolsets: [], mcpConfigs: [], mcpServers: ['aops'], unresolved: ['ghost'],
  },
}

function at(path: string, route: string, el: JSX.Element) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route path={route} element={el} />
        <Route path="*" element={<div>elsewhere</div>} />
      </Routes>
    </MemoryRouter>,
  )
}

beforeEach(() => {
  kinds.mockReturnValue(ok(KINDS))
  inventory.mockReturnValue(ok(ROWS))
  detail.mockReturnValue(ok(DETAIL))
})

describe('ConfigPage', () => {
  it('lists each kind as a link with its count and the not-synced note', () => {
    at('/config', '/config', <ConfigPage />)
    expect(screen.getByRole('heading', { level: 1, name: 'Configuration' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Pipelines/ })).toHaveAttribute('href', '/config/pipelines')
    expect(screen.getByText('2 objects')).toBeInTheDocument()
    expect(screen.getByText('1 object')).toBeInTheDocument()
    expect(screen.getByText('not synced yet')).toBeInTheDocument()
  })

  it('shows loading and error states', () => {
    kinds.mockReturnValue({ isLoading: true, error: undefined })
    const { unmount } = at('/config', '/config', <ConfigPage />)
    expect(screen.getByText('Loading…')).toBeInTheDocument()
    unmount()
    kinds.mockReturnValue({ isLoading: false, error: new Error('boom') })
    at('/config', '/config', <ConfigPage />)
    expect(screen.getByRole('alert')).toHaveTextContent('Could not load kinds')
  })
})

describe('ConfigKindPage', () => {
  const kindAt = (k = 'pipelines') => at(`/config/${k}`, '/config/:kind', <ConfigKindPage />)

  it('renders Name, the kind columns, then Status, Labels, Age', () => {
    kindAt()
    const heads = within(screen.getByRole('table', { name: 'pipelines' })).getAllByRole('columnheader').map((h) => h.textContent)
    expect(heads).toEqual(['Name', 'profile', 'capability', 'sources', 'channels', 'toolsets', 'toolsMode', 'mcpConfigs', 'Status', 'Labels', 'Age'])
    expect(screen.getByRole('link', { name: /triage/ })).toHaveAttribute('href', '/config/pipelines/triage')
    expect(screen.getByTitle(/cross-reference checks/)).toHaveTextContent('2')
    expect(screen.getByText('Ready')).toBeInTheDocument()
    expect(screen.getByText('+1 more')).toBeInTheDocument()
    expect(screen.getByText('3h')).toBeInTheDocument()
    expect(screen.getByText('a')).toBeInTheDocument()
  })

  it('says nothing of the kind exists, distinct from a filter hiding everything', async () => {
    inventory.mockReturnValue(ok([]))
    kindAt()
    expect(screen.getByText('No pipelines')).toBeInTheDocument()
  })

  it('filters signals by source and phase, with its own empty state', async () => {
    const sig = (n: string, source: string, phase: string): InventoryRow => ({
      name: n, health: 'none', findings: 0, columns: { source, initiationId: `i-${n}`, phase },
    })
    inventory.mockReturnValue(ok([sig('a', 's1', 'Routed'), sig('b', 's2', 'Dropped')]))
    kindAt('signals')
    const user = userEvent.setup()
    await user.selectOptions(screen.getByLabelText('Filter by source'), 's1')
    expect(screen.queryByText('i-b')).not.toBeInTheDocument()
    await user.selectOptions(screen.getByLabelText('Filter by phase'), 'Dropped')
    expect(screen.getByText('No signal matches these filters')).toBeInTheDocument()
  })

  it('shows error', () => {
    inventory.mockReturnValue({ isLoading: false, error: new Error('x') })
    kindAt()
    expect(screen.getByRole('alert')).toHaveTextContent('Could not load pipelines')
  })
})

describe('ConfigDetailPage', () => {
  const detailAt = (k = 'pipelines') => at(`/config/${k}/triage`, '/config/:kind/:name', <ConfigDetailPage />)

  it('Overview: metadata, conditions, console checks, used by, resolved capabilities', () => {
    detailAt()
    expect(screen.getByRole('tab', { name: 'Overview' })).toHaveAttribute('aria-selected', 'true')
    const meta = screen.getByRole('region', { name: 'Metadata' })
    for (const t of ['Name', 'Namespace', 'Health', 'Conditions', 'Created', 'Labels', 'Annotations', 'UID', 'Resource version']) {
      expect(within(meta).getByText(t)).toBeInTheDocument()
    }
    expect(within(meta).getByText('u-1')).toBeInTheDocument()
    expect(within(screen.getByRole('table', { name: 'conditions' })).getByText('AllGood')).toBeInTheDocument()
    expect(within(screen.getByRole('table', { name: 'findings' })).getByText('no such channel')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /channels\/tg/ })).toHaveAttribute('href', '/config/channels/tg')
    const res = screen.getByRole('region', { name: 'Resolved capabilities' })
    expect(within(res).getByText('claude')).toBeInTheDocument()
    expect(within(res).getByText('aops')).toBeInTheDocument()
    expect(within(res).getByText('ghost')).toBeInTheDocument()
  })

  it('empty states for no conditions and no references; no resolved card off pipelines', () => {
    detail.mockReturnValue(ok({ ...DETAIL, conditions: [], usedBy: null, findings: [] }))
    detailAt('channels')
    expect(screen.getByText('This kind reports no conditions')).toBeInTheDocument()
    expect(screen.getByText('Nothing references this')).toBeInTheDocument()
    expect(screen.queryByRole('region', { name: 'Console checks' })).not.toBeInTheDocument()
    expect(screen.queryByRole('region', { name: 'Resolved capabilities' })).not.toBeInTheDocument()
  })

  it('YAML tab shows the numbered, highlighted document and copies it', async () => {
    const user = userEvent.setup()
    detailAt()
    await user.click(screen.getByRole('tab', { name: 'YAML' }))
    expect(screen.getByRole('tab', { name: 'YAML' })).toHaveAttribute('aria-selected', 'true')
    const pre = screen.getByTestId('yaml')
    expect(pre).toHaveTextContent('kind:')
    expect(pre.textContent).toContain('Pipeline')
    expect(screen.queryByRole('region', { name: 'Metadata' })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'copy pipelines/triage YAML' })).toBeInTheDocument()
    await user.click(screen.getByLabelText('Wrap long lines'))
    expect(pre).toHaveClass('wrap')
  })

  it('signals show the routing record ahead of the tabs', () => {
    detail.mockReturnValue(ok({
      ...DETAIL,
      object: { ...DETAIL.object, status: { phase: 'Dropped', reason: 'unclaimed', claimants: [{ ref: { kind: 'Pipeline', name: 'p' }, state: 'Routed', conversationRef: { name: 'c1' } }] } },
    }))
    detailAt('signals')
    const routing = screen.getByRole('region', { name: 'Routing' })
    expect(within(routing).getByText('unclaimed')).toBeInTheDocument()
    expect(within(routing).getByRole('link', { name: 'c1' })).toHaveAttribute('href', '/conversations/c1')
  })

  it('shows not found', () => {
    detail.mockReturnValue({ isLoading: false, error: new Error('nf') })
    detailAt()
    expect(screen.getByRole('alert')).toHaveTextContent('Not found')
  })
})

describe('yaml tokenizer', () => {
  it('classifies keys, values and comments', () => {
    expect(tokenizeLine('  n: 3').map((t) => t.kind)).toEqual(['text', 'key', 'punct', 'text', 'number'])
    expect(tokenizeLine('# c')[0].kind).toBe('comment')
    expect(tokenizeLine('- x').map((t) => t.kind)).toEqual(['text', 'punct', 'string'])
    expect(tokenizeLine('a: true').at(-1)?.kind).toBe('literal')
  })
})
