import type { ReactElement, ReactNode } from 'react'
import { Link } from 'react-router-dom'
import {
  PROBLEM_SOURCE, capacityShare, healthTone, sinceAge, useMetricsBackend, useOverviewPage,
} from '../../../model/overview'
import { plain } from '../../../model/text'
import type { Overview, Problem, StreamHealth } from '../../../api/types'
import './Overview.css'

// THE OVERVIEW PAGE, in the workspace look: the same cards in the same order
// as `classic`, drawn with this look's own markup. Every fact is the model's.

const Text = ({ children }: Readonly<{ children?: string | null }>) => (children ? <>{plain(children)}</> : null)

const PILL: Readonly<Record<ReturnType<typeof healthTone>, string>> = {
  success: 'ok',
  danger: 'bad',
  warning: 'warn',
  custom: '',
}

const SOURCE_PILL: Readonly<Record<'blue' | 'grey' | 'purple', string>> = { blue: 'working', grey: '', purple: 'warn' }

function Crumb({ label }: Readonly<{ label: string }>): ReactElement {
  return (
    <nav className="wk-ov-crumb" aria-label="Breadcrumb">
      <span aria-current="page">{label}</span>
    </nav>
  )
}

function Alert({ tone, title, children }: Readonly<{ tone: 'danger' | 'warning'; title: string; children: ReactNode }>) {
  return (
    <div className={`wk-ov-alert ${tone}`} role={tone === 'danger' ? 'alert' : 'status'}>
      <b>{title}</b>
      {children}
    </div>
  )
}

function Empty({ title, children }: Readonly<{ title: string; children?: ReactNode }>) {
  return (
    <div className="wk-ov-empty">
      <b>{title}</b>
      {children}
    </div>
  )
}

function Card({ title, children }: Readonly<{ title: string; children: ReactNode }>) {
  return (
    <section className="wk-card wk-ov-card">
      <h2>{title}</h2>
      {children}
    </section>
  )
}

function Term({ term, children }: Readonly<{ term: string; children: ReactNode }>) {
  return (
    <div>
      <dt>{term}</dt>
      <dd>{children}</dd>
    </div>
  )
}

