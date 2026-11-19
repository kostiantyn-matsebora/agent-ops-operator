import { useTopologyPage } from '../../../../model/topology'
import { GraphView } from '../../../../graph/GraphView'
import { workspaceChrome } from './chrome'
import { HistoryCard } from './History'
import { Plain } from './parts'
import './topology.css'

// THE TOPOLOGY PAGE, in the workspace look: the sections, order and fields the
// classic page has — notices about what the picture cannot say, the graph with
// its filters and panels, the pipelines the console is not joined to, and the
// History card. The look changes how they are drawn, not what they say.

function Crumb() {
  return (
    <nav className="wk-tp-crumb" aria-label="Breadcrumb">
      <span aria-current="page">Topology</span>
    </nav>
  )
}

function State({ kind, title, children }: Readonly<{ kind: 'loading' | 'error'; title: string; children?: string }>) {
  return (
    <main className="wk-pane wk-page wk-tp-page" aria-label="Topology" aria-busy={kind === 'loading' || undefined}>
      <Crumb />
      <div className="wk-empty" role={kind === 'error' ? 'alert' : 'status'}>
        <h2>{title}</h2>
        {children}
      </div>
    </main>
  )
}

export function TopologyPage() {
  const { data, isLoading, error, events, bufferStart, bufferCovers, historyOpen, toggleHistory } = useTopologyPage()

  if (isLoading && !data) return <State kind="loading" title="Loading the topology…" />
  if (error || !data) return <State kind="error" title="Could not load the topology">{String(error ?? '')}</State>
  const unjoined = data.unjoinedPipelines ?? []

  return (
    <main className="wk-pane wk-page wk-tp-page" aria-label="Topology">
      <Crumb />
      <h1 className="wk-sr">Topology</h1>
      {!bufferCovers &&
        // Two DIFFERENT situations that one sentence used to describe, which
        // read as "no backend configured" even when one was.
        (data.metricsAvailable ? (
          <div className="wk-tp-notice info" role="status">
            <b>Edge rates cover the manager's live buffer, not the whole window</b>
            <div>
              A metrics backend <strong>is</strong> connected, but it cannot extend these particular numbers: metric
              labels are bounded by CR count, so there is no per-edge series to read. Edge rates are therefore always
              live-buffer only. The aggregate charts below do cover this window.
            </div>
          </div>
        ) : (
          <div className="wk-tp-notice warn" role="status">
            <b>No metrics backend is configured</b>
            <div>
              Edge rates cover only what the manager still holds in memory, and nothing covers longer windows. Set{' '}
              <code>console.metrics.url</code> to a Prometheus or VictoriaMetrics query endpoint for historical aggregates.
            </div>
          </div>
        ))}
      {!data.stream.connected && (
        <div className="wk-tp-notice warn" role="status">
          <b>The activity stream is disconnected</b>
          <div>Edges will not animate — this graph shows wiring, not current traffic.</div>
        </div>
      )}
      {data.stream.lastGap && (
        // A quiet edge and an edge whose traffic was never recorded look
        // identical. Only one of them means the system was idle.
        <div className="wk-tp-notice info" role="status">
          <b>This window has a gap in recorded activity</b>
          <div>
            Nothing was recorded before {new Date(data.stream.lastGap.ts).toLocaleTimeString()} — the manager's activity
            buffer is in memory, so a restart or an overflow ends its history. Edge rates covering that stretch
            under-report; the wiring itself is read from Kubernetes and is complete.
          </div>
        </div>
      )}

      <GraphView chrome={workspaceChrome} topology={data.topology} events={events} bufferStart={bufferStart} reserveBelow={120} />

      {unjoined.length > 0 && (
        <section className="wk-card wk-tp-card">
          <h3 className="wk-tp-h">Pipelines the console is not joined to</h3>
          <p>
            Their conversations are visible but read-only — no console thread means nowhere to reply. Add{' '}
            <code>{data.consoleChannel}</code> to a pipeline's <code>channelRefs</code> to join it. The console never
            edits a Pipeline.
          </p>
          <div className="wk-tp-chips">
            {unjoined.map((p) => <span key={p} className="wk-pill"><Plain>{p}</Plain></span>)}
          </div>
        </section>
      )}

      <HistoryCard available={data.metricsAvailable} expanded={historyOpen} onToggle={toggleHistory} />
    </main>
  )
}
