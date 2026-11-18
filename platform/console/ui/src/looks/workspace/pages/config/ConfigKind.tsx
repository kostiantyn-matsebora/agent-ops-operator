import { useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import type { InventoryRow } from '../../../../api/types'
import {
  CHIP_COLUMNS, STATUS_COLUMNS, chipParts, columnsFor, kindTable, truncateCell, useConfigInventory,
} from '../../../../model/config'
import { FILTER_ALL, SIGNAL_PHASES, signalPhaseStatus } from '../../../../model/signals'
import { Crumbs, Dash, Empty, ConditionChips, KeyValueChips, KindGlyph, PageState, PipelineName, Pill, Plain, TableWrap, age } from './parts'

export function SignalPhasePill({ phase }: Readonly<{ phase?: string }>) {
  if (!phase) return <Dash />
  const t = signalPhaseStatus(phase)
  return <Pill tone={t}>{phase}</Pill>
}

function ColumnValue({ name, value }: Readonly<{ name: string; value?: string }>) {
  if (!value) return <Dash />
  if (name === 'phase' && SIGNAL_PHASES.has(value)) return <SignalPhasePill phase={value} />
  if (STATUS_COLUMNS.has(name)) {
    return (
      <Pill tone={value === 'True' ? 'success' : 'danger'}>
        {name}={value}
      </Pill>
    )
  }
  if (CHIP_COLUMNS.has(name)) {
    return (
      <span className="wk-cfg-chips">
        {chipParts(value).map((p) => (
          <span key={p} className="wk-cfg-kvchip blue" title={p}>
            <Plain>{p}</Plain>
          </span>
        ))}
      </span>
    )
  }
  return (
    <span title={value.length > 34 ? value : undefined}>
      <Plain>{truncateCell(value)}</Plain>
    </span>
  )
}

function SignalFilters(props: Readonly<{
  sources: string[]
  phases: string[]
  source: string
  phase: string
  onSource: (v: string) => void
  onPhase: (v: string) => void
}>) {
  return (
    <div className="wk-cfg-filters">
      <select aria-label="Filter by source" value={props.source} onChange={(e) => props.onSource(e.target.value)}>
        <option value={FILTER_ALL}>All sources</option>
        {props.sources.map((s) => (
          <option key={s} value={s}>{s}</option>
        ))}
      </select>
      <select aria-label="Filter by phase" value={props.phase} onChange={(e) => props.onPhase(e.target.value)}>
        <option value={FILTER_ALL}>All phases</option>
        {props.phases.map((p) => (
          <option key={p} value={p}>{p}</option>
        ))}
      </select>
    </div>
  )
}

/** One kind's objects as a table: Name, the kind's own columns, then Status, Labels, Age. */
export function ConfigKindPage() {
  const { kind = '' } = useParams()
  const { data, isLoading, error } = useConfigInventory(kind)
  const cols = columnsFor(kind)
  // The Signal kind's filters stay local to this page: nothing else reads them.
  const [source, setSource] = useState(FILTER_ALL)
  const [phase, setPhase] = useState(FILTER_ALL)
  if (isLoading) return <PageState kind="loading" title="Loading…" />
  if (error || !data) return <PageState kind="error" title={`Could not load ${kind}`}>{String(error)}</PageState>

  const { rows, empty, filters } = kindTable(kind, data, source, phase)
  const isSignals = kind === 'signals'

  return (
    <main className="wk-pane wk-page wk-cfg" aria-label={`Configuration: ${kind}`}>
      <Crumbs items={[{ label: 'Configuration', to: '/config' }, { label: kind }]} />
      <h1 className="wk-cfg-title">
        <KindGlyph kind={kind} size={22} />
        {kind}
      </h1>
      {isSignals && data.length > 0 && (
        <SignalFilters {...filters} source={source} phase={phase} onSource={setSource} onPhase={setPhase} />
      )}
      <section className="wk-card wk-cfg-flush">
        {empty === 'none' && <Empty title={`No ${kind}`}>Nothing of this kind exists in the namespace.</Empty>}
        {empty === 'filtered' && <Empty title="No signal matches these filters">Clear a filter to see more.</Empty>}
        {!empty && (
          <TableWrap>
            <table className="wk-tbl wk-cfg-tbl" aria-label={kind}>
              <thead>
                <tr>
                  <th scope="col">Name</th>
                  {cols.map((c) => (
                    <th key={c} scope="col">{c}</th>
                  ))}
                  <th scope="col">Status</th>
                  <th scope="col">Labels</th>
                  <th scope="col">Age</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((row: InventoryRow) => (
                  <tr key={row.name}>
                    <td>
                      <Link className="wk-cfg-link" to={`/config/${kind}/${row.name}`}>
                        {row.icon ? <PipelineName name={row.name} icon={row.icon} /> : <Plain>{row.name}</Plain>}
                      </Link>
                      {row.findings > 0 && (
                        <span className="wk-pill warn wk-cfg-findings" title="the console's own cross-reference checks flagged this">
                          {row.findings}
                        </span>
                      )}
                    </td>
                    {cols.map((c) => (
                      <td key={c}>
                        <ColumnValue name={c} value={row.columns?.[c]} />
                      </td>
                    ))}
                    <td><ConditionChips conditions={row.conditions} /></td>
                    <td><KeyValueChips values={row.labels} max={2} /></td>
                    <td className="wk-num">{age(row.created)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </TableWrap>
        )}
      </section>
    </main>
  )
}
