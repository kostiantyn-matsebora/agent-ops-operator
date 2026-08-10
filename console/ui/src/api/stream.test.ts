import { afterEach, describe, expect, it, vi } from 'vitest'
import { connectStream, useStream } from './stream'

// The masthead chip reads `connected` from this store. It used to be able to
// stick on "stream disconnected" until the page was reloaded, because
// EventSource retries a dropped connection by itself but gives up PERMANENTLY
// on an HTTP error (a 502 while the console rolls, a 401 when the session
// expires): readyState goes CLOSED and nothing reopened it. These tests are
// about who owns reconnection.

class FakeEventSource {
  static CONNECTING = 0
  static OPEN = 1
  static CLOSED = 2
  static instances: FakeEventSource[] = []

  readyState = FakeEventSource.CONNECTING
  onopen: (() => void) | null = null
  onerror: (() => void) | null = null
  closed = false

  constructor(public url: string) {
    FakeEventSource.instances.push(this)
  }
  addEventListener() {}
  close() {
    this.closed = true
    this.readyState = FakeEventSource.CLOSED
  }
  /** The browser gave up: an HTTP error, not a transport blip. */
  failPermanently() {
    this.readyState = FakeEventSource.CLOSED
    this.onerror?.()
  }
  /** A transport blip the browser will retry on its own. */
  blip() {
    this.readyState = FakeEventSource.CONNECTING
    this.onerror?.()
  }
  open() {
    this.readyState = FakeEventSource.OPEN
    this.onopen?.()
  }
}

vi.stubGlobal('EventSource', FakeEventSource)

function reset() {
  FakeEventSource.instances = []
  useStream.setState({ connected: false })
}

afterEach(() => {
  vi.useRealTimers()
})

describe('connectStream', () => {
  it('reopens after the browser gives up, so the chip recovers without a reload', () => {
    vi.useFakeTimers()
    reset()
    const stop = connectStream(() => {})

    const first = FakeEventSource.instances[0]
    first.open()
    expect(useStream.getState().connected).toBe(true)

    first.failPermanently()
    expect(useStream.getState().connected).toBe(false)
    expect(first.closed).toBe(true)

    // Nothing reopened it before — this is the bug.
    vi.advanceTimersByTime(1_000)
    expect(FakeEventSource.instances).toHaveLength(2)

    FakeEventSource.instances[1].open()
    expect(useStream.getState().connected).toBe(true)

    stop()
  })

  it('leaves a transport blip to the browser', () => {
    vi.useFakeTimers()
    reset()
    const stop = connectStream(() => {})

    const es = FakeEventSource.instances[0]
    es.open()
    es.blip() // still CONNECTING: EventSource is retrying by itself

    expect(useStream.getState().connected).toBe(false)
    vi.advanceTimersByTime(30_000)
    // Opening a second connection here would double every browser's stream.
    expect(FakeEventSource.instances).toHaveLength(1)
    expect(es.closed).toBe(false)

    stop()
  })

  it('backs off instead of hammering a console that is down', () => {
    vi.useFakeTimers()
    reset()
    const stop = connectStream(() => {})

    FakeEventSource.instances[0].failPermanently()
    vi.advanceTimersByTime(1_000)
    expect(FakeEventSource.instances).toHaveLength(2)

    FakeEventSource.instances[1].failPermanently()
    vi.advanceTimersByTime(1_000) // too early: the wait has doubled
    expect(FakeEventSource.instances).toHaveLength(2)
    vi.advanceTimersByTime(1_000)
    expect(FakeEventSource.instances).toHaveLength(3)

    stop()
  })

  it('stops retrying once the caller unmounts', () => {
    vi.useFakeTimers()
    reset()
    const stop = connectStream(() => {})

    FakeEventSource.instances[0].failPermanently()
    stop()

    vi.advanceTimersByTime(60_000)
    expect(FakeEventSource.instances).toHaveLength(1)
    expect(useStream.getState().connected).toBe(false)
  })
})
