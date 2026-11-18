import type { ActivityEvent } from '../api/types'

// The two small formatters every surface of the graph shares, kept apart from
// the panels so a look drawing its own chrome needs none of the classic one.

export const clock = (ts: string | number) =>
  new Date(ts).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })

export function hopPath(ev: ActivityEvent): string {
  return `${ev.from?.name ?? '∅'} → ${ev.to?.name ?? '∅'}`
}
