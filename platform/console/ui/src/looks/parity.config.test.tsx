import { describe, expect, it, vi } from 'vitest'
import { render, within } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import type { Detail, InventoryRow, KindInfo } from '../api/types'
import { ConfigDetailPage as ClassicDetail, ConfigKindPage as ClassicKind, ConfigPage as ClassicIndex } from './classic/pages/Config'
import { ConfigDetailPage, ConfigKindPage, ConfigPage } from './workspace/pages/config'

// TASK 4.3: both looks draw the same Configuration sections, in the same
// order, from the same data. This file sits at the seam (looks/), the one
// place that may name two looks.

type Q<T> = { data?: T; isLoading: boolean; error: unknown }
const ok = <T,>(data: T): Q<T> => ({ data, isLoading: false, error: undefined })
const KINDS: KindInfo[] = [
  { kind: 'pipelines', title: 'Pipelines', count: 2, synced: true },
  { kind: 'channels', title: 'Channels', count: 1, synced: true },
  { kind: 'signals', title: 'Signals', count: 0, synced: false },
]
const ROWS: InventoryRow[] = [{ name: 'triage', health: 'ok', findings: 1, columns: { profile: 'p1' } }]
const DETAIL: Detail = {
  object: { kind: 'Pipeline', metadata: { name: 'triage', namespace: 'agent-ops' }, spec: {} },
  health: 'ok',
  conditions: [{ type: 'Ready', status: 'True', reason: 'AllGood' }],
  yaml: 'kind: Pipeline\n',
  usedBy: [{ kind: 'channels', name: 'tg', field: 'spec.pipelineRef' }],
  findings: [{ kind: 'pipelines', name: 'triage', check: 'c', reason: 'R', message: 'm' }],
  resolved: { pipeline: 'triage', profile: 'p', runtime: 'claude', allowedTools: [], toolsMode: 'overwrite', toolsets: [], mcpConfigs: [], mcpServers: [] },
}

vi.mock('../api/hooks', async (orig) => ({
  ...(await orig<object>()),
  useKinds: () => ok(KINDS),
  useInventory: () => ok(ROWS),
  useDetail: () => ok(DETAIL),
  usePipelineIcon: () => () => undefined,
}))

function at(path: string, route: string, el: JSX.Element) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route path={route} element={el} />
      </Routes>
    </MemoryRouter>,
  )
}

describe('Configuration parity: classic and workspace', () => {
  const order = (text: string, titles: string[]) =>
    titles.map((t) => [t, text.indexOf(t)] as const).filter(([, i]) => i >= 0).sort((a, b) => a[1] - b[1]).map(([t]) => t)

  it('kind index: same kinds in the same order', () => {
    const a = at('/config', '/config', <ConfigPage />)
    const wk = a.container.textContent ?? ''
    a.unmount()
    const b = at('/config', '/config', <ClassicIndex />)
    const cl = b.container.textContent ?? ''
    const names = KINDS.map((k) => k.title)
    expect(order(wk, names)).toEqual(order(cl, names))
    expect(order(wk, names)).toEqual(names)
    expect(wk).toContain('not synced yet')
    expect(cl).toContain('not synced yet')
  })

  it('kind table: same columns in the same order', () => {
    const heads = (c: HTMLElement) => within(c).getAllByRole('columnheader').map((h) => h.textContent)
    const a = at('/config/pipelines', '/config/:kind', <ConfigKindPage />)
    const wk = heads(a.container)
    a.unmount()
    const b = at('/config/pipelines', '/config/:kind', <ClassicKind />)
    expect(wk).toEqual(heads(b.container))
  })

  it('object Overview: same sections in the same order', () => {
    const sections = ['Metadata', 'Conditions', 'Console checks', 'Used by', 'Resolved capabilities']
    const a = at('/config/pipelines/triage', '/config/:kind/:name', <ConfigDetailPage />)
    const wk = order(a.container.textContent ?? '', sections)
    a.unmount()
    const b = at('/config/pipelines/triage', '/config/:kind/:name', <ClassicDetail />)
    const cl = order(b.container.textContent ?? '', sections)
    expect(wk).toEqual(sections)
    expect(wk).toEqual(cl)
  })
})
