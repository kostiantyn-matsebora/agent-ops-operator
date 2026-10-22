import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { GeneralFeedPane } from './GeneralFeedPane'
import type { ChannelFeed } from '../../api/types'

// B1 (UI half): a thread-less send has no conversation behind it, so it used
// to be invisible the moment it had nowhere to attach to. This pane is the
// surface that makes it visible — reusing `Timeline`, the SAME component a
// conversation's own transcript renders with, never a bespoke renderer.

let FEED_STATE: { data: ChannelFeed | undefined; isLoading: boolean; error: unknown } = {
  data: undefined, isLoading: true, error: null,
}

vi.mock('../../api/hooks', () => ({
  useChannelFeed: () => FEED_STATE,
}))

describe('GeneralFeedPane', () => {
  it('shows a spinner while loading', () => {
    FEED_STATE = { data: undefined, isLoading: true, error: null }
    render(<GeneralFeedPane />)
    expect(screen.getByLabelText('loading')).toBeInTheDocument()
  })

  it('shows an error state when the feed could not be loaded', () => {
    FEED_STATE = { data: undefined, isLoading: false, error: new Error('network down') }
    render(<GeneralFeedPane />)
    expect(screen.getByText('Could not load the channel feed')).toBeInTheDocument()
  })

  it('renders a thread-less send, visible rather than dropped, with its choices', () => {
    FEED_STATE = {
      isLoading: false,
      error: null,
      data: {
        channel: 'console',
        messages: [{
          id: 'send:agents-listing:1', thread: 'channel:console', kind: 'answer',
          text: 'Available: ha-ops, ha-control', at: '2024-01-01T00:00:00Z',
          choices: [{ command: '/ha-ops', label: 'ha-ops' }],
        }],
      },
    }
    render(<GeneralFeedPane />)
    expect(screen.getByText('Available: ha-ops, ha-control')).toBeInTheDocument()
    expect(screen.getByText('console')).toBeInTheDocument()
    // Read-only: there is no conversation to insert a choice into, so even
    // the latest (here, only) message's choice is never an interactive
    // control — it still renders, as a plain list (B2's degrade rule).
    expect(screen.getByText('ha-ops').closest('button')).toBeNull()
  })

  it('degrades to the shared empty state with no thread-less sends yet', () => {
    FEED_STATE = { isLoading: false, error: null, data: { channel: 'console', messages: [] } }
    render(<GeneralFeedPane />)
    expect(screen.getByText('No messages on the console thread yet')).toBeInTheDocument()
  })
})
