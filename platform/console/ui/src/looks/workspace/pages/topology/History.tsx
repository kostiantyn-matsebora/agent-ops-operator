import { useId, type ReactNode } from 'react'
import type { ChartResponse } from '../../../../api/types'
import { HISTORY_CHARTS, useHistoryChart } from '../../../../model/topology'
import { Fold } from './parts'

// The History card: aggregates from the metrics backend over a horizon longer
// than the manager's activity buffer. Every chart is an AGGREGATE — the
// cardinality rule keeps ids out of metric labels, so no point can identify a
// conversation. Live, per-item detail lives in the activity stream.

const W = 300
const H = 150
const PAD = { top: 10, right: 10, bottom: 26, left: 42 }
const SERIES = ['var(--ao-brand)', 'var(--ao-accent)', 'var(--ao-warning)', 'var(--ao-success)', 'var(--ao-danger)']

function Empty({ title, children }: Readonly<{ title: string; children?: ReactNode }>) {
  return (
    <div className="wk-tp-empty">
      <b>{title}</b>
      {children && <div className="wk-note">{children}</div>}
    </div>
  )
}

/** Lines over a shared time axis and a zero-based value axis, drawn as plain SVG. */
export function Lines({ title, unit, series }: Readonly<{ title: string; unit: string; series: ChartResponse['series'] }>) {
  const all = series.flatMap((s) => s.points)
  const t0 = Math.min(...all.map((p) => p.ts))
  const t1 = Math.max(...all.map((p) => p.ts))
  const top = Math.max(...all.map((p) => p.value), 0) || 1
  const x = (ts: number) => PAD.left + (t1 === t0 ? 0.5 : (ts - t0) / (t1 - t0)) * (W - PAD.left - PAD.right)
  const y = (v: number) => PAD.top + (1 - v / top) * (H - PAD.top - PAD.bottom)
  const name = (s: ChartResponse['series'][number], i: number) => Object.values(s.labels).join('/') || String(i)
  const time = (ts: number) => new Date(ts * 1000).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
  return (
    <figure className="wk-tp-chart">
      <svg viewBox={`0 0 ${W} ${H}`} role="img" aria-label={title}>
        {[0, 0.5, 1].map((f) => (
          <g key={f}>
            <line x1={PAD.left} x2={W - PAD.right} y1={y(top * f)} y2={y(top * f)} className="grid" />
            <text x={PAD.left - 4} y={y(top * f) + 3} textAnchor="end">{`${(top * f).toFixed(1)}${unit}`}</text>
          </g>
        ))}
        <text x={PAD.left} y={H - 8}>{time(t0)}</text>
        <text x={W - PAD.right} y={H - 8} textAnchor="end">{time(t1)}</text>
        {series.map((s, i) => (
          <polyline
            key={name(s, i)}
            fill="none"
            stroke={SERIES[i % SERIES.length]}
            strokeWidth={1.75}
            points={s.points.map((p) => `${x(p.ts).toFixed(1)},${y(p.value).toFixed(1)}`).join(' ')}
          />
        ))}
      </svg>
      {series.length > 1 && (
        <figcaption>
          {series.map((s, i) => (
            <span key={name(s, i)}><i style={{ background: SERIES[i % SERIES.length] }} />{name(s, i)}</span>
          ))}
        </figcaption>
      )}
    </figure>
  )
}

function HistoryChart({ chartKey, title, unit }: Readonly<{ chartKey: string; title: string; unit: string }>) {
  const { data, error, isLoading } = useHistoryChart(chartKey)
  const id = useId()
  return (
    <section className="wk-tp-histchart" aria-labelledby={id}>
      <h4 id={id} className="wk-tp-h">{title} <span className="wk-pill">aggregate</span></h4>
      {isLoading && <Empty title="Loading…" />}
      {error && <Empty title="Unavailable">{String(error)}</Empty>}
      {data && data.series.length === 0 && <Empty title="No data in this range">The backend returned no samples.</Empty>}
      {data && data.series.length > 0 && <Lines title={title} unit={unit} series={data.series} />}
    </section>
  )
}

export function HistoryCard({
  available, expanded, onToggle,
}: Readonly<{ available: boolean; expanded: boolean; onToggle: () => void }>) {
  return (
    <Fold title="History" expanded={expanded} onToggle={onToggle} label="Toggle history" testId="history">
      {available ? (
        <div className="wk-tp-histgrid">
          {HISTORY_CHARTS.map((c) => <HistoryChart key={c.key} chartKey={c.key} title={c.title} unit={c.unit} />)}
        </div>
      ) : (
        <Empty title="No metrics backend is configured">
          Windows beyond the manager's activity buffer are unavailable. Set <code>console.metrics.url</code> to a Prometheus
          or VictoriaMetrics query endpoint to enable throughput, run-duration percentiles and queue depth over long ranges.
        </Empty>
      )}
    </Fold>
  )
}
