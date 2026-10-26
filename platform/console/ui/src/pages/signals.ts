import type { InventoryRow, SignalPhase, SignalStatusWire } from '../api/types'

// Pure helpers for the Signal kind's extra behaviour on top of the generic
// Configuration pages (Config.tsx): the source/phase filters the inventory
// adds, and reading the per-claimant routing record off the detail object's
// own `status` — which the BFF passes through verbatim, so there is nothing
// here to project, only to NAME.
//
// Kept side-effect-free and apart from the components so the filtering logic
// and the claimant parsing are each testable without mounting a page.

/** The distinct `source`/`phase` values a signals inventory offers to filter by. */
export function signalFilterOptions(rows: InventoryRow[]): { sources: string[]; phases: string[] } {
  const sources = new Set<string>()
  const phases = new Set<string>()
  for (const r of rows) {
    const source = r.columns?.source
    const phase = r.columns?.phase
    if (source) sources.add(source)
    if (phase) phases.add(phase)
  }
  return {
    sources: [...sources].sort((a, b) => a.localeCompare(b)),
    phases: [...phases].sort((a, b) => a.localeCompare(b)),
  }
}

/** `'all'` matches everything — the two selects' own "no filter" option. */
export const FILTER_ALL = 'all'

export function filterSignalRows(rows: InventoryRow[], source: string, phase: string): InventoryRow[] {
  return rows.filter((r) => {
    if (source !== FILTER_ALL && r.columns?.source !== source) return false
    if (phase !== FILTER_ALL && r.columns?.phase !== phase) return false
    return true
  })
}

/** Every value `SignalStatus.phase` can hold — used to scope the inventory's
 * phase-chip rendering to Signal rows, so a Conversation's own unrelated
 * phase vocabulary (Working/Idle/Closed) keeps its plain rendering. */
export const SIGNAL_PHASES = new Set<string>(['Pending', 'Routed', 'PartiallyRouted', 'Dropped', 'Failed'])

/** PatternFly Label status for a signal's phase — green once routed, red where nothing was, amber in between. */
export function signalPhaseStatus(phase?: string): 'success' | 'danger' | 'warning' | 'info' | undefined {
  switch (phase as SignalPhase | undefined) {
    case 'Routed':
      return 'success'
    case 'Dropped':
    case 'Failed':
      return 'danger'
    case 'PartiallyRouted':
      return 'warning'
    case 'Pending':
      return 'info'
    default:
      return undefined
  }
}

/** PatternFly Label status for one claimant's routing state — green once routed, red failed, amber skipped, blue still routing. */
export function claimantStateStatus(state?: string): 'success' | 'danger' | 'warning' | 'info' {
  switch (state) {
    case 'Routed':
      return 'success'
    case 'Failed':
      return 'danger'
    case 'Skipped':
      return 'warning'
    default:
      return 'info'
  }
}

/** A signal's phase is terminal-and-unhappy — the case the detail view must show as loudly as the phase itself. */
export function isSignalTrouble(phase?: string): boolean {
  return phase === 'Dropped' || phase === 'Failed'
}

/**
 * Reads a Signal's `status` off the generic `K8sObject.status` (`unknown`)
 * — the one cast this kind needs, since the BFF sends the CRD's status
 * verbatim rather than projecting it into a kind-specific wire type.
 *
 * Defensive on a missing/malformed value rather than throwing: an object
 * this console cannot yet interpret should render as "no routing record
 * yet", never crash the detail page.
 */
export function asSignalStatus(status: unknown): SignalStatusWire {
  if (!status || typeof status !== 'object') return {}
  const s = status as Partial<SignalStatusWire>
  return {
    phase: s.phase,
    reason: s.reason,
    claimants: Array.isArray(s.claimants) ? s.claimants : undefined,
    firstAttemptAt: s.firstAttemptAt,
  }
}
