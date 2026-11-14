import { useQueues } from '../api/hooks'
import type { DeliveryRow, StuckReason, WorkRow } from '../api/types'

// THE QUEUES PAGE'S FACTS: the view that separates "queued" from "stalled".
//
// An agent has not replied — is it queued, or is it stuck? Every row carries
// an age, and a flag names the CAUSE, because each cause has a different fix.
// Every look draws these words; none writes its own.

/** The work and delivery queues and the cooldowns, pushed by the stream after a first read. */
export function useQueuesPage() {
  return useQueues()
}

export const STUCK: Readonly<Record<StuckReason, { title: string; detail: string }>> = {
  'nothing-claiming': {
    title: 'nothing claiming',
    detail: 'ops are queued and no adapter is polling — the adapter is down or not running',
  },
  'adapter-wedged': {
    title: 'adapter wedged',
    detail: 'an adapter claimed these ops and never completed them — it is stuck mid-delivery',
  },
  'at-runtime-ceiling': {
    title: 'at runtime ceiling',
    detail: 'capacity-bound, not broken — raise MAX_ACTIVE_CONVERSATIONS or shed load',
  },
  'runtime-hung': {
    title: 'runtime hung',
    detail: 'inflight far beyond a typical run — the runtime is hung, not queued',
  },
}

/** A queue age: 42s, 7m, 1.5h — or an em dash where there is none. */
export function queueAge(seconds?: number): string {
  if (!seconds) return '—'
  if (seconds < 60) return `${Math.round(seconds)}s`
  if (seconds < 3600) return `${Math.round(seconds / 60)}m`
  return `${(seconds / 3600).toFixed(1)}h`
}

/** A work row's age: how long its run has been in flight, else how long it has waited. */
export function workAge(row: WorkRow): string {
  return row.inflight ? queueAge(row.inflightAgeSeconds) : queueAge(row.ageSeconds)
}

/**
 * WHICH op is oldest, not just how many — that identity is exactly what a
 * metric label may never carry. A claimed op outranks a queued one.
 */
export function oldestOp(row: DeliveryRow): { id: string; state: 'claimed' | 'queued'; age: string } | undefined {
  if (row.oldestClaimedOpId) return { id: row.oldestClaimedOpId, state: 'claimed', age: queueAge(row.oldestClaimedAgeSeconds) }
  if (row.oldestQueuedOpId) return { id: row.oldestQueuedOpId, state: 'queued', age: queueAge(row.oldestQueuedAgeSeconds) }
  return undefined
}
