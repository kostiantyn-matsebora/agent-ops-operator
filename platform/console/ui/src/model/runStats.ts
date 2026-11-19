import type { Run, RunToolCall, RunTurn } from '../api/types'

// A RUN'S STATISTICS — the token accounting per model call and the per-tool
// timings the runtime reports. Derived here so every look shows the same
// numbers; a look draws them and never repeats the arithmetic.

/** "410ms" / "1.2s" — a tool call's own duration column. */
export function formatDuration(ms?: number): string {
  if (ms === undefined) return '—'
  return ms >= 1000 ? `${(ms / 1000).toFixed(1)}s` : `${ms}ms`
}

/** "512 B" / "1.2 KB" / "3.4 MB" — a tool call's own result-size column. */
export function formatBytes(bytes?: number): string {
  if (bytes === undefined) return '—'
  if (bytes < 1024) return `${bytes} B`
  const units = ['KB', 'MB', 'GB']
  let value = bytes / 1024
  let unit = 0
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024
    unit += 1
  }
  return `${value.toFixed(1)} ${units[unit]}`
}

/**
 * The input a call actually paid full price for: total in, minus what the
 * provider served from its prompt cache, minus what the call wrote INTO that
 * cache. Derived rather than reported. Undefined while the total is, so a turn
 * with no usage reads as dashes, never zeros.
 */
export function freshTokens(t: RunTurn): number | undefined {
  if (t.tokensIn === undefined) return undefined
  return t.tokensIn - (t.cacheReadTokens ?? 0) - (t.cacheCreationTokens ?? 0)
}

const dash = (n: number | undefined) => (n === undefined ? '—' : String(n))

export interface TurnRow {
  key: string
  model: string
  in: string
  cached: string
  written: string
  fresh: string
  out: string
  stopReason: string
}

/** One row per model call, or none when the runtime did not report turns. */
export function turnRows(run: Pick<Run, 'turns'>): TurnRow[] {
  return (run.turns ?? []).map((t, i) => ({
    key: `${i}`,
    model: t.model || '—',
    in: dash(t.tokensIn),
    cached: dash(t.cacheReadTokens),
    written: dash(t.cacheCreationTokens),
    fresh: dash(freshTokens(t)),
    out: dash(t.tokensOut),
    stopReason: t.stopReason || '—',
  }))
}

export interface ToolRow {
  key: string
  tool: string
  server: string
  duration: string
  size: string
}

/** The server a tool call went through: the one reported, else read off an `mcp__<server>__<tool>` name, else built-in. */
export function serverOf(c: RunToolCall): string {
  if (c.server) return c.server
  return /^mcp__([^_]+)__/.exec(c.tool ?? '')?.[1] ?? 'built-in'
}

/** One row per tool call, or none when the runtime did not report them. */
export function toolRows(run: Pick<Run, 'toolCalls'>): ToolRow[] {
  return (run.toolCalls ?? []).map((c, i) => ({
    key: `${i}`,
    tool: (c.tool ?? '—').replace(/^mcp__[^_]+__/, ''),
    server: serverOf(c),
    duration: formatDuration(c.durationMs),
    size: formatBytes(c.resultBytes),
  }))
}

/** The run's total tokens across its turns, in and out; undefined when no turn reported any. */
export function runTokens(run: Pick<Run, 'turns'>): { in: number; out: number } | undefined {
  const turns = (run.turns ?? []).filter((t) => t.tokensIn !== undefined || t.tokensOut !== undefined)
  if (turns.length === 0) return undefined
  return {
    in: turns.reduce((n, t) => n + (t.tokensIn ?? 0), 0),
    out: turns.reduce((n, t) => n + (t.tokensOut ?? 0), 0),
  }
}
