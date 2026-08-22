import { describe, expect, it } from 'vitest'

// A REFETCH IS AN EXCEPTION WITH A STATED REASON.
//
// Four reasons survive — first load, a resync, an explicit action by the
// reader, and a value that decays with TIME rather than with change. There is
// no fifth, and the one that needs watching is the fourth: a timer is the
// easiest way to quietly reintroduce polling, and a timer added to observe
// CHANGE is exactly what this change removed.
//
// Written as a scan rather than a convention because a convention decays one
// call site at a time — which is how twelve queries came to differ from the one
// that had it right.

const sources = import.meta.glob('../**/*.{ts,tsx}', {
  query: '?raw', import: 'default', eager: true,
}) as Record<string, string>

function appSources(): [string, string][] {
  return Object.entries(sources).filter(([path]) => !/\.test\.(ts|tsx)$/.test(path))
}

describe('timed refreshes', () => {
  it('exist only where a value decays with time, and say so where they are set', () => {
    const unexplained: string[] = []
    for (const [path, raw] of appSources()) {
      const lines = raw.split('\n')
      lines.forEach((line, i) => {
        if (!line.includes('refetchInterval')) return
        // The reason is stated in the comment immediately above it, where
        // somebody changing the number will read it.
        const preamble = lines.slice(Math.max(0, i - 6), i).join(' ').toLowerCase()
        if (!/decay|rate|age|time passed/.test(preamble)) {
          unexplained.push(`${path}:${i + 1}`)
        }
      })
    }
    expect(unexplained).toEqual([])
  })

  it('are the only polling in the app', () => {
    const timers: string[] = []
    for (const [path, raw] of appSources()) {
      // The stream owns its own reconnect backoff and its coalescing window —
      // both are one-shot setTimeouts, not polling.
      if (path.endsWith('api/stream.ts')) continue
      const code = raw.replace(/\/\*[\s\S]*?\*\//g, '').replace(/\/\/.*$/gm, '')
      if (/setInterval\s*\(/.test(code)) timers.push(path)
    }
    expect(timers).toEqual([])
  })
})