export function OverviewPage(): ReactElement {
  const { data, isLoading, error } = useOverviewPage()
  if (isLoading) {
    return (
      <main className="wk-pane wk-page" aria-label="Overview">
        <div className="wk-ov-state" role="status" aria-label="loading">Loading…</div>
      </main>
    )
  }
  if (error || !data) {
    return (
      <main className="wk-pane wk-page" aria-label="Overview">
        <div className="wk-ov-state error" role="alert">
          <b>Could not load the overview</b>
          {String(error)}
        </div>
      </main>
    )
  }

  const slots = data.manager?.runtimeSlots
  const share = slots ? capacityShare(slots) : undefined
  const outage = data.manager?.storageOutage

  return (
    <main className="wk-pane wk-page" aria-label="Overview">
      <Crumb label="Overview" />
      <h1>Installation</h1>

      {/* THE answer to "why is nothing running": first, above everything. */}
      {outage && (
        <Alert tone="danger" title="Context storage is unavailable — work is being held">
          No runtime pods are being started. Queued work is <strong>held, not failed</strong>, and will run once
          storage is reachable again.
          {outage.forSeconds > 0 && (
            <>
              {' '}
              Blocked for <strong>{Math.round(outage.forSeconds / 60)} minutes</strong>.
            </>
          )}
        </Alert>
      )}
      {data.managerError && (
        <Alert tone="warning" title="The manager's status surface is unreachable">
          <Text>{data.managerError}</Text> — capacity and queue depth come only from the manager's memory, so they are
          unavailable rather than zero.
        </Alert>
      )}
      {!data.stream.connected && (
        <Alert tone="warning" title="The activity stream is disconnected">
          Graphs will not animate. <Text>{data.stream.error}</Text>
        </Alert>
      )}

      <div className="wk-ov-grid">
        <Card title="Manager">
          <dl className="wk-ov-dl">
            <Term term="Version">
              <Text>{data.manager?.version ?? 'unknown'}</Text>
            </Term>
            <Term term="Leader">
              <Text>{data.manager?.leader || 'unknown'}</Text>
            </Term>
            <Term term="Namespace">{data.namespace}</Term>
          </dl>
        </Card>

        <Card title="Capacity">
          {slots && share ? (
            <>
              <div className="wk-ov-row">
                <span>
                  {slots.inUse} of {slots.max} runtime slots
                </span>
                <span className="wk-num">{Math.round(share.percent)}%</span>
              </div>
              <div
                className={`wk-ov-meter${share.full ? ' full' : ''}`}
                role="progressbar"
                aria-label={`${slots.inUse} of ${slots.max} runtime slots`}
                aria-valuemin={0}
                aria-valuemax={100}
                aria-valuenow={Math.round(share.percent)}
              >
                <i style={{ width: `${Math.min(100, share.percent)}%` }} />
              </div>
              <p>
                {slots.waiting} conversation(s) waiting for a slot.{' '}
                <Link className="wk-ov-link" to="/queues">
                  Queues
                </Link>
              </p>
            </>
          ) : (
            <Empty title="Unavailable">Capacity is counted in the manager's memory and cannot be read right now.</Empty>
          )}
        </Card>

        <Card title="Runtimes">
          <dl className="wk-ov-dl">
            {data.runtimes.map((rt) => (
              <Term key={rt.name} term={rt.name}>
                <Text>{rt.image}</Text>
              </Term>
            ))}
          </dl>
        </Card>

        <TelemetryCard stream={data.stream} />
      </div>

      <WorkloadsCard workloads={data.workloads} />
      <AdaptersCard adapters={data.adapters} />
      <ProblemsCard problems={data.problems ?? []} />
    </main>
  )
}

