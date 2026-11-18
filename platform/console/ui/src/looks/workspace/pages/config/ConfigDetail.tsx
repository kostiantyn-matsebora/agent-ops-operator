import { useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import type { Detail, SignalStatusWire } from '../../../../api/types'
import { conditionTone, declaredIcon, deliveryHistory, useConfigDetail, usePipelineIconOf } from '../../../../model/config'
import { asSignalStatus, claimantStateStatus, isSignalTrouble } from '../../../../model/signals'
import { SignalPhasePill } from './ConfigKind'
import {
  Crumbs, Dash, Empty, HealthPill, Icon, KindGlyph, MetadataCard, PageState, PipelineName, Pill, Plain, TableWrap, Yaml, age,
} from './parts'

/** The per-claimant routing record, and the terminal reason as loud as the phase. */
function SignalRouting({ status }: Readonly<{ status: SignalStatusWire }>) {
  const claimants = status.claimants ?? []
  const history = deliveryHistory(status)
  return (
    <section className="wk-card" aria-label="Routing">
      <h3 className="wk-cfg-h">Routing</h3>
      <div className="wk-cfg-routehead">
        <SignalPhasePill phase={status.phase} />
        {status.reason && (
          <span className={`wk-cfg-reason ${isSignalTrouble(status.phase) ? 'bad' : 'warn'}`} role="alert">
            <Plain>{status.reason}</Plain>
          </span>
        )}
      </div>
      {history.length > 0 && (
        <dl className="wk-kv wk-cfg-kv wk-cfg-history" aria-label="delivery history">
          {history.map(([term, value]) => (
            <div key={term} className="wk-cfg-kvrow">
              <dt>{term}</dt>
              <dd>{value}</dd>
            </div>
          ))}
        </dl>
      )}
      {claimants.length === 0 ? (
        <Empty title="No claimant was ever snapshotted">
          {status.phase === 'Dropped'
            ? 'This signal was dropped before any claimant saw it.'
            : 'No routing attempt has run for this signal yet.'}
        </Empty>
      ) : (
        <TableWrap>
          <table className="wk-tbl wk-cfg-tbl" aria-label="claimants">
            <thead>
              <tr>
                <th scope="col">Kind</th>
                <th scope="col">Name</th>
                <th scope="col">State</th>
                <th scope="col">Conversation</th>
                <th scope="col">Reason</th>
              </tr>
            </thead>
            <tbody>
              {claimants.map((c) => (
                <tr key={`${c.ref.kind}/${c.ref.name}`}>
                  <td>{c.ref.kind}</td>
                  <td><Plain>{c.ref.name}</Plain></td>
                  <td><Pill tone={claimantStateStatus(c.state)}>{c.state}</Pill></td>
                  <td>
                    {c.conversationRef ? (
                      <Link className="wk-cfg-link" to={`/conversations/${c.conversationRef.name}`}>{c.conversationRef.name}</Link>
                    ) : (
                      <Dash />
                    )}
                  </td>
                  <td>{c.reason ? <Plain multiline>{c.reason}</Plain> : <Dash />}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </TableWrap>
      )}
    </section>
  )
}

function ConditionsCard({ detail }: Readonly<{ detail: Detail }>) {
  const conditions = detail.conditions ?? []
  return (
    <section className="wk-card" aria-label="Conditions">
      <h3 className="wk-cfg-h">Conditions</h3>
      {conditions.length === 0 ? (
        <Empty title="This kind reports no conditions">Nothing writes conditions here, so there is nothing to wait for.</Empty>
      ) : (
        <TableWrap>
          <table className="wk-tbl wk-cfg-tbl" aria-label="conditions">
            <thead>
              <tr>
                <th scope="col">Type</th>
                <th scope="col">Status</th>
                <th scope="col">Reason</th>
                <th scope="col">Message</th>
                <th scope="col">Since</th>
              </tr>
            </thead>
            <tbody>
              {conditions.map((c) => (
                <tr key={c.type}>
                  <td>{c.type}</td>
                  <td><Pill tone={conditionTone(c.status)}>{c.status}</Pill></td>
                  <td><Plain>{c.reason}</Plain></td>
                  <td><Plain multiline>{c.message}</Plain></td>
                  <td className="wk-num">{age(c.lastTransitionTime)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </TableWrap>
      )}
    </section>
  )
}

function FindingsCard({ detail }: Readonly<{ detail: Detail }>) {
  return (
    <section className="wk-card" aria-label="Console checks">
      <h3 className="wk-cfg-h">Console checks</h3>
      {/* The console's OWN cross-reference, never presented as something the cluster reported. */}
      <TableWrap>
        <table className="wk-tbl wk-cfg-tbl" aria-label="findings">
          <thead>
            <tr>
              <th scope="col">Check</th>
              <th scope="col">Reason</th>
              <th scope="col">Message</th>
            </tr>
          </thead>
          <tbody>
            {detail.findings.map((f, i) => (
              <tr key={`${f.check}-${i}`}>
                <td>{f.check}</td>
                <td><Plain>{f.reason}</Plain></td>
                <td><Plain multiline>{f.message}</Plain></td>
              </tr>
            ))}
          </tbody>
        </table>
      </TableWrap>
    </section>
  )
}

function UsedByCard({ detail }: Readonly<{ detail: Detail }>) {
  const iconFor = usePipelineIconOf()
  const used = detail.usedBy ?? []
  return (
    <section className="wk-card" aria-label="Used by">
      <h3 className="wk-cfg-h">Used by</h3>
      {used.length === 0 ? (
        <Empty title="Nothing references this">No other object points at it.</Empty>
      ) : (
        <span className="wk-cfg-chips">
          {used.map((r) => (
            <span key={`${r.kind}/${r.name}/${r.field}`} className="wk-cfg-kvchip">
              <Link className="wk-cfg-link" to={`/config/${r.kind}/${r.name}`}>
                {r.kind}/{r.kind === 'pipelines' ? <PipelineName name={r.name} icon={iconFor(r.name)} /> : r.name}
              </Link>{' '}
              ({r.field})
            </span>
          ))}
        </span>
      )}
    </section>
  )
}

/**
 * Resolved capabilities, rendered VERBATIM from the manager. The console does
 * not recompute composition: a second implementation would eventually
 * disagree with the one that runs.
 */
function ResolvedCard({ detail }: Readonly<{ detail: Detail }>) {
  const r = detail.resolved
  return (
    <section className="wk-card" aria-label="Resolved capabilities">
      <h3 className="wk-cfg-h">Resolved capabilities</h3>
      {detail.resolvedError && (
        <Empty title="The manager could not resolve this pipeline">
          <Plain>{detail.resolvedError}</Plain>
        </Empty>
      )}
      {r && (
        <dl className="wk-kv wk-cfg-kv">
          <dt>Runtime</dt>
          <dd><Plain>{r.runtime}</Plain></dd>
          <dt>Composition mode</dt>
          <dd><Pill>{r.toolsMode}</Pill></dd>
          <dt>Allowed tools</dt>
          <dd>
            {r.allowedTools.length === 0 ? (
              // An empty allowlist means EMPTY: a configuration, not a defect to paper over.
              <em>none — this wiring grants no tools</em>
            ) : (
              <pre className="wk-yaml wk-cfg-tools">{r.allowedTools.join('\n')}</pre>
            )}
            <small className="wk-cfg-dim">
              This is the wiring's half. The runtime composes it with the agent definition's own <code>tools:</code> per the mode above.
            </small>
          </dd>
          <dt>MCP servers</dt>
          <dd>
            {r.mcpServers.length === 0 ? (
              <em>none</em>
            ) : (
              <span className="wk-cfg-chips">
                {r.mcpServers.map((s) => (
                  <span key={s} className="wk-cfg-kvchip blue"><Plain>{s}</Plain></span>
                ))}
              </span>
            )}
          </dd>
          {(r.unresolved ?? []).length > 0 && (
            <>
              <dt>Unresolved</dt>
              <dd>
                <span className="wk-cfg-chips">
                  {(r.unresolved ?? []).map((u) => (
                    <Pill key={u} tone="danger"><Plain>{u}</Plain></Pill>
                  ))}
                </span>
              </dd>
            </>
          )}
        </dl>
      )}
    </section>
  )
}

type Tab = 'overview' | 'yaml'

/** An object: Overview (metadata, conditions, checks, used by, resolved) and its YAML. */
export function ConfigDetailPage() {
  const { kind = '', name = '' } = useParams()
  const { data, isLoading, error } = useConfigDetail(kind, name)
  const [tab, setTab] = useState<Tab>('overview')
  if (isLoading) return <PageState kind="loading" title="Loading…" />
  if (error || !data) return <PageState kind="error" title="Not found">{String(error)}</PageState>
  const icon = declaredIcon(kind, data)

  return (
    <main className="wk-pane wk-page wk-cfg" aria-label={`Configuration: ${kind}/${name}`}>
      <Crumbs
        items={[
          { label: 'Configuration', to: '/config' },
          { label: kind, to: `/config/${kind}` },
          { label: name, icon },
        ]}
      />
      <h1 className="wk-cfg-title">
        <KindGlyph kind={kind} size={24} />
        {icon && <Icon icon={icon} />}
        <Plain>{name}</Plain>
        <HealthPill health={data.health} />
      </h1>
      {kind === 'signals' && <SignalRouting status={asSignalStatus(data.object.status)} />}
      <div className="wk-cfg-tabs" role="tablist" aria-label="Object views">
        {(['overview', 'yaml'] as const).map((t) => (
          <button
            key={t}
            type="button"
            role="tab"
            id={`wk-cfg-tab-${t}`}
            aria-selected={tab === t}
            aria-controls="wk-cfg-panel"
            onClick={() => setTab(t)}
          >
            {t === 'yaml' ? 'YAML' : 'Overview'}
          </button>
        ))}
      </div>
      <div id="wk-cfg-panel" role="tabpanel" aria-labelledby={`wk-cfg-tab-${tab}`} className="wk-cfg-panel">
        {tab === 'overview' ? (
          <>
            <MetadataCard meta={data.object.metadata} health={data.health} conditions={data.conditions} />
            <ConditionsCard detail={data} />
            {data.findings.length > 0 && <FindingsCard detail={data} />}
            <UsedByCard detail={data} />
            {kind === 'pipelines' && <ResolvedCard detail={data} />}
          </>
        ) : (
          <Yaml value={data.yaml} title={`${kind}/${name} YAML`} />
        )}
      </div>
    </main>
  )
}
