import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, fireEvent, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { QueryClientProvider } from '@tanstack/react-query'
import { createQueryClient } from '../api/queryClient'
import type { ActivityEvent, Topology } from '../api/types'
import { CLAUDE_IMAGE, detailWithRun, fixtureTopology, hop } from '../test-fixtures/topology'
import { useDisplay } from './display'
import { Graph, type GraphProps } from './Graph'
import { BUILTIN_ICONS } from '../icons/builtin'

// The graph's behavioural contract: edges with traffic carry a stream, a hop
// that arrives pulses the edge it crossed and opens on a click, each view keeps
// its own classes, hiding never hides a failure silently, and both replays
// show what the buffer holds and say what it does not.

const recent = (msAgo: number, e: ActivityEvent): ActivityEvent => ({ ...e, ts: new Date(Date.now() - msAgo).toISOString() })

function events(): ActivityEvent[] {
  return [
    recent(40_000, hop('signal.received', 'signal-adapter/alertmanager', 'signal-source/prometheus-alerts')),
    recent(39_000, hop('signal.claimed', 'signal-source/prometheus-alerts', 'pipeline/alert-triage')),
    recent(38_000, hop('channel.op.enqueued', 'conversation/prometheus-alerts-a1', 'channel/ops-chat', {
      conversation: 'prometheus-alerts-a1', opId: 'send:prometheus-alerts-a1:ops-chat:r1',
    })),
  ]
}

let client = createQueryClient()

function draw(props: Partial<GraphProps> = {}, topology: Topology = fixtureTopology()) {
  const all = { topology, events: events(), ...props }
  const ui = (p: GraphProps) => (
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <Graph {...p} />
      </MemoryRouter>
    </QueryClientProvider>
  )
  const r = render(ui(all))
  return { ...r, redraw: (over: Partial<GraphProps>) => r.rerender(ui({ ...all, ...over })) }
}

beforeEach(() => {
  client = createQueryClient()
  useDisplay.setState({ ...useDisplay.getInitialState(), panelOpen: true })
})

afterEach(() => {
  vi.useRealTimers()
})

const node = (id: string) => screen.getByTestId(`node-${id}`)
const view = (label: string) => userEvent.click(within(screen.getByLabelText('view')).getByText(label))

describe('the three views', () => {
  it('draws the Model, and switches to Components and Infrastructure', async () => {
    draw()
    expect(node('pipelines/alert-triage')).toBeInTheDocument()
    expect(screen.getByTestId('edge-pipelines/alert-triage->agentruntimes/default')).toBeInTheDocument()
    await view('Components')
    expect(node('manager/manager')).toBeInTheDocument()
    expect(screen.getByTestId(`badge-runtime-image/${CLAUDE_IMAGE}`)).toHaveTextContent('×2')
    await view('Infrastructure')
    expect(node('pods/agentops-manager-7c9d')).toBeInTheDocument()
    expect(screen.queryByTestId('node-pipelines/alert-triage')).toBeNull()
  })

  it('shows an unclaimed source detached, with its reason verbatim on select', async () => {
    draw()
    const n = node('signalsources/bench-sensors')
    expect(n).toHaveAttribute('data-detached', 'true')
    fireEvent.click(n)
    expect(screen.getByTestId('node-panel')).toHaveTextContent('no Ready Pipeline lists this source')
  })
})

