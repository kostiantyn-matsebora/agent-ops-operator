import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { QueryClientProvider } from '@tanstack/react-query'
import { createQueryClient } from '../../../../api/queryClient'
import { useDisplay } from '../../../../graph/display'
import { fixtureTopology, hop } from '../../../../test-fixtures/topology'
import { Lines } from './History'
import { TopologyPage } from './Topology'

// The workspace Topology page: the sections the classic page has, in its order,
// each with its empty and degraded states.

type Page = ReturnType<typeof import('../../../../model/topology').useTopologyPage>
let page: Partial<Page>
let chart: { data?: unknown; error?: unknown; isLoading?: boolean } = {}

vi.mock('../../../../model/topology', async (orig) => ({
  ...(await orig<typeof import('../../../../model/topology')>()),
  useTopologyPage: () => page,
  useHistoryChart: () => chart,
  useHopConversation: () => ({ data: undefined }),
}))

const data = (over: Record<string, unknown> = {}) => ({
  consoleChannel: 'console',
  unjoinedPipelines: [],
  synced: {},
  stream: { connected: true, events: 3, resyncs: 0 },
  metricsAvailable: false,
  topology: fixtureTopology(),
  ...over,
})

function draw() {
  return render(
    <QueryClientProvider client={createQueryClient()}>
      <MemoryRouter>
        <TopologyPage />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

beforeEach(() => {
  useDisplay.setState({ ...useDisplay.getInitialState(), historyOpen: true })
  chart = {}
  page = {
    data: data() as never,
    isLoading: false,
    error: null,
    events: [hop('signal.claimed', 'signal-source/prometheus-alerts', 'pipeline/alert-triage')],
    bufferCovers: true,
    historyOpen: true,
    toggleHistory: vi.fn(),
  }
})

describe('the page', () => {
  it('draws the crumb, the graph and the History card, in that order', () => {
    draw()
    const main = screen.getByRole('main', { name: 'Topology' })
    expect(within(main).getByRole('navigation', { name: 'Breadcrumb' })).toHaveTextContent('Topology')
    const graph = screen.getByTestId('graph-viewport')
    const history = screen.getByTestId('history')
    expect(graph.compareDocumentPosition(history) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    expect(screen.getByTestId('node-pipelines/alert-triage')).toBeInTheDocument()
  })

  it('says so while loading and when the topology cannot be read', () => {
    page = { isLoading: true, data: undefined }
    const { unmount } = draw()
    expect(screen.getByRole('status')).toHaveTextContent('Loading the topology')
    unmount()
    page = { isLoading: false, data: undefined, error: new Error('boom') }
    draw()
    expect(screen.getByRole('alert')).toHaveTextContent('Could not load the topology')
    expect(screen.getByRole('alert')).toHaveTextContent('boom')
  })

  it('stays quiet about the buffer while it covers the window', () => {
    draw()
    expect(screen.queryByText(/Edge rates cover/)).toBeNull()
  })

  it('tells a missing metrics backend from one that cannot extend edge rates', () => {
    page.bufferCovers = false
    const { unmount } = draw()
    expect(screen.getByText(/Edge rates cover only what the manager still holds/)).toBeInTheDocument()
    unmount()
    page.data = data({ metricsAvailable: true }) as never
    draw()
    expect(screen.getByText(/live buffer, not the whole window/)).toBeInTheDocument()
    expect(screen.queryByText(/Edge rates cover only what the manager still holds/)).toBeNull()
  })

  it('warns that a disconnected stream animates nothing, and names a gap in the record', () => {
    page.data = data({ stream: { connected: false, events: 0, resyncs: 1, lastGap: { ts: '2025-06-11T09:00:00Z' } } }) as never
    draw()
    expect(screen.getByText('The activity stream is disconnected')).toBeInTheDocument()
    expect(screen.getByText('This window has a gap in recorded activity')).toBeInTheDocument()
  })

  it('names the pipelines the console is not joined to', () => {
    page.data = data({ unjoinedPipelines: ['batch-a', 'batch-b'] }) as never
    draw()
    const card = screen.getByText('Pipelines the console is not joined to').closest('section')!
    expect(card).toHaveTextContent('batch-a')
    expect(card).toHaveTextContent('batch-b')
    expect(card).toHaveTextContent('console')
  })

  it('shows an empty graph as a stated empty state, not a blank canvas', () => {
    page.data = data({ topology: { ...fixtureTopology(), nodes: [], edges: [] } }) as never
    draw()
    expect(screen.getByTestId('graph-empty')).toHaveTextContent('Nothing to show')
  })
})

describe('the History card', () => {
  it('folds to its title from the same chevron', async () => {
    draw()
    await userEvent.click(screen.getByLabelText('Toggle history'))
    expect(page.toggleHistory).toHaveBeenCalled()
  })

  it('says no metrics backend is configured, and draws no chart', () => {
    draw()
    expect(within(screen.getByTestId('history')).getByText('No metrics backend is configured')).toBeInTheDocument()
    expect(within(screen.getByTestId('history')).queryByRole('img')).toBeNull()
  })

  it('loads, fails and runs dry per chart, each saying so', () => {
    page.data = data({ metricsAvailable: true }) as never
    chart = { isLoading: true }
    const a = draw()
    expect(screen.getAllByText('Loading…')).toHaveLength(3)
    a.unmount()
    chart = { error: new Error('backend down') }
    const b = draw()
    expect(screen.getAllByText('Unavailable')).toHaveLength(3)
    expect(screen.getAllByText(/backend down/)).toHaveLength(3)
    b.unmount()
    chart = { data: { series: [] } }
    draw()
    expect(screen.getAllByText('No data in this range')).toHaveLength(3)
  })

  it('draws an aggregate line per series, labelled', () => {
    page.data = data({ metricsAvailable: true }) as never
    chart = { data: { series: [
      { labels: { pipeline: 'a' }, points: [{ ts: 1000, value: 1 }, { ts: 2000, value: 3 }] },
      { labels: { pipeline: 'b' }, points: [{ ts: 1000, value: 2 }, { ts: 2000, value: 2 }] },
    ] } }
    draw()
    const card = screen.getByTestId('history')
    expect(within(card).getAllByText('aggregate')).toHaveLength(3)
    expect(within(card).getAllByRole('img', { name: 'Conversations started' })).toHaveLength(1)
    expect(card.querySelectorAll('polyline')).toHaveLength(6)
    expect(within(card).getAllByText('a')).not.toHaveLength(0)
  })

  it('draws a single sample without dividing by a zero span', () => {
    const { container } = render(<Lines title="t" unit="s" series={[{ labels: {}, points: [{ ts: 5, value: 0 }] }]} />)
    expect(container.querySelector('polyline')?.getAttribute('points')).not.toMatch(/NaN/)
  })
})

describe('the toolbar', () => {
  it('lists the pipelines and narrows the picture to the ones ticked', async () => {
    draw()
    await userEvent.click(screen.getByLabelText('pipelines'))
    await userEvent.click(within(screen.getByRole('group', { name: 'pipelines to show' })).getByLabelText('alert-triage'))
    expect(useDisplay.getState().pipelines).not.toBeNull()
    await userEvent.click(screen.getByText('Select all'))
    expect(useDisplay.getState().pipelines).toBeNull()
  })

  it('closes the pipeline menu with Escape and a click away', async () => {
    draw()
    await userEvent.click(screen.getByLabelText('pipelines'))
    expect(screen.getByRole('group', { name: 'pipelines to show' })).toBeInTheDocument()
    await userEvent.keyboard('{Escape}')
    expect(screen.queryByRole('group', { name: 'pipelines to show' })).toBeNull()
    await userEvent.click(screen.getByLabelText('pipelines'))
    fireEvent.pointerDown(document.body)
    expect(screen.queryByRole('group', { name: 'pipelines to show' })).toBeNull()
  })

  it('keeps the model-only detail control out of the other views', async () => {
    draw()
    expect(screen.getByLabelText('detail')).toBeInTheDocument()
    await userEvent.click(within(screen.getByRole('group', { name: 'view' })).getByText('Infrastructure'))
    expect(screen.queryByLabelText('detail')).toBeNull()
  })

  it('applies a hide expression on Enter and marks a term it does not understand', async () => {
    draw()
    await userEvent.type(screen.getByLabelText('hide expression'), 'nonsense=1{Enter}')
    expect(screen.getByText(/Not understood, matching nothing/)).toBeInTheDocument()
  })

  it('changes the window and the layout through the display store', async () => {
    draw()
    await userEvent.selectOptions(screen.getByLabelText('window'), '600')
    expect(useDisplay.getState().windowSeconds).toBe(600)
    await userEvent.selectOptions(screen.getByLabelText('layout'), 'dagre')
    expect(useDisplay.getState().views.model.layout).toBe('dagre')
  })

  it('resets the display and sets the edge labels', async () => {
    useDisplay.setState({ panelOpen: true })
    draw()
    await userEvent.selectOptions(screen.getByLabelText('Edge labels'), 'rate')
    expect(useDisplay.getState().edgeLabels).toBe('rate')
    await userEvent.click(screen.getByText('Reset display'))
    expect(useDisplay.getState().edgeLabels).not.toBe('rate')
  })
})

describe('the panels', () => {
  it('opens a hop from the feed, shows what it carried, and goes back', async () => {
    draw()
    await userEvent.click(screen.getAllByTestId('hop-row')[0])
    const panel = screen.getByTestId('hop-panel')
    expect(panel).toHaveTextContent('signal.claimed')
    expect(panel).toHaveTextContent('What crossed')
    await userEvent.click(within(panel).getByText('← Back'))
    expect(screen.queryByTestId('hop-panel')).toBeNull()
  })

  it('shows an edge with its rate and the hops recorded on it', async () => {
    draw()
    fireEvent.click(screen.getByTestId('edge-signalsources/prometheus-alerts->pipelines/alert-triage').querySelector('path[style*="pointer"]')!)
    const panel = screen.getByTestId('edge-panel')
    expect(panel).toHaveTextContent('Rate')
    expect(panel).toHaveTextContent('Recent hops on this edge')
  })

  it('offers the scope, the hide and the object link on a node', async () => {
    draw()
    fireEvent.click(screen.getByTestId('node-pipelines/alert-triage'))
    const panel = screen.getByTestId('node-panel')
    expect(within(panel).getByText('Scope to route')).toBeInTheDocument()
    expect(within(panel).getByText('Inbound')).toBeInTheDocument()
    await userEvent.click(within(panel).getByText('Scope to route'))
    expect(screen.getByTestId('scope-bar')).toBeInTheDocument()
  })
})
