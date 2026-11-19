import { useDetail, useInventory, useKinds, usePipelineIcon } from '../api/hooks'
import type { Detail, InventoryRow } from '../api/types'
import { FILTER_ALL, filterSignalRows, signalFilterOptions } from './signals'

// THE CONFIGURATION BROWSER'S FACTS: the kind index, each kind's table and its
// columns, and an object's page. Read-only, and that is a position rather than
// a gap: Pipelines are the wiring, the wiring is GitOps-managed, and a console
// that edits them competes with helmfile.
//
// Which columns a kind shows is a FACT about the kind, so it lives here and
// every look draws the same table.

/** Every kind the console watches, with its count and whether the cache has synced. */
export function useConfigKinds() {
  return useKinds()
}

/** One kind's objects. */
export function useConfigInventory(kind: string) {
  return useInventory(kind)
}

/** One object: its health, conditions, findings, references and YAML. */
export function useConfigDetail(kind: string, name: string) {
  return useDetail(kind, name)
}

/** A Pipeline's declared icon by name, from the shared vocabulary. */
export function usePipelineIconOf(): (name?: string) => string | undefined {
  return usePipelineIcon()
}

/**
 * Kind-specific columns. A Pipeline row is not a Channel row, and a generic
 * table of name+age would make every kind equally uninformative. Every table
 * also carries Name first and Status, Labels and Age last (TABLE_TAIL).
 */
export function columnsFor(kind: string): string[] {
  switch (kind) {
    case 'pipelines':
      // A Pipeline using capabilityRef carries no profile/toolsets/mcpConfigs
      // columns — see kindColumns in configapi.go — so those cells render "—"
      // for it, and `capability` fills in instead.
      return ['profile', 'capability', 'sources', 'channels', 'toolsets', 'toolsMode', 'mcpConfigs']
    case 'coordinators':
      return ['sources', 'escalatesTo', 'capability', 'profile', 'agents']
    case 'agentcapabilities':
      return ['profile', 'runtime', 'toolsets', 'mcpConfigs']
    case 'channels':
      return ['adapter', 'served']
    case 'signalsources':
      return ['adapter', 'served', 'wired']
    case 'agentprofiles':
      return ['runtime', 'repository']
    case 'channeladapters':
    case 'signaladapters':
      return ['image', 'servedBy']
    case 'agentruntimes':
      return ['image']
    case 'mcptoolsets':
      return ['tools']
    case 'mcpconfigs':
      return ['servers']
    case 'conversations':
      return ['phase', 'profile', 'coordinator', 'causedBy']
    case 'signals':
      // console-signals: "source, initiationId, phase and age"
      // — age is the generic column every kind already gets.
      return ['source', 'initiationId', 'phase']
    default:
      return []
  }
}

/** The columns every kind table ends with, after its own. */
export const TABLE_TAIL = ['Status', 'Labels', 'Age'] as const

/** Columns whose values are lists worth rendering as chips rather than prose. */
export const CHIP_COLUMNS: ReadonlySet<string> = new Set([
  'sources', 'channels', 'toolsets', 'mcpConfigs', 'tools', 'servers', 'escalatesTo', 'agents',
])

/** Columns whose value is a True/False verdict. */
export const STATUS_COLUMNS: ReadonlySet<string> = new Set(['served', 'wired'])

/** A chip column's value, as its parts. */
export function chipParts(value: string): string[] {
  return value.split(',').map((v) => v.trim()).filter(Boolean)
}

/** A long cell value, cut to fit, with the whole kept for a tooltip. */
export function truncateCell(value: string, max = 34): string {
  return value.length > max ? `${value.slice(0, max - 1)}…` : value
}

/** An object age: 42s, 7m, 3h, 2d — or an em dash for no or an unreadable timestamp. */
export function objectAge(iso?: string, now: number = Date.now()): string {
  if (!iso) return '—'
  const then = new Date(iso).getTime()
  if (Number.isNaN(then)) return '—'
  const s = Math.max(0, (now - then) / 1000)
  if (s < 60) return `${Math.round(s)}s`
  if (s < 3600) return `${Math.round(s / 60)}m`
  if (s < 86400) return `${Math.round(s / 3600)}h`
  return `${Math.round(s / 86400)}d`
}

/** A condition status's tone: True is good, False bad, anything else unknown. */
export function conditionTone(status: string): 'success' | 'danger' | 'warning' {
  if (status === 'True') return 'success'
  if (status === 'False') return 'danger'
  return 'warning'
}

/** What a kind table shows: its rows after the kind's own filters, and why it is empty when it is. */
export interface KindTable {
  rows: InventoryRow[]
  /** `none` — nothing of the kind exists; `filtered` — a filter hides every row. */
  empty?: 'none' | 'filtered'
  /** The Signal kind's filter options, empty for every other kind. */
  filters: { sources: string[]; phases: string[] }
}

/**
 * The kind table's rows. Signals add filters by source and phase
 * (console-signals), client-side over the inventory the kind already fetched
 * whole: a signals backlog is bounded by the manager's per-source cap and TTL.
 * Nothing of the kind at all reads differently from a filter hiding every row.
 */
export function kindTable(kind: string, data: InventoryRow[], source = FILTER_ALL, phase = FILTER_ALL): KindTable {
  const isSignals = kind === 'signals'
  const filters = isSignals ? signalFilterOptions(data) : { sources: [], phases: [] }
  const rows = isSignals ? filterSignalRows(data, source, phase) : data
  let empty: KindTable['empty']
  if (data.length === 0) empty = 'none'
  else if (rows.length === 0) empty = 'filtered'
  return { rows, empty, filters }
}

/**
 * A Pipeline's own declared icon, read off its spec: a Pipeline that is not
 * Ready is absent from the vocabulary and still has its icon here.
 */
export function declaredIcon(kind: string, detail: Detail): string | undefined {
  return kind === 'pipelines' ? (detail.object.spec as { icon?: string } | undefined)?.icon : undefined
}

/** The consumer and delivery history of a Signal, only what the manager recorded. */
export function deliveryHistory(status: {
  attempts?: number
  firstAttemptAt?: string
  consumedBy?: string
  consumedAt?: string
  completedAt?: string
}): [string, string][] {
  const rows: [string, string | number | undefined][] = [
    ['Attempts', status.attempts],
    ['First attempt', status.firstAttemptAt],
    ['Consumed by', status.consumedBy],
    ['Consumed at', status.consumedAt],
    ['Completed at', status.completedAt],
  ]
  return rows.filter(([, v]) => v !== undefined && v !== '').map(([k, v]) => [k, String(v)])
}