describe('traffic', () => {
  it('streams an edge with events and leaves a quiet one idle', () => {
    draw()
    const busy = screen.getByTestId('edge-signalsources/prometheus-alerts->pipelines/alert-triage')
    expect(busy).toHaveAttribute('data-stream', 'on')
    expect(busy).toHaveAttribute('data-tone', 'ok')
    expect(screen.getByTestId('stream-signalsources/prometheus-alerts->pipelines/alert-triage')).toBeInTheDocument()
    const quiet = screen.getByTestId('edge-pipelines/nightly-report->channels/console')
    expect(quiet).toHaveAttribute('data-tone', 'idle')
    expect(quiet).toHaveAttribute('data-stream', 'off')
  })

  it('marks an enqueued-but-unconfirmed edge distinctly from a delivered one', () => {
    draw()
    expect(screen.getByTestId('edge-pipelines/alert-triage->channels/ops-chat')).toHaveAttribute('data-tone', 'unconfirmed')
  })

  it('pulses an arriving hop on the edge it names, and opens it on a click', async () => {
    const held = events()
    const { redraw } = draw({ events: held })
    expect(screen.queryAllByTestId('pulse')).toHaveLength(0)
    const dispatch = recent(0, hop('run.dispatched', 'pipeline/alert-triage', 'runtime/default', {
      conversation: 'prometheus-alerts-a1', pipeline: 'alert-triage', runId: 'r9',
    }))
    redraw({ events: [...held, dispatch] })
    const pulse = screen.getByTestId('pulse')
    expect(pulse).toHaveAttribute('data-edge', 'pipelines/alert-triage->agentruntimes/default')
    fireEvent.click(pulse)
    expect(screen.getByTestId('hop-panel')).toHaveTextContent('run.dispatched')
  })

  it('pulses a hop with no destination on its node', () => {
    const held = events()
    const { redraw } = draw({ events: held })
    redraw({ events: [...held, recent(0, hop('signal.dropped', 'signal-source/bench-sensors', null, { status: 'error' }))] })
    expect(screen.getByTestId('pulse')).toHaveAttribute('data-edge', 'signalsources/bench-sensors')
  })

  it('shows latency only on an edge that measured one', async () => {
    useDisplay.setState({ edgeLabels: 'latency' })
    draw({ events: [...events(), recent(1000, hop('run.completed', 'runtime/default', 'pipeline/alert-triage', { latencyMs: 2400 }))] })
    expect(screen.getByTestId('edge-pipelines/alert-triage->agentruntimes/default')).toHaveTextContent('2.4s')
    expect(screen.getByTestId('edge-pipelines/nightly-report->channels/console').textContent).not.toMatch(/\d/)
  })
})

describe('the display control', () => {
  it('keeps each view\'s own classes', async () => {
    draw()
    await view('Components')
    await userEvent.click(screen.getByLabelText('Channel adapters'))
    expect(screen.queryByTestId('node-channel-adapter/telegram')).toBeNull()
    await view('Model')
    expect(node('channeladapters/telegram')).toBeInTheDocument()
    await view('Components')
    expect(screen.queryByTestId('node-channel-adapter/telegram')).toBeNull()
  })

  it('lists the spine and will not hide it', async () => {
    draw()
    expect(screen.getByLabelText('Pipelines')).toBeDisabled()
  })

  it('still reports a failure it hides', async () => {
    draw()
    await userEvent.click(screen.getByLabelText('Signal sources'))
    expect(screen.queryByTestId('node-signalsources/bench-sensors')).toBeNull()
    expect(screen.getByTestId('failing-hidden')).toHaveTextContent('Signal source')
    expect(screen.getByText('1 failing')).toBeInTheDocument()
    expect(screen.getByTestId('hidden-chip')).toHaveTextContent('5 hidden · 1 failing')
  })

  it('removes idle elements and counts them', async () => {
    draw()
    await userEvent.click(screen.getByLabelText('Idle elements'))
    await userEvent.click(screen.getByLabelText('Idle edges'))
    expect(screen.queryByTestId('node-mcptoolsets/agentops-shell')).toBeNull()
    expect(node('pipelines/alert-triage')).toBeInTheDocument()
    expect(screen.getByTestId('hidden-chip')).toBeInTheDocument()
  })
})

describe('scope', () => {
  it('scopes to an element\'s route, names it, and returns without a reload', async () => {
    draw()
    fireEvent.doubleClick(node('pipelines/nightly-report'))
    expect(screen.getByTestId('scope-bar')).toHaveTextContent('nightly-report')
    expect(screen.queryByTestId('node-pipelines/k8s-observe')).toBeNull()
    await userEvent.click(screen.getByLabelText('reset scope'))
    expect(node('pipelines/k8s-observe')).toBeInTheDocument()
  })

  it('keeps only the selected pipelines\' routes', async () => {
    useDisplay.setState({ pipelines: ['nightly-report'] })
    draw()
    expect(node('pipelines/nightly-report')).toBeInTheDocument()
    expect(screen.queryByTestId('node-pipelines/alert-triage')).toBeNull()
    expect(screen.getByText(/on other routes/)).toBeInTheDocument()
  })
})

