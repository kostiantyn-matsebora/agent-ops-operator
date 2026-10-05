import { describe, expect, it } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { Timeline } from './Timeline'
import type { ActivityEvent, Message } from '../../api/types'

function msg(id: string, kind: string, at: string, over: Partial<Message> = {}): Message {
  return { id, thread: 't', kind, text: `${id}-text`, at, ...over }
}

function ev(cursor: string, kind: string, runId: string, ts: string, over: Partial<ActivityEvent> = {}): ActivityEvent {
  return { cursor, kind, runId, ts, status: 'ok', ...over }
}

describe('ordering and folding (design D-G)', () => {
  it('interleaves messages and run events by time, folding a run to one line', () => {
    const messages = [
      msg('m1', 'signal', '2024-01-01T00:00:00Z'),
      msg('m2', 'agent', '2024-01-01T00:00:05Z'),
    ]
    const events = [
      ev('c1', 'run.dispatched', 'r1', '2024-01-01T00:00:01Z'),
      ev('c2', 'runtime.starting', 'r1', '2024-01-01T00:00:02Z'),
      ev('c3', 'run.completed', 'r1', '2024-01-01T00:00:04Z'),
    ]
    render(<Timeline messages={messages} events={events} presence={false} />)
    const timeline = screen.getByTestId('timeline')
    const text = timeline.textContent ?? ''
    // the signal, then the run's folded line, then the answer, in time order
    expect(text.indexOf('m1-text')).toBeLessThan(text.indexOf('run dispatched'))
    expect(text.indexOf('run dispatched')).toBeLessThan(text.indexOf('m2-text'))
    // folded to ONE line rather than three
    expect(screen.getByText('run dispatched · runtime starting · run completed')).toBeInTheDocument()
  })
})

describe('a coordinator root interleaves extraItems with its own messages by time (item 15)', () => {
  it('renders signal, reasoning-1, invoke-card, reasoning-2 in time order — one transcript, not a separate status log', () => {
    const messages = [
      msg('sig-1', 'signal', '2024-01-01T00:00:00Z', { text: 'Disk pressure on node-3' }),
      msg('reason-1', 'agent', '2024-01-01T00:00:10Z', { text: 'Investigating node-3 for disk pressure causes' }),
      msg('reason-2', 'agent', '2024-01-01T00:00:50Z', { text: 'Node-3 disk pressure resolved after eviction' }),
    ]
    const extraItems = [
      { key: 'invoke-mid-1', at: Date.parse('2024-01-01T00:00:30Z'), node: <div>invoked diagnose</div> },
    ]
    render(<Timeline messages={messages} events={[]} presence={false} extraItems={extraItems} />)
    const timeline = screen.getByTestId('timeline')
    const text = timeline.textContent ?? ''
    expect(text.indexOf('Disk pressure on node-3')).toBeLessThan(text.indexOf('Investigating node-3'))
    expect(text.indexOf('Investigating node-3')).toBeLessThan(text.indexOf('invoked diagnose'))
    expect(text.indexOf('invoked diagnose')).toBeLessThan(text.indexOf('resolved after eviction'))
  })

  it('renders the signal as a real card — not raw JSON — and the reasoning turns as ordinary agent text', () => {
    const messages = [
      msg('sig-1', 'signal', '2024-01-01T00:00:00Z', {
        text: '📣 **Disk pressure**\n\n**Source** `node-3` · **Coordinator** `agentops-coordinator`',
        payload: '{"raw":"event document"}',
      }),
      msg('reason-1', 'agent', '2024-01-01T00:00:10Z', { text: 'Investigating node-3 for disk pressure causes' }),
    ]
    render(<Timeline messages={messages} events={[]} presence={false} />)
    // The card's own fields render as text, never as a JSON blob on screen.
    expect(screen.getByText('Disk pressure')).toBeInTheDocument()
    expect(screen.queryByText(/"raw":"event document"/)).toBeNull()
    // The payload is foldable, not inline.
    expect(screen.getByText(/Payload/)).toBeInTheDocument()
    expect(screen.getByText('Investigating node-3 for disk pressure causes')).toBeInTheDocument()
  })
})

