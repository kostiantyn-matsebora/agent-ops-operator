import { type ReactElement, lazy, Suspense } from 'react'
import type { ActivityEvent } from '../../../api/types'
import { sequenceRows } from '../../../model/sequence'
import { useConversationGraphPage } from '../../../model/topology'

// THE THREAD'S GRAPH AND SEQUENCE VIEWS, a root's own: both read the
// conversation's topology and recorded hops, which for a member is a single
// node its parent's view already reaches with more context.

// The graph canvas is loaded on first use, so a viewer who never opens the
// tab never pays for it.
const Graph = lazy(() => import('../../../graph/Graph').then((m) => ({ default: m.Graph })))

export function GraphTab({ name }: Readonly<{ name: string }>): ReactElement {
  const page = useConversationGraphPage(name)
  if (page.loading) return <div className="wk-empty" aria-busy="true">Building the graph…</div>
  if (page.graph.error || !page.graph.data) {
    return <div className="wk-empty"><h2>Could not build the graph</h2>{String(page.graph.error ?? '')}</div>
  }
  if (page.topology.error || !page.topology.data) {
    return <div className="wk-empty"><h2>Could not load the topology</h2>{String(page.topology.error ?? '')}</div>
  }
  return (
    <>
      {page.graph.data.diverged && (
        <p className="wk-alert">
          The pipeline has been re-wired since this ran: the conversation materialized different bindings from{' '}
          {page.graph.data.pipeline ?? 'the pipeline'}'s current wiring.
        </p>
      )}
      <Suspense fallback={<div className="wk-empty" aria-busy="true">Building the graph…</div>}>
        <Graph
          topology={page.topology.data.topology}
          events={page.events}
          bufferStart={page.bufferStart}
          conversation={name}
          emptyMessage="This conversation involved no elements the Display panel is showing."
        />
      </Suspense>
    </>
  )
}

export function SequenceTab({ events }: Readonly<{ events: ActivityEvent[] }>): ReactElement {
  const rows = sequenceRows(events)
  if (rows.length === 0) {
    return (
      <div className="wk-empty">
        <h2>No recorded hops for this conversation</h2>
        The manager's activity buffer is bounded: hops older than it are gone, and the runs stay the durable record.
      </div>
    )
  }
  return (
    <table className="wk-seq" aria-label="Sequence">
      <thead>
        <tr><th>Hop</th><th>From → To</th><th>At</th><th>Latency</th></tr>
      </thead>
      <tbody>
        {rows.map((r) => (
          <tr key={r.key}>
            <td>{r.hop}{r.errored && <span className="wk-pill bad">error</span>}</td>
            <td><small>{r.route}</small></td>
            <td className="wk-num">{r.at}</td>
            <td className="wk-num">{r.latency}</td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}