function WorkloadsCard({ workloads }: Readonly<{ workloads: Overview['workloads'] }>) {
  return (
    <Card title="Workloads">
      <div className="wk-tw">
        <table className="wk-tbl" aria-label="workloads">
          <thead>
            <tr>
              <th>Name</th>
              <th>Image</th>
              <th>Ready</th>
              <th>Restarts</th>
              <th>Problem</th>
            </tr>
          </thead>
          <tbody>
            {workloads.map((w) => (
              <tr key={w.name}>
                <td>
                  <Text>{w.name}</Text>
                </td>
                <td>
                  <Text>{w.image}</Text>
                  {w.digest && (
                    <small className="wk-ov-sub">
                      <Text>{w.digest}</Text>
                    </small>
                  )}
                </td>
                <td className="wk-num">
                  {w.ready}/{w.desired}
                </td>
                <td className="wk-num">{w.restarts}</td>
                <td>
                  {w.problem ? (
                    <span className="wk-pill bad">
                      <Text>{w.problem}</Text>
                    </span>
                  ) : null}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </Card>
  )
}

function AdaptersCard({ adapters }: Readonly<{ adapters: Overview['adapters'] }>) {
  return (
    <Card title="Adapters">
      <div className="wk-tw">
        <table className="wk-tbl" aria-label="adapters">
          <thead>
            <tr>
              <th>Kind</th>
              <th>Name</th>
              <th>Image</th>
              <th>Serves</th>
              <th>Health</th>
            </tr>
          </thead>
          <tbody>
            {adapters.map((a) => (
              <tr key={`${a.kind}/${a.name}`}>
                <td>{a.kind}</td>
                <td>
                  <Link className="wk-ov-link" to={`/config/${a.kind}/${a.name}`}>
                    <Text>{a.name}</Text>
                  </Link>
                </td>
                <td>
                  {/* An externally-served adapter owns no workload — a configuration, not a missing field. */}
                  {a.servedBy ? <span className="wk-pill">served by {a.servedBy}</span> : <Text>{a.image}</Text>}
                </td>
                <td className="wk-num">{a.serves}</td>
                <td>
                  <span className={`wk-pill ${PILL[healthTone(a.health)]}`}>
                    <Text>{a.reason || a.health}</Text>
                  </span>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </Card>
  )
}

/** What a resync COST, stated where the count is. */
function GapNotice({ lastGap }: Readonly<{ lastGap: StreamHealth['lastGap'] }>) {
  if (!lastGap) return null
  return (
    <small className="wk-ov-sub">
      Activity history before {new Date(lastGap.ts).toLocaleTimeString()} is unavailable
      {lastGap.detail ? ` — ${lastGap.detail}` : ''}. Conversations, topology and configuration are unaffected: those
      are read from Kubernetes, not from this buffer.
    </small>
  )
}

function TelemetryCard({ stream }: Readonly<{ stream: Overview['stream'] }>) {
  const backend = useMetricsBackend()
  return (
    <Card title="Telemetry">
      <dl className="wk-ov-dl">
        <Term term="Live activity">
          <span className={`wk-pill ${stream.connected ? 'ok' : 'warn'}`}>
            {stream.connected ? 'connected' : 'disconnected'}
          </span>{' '}
          <small>{stream.events} event(s) held</small>
          {stream.resyncs > 0 && <small> · {stream.resyncs} resync(s)</small>}
          <GapNotice lastGap={stream.lastGap} />
        </Term>
        <Term term="Metrics backend">
          <span className={`wk-pill ${backend ? 'ok' : 'warn'}`}>{backend ? 'connected' : 'not configured'}</span>
          <small className="wk-ov-sub">
            {backend ? (
              <>
                Historical aggregates available.{' '}
                <Link className="wk-ov-link" to="/topology">
                  See the charts
                </Link>
                .
              </>
            ) : (
              <>
                Set <code>console.metrics.url</code> for windows beyond the activity buffer.
              </>
            )}
          </small>
        </Term>
        <Term term="Scraped by">
          <small>
            the manager exposes <code>agentops_*</code> on <code>:9090/metrics</code>
          </small>
        </Term>
      </dl>
    </Card>
  )
}

/** The page's real job: "is my install healthy" answered on one page. */
export function ProblemsCard({ problems }: Readonly<{ problems: Problem[] }>) {
  return (
    <Card title={`Problems (${problems.length})`}>
      {problems.length === 0 ? (
        <Empty title="Nothing is reporting a problem">
          Every condition across every kind is True, and no pod is failing.
        </Empty>
      ) : (
        <div className="wk-tw">
          <table className="wk-tbl" aria-label="problems">
            <thead>
              <tr>
                <th>Object</th>
                <th>Condition</th>
                <th>Reason</th>
                <th>Message</th>
                <th>Source</th>
                <th>Since</th>
              </tr>
            </thead>
            <tbody>
              {problems.map((p, i) => (
                <tr key={`${p.kind}/${p.name}/${p.type}/${i}`}>
                  <td>
                    {p.kind === 'pods' ? (
                      <Text>{p.name}</Text>
                    ) : (
                      <Link className="wk-ov-link" to={`/config/${p.kind}/${p.name}`}>
                        <Text>{`${p.kind}/${p.name}`}</Text>
                      </Link>
                    )}
                  </td>
                  <td>{p.type}</td>
                  <td>
                    <Text>{p.reason}</Text>
                  </td>
                  <td>
                    <Text>{p.message}</Text>
                  </td>
                  <td>
                    <span className={`wk-pill ${SOURCE_PILL[PROBLEM_SOURCE[p.source].tone]}`}>
                      {PROBLEM_SOURCE[p.source].text}
                    </span>
                  </td>
                  <td className="wk-num">
                    {p.since ? <span title={p.since}>{sinceAge(p.since)}</span> : '—'}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </Card>
  )
}
