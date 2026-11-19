import { healthVariant, useCharts, useOverview } from '../api/hooks'
import type { Health, ProblemSource } from '../api/types'

// THE OVERVIEW PAGE'S FACTS: the installation, its capacity, its telemetry
// paths and what is reporting a problem. Every look draws these and none
// recomputes them (console-looks: "A fact is computed once").

/** The installation as the console reads it. Refetched on a timer: slots and ages decay with time. */
export function useOverviewPage() {
  return useOverview()
}

/** Whether a metrics backend answers — the Telemetry card's second path. */
export function useMetricsBackend(): boolean {
  const charts = useCharts()
  return charts.data?.available ?? false
}

/** The runtime slots as a share, and whether they are all taken. */
export function capacityShare(slots: { inUse: number; max: number }): { percent: number; full: boolean } {
  return { percent: slots.max ? (slots.inUse / slots.max) * 100 : 0, full: slots.inUse >= slots.max }
}

/** An age with one decimal past the hour: 42s, 7m, 2.0h, 1.5d. */
export function relativeAge(seconds: number): string {
  if (seconds < 60) return `${Math.round(seconds)}s`
  if (seconds < 3600) return `${Math.round(seconds / 60)}m`
  if (seconds < 86400) return `${(seconds / 3600).toFixed(1)}h`
  return `${(seconds / 86400).toFixed(1)}d`
}

/**
 * Problem.since is a condition's lastTransitionTime — absent on a pod- or
 * console-derived row, which carries no condition to date. An em dash names
 * that absence rather than a blank cell reading as a missed render.
 */
export function sinceAge(since?: string, now: number = Date.now()): string {
  if (!since) return '—'
  const ms = now - new Date(since).getTime()
  if (!Number.isFinite(ms) || ms < 0) return '—'
  return `${relativeAge(ms / 1000)} ago`
}

/**
 * Where a problem came from. A condition a reconciler wrote and a
 * cross-reference the console derived carry different authority — labelling
 * them apart is what stops the console appearing to speak for the cluster.
 */
export const PROBLEM_SOURCE: Readonly<Record<ProblemSource, { text: string; tone: 'blue' | 'grey' | 'purple' }>> = {
  reported: { text: 'reported', tone: 'blue' },
  pod: { text: 'pod', tone: 'purple' },
  derived: { text: 'console check', tone: 'grey' },
}

/**
 * A health value's status tone. `none` is `custom`: a kind that asserts no
 * health has nothing to be green or red about.
 */
export function healthTone(h: Health): 'success' | 'danger' | 'warning' | 'custom' {
  return healthVariant(h)
}
