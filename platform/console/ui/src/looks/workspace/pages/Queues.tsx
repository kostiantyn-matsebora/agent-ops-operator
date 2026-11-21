import { useState } from 'react'
import type { ReactElement, ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { resolveIcon } from '../../../model/icon'
import { usePipelineIconLookup } from '../../../model/pipelineIcon'
import { STUCK, oldestOp, queueAge, useQueuesPage, workAge, workQueueTitle } from '../../../model/queues'
import { plain } from '../../../model/text'
import type { StuckReason } from '../../../api/types'
import './Queues.css'

// THE QUEUES PAGE, in the workspace look: "queued" separated from "stalled".
// Every row carries an age, and the flag names the CAUSE — each cause has a
// different fix. Same cards, same order as `classic`; every word is the model's.

const Text = ({ children }: Readonly<{ children?: string | null }>) => (children ? <>{plain(children)}</> : null)

function StuckFlag({ reason }: Readonly<{ reason?: StuckReason }>): ReactElement | null {
  if (!reason) return null
  const s = STUCK[reason]
  return (
    <span className="wk-pill bad" title={s.detail}>
      {s.title}
      <span className="wk-sr"> — {s.detail}</span>
    </span>
  )
}

function PipelineMention({ name }: Readonly<{ name: string }>): ReactElement {
  const lookup = usePipelineIconLookup()
  const icon = resolveIcon(lookup(name))
  return (
    <span className="wk-q-pipeline">
      {icon?.kind === 'path' && (
        <svg viewBox="0 0 24 24" width="1em" height="1em" fill="currentColor" aria-hidden focusable="false">
          <path d={icon.d} />
        </svg>
      )}
      {icon?.kind === 'image' && <img src={icon.src} alt="" width="16" height="16" />}
      {icon?.kind === 'text' && <span aria-hidden>{icon.text}</span>}
      <Text>{name}</Text>
    </span>
  )
}

function Card({ title, children }: Readonly<{ title: string; children: ReactNode }>) {
  return (
    <section className="wk-card wk-q-card">
      <h2>{title}</h2>
      {children}
    </section>
  )
}

function Empty({ title, children }: Readonly<{ title: string; children?: ReactNode }>) {
  return (
    <div className="wk-q-empty">
      <b>{title}</b>
      {children}
    </div>
  )
}

const WORK_PAGE_SIZE = 5

export function QueuesPage(): ReactElement {
  const { data, isLoading, error } = useQueuesPage()
  const [page, setPage] = useState(0)
  if (isLoading && !data) {
    return (
      <main className="wk-pane wk-page" aria-label="Queues and capacity">
        <div className="wk-q-state" role="status" aria-label="loading">Loading…</div>
      </main>
    )
  }
  if (error || !data) {
    return (
      <main className="wk-pane wk-page" aria-label="Queues and capacity">
        <div className="wk-q-state error" role="alert">
          <b>Could not load queues</b>
          {String(error)}
        </div>
      </main>
    )
  }

  const pages = Math.max(1, Math.ceil(data.work.length / WORK_PAGE_SIZE))
  const cur = Math.min(page, pages - 1)
  const rows = data.work.slice(cur * WORK_PAGE_SIZE, (cur + 1) * WORK_PAGE_SIZE)

  return (
    <main className="wk-pane wk-page" aria-label="Queues and capacity">
      <nav className="wk-q-crumb" aria-label="Breadcrumb">
        <span aria-current="page">Queues</span>
      </nav>
      <h1>Queues and capacity</h1>

      {/* The delivery queue exists in NO Kubernetes object: without the manager
          it is unavailable — not empty, which would read as "no ops pending". */}
      {data.error && (
        <div className="wk-q-alert" role="status">
          <b>Delivery queue unavailable</b>
          <Text>{data.error}</Text>
        </div>
      )}

      <div className="wk-q-grid">
        <Card title={workQueueTitle(data)}>
          {data.work.length === 0 ? (
            <Empty title="Nothing is waiting">No conversation is queued and none has a run in flight.</Empty>
          ) : (
            <div className="wk-tw">
              <table className="wk-tbl wk-q-work" aria-label="work queue">
                <thead>
                  <tr>
                    <th className="wk-q-conv">Conversation</th>
                    <th>Phase</th>
                    <th>Queued</th>
                    <th>Age</th>
                    <th>Flag</th>
                  </tr>
                </thead>
                <tbody>
                  {rows.map((row) => (
                    <tr key={row.conversation}>
                      <td className="wk-q-conv">
                        <Link
                          className="wk-q-link wk-q-clip"
                          to={`/conversations/${row.conversation}`}
                          title={plain(row.title || row.conversation)}
                        >
                          <Text>{row.title || row.conversation}</Text>
                        </Link>
                        {row.pipeline && (
                          <small className="wk-q-sub">
                            <PipelineMention name={row.pipeline} />
                          </small>
                        )}
                      </td>
                      <td>
                        <Text>{row.phase}</Text>
                      </td>
                      <td className="wk-num">{row.queued}</td>
                      <td className="wk-num">{workAge(row)}</td>
                      <td>
                        <StuckFlag reason={row.stuck} />
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
              {pages > 1 && (
                <div className="wk-q-pager">
                  <button type="button" aria-label="Previous page" disabled={cur === 0} onClick={() => setPage(cur - 1)}>
                    &lt;
                  </button>
                  <span>
                    {cur + 1} / {pages}
                  </span>
                  <button type="button" aria-label="Next page" disabled={cur >= pages - 1} onClick={() => setPage(cur + 1)}>
                    &gt;
                  </button>
                </div>
              )}
            </div>
          )}
        </Card>

        <Card title="Delivery queue">
          {data.delivery.length === 0 ? (
            <Empty title="No outstanding channel ops">Every enqueued op has been claimed and completed.</Empty>
          ) : (
            <div className="wk-tw">
              <table className="wk-tbl" aria-label="delivery queue">
                <thead>
                  <tr>
                    <th>Adapter</th>
                    <th>Queued</th>
                    <th>Claimed</th>
                    <th>Oldest</th>
                    <th>Flag</th>
                  </tr>
                </thead>
                <tbody>
                  {data.delivery.map((row) => {
                    // WHICH op, not just how many — identity a metric label may never carry.
                    const op = oldestOp(row)
                    return (
                      <tr key={row.adapter}>
                        <td>
                          <Link className="wk-q-link" to={`/config/channeladapters/${row.adapter}`}>
                            <Text>{row.adapter}</Text>
                          </Link>
                        </td>
                        <td className="wk-num">{row.queued}</td>
                        <td className="wk-num">{row.claimed}</td>
                        <td>
                          {op ? (
                            <>
                              <Text>{op.id}</Text>{' '}
                              <small>
                                {op.state} {op.age}
                              </small>
                            </>
                          ) : (
                            '—'
                          )}
                        </td>
                        <td>
                          <StuckFlag reason={row.stuck} />
                        </td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>
          )}
        </Card>
      </div>

      {/* A suppressed signal lane looks exactly like an idle one on a graph. */}
      <Card title="Cooldowns">
        {data.cooldowns.length === 0 ? (
          <Empty title="No signals are being suppressed">Nothing is inside a fingerprint cooldown window.</Empty>
        ) : (
          <div className="wk-tw">
            <table className="wk-tbl" aria-label="cooldowns">
              <thead>
                <tr>
                  <th>Source</th>
                  <th>Suppressed</th>
                  <th>Window</th>
                </tr>
              </thead>
              <tbody>
                {data.cooldowns.map((c) => (
                  <tr key={c.source}>
                    <td>
                      <Link className="wk-q-link" to={`/config/signalsources/${c.source}`}>
                        <Text>{c.source}</Text>
                      </Link>
                    </td>
                    <td className="wk-num">{c.suppressed}</td>
                    <td className="wk-num">{queueAge(c.windowSeconds)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>
    </main>
  )
}
