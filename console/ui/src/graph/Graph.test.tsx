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