describe('an Infrastructure pod', () => {
  it('opens into its containers and collapses again', async () => {
    draw()
    await view('Infrastructure')
    fireEvent.doubleClick(node('pods/agentops-conv-prometheus-alerts-a1'))
    expect(node('containers/agentops-conv-prometheus-alerts-a1/egress-proxy')).toBeInTheDocument()
    expect(screen.getByTestId('box:pod:agentops-conv-prometheus-alerts-a1')).toBeInTheDocument()
    fireEvent.click(screen.getByTestId('box:pod:agentops-conv-prometheus-alerts-a1'))
    expect(node('pods/agentops-conv-prometheus-alerts-a1')).toBeInTheDocument()
  })
})

describe('replay', () => {
  it('replays a window and admits what the buffer does not hold', async () => {
    draw({ bufferStart: Date.now() - 2 * 60_000 })
    await userEvent.click(within(screen.getByLabelText('live or replay')).getByText('Replay'))
    expect(screen.getByTestId('replay-bar')).toBeInTheDocument()
    expect(screen.getByTestId('replay-not-held')).toHaveTextContent('the first 3 minutes')
    fireEvent.change(screen.getByLabelText('replay frame'), { target: { value: '30' } })
    expect(screen.getByTestId('replay-time')).toHaveTextContent('[30/30]')
    await userEvent.click(screen.getByLabelText('close replay'))
    expect(screen.queryByTestId('replay-bar')).toBeNull()
  })

  it('replays one conversation, dimming what its run did not touch', async () => {
    const run = [
      recent(20_000, hop('input.queued', 'signal-source/prometheus-alerts', 'conversation/prometheus-alerts-a1', { conversation: 'prometheus-alerts-a1', pipeline: 'alert-triage' })),
      recent(19_000, hop('run.dispatched', 'pipeline/alert-triage', 'runtime/default', { conversation: 'prometheus-alerts-a1', pipeline: 'alert-triage', runId: 'r1' })),
      recent(9_000, hop('run.completed', 'runtime/default', 'pipeline/alert-triage', { conversation: 'prometheus-alerts-a1', pipeline: 'alert-triage', runId: 'r1', latencyMs: 10_000 })),
    ]
    draw({ events: run })
    await userEvent.click(screen.getByLabelText('replay prometheus-alerts-a1'))
    expect(screen.getByTestId('conversation-bar')).toHaveTextContent('+0.0s · input.queued')
    expect(node('pipelines/alert-triage')).toHaveAttribute('data-dim', 'false')
    expect(node('pipelines/nightly-report')).toHaveAttribute('data-dim', 'true')
    await userEvent.click(screen.getByText('Step ›'))
    expect(screen.getByTestId('conversation-time')).toHaveTextContent('+1.0s · run.dispatched')
    await view('Components')
    expect(node('context-sync/context-sync')).toHaveAttribute('data-dim', 'false')
    expect(node('signal-adapter/cron')).toHaveAttribute('data-dim', 'true')
    await userEvent.click(screen.getByLabelText('leave conversation replay'))
    expect(screen.queryByTestId('conversation-bar')).toBeNull()
  })

  it('advances a frame a second at the fastest speed, and pausing keeps the frame', () => {
    vi.useFakeTimers({ shouldAdvanceTime: false })
    draw({ bufferStart: Date.now() - 3_600_000 })
    fireEvent.click(within(screen.getByLabelText('live or replay')).getByText('Replay'))
    fireEvent.click(within(screen.getByLabelText('replay speed')).getByText('fast'))
    fireEvent.click(screen.getByText('▶ Play'))
    for (let i = 0; i < 3; i++) act(() => vi.advanceTimersByTime(1000))
    expect(screen.getByTestId('replay-time')).toHaveTextContent('[3/30]')
    fireEvent.click(screen.getByText('❚❚ Pause'))
    act(() => vi.advanceTimersByTime(5000))
    expect(screen.getByTestId('replay-time')).toHaveTextContent('[3/30]')
  })

  it('opens on a conversation when asked to', () => {
    draw({ conversation: 'prometheus-alerts-a1' })
    expect(screen.getByTestId('conversation-bar')).toBeInTheDocument()
  })
})

