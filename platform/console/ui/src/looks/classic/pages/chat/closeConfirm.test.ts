import { afterEach, describe, expect, it, vi } from 'vitest'
import { isCloseCommand, skipCloseConfirm, writeSkipCloseConfirm } from './closeConfirm'

describe('skipCloseConfirm / writeSkipCloseConfirm', () => {
  afterEach(() => {
    localStorage.clear()
    vi.restoreAllMocks()
  })

  it('defaults to asking — nothing stored means the dialog shows', () => {
    expect(skipCloseConfirm()).toBe(false)
  })

  it('remembers an opt-out, and forgetting it brings the dialog back', () => {
    writeSkipCloseConfirm(true)
    expect(skipCloseConfirm()).toBe(true)
    writeSkipCloseConfirm(false)
    expect(skipCloseConfirm()).toBe(false)
  })

  it('reads as "still asking" rather than throwing when storage refuses', () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new DOMException('blocked', 'SecurityError')
    })
    expect(skipCloseConfirm()).toBe(false)
  })

  it('does not throw when storage refuses the write', () => {
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new DOMException('quota', 'QuotaExceededError')
    })
    expect(() => writeSkipCloseConfirm(true)).not.toThrow()
  })
})

describe('isCloseCommand', () => {
  it('matches the bare command and a reasoned one, trimmed', () => {
    expect(isCloseCommand('/close')).toBe(true)
    expect(isCloseCommand('  /close  ')).toBe(true)
    expect(isCloseCommand('/close no longer needed')).toBe(true)
  })

  it('never matches a prefix that merely starts with the word', () => {
    expect(isCloseCommand('/closeup')).toBe(false)
    expect(isCloseCommand('/exit')).toBe(false)
    expect(isCloseCommand('please /close this')).toBe(false)
  })
})
