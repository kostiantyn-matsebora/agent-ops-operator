import { beforeEach, describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import type { ActivityEvent, Topology } from '../api/types'
import { Graph } from './Graph'
import { useDisplay } from './display'

// The graph's behavioural contract: the animated edges are the ones the events
// name, hiding a class does not change the health summary, and a broken
// reference is drawn rather than dropped.

const topology: Topology = {
  eventNodeKinds: {
    'signal-source': 'signalsources',
    pipeline: 'pipelines',
    channel: 'channels',
    runtime: 'agentruntimes',
    toolset: 'mcptoolsets',
  },
  windowSeconds: 300,
  nodes: [
    { id: 'signalsources/events', kind: 'signalsources', name: 'events', health: 'ok', active: 0, recent: 0 },
    { id: 'signalsources/orphan', kind: 'signalsources', name: 'orphan', health: 'bad', reason: 'NoPipelineClaim', message: 'no Ready Pipeline references this source', detached: true, active: 0, recent: 0 },
    { id: 'pipelines/ops', kind: 'pipelines', name: 'ops', health: 'ok', active: 2, recent: 5 },
    { id: 'channels/console', kind: 'channels', name: 'console', health: 'ok', active: 0, recent: 0 },
    { id: 'mcptoolsets/admin', kind: 'mcptoolsets', name: 'admin', health: 'bad', active: 0, recent: 0 },
  ],
  edges: [
    { from: 'signalsources/events', to: 'pipelines/ops', kind: 'feeds', traffic: { events: 8, errors: 0, ratePerMin: 3 } },
    { from: 'pipelines/ops', to: 'channels/console', kind: 'posts', traffic: { events: 2, errors: 0, ratePerMin: 1, unconfirmed: true } },
    { from: 'pipelines/ops', to: 'mcptoolsets/admin', kind: 'uses' },
  ],
}

function draw(events: ActivityEvent[] = []) {
  return render(
    <MemoryRouter>
      <Graph topology={topology} liveEvents={events} />
    </MemoryRouter>,
  )
}

beforeEach(() => {
  useDisplay.getState().reset()
  // the capability layer starts folded away; these tests want it visible
  useDisplay.setState({ hidden: {} })
})

describe('Graph', () => {
  it('draws every node class and the wiring between them', () => {
    draw()
    expect(screen.getByTestId('node-pipelines/ops')).toBeInTheDocument()
    expect(screen.getByTestId('node-mcptoolsets/admin')).toBeInTheDocument()
    expect(screen.getByTestId('edge-signalsources/events->pipelines/ops')).toBeInTheDocument()
  })

  it('renders an unclaimed source as detached with its reason on select', async () => {
    draw()
    const node = screen.getByTestId('node-signalsources/orphan')
    expect(node).toHaveAttribute('data-detached', 'true')
    await userEvent.click(node)
    // the Wired=False message is the whole value of showing this node
    expect(screen.getByText(/no Ready Pipeline references this source/)).toBeInTheDocument()
  })

  it('marks an enqueued-but-unconfirmed edge distinctly from a delivered one', () => {
    draw()
    expect(screen.getByTestId('edge-pipelines/ops->channels/console')).toHaveAttribute(
      'data-tone',
      'unconfirmed',
    )
    expect(screen.getByTestId('edge-signalsources/events->pipelines/ops')).toHaveAttribute(
      'data-tone',
      'ok',
    )
  })

  it('flashes exactly the edges the live events name', () => {
    draw([
      {
        cursor: '1', ts: new Date().toISOString(), kind: 'signal.claimed', status: 'ok',
        from: { kind: 'signal-source', name: 'events' },
        to: { kind: 'pipeline', name: 'ops' },
      },
    ])
    expect(screen.getByTestId('edge-signalsources/events->pipelines/ops')).toHaveAttribute(
      'data-flashing',
      'true',
    )
    expect(screen.getByTestId('edge-pipelines/ops->channels/console')).toHaveAttribute(
      'data-flashing',
      'false',
    )
  })

  it('stops animating when the Display panel turns traffic off', async () => {
    draw()
    expect(screen.getByTestId('edge-signalsources/events->pipelines/ops')).toHaveAttribute(
      'data-animated',
      'true',
    )
    await userEvent.click(screen.getByLabelText('Animate traffic'))
    expect(screen.getByTestId('edge-signalsources/events->pipelines/ops')).toHaveAttribute(
      'data-animated',
      'false',
    )
  })

  it('keeps health stable and warns when a hidden class is failing', async () => {
    draw()
    expect(screen.getByText('2 failing')).toBeInTheDocument()

    await userEvent.click(screen.getByLabelText('Toolsets'))
    expect(screen.queryByTestId('node-mcptoolsets/admin')).toBeNull()
    // the count MUST NOT move because something was hidden
    expect(screen.getByText('2 failing')).toBeInTheDocument()
    expect(screen.getByText(/hidden element\(s\) are failing/)).toBeInTheDocument()
  })

  it('gives each element class its own shape', () => {
    draw()
    const shapeOf = (id: string) => screen.getByTestId(`node-${id}`).getAttribute('data-shape')
    expect(shapeOf('signalsources/events')).toBe('hexagon')
    expect(shapeOf('pipelines/ops')).toBe('rect')
    expect(shapeOf('channels/console')).toBe('cylinder')
    expect(shapeOf('mcptoolsets/admin')).toBe('diamond')
  })

  it('draws a labelled boundary per lane', () => {
    draw()
    expect(screen.getByTestId('lane-ingest')).toBeInTheDocument()
    expect(screen.getByTestId('lane-wiring')).toBeInTheDocument()
    expect(screen.getByTestId('lane-delivery')).toBeInTheDocument()
    expect(screen.getByText('Ingest')).toBeInTheDocument()
  })

  it('offers zoom controls over a pannable canvas', () => {
    draw()
    expect(screen.getByTestId('graph-viewport')).toBeInTheDocument()
    expect(screen.getByLabelText('zoom in')).toBeInTheDocument()
    expect(screen.getByLabelText('zoom out')).toBeInTheDocument()
    expect(screen.getByLabelText('fit to view')).toBeInTheDocument()
    expect(screen.getByLabelText('zoom level')).toBeInTheDocument()
  })

  it('zooms in and out, and reports the level', async () => {
    draw()
    const level = () => screen.getByLabelText('zoom level').textContent
    const before = level()
    await userEvent.click(screen.getByLabelText('zoom in'))
    expect(level()).not.toBe(before)
    await userEvent.click(screen.getByLabelText('zoom out'))
    expect(level()).toBe(before)
  })

  it('hides the Display panel on request, keeping the health summary visible', async () => {
    draw()
    expect(screen.getByText('Element classes')).toBeInTheDocument()

    await userEvent.click(screen.getByLabelText('hide display panel'))
    expect(screen.queryByText('Element classes')).toBeNull()
    // a hidden panel must not also hide that a filter is on
    expect(screen.getByText('2 failing')).toBeInTheDocument()

    await userEvent.click(screen.getByLabelText('show display panel'))
    expect(screen.getByText('Element classes')).toBeInTheDocument()
  })
})

// Scoping. Clicking an element narrows the graph to what it is connected to,
// and the same click that opened its details is the one that closes both.
describe('scoped view', () => {
  const nodes = () => screen.getAllByTestId(/^node-/).map((n) => n.getAttribute('data-testid'))

  it('narrows to the clicked element and what it is connected to', async () => {
    draw()
    await userEvent.click(screen.getByTestId('node-pipelines/ops'))

    // the strand around the pipeline stays; the unclaimed source is not on it
    expect(nodes()).toEqual(expect.arrayContaining([
      'node-pipelines/ops', 'node-signalsources/events',
      'node-channels/console', 'node-mcptoolsets/admin',
    ]))
    expect(screen.queryByTestId('node-signalsources/orphan')).toBeNull()
    expect(screen.getByTestId('scope-bar')).toHaveTextContent('Scoped to ops')
  })

  it('reports a failing element it put out of view, separately from hidden classes', async () => {
    draw()
    await userEvent.click(screen.getByTestId('node-pipelines/ops'))

    const alert = screen.getByTestId('scope-failing')
    expect(alert).toHaveTextContent('1 failing element(s) are outside this scope')
    expect(alert).toHaveTextContent('signalsources')
  })

  it('does not change the reported health', async () => {
    draw()
    await userEvent.click(screen.getByLabelText('hide display panel'))
    const before = screen.getByText('2 failing').textContent

    await userEvent.click(screen.getByTestId('node-pipelines/ops'))
    // the orphan is out of scope and still counted
    expect(screen.getByText('2 failing').textContent).toBe(before)
  })

  it('narrows further on request and says what the depth cut off', async () => {
    draw()
    await userEvent.click(screen.getByTestId('node-channels/console'))
    // the channel's route: the pipeline that posts to it and that pipeline's
    // source. The toolset hangs off the pipeline but is not on the channel's
    // route, so it is not here.
    expect(nodes().sort()).toEqual([
      'node-channels/console', 'node-pipelines/ops', 'node-signalsources/events',
    ])

    await userEvent.click(screen.getByLabelText('1 hop'))
    expect(nodes().sort()).toEqual(['node-channels/console', 'node-pipelines/ops'])
    expect(screen.getByTestId('scope-bar')).toHaveTextContent('1 connected beyond this depth')
  })

  it('offers only the levels the route actually has', async () => {
    draw()
    // The channel's route is 2 deep — the pipeline that posts to it, then that
    // pipeline's source — so exactly one ring sits inside All.
    await userEvent.click(screen.getByTestId('node-channels/console'))
    expect(screen.getByLabelText('1 hop')).toBeInTheDocument()
    expect(screen.queryByLabelText('2 hop')).toBeNull()
    expect(screen.getByLabelText('all connected')).toBeInTheDocument()

    // The pipeline is the CENTRE of its own route: here everything it binds is
    // one hop, so there is no ring inside All and no choice to offer.
    await userEvent.click(screen.getByLabelText('reset scope'))
    await userEvent.click(screen.getByTestId('node-pipelines/ops'))
    expect(screen.queryByLabelText('1 hop')).toBeNull()
  })

  it('offers no level control for an element whose route is only itself', async () => {
    // A detached source has no route at all. Offering levels over it once threw
    // RangeError: Invalid array length, which took the whole view down.
    draw()
    await userEvent.click(screen.getByTestId('node-signalsources/orphan'))
    expect(screen.getByTestId('scope-bar')).toBeInTheDocument()
    expect(screen.queryByLabelText('all connected')).toBeNull()
    expect(screen.queryByLabelText('1 hop')).toBeNull()
  })

  it('returns to the whole picture on Reset, and on clicking the focused element again', async () => {
    draw()
    await userEvent.click(screen.getByTestId('node-pipelines/ops'))
    await userEvent.click(screen.getByLabelText('reset scope'))
    expect(screen.getByTestId('node-signalsources/orphan')).toBeInTheDocument()
    expect(screen.queryByTestId('scope-bar')).toBeNull()

    await userEvent.click(screen.getByTestId('node-pipelines/ops'))
    expect(screen.queryByTestId('node-signalsources/orphan')).toBeNull()
    await userEvent.click(screen.getByTestId('node-pipelines/ops'))
    expect(screen.getByTestId('node-signalsources/orphan')).toBeInTheDocument()
  })

  it('opens unscoped, so a scope is never restored as though it were the graph', () => {
    // The display panel's selections persist on purpose. A scope is a question
    // asked once, and a page that reopened narrowed would present a filtered
    // graph as the whole one.
    const { unmount } = draw()
    unmount()
    draw()
    expect(screen.queryByTestId('scope-bar')).toBeNull()
    expect(screen.getByTestId('node-signalsources/orphan')).toBeInTheDocument()
  })
})
