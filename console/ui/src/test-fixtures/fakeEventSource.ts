/**
 * An EventSource the tests drive by hand.
 *
 * Shared so the stream's own tests and the per-page live tests deliver events
 * the SAME way a browser does — through the listeners connectStream registers.
 * A page test that reached past those would be pinning the appliers twice and
 * the wiring never.
 */
export class FakeEventSource {
  static CONNECTING = 0
  static OPEN = 1
  static CLOSED = 2
  static instances: FakeEventSource[] = []

  readyState = FakeEventSource.CONNECTING
  onopen: (() => void) | null = null
  onerror: (() => void) | null = null
  closed = false
  listeners: Record<string, (ev: unknown) => void> = {}

  constructor(public url: string) {
    FakeEventSource.instances.push(this)
  }
  addEventListener(name: string, fn: (ev: unknown) => void) {
    this.listeners[name] = fn
  }
  /** Deliver one server event, exactly as EventSource would. */
  emit(name: string, payload: unknown) {
    this.listeners[name]?.({ data: JSON.stringify(payload) })
  }
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
  /** The most recently opened connection. */
  static latest(): FakeEventSource {
    return FakeEventSource.instances[FakeEventSource.instances.length - 1]
  }
  static reset() {
    FakeEventSource.instances = []
  }
}
