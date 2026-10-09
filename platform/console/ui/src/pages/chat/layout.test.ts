import { afterEach, describe, expect, it, vi } from 'vitest'
import { DEFAULT_LAYOUT, LAYOUT_STORAGE_KEY, readLayout, writeLayout } from './layout'

describe('readLayout', () => {
  afterEach(() => {
    localStorage.clear()
    vi.restoreAllMocks()
  })

  it('falls back to the defaults when nothing is stored', () => {
    expect(readLayout()).toEqual(DEFAULT_LAYOUT)
  })

  it('reads back what was written', () => {
    writeLayout({ inboxWidth: 300, listWidth: 400, inboxCollapsed: true, showClosed: true, treeCollapsedByDefault: false })
    expect(readLayout()).toEqual({ inboxWidth: 300, listWidth: 400, inboxCollapsed: true, showClosed: true, treeCollapsedByDefault: false })
  })

  it('fills a partial or malformed record with the defaults rather than failing', () => {
    localStorage.setItem(LAYOUT_STORAGE_KEY, JSON.stringify({ inboxWidth: 311 }))
    expect(readLayout()).toEqual({ ...DEFAULT_LAYOUT, inboxWidth: 311 })

    localStorage.setItem(LAYOUT_STORAGE_KEY, '"not an object"')
    expect(readLayout()).toEqual(DEFAULT_LAYOUT)

    localStorage.setItem(LAYOUT_STORAGE_KEY, 'not even json')
    expect(readLayout()).toEqual(DEFAULT_LAYOUT)
  })

  it('renders at the defaults rather than throwing when storage itself refuses', () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new DOMException('blocked', 'SecurityError')
    })
    expect(readLayout()).toEqual(DEFAULT_LAYOUT)
  })

  it('does not throw when storage refuses the write', () => {
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new DOMException('quota', 'QuotaExceededError')
    })
    expect(() => writeLayout(DEFAULT_LAYOUT)).not.toThrow()
  })
})