describe('what crossed', () => {
  it('shows a completion\'s result from the conversation\'s recorded status', async () => {
    const detail = detailWithRun()
    client.setQueryData(['conversation', 'cluster-events-b7'], detail)
    const done = recent(1000, hop('run.completed', 'runtime/default', 'pipeline/k8s-observe', {
      conversation: 'cluster-events-b7', pipeline: 'k8s-observe', runId: 'r7', detail: 'succeeded (exit 0)',
    }))
    draw({ events: [...events(), done] })
    const row = screen.getAllByTestId('hop-row').find((r) => r.textContent?.includes('run.completed'))!
    await act(async () => fireEvent.click(row))
    expect(screen.getByTestId('hop-body')).toHaveTextContent('Container `checkout` is killed at 512Mi')
  })
})

describe('the side panels', () => {
  it('folds the column to a strip and brings it back from the same place, and a selection still shows', async () => {
    draw()
    expect(screen.getByTestId('hop-feed')).toBeInTheDocument()
    expect(screen.getByTestId('display-panel')).toBeInTheDocument()
    await userEvent.click(screen.getByLabelText('Hide side panels'))
    expect(screen.queryByTestId('hop-feed')).not.toBeInTheDocument()
    expect(screen.queryByTestId('display-panel')).not.toBeInTheDocument()
    expect(useDisplay.getState().sideOpen).toBe(false)
    // Selecting an element shows its panel whatever the fold says.
    await userEvent.click(node('pipelines/alert-triage'))
    expect(screen.getByTestId('node-panel')).toBeInTheDocument()
    await userEvent.click(screen.getByLabelText('Hide side panels'))
    expect(screen.queryByTestId('node-panel')).not.toBeInTheDocument()
    await userEvent.click(screen.getByLabelText('Show side panels'))
    expect(screen.getByTestId('hop-feed')).toBeInTheDocument()
  })

  it('folds the feed and the display card to their titles', async () => {
    draw()
    expect(screen.getAllByTestId('hop-row').length).toBeGreaterThan(0)
    await userEvent.click(screen.getByLabelText('Toggle hop feed'))
    expect(screen.queryByTestId('hop-row')).not.toBeInTheDocument()
    expect(screen.getByTestId('hop-feed')).toBeInTheDocument()
    expect(useDisplay.getState().feedOpen).toBe(false)
    expect(screen.getByLabelText('Traffic animation')).toBeInTheDocument()
    await userEvent.click(screen.getByLabelText('Toggle display panel'))
    expect(screen.queryByLabelText('Traffic animation')).not.toBeInTheDocument()
    expect(useDisplay.getState().panelOpen).toBe(false)
  })
})

describe('a declared icon', () => {
  it('is drawn in the pipeline\'s mark in place of the class glyph, in whichever form it takes', async () => {
    const topo = fixtureTopology()
    const p = topo.nodes.find((n) => n.id === 'pipelines/alert-triage')!
    p.icon = 'aops:kubernetes'
    const { redraw } = draw({}, topo)
    expect(within(node('pipelines/alert-triage')).getByTestId('mark-icon').querySelector('path')?.getAttribute('d')).toBe(BUILTIN_ICONS.kubernetes)
    expect(within(node('pipelines/nightly-report')).queryByTestId('mark-icon')).toBeNull()
    // The same icon beside the name in its panel and in the pipeline selector.
    await userEvent.click(node('pipelines/alert-triage'))
    expect(within(screen.getByTestId('node-panel')).getByText('alert-triage').parentElement?.querySelector('svg path')?.getAttribute('d')).toBe(BUILTIN_ICONS.kubernetes)
    await userEvent.click(screen.getByLabelText('pipelines'))
    const option = screen.getAllByText('alert-triage').map((e) => e.closest('li')).find(Boolean)
    expect(option?.querySelector('svg path')?.getAttribute('d')).toBe(BUILTIN_ICONS.kubernetes)
    await userEvent.keyboard('{Escape}')
    const emoji = fixtureTopology()
    emoji.nodes.find((n) => n.id === 'pipelines/alert-triage')!.icon = '🛠'
    redraw({ topology: emoji })
    expect(within(node('pipelines/alert-triage')).getByTestId('mark-icon')).toHaveTextContent('🛠')
    // An unknown built-in name draws nothing rather than a broken mark: the glyph stays.
    const unknown = fixtureTopology()
    unknown.nodes.find((n) => n.id === 'pipelines/alert-triage')!.icon = 'aops:nothing-like-this'
    redraw({ topology: unknown })
    expect(within(node('pipelines/alert-triage')).queryByTestId('mark-icon')).toBeNull()
  })
})
