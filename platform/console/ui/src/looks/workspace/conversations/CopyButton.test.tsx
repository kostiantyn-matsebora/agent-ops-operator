import { afterEach, describe, expect, it, vi } from 'vitest'
import { act, render, screen } from '@testing-library/react'
import { createRef } from 'react'
import { COPIED_MS, CopyButton, MANUAL_MS, writeClipboard } from './CopyButton'

afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

function mount(write: (t: string) => Promise<boolean>) {
  const target = createRef<HTMLDivElement>()
  render(
    <div>
      <div ref={target}>the **message** as drawn</div>
      <CopyButton text="the **message** as written" target={target} write={write} />
    </div>,
  )
  return target
}

describe('the copy control', () => {
  it('copies the markdown it was given and confirms in place, then returns', async () => {
    vi.useFakeTimers()
    const write = vi.fn(async () => true)
    mount(write)
    await act(async () => screen.getByRole('button', { name: 'Copy message' }).click())
    expect(write).toHaveBeenCalledWith('the **message** as written')
    const button = screen.getByRole('button', { name: 'Copied' })
    expect(button).toHaveAttribute('data-state', 'done')
    expect(button).toHaveTextContent('Copied')
    // No toast: the confirmation is the control itself.
    expect(document.querySelectorAll('[data-testid="toast"]')).toHaveLength(0)
    act(() => vi.advanceTimersByTime(COPIED_MS))
    expect(screen.getByRole('button', { name: 'Copy message' })).toHaveAttribute('data-state', 'idle')
  })

  it('selects the text and says to copy it by hand where the clipboard is refused', async () => {
    vi.useFakeTimers()
    const target = mount(async () => false)
    await act(async () => screen.getByRole('button', { name: 'Copy message' }).click())
    expect(screen.getByRole('button', { name: 'Copy by hand' })).toHaveAttribute('data-state', 'manual')
    expect(window.getSelection()?.toString()).toBe(target.current?.textContent)
    act(() => vi.advanceTimersByTime(MANUAL_MS))
    expect(screen.getByRole('button', { name: 'Copy message' })).toBeInTheDocument()
  })

  it('reports a refusal from the browser, and a missing clipboard, as false', async () => {
    vi.stubGlobal('navigator', { clipboard: { writeText: vi.fn(async () => Promise.reject(new Error('denied'))) } })
    expect(await writeClipboard('x')).toBe(false)
    vi.stubGlobal('navigator', {})
    expect(await writeClipboard('x')).toBe(false)
    const writeText = vi.fn(async () => undefined)
    vi.stubGlobal('navigator', { clipboard: { writeText } })
    expect(await writeClipboard('x')).toBe(true)
    expect(writeText).toHaveBeenCalledWith('x')
  })
})
