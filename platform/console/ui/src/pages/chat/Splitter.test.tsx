import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { Splitter } from './Splitter'

// jsdom (25.x, as this tree pins it) implements no `PointerEvent`
// constructor at all, so `fireEvent.pointerDown({clientX, button})` loses
// both properties — `createEvent` falls back to the plain `Event`
// constructor, which reads neither. A plain `Event` with the properties
// attached by hand afterwards carries them through `dispatchEvent`
// unchanged, which is all the component ever reads off it.
function pointerEvent(type: string, props: Record<string, unknown>): Event {
  const ev = new Event(type, { bubbles: true, cancelable: true })
  Object.assign(ev, props)
  return ev
}

function renderSplitter(onChange = vi.fn(), props: Partial<React.ComponentProps<typeof Splitter>> = {}) {
  render(
    <Splitter width={240} min={200} max={400} defaultWidth={240} onChange={onChange} ariaLabel="resize" {...props} />,
  )
  return { onChange, handle: screen.getByTestId('splitter') }
}

describe('Splitter', () => {
  it('reports a clamped width while dragging, never past the minimum or the maximum', () => {
    const { onChange, handle } = renderSplitter()
    fireEvent(handle, pointerEvent('pointerdown', { clientX: 500, button: 0 }))
    fireEvent(window, pointerEvent('pointermove', { clientX: 500 - 100 })) // 240 - 100 = 140, below min 200
    expect(onChange).toHaveBeenLastCalledWith(200)
    fireEvent(window, pointerEvent('pointermove', { clientX: 500 + 300 })) // 240 + 300 = 540, above max 400
    expect(onChange).toHaveBeenLastCalledWith(400)
    fireEvent(window, pointerEvent('pointermove', { clientX: 500 + 20 })) // 260, inside bounds
    expect(onChange).toHaveBeenLastCalledWith(260)
    fireEvent(window, pointerEvent('pointerup', {}))
  })

  it('stops reporting once the pointer is released', () => {
    const { onChange, handle } = renderSplitter()
    fireEvent(handle, pointerEvent('pointerdown', { clientX: 100, button: 0 }))
    fireEvent(window, pointerEvent('pointerup', {}))
    onChange.mockClear()
    fireEvent(window, pointerEvent('pointermove', { clientX: 300 }))
    expect(onChange).not.toHaveBeenCalled()
  })

  it('resets to the default width on double-click', () => {
    const { onChange, handle } = renderSplitter(vi.fn(), { width: 320 })
    fireEvent.doubleClick(handle)
    expect(onChange).toHaveBeenCalledWith(240)
  })

  it('steps by keyboard and resets with Enter', () => {
    const { onChange, handle } = renderSplitter(vi.fn(), { width: 240 })
    handle.focus()
    fireEvent.keyDown(handle, { key: 'ArrowRight' })
    expect(onChange).toHaveBeenLastCalledWith(256)
    fireEvent.keyDown(handle, { key: 'ArrowLeft' })
    expect(onChange).toHaveBeenLastCalledWith(224)
    fireEvent.keyDown(handle, { key: 'Enter' })
    expect(onChange).toHaveBeenLastCalledWith(240)
  })

  it('only drags on the primary button', () => {
    const { onChange, handle } = renderSplitter()
    fireEvent(handle, pointerEvent('pointerdown', { clientX: 100, button: 2 }))
    fireEvent(window, pointerEvent('pointermove', { clientX: 300 }))
    expect(onChange).not.toHaveBeenCalled()
  })
})
