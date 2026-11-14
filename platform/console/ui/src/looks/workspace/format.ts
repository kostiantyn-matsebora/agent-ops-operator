// How the workspace writes times and durations. Presentation only: which
// moment a fact names is the model's, how it reads is this look's.

/** A relative age, the list's short form: now, 4m, 3h, 2d. */
export function ago(iso: string | undefined, now: number = Date.now()): string {
  const at = Date.parse(iso ?? '')
  if (!at) return ''
  const minutes = Math.round((now - at) / 60_000)
  if (minutes < 1) return 'now'
  if (minutes < 60) return `${minutes}m`
  if (minutes < 1440) return `${Math.round(minutes / 60)}h`
  return `${Math.round(minutes / 1440)}d`
}

/** A message's clock time. */
export function clock(iso: string | undefined): string {
  const at = Date.parse(iso ?? '')
  if (!at) return ''
  return new Date(at).toLocaleTimeString([], { hour: 'numeric', minute: '2-digit', second: '2-digit' })
}

/** A day, as a divider names it: Sat, Nov 14. */
export function day(iso: string | undefined): string {
  const at = Date.parse(iso ?? '')
  if (!at) return ''
  return new Date(at).toLocaleDateString(undefined, { weekday: 'short', month: 'short', day: 'numeric' })
}

/** A conversation's start, as the line under the tabs names it: Sat, Nov 14 · 10:58 AM. */
export function started(iso: string | undefined): string {
  const at = Date.parse(iso ?? '')
  if (!at) return ''
  const time = new Date(at).toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit' })
  return `${day(iso)} · ${time}`
}

/** The calendar day a moment falls on, in the viewer's zone — what a divider compares. */
export function dayKey(iso: string | undefined): string {
  const at = Date.parse(iso ?? '')
  return at ? new Date(at).toDateString() : ''
}

/** A duration: 9s, 1m 05s. */
export function duration(ms: number | undefined): string {
  if (ms === undefined || ms < 0) return ''
  const s = Math.round(ms / 1000)
  if (s < 60) return `${s}s`
  return `${Math.floor(s / 60)}m ${String(s % 60).padStart(2, '0')}s`
}

/** A working run's elapsed time on its card: 0:42. */
export function elapsed(ms: number): string {
  const s = Math.max(0, Math.floor(ms / 1000))
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`
}

/** A tool call's duration: 410ms, 1.5s. */
export function toolDuration(ms: number | undefined): string {
  if (ms === undefined) return ''
  return ms >= 1000 ? `${(ms / 1000).toFixed(1)}s` : `${ms}ms`
}
