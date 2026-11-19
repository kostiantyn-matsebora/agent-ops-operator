import { describe, expect, it } from 'vitest'
import { formatBytes, formatDuration, freshTokens, runTokens, serverOf, toolRows, turnRows } from './runStats'

describe('freshTokens', () => {
  it('is what was paid in full: total in less cache reads and cache writes', () => {
    expect(freshTokens({ tokensIn: 1000, cacheReadTokens: 700, cacheCreationTokens: 100 })).toBe(200)
  })
  it('is undefined while the total is, never zero', () => {
    expect(freshTokens({ tokensOut: 5 })).toBeUndefined()
  })
})

describe('turnRows', () => {
  it('has no rows without reported turns, and dashes for what a turn did not report', () => {
    expect(turnRows({})).toEqual([])
    const [row] = turnRows({ turns: [{ model: 'm', tokensIn: 10, tokensOut: 2, stopReason: 'end_turn' }] })
    expect(row).toMatchObject({ model: 'm', in: '10', cached: '—', written: '—', fresh: '10', out: '2', stopReason: 'end_turn' })
  })
})

describe('toolRows', () => {
  it('names the server a call went through, and calls an unnamed one built-in', () => {
    expect(serverOf({ tool: 'mcp__aops__invoke' })).toBe('aops')
    expect(serverOf({ tool: 'mcp__aops__invoke', server: 'other' })).toBe('other')
    expect(serverOf({ tool: 'Bash' })).toBe('built-in')
  })
  it('formats duration and size, and drops the server prefix from the tool', () => {
    const [row] = toolRows({ toolCalls: [{ tool: 'mcp__aops__list_agents', durationMs: 1500, resultBytes: 2048 }] })
    expect(row).toMatchObject({ tool: 'list_agents', server: 'aops', duration: '1.5s', size: '2.0 KB' })
    expect(formatDuration(150)).toBe('150ms')
    expect(formatBytes(12)).toBe('12 B')
  })
})

describe('runTokens', () => {
  it('totals in and out across turns, and is absent when nothing was reported', () => {
    expect(runTokens({})).toBeUndefined()
    expect(runTokens({ turns: [{ tokensIn: 10, tokensOut: 1 }, { tokensIn: 5, tokensOut: 2 }] })).toEqual({ in: 15, out: 3 })
  })
})