describe('an ack is presence, never a bubble', () => {
  it('drops the ack from the transcript and shows the typing row instead', () => {
    const messages = [msg('m1', 'agent', '2024-01-01T00:00:00Z'), msg('m2', 'ack', '2024-01-01T00:00:01Z', { text: 'On it…' })]
    render(<Timeline messages={messages} events={[]} presence />)
    expect(screen.queryByText('m2-text')).toBeNull()
    expect(screen.queryByText('On it…')).toBeNull()
    expect(screen.getByTestId('typing')).toBeInTheDocument()
  })

  it('shows no typing row once the run is no longer inflight', () => {
    const messages = [msg('m1', 'agent', '2024-01-01T00:00:00Z')]
    render(<Timeline messages={messages} events={[]} presence={false} />)
    expect(screen.queryByTestId('typing')).toBeNull()
  })
})

describe('the new-messages divider', () => {
  it('sits above the first message after the watermark', () => {
    const messages = [
      msg('m1', 'agent', '2024-01-01T00:00:00Z'),
      msg('m2', 'relay', '2024-01-01T00:01:00Z', { sender: 'oncall' }),
      msg('m3', 'agent', '2024-01-01T00:02:00Z'),
    ]
    render(<Timeline messages={messages} events={[]} presence={false} readAt="2024-01-01T00:00:30Z" />)
    expect(screen.getByText('New messages')).toBeInTheDocument()
    // m1 (before the watermark) has no divider above it; m2 does.
    const timeline = screen.getByTestId('timeline')
    const text = timeline.textContent ?? ''
    expect(text.indexOf('New messages')).toBeGreaterThan(text.indexOf('m1-text'))
    expect(text.indexOf('New messages')).toBeLessThan(text.indexOf('m2-text'))
  })

  it('never places a divider when every message is already read', () => {
    const messages = [msg('m1', 'agent', '2024-01-01T00:00:00Z')]
    render(<Timeline messages={messages} events={[]} presence={false} readAt="2024-01-01T00:05:00Z" />)
    expect(screen.queryByText('New messages')).toBeNull()
  })
})

describe('autoscroll follows only at the bottom', () => {
  it('does not move while scrolled up, and the pill names how many arrived', () => {
    const messages = [msg('m1', 'agent', '2024-01-01T00:00:00Z')]
    const { rerender } = render(<Timeline messages={messages} events={[]} presence={false} />)
    const el = screen.getByTestId('timeline')
    Object.defineProperty(el, 'scrollHeight', { value: 1000, configurable: true })
    Object.defineProperty(el, 'clientHeight', { value: 300, configurable: true })
    Object.defineProperty(el, 'scrollTop', { value: 100, writable: true, configurable: true })
    fireEvent.scroll(el)

    rerender(
      <Timeline
        messages={[...messages, msg('m2', 'agent', '2024-01-01T00:01:00Z')]}
        events={[]}
        presence={false}
      />,
    )
    expect(el.scrollTop).toBe(100)
    expect(screen.getByTestId('jump-pill')).toHaveTextContent('1 new message')
  })

  it('jumping scrolls to the end and clears the pill', () => {
    const messages = [msg('m1', 'agent', '2024-01-01T00:00:00Z')]
    const { rerender } = render(<Timeline messages={messages} events={[]} presence={false} />)
    const el = screen.getByTestId('timeline')
    Object.defineProperty(el, 'scrollHeight', { value: 1000, configurable: true })
    Object.defineProperty(el, 'clientHeight', { value: 300, configurable: true })
    Object.defineProperty(el, 'scrollTop', { value: 100, writable: true, configurable: true })
    fireEvent.scroll(el)
    rerender(
      <Timeline
        messages={[...messages, msg('m2', 'agent', '2024-01-01T00:01:00Z')]}
        events={[]}
        presence={false}
      />,
    )
    fireEvent.click(screen.getByTestId('jump-pill'))
    expect(el.scrollTop).toBe(1000)
    expect(screen.queryByTestId('jump-pill')).toBeNull()
  })
})

describe('an activity gap', () => {
  it('is marked rather than shown as adjacent', () => {
    render(<Timeline messages={[]} events={[]} presence={false} activityGap />)
    expect(screen.getByText(/not recorded/)).toBeInTheDocument()
  })
})
