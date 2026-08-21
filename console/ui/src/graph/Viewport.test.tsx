import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, act } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Viewport } from './Viewport'

// The fit contract. The defect this pins: a graph rendered inside a container
// that is in the DOM but not displayed — an inactive tab — measured a 0x0 host,
// fitted to it, and landed at minimum zoom in the top-left corner for good.

let observers: Array<() => void> = []

class FakeResizeObserver {
  constructor(private cb: () => void) {
    observers.push(() => this.cb())
  }
  observe() {}
  disconnect() {}
}

/** Make every host measure this size, as jsdom reports zeros for everything. */
function measuring(width: number, height: number) {
  return vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockReturnValue({
    width, height, top: 0, left: 0, right: width, bottom: height, x: 0, y: 0,
    toJSON: () => ({}),
  } as DOMRect)
}

function draw() {
  return render(
    <Viewport contentWidth={400} contentHeight={200} ariaLabel="test graph">
      <rect width={400} height={200} />
    </Viewport>,
  )
}

const canvas = () => screen.getByTestId('graph-canvas').getAttribute('transform')
const zoom = () => screen.getByLabelText('zoom level').textContent

beforeEach(() => {
  observers = []
  vi.stubGlobal('ResizeObserver', FakeResizeObserver)
})

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe('Viewport fitting', () => {
  it('does not fit to a host that has no size', () => {
    measuring(0, 0)
    draw()
    // Untouched, rather than MIN_SCALE at a negative offset.
    expect(canvas()).toBe('translate(0,0) scale(1)')
    expect(zoom()).toBe('100%')
  })

  it('fits when the host becomes measurable, as a hidden tab does when shown', () => {
    const rect = measuring(0, 0)
    draw()
    expect(canvas()).toBe('translate(0,0) scale(1)')

    // the tab is selected: the host now has a size, and the observer fires
    rect.mockReturnValue({
      width: 800, height: 400, top: 0, left: 0, right: 800, bottom: 400, x: 0, y: 0,
      toJSON: () => ({}),
    } as DOMRect)
    act(() => observers.forEach((fire) => fire()))

    // scaled to the area and centred in it
    expect(canvas()).toBe('translate(200,100) scale(1)')
  })

  it('re-fits on resize while the operator has not moved the view', () => {
    const rect = measuring(800, 400)
    draw()
    expect(canvas()).toBe('translate(200,100) scale(1)')

    rect.mockReturnValue({
      width: 400, height: 400, top: 0, left: 0, right: 400, bottom: 400, x: 0, y: 0,
      toJSON: () => ({}),
    } as DOMRect)
    act(() => observers.forEach((fire) => fire()))
    expect(canvas()).toBe('translate(12,106) scale(0.94)')
  })

  it('re-fits when the graph is rebuilt, which is how a new scope gets centred', async () => {
    // Graph passes the visible node ids as fitKey, and those ids come from the
    // SCOPED set — so narrowing the graph re-fits it rather than leaving the
    // subgraph at the whole graph's transform.
    measuring(800, 400)
    const { rerender } = render(
      <Viewport contentWidth={400} contentHeight={200} fitKey="a|b|c">
        <rect />
      </Viewport>,
    )
    await userEvent.click(screen.getByLabelText('zoom in'))
    const chosen = canvas()

    rerender(
      <Viewport contentWidth={200} contentHeight={100} fitKey="a|b">
        <rect />
      </Viewport>,
    )
    expect(canvas()).not.toBe(chosen)
    expect(canvas()).toBe('translate(300,150) scale(1)')
  })

  it('keeps a view the operator chose when the area is resized', async () => {
    const rect = measuring(800, 400)
    draw()
    await userEvent.click(screen.getByLabelText('zoom in'))
    const chosen = canvas()

    rect.mockReturnValue({
      width: 400, height: 400, top: 0, left: 0, right: 400, bottom: 400, x: 0, y: 0,
      toJSON: () => ({}),
    } as DOMRect)
    act(() => observers.forEach((fire) => fire()))
    expect(canvas()).toBe(chosen)

    // ...and the fit control gives fitting back on demand
    await userEvent.click(screen.getByLabelText('fit to view'))
    expect(canvas()).toBe('translate(12,106) scale(0.94)')
  })
})
