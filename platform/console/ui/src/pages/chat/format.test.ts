import { describe, expect, it } from 'vitest'
import { formatBytes, formatDuration, relativeAge } from './format'

describe('relativeAge', () => {
  it.each([
    [4, '4s'],
    [120, '2m'],
    [5400, '1.5h'],
    [172800, '2.0d'],
  ])('%s seconds reads %s', (seconds, want) => {
    expect(relativeAge(seconds)).toBe(want)
  })
})

describe('formatDuration', () => {
  it('shows a dash when the duration is absent', () => {
    expect(formatDuration()).toBe('—')
  })

  it('stays in milliseconds below a second', () => {
    expect(formatDuration(410)).toBe('410ms')
  })

  it('switches to seconds from a second up', () => {
    expect(formatDuration(1200)).toBe('1.2s')
  })
})

describe('formatBytes', () => {
  it('shows a dash when the size is absent', () => {
    expect(formatBytes()).toBe('—')
  })

  it.each([
    [512, '512 B'],
    [1536, '1.5 KB'],
    [3 * 1024 * 1024, '3.0 MB'],
    [2 * 1024 * 1024 * 1024, '2.0 GB'],
    [5000 * 1024 * 1024 * 1024, '5000.0 GB'],
  ])('%s bytes reads %s', (bytes, want) => {
    expect(formatBytes(bytes)).toBe(want)
  })
})
