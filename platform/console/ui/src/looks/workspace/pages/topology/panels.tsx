import { Link } from 'react-router-dom'
import type { ComponentProps, ReactNode } from 'react'
import { hopContent } from '../../../../graph/content'
import { clock, hopPath } from '../../../../graph/format'
import type { ConversationPanel, EdgePanel, FeedPanel, HopPanel, NodePanel } from '../../../../graph/Panel'
import { plural, styleFor } from '../../../../graph/shapes'
import { useHopConversation } from '../../../../model/topology'
import { Card, Facts, Fold, HopList, HopRow, PipelineName, Plain, latency } from './parts'

// The side panels: the hop feed when nothing is selected, else what was.
//
// Reasons and messages are shown VERBATIM. An unclaimed source's whole value on
// this graph is the Wired=False message a reconciler wrote, and paraphrasing it
// would put the console's words in the cluster's mouth.

const HEALTH_PILL = { ok: 'ok', bad: 'bad', unknown: 'warn', none: '' } as const

export function Feed({
  title, feed, conversations, onHop, onConversation, expanded, onToggle,
}: ComponentProps<typeof FeedPanel>) {
  return (
    <Fold title={title} expanded={expanded} onToggle={onToggle} label="Toggle hop feed" testId="hop-feed">
      {feed.length ? (
        <HopList>
          {feed.map((ev) => <HopRow key={`${ev.cursor}-${ev.ts}-${ev.kind}`} ev={ev} onOpen={() => onHop(ev)} />)}
        </HopList>
      ) : (
        <p className="wk-note">quiet</p>
      )}
      <h4 className="wk-tp-h">Conversations in window</h4>
      {conversations.length ? (
        <div className="wk-tp-chips">
          {conversations.map((c) => (
            <button key={c} type="button" className="wk-btn" aria-label={`replay ${c}`} onClick={() => onConversation(c)}>
              <Plain>{c}</Plain>
            </button>
          ))}
        </div>
      ) : (
        <p className="wk-note">none</p>
      )}
    </Fold>
  )
}

function Neighbours({ title, list }: Readonly<{ title: string; list: ComponentProps<typeof NodePanel>['inbound'] }>) {
  return (
    <>
      <h4 className="wk-tp-h">{title}</h4>
      {list.length ? (
        <table className="wk-tp-table">
          <thead>
            <tr><th scope="col">element</th><th scope="col">/min</th><th scope="col">err</th><th scope="col">p50</th></tr>
          </thead>
          <tbody>
            {list.map((n) => (
              <tr key={n.edge.id}>
                <td><Plain>{n.other?.label ?? n.other?.name ?? '?'}</Plain></td>
                <td className="wk-num">{(n.traffic?.ratePerMin ?? 0).toFixed(1)}</td>
                <td className="wk-num">{n.traffic?.errors ?? 0}</td>
                <td className="wk-num">{latency(n.traffic?.p50LatencyMs)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      ) : (
        <p className="wk-note">none</p>
      )}
    </>
  )
}

export function Node({
  node, events, scoped, spine, inbound, outbound, canReplay, podToggle, onScope, onReplay, onTogglePod, onHide,
}: ComponentProps<typeof NodePanel>) {
  const style = styleFor(node.cls)
  const rows: [string, ReactNode][] = [
    ['Health', node.health === 'none' ? 'reports none' : <span key="h" className={`wk-pill ${HEALTH_PILL[node.health]}`}>{node.health}</span>],
  ]
  if (node.reason) rows.push(['Reason', <Plain key="r">{node.reason}</Plain>])
  if (node.message) rows.push(['Message', <Plain key="m" multiline>{node.message}</Plain>])
  if (node.detached) rows.push(['Detached', 'nothing in the wiring references this'])
  if (node.count !== undefined) rows.push(['Instances', `${node.count} running`])
  for (const [k, v] of node.facts) rows.push([k, <Plain key={k}>{v}</Plain>])
  rows.push(['Events', `${events} in window`])
  if (node.config) rows.push(['Object', <Link key="o" to={`/config/${node.config}`}>open</Link>])
  return (
    <Card
      testId="node-panel"
      kicker={style.label}
      title={node.icon ? <PipelineName name={node.name} icon={node.icon} /> : <Plain>{node.name}</Plain>}
    >
      <Facts rows={rows} />
      <div className="wk-tp-actions">
        <button type="button" className="wk-btn" onClick={onScope}>{scoped ? 'Clear scope' : 'Scope to route'}</button>
        {canReplay && <button type="button" className="wk-btn primary" onClick={onReplay}>Replay conversation</button>}
        {podToggle && <button type="button" className="wk-btn" onClick={onTogglePod}>{podToggle === 'expand' ? 'Expand pod' : 'Collapse pod'}</button>}
        {!spine && <button type="button" className="wk-btn link" onClick={onHide}>Hide {plural(style.label).toLowerCase()}</button>}
      </div>
      <Neighbours title="Inbound" list={inbound} />
      <Neighbours title="Outbound" list={outbound} />
    </Card>
  )
}

export function Edge({ edge, from, to, tone, traffic, history, onHop }: ComponentProps<typeof EdgePanel>) {
  const name = (n?: typeof from) => (n ? `${styleFor(n.cls).label} ${n.label ?? n.name}` : '?')
  return (
    <Card
      testId="edge-panel"
      kicker={`edge · ${edge.kind}`}
      title={<Plain>{`${from?.label ?? from?.name ?? edge.from} → ${to?.label ?? to?.name ?? edge.to}`}</Plain>}
    >
      <Facts rows={[
        ['From', <Plain key="f">{name(from)}</Plain>],
        ['To', <Plain key="t">{name(to)}</Plain>],
        ['Tone', tone],
        ['Rate', traffic ? `${traffic.ratePerMin.toFixed(2)}/min` : 'idle'],
        ['Events', String(traffic?.events ?? 0)],
        ['Errors', String(traffic?.errors ?? 0)],
        ['p50 / max', traffic?.p50LatencyMs ? `${latency(traffic.p50LatencyMs)} / ${latency(traffic.maxLatencyMs)}` : '—'],
        ['Last', traffic?.lastTs ? clock(traffic.lastTs) : '—'],
      ]} />
      <h4 className="wk-tp-h">Recent hops on this edge</h4>
      {history.length ? (
        <HopList>{history.map((ev) => <HopRow key={`${ev.cursor}-${ev.kind}`} ev={ev} onOpen={() => onHop(ev)} />)}</HopList>
      ) : (
        <p className="wk-note">none in window</p>
      )}
    </Card>
  )
}

export function Hop({ ev, onBack, onShow, onReplay }: ComponentProps<typeof HopPanel>) {
  const { data } = useHopConversation(ev.conversation)
  const carried = hopContent(ev, data)
  const rows: [string, ReactNode][] = [
    ['Path', <Plain key="p">{hopPath(ev)}</Plain>],
    ['Status', ev.status === 'error'
      ? <span key="s" className="wk-pill bad">error{ev.code ? ` · ${ev.code}` : ''}</span>
      : <span key="s" className="wk-pill ok">ok</span>],
  ]
  if (ev.latencyMs) rows.push(['Latency', `${(ev.latencyMs / 1000).toFixed(2)}s`])
  if (ev.conversation) rows.push(['Conversation', <Plain key="c">{ev.conversation}</Plain>])
  if (ev.pipeline) rows.push(['Pipeline', <PipelineName key="pl" name={ev.pipeline} />])
  return (
    <Card testId="hop-panel" kicker={`hop · ${clock(ev.ts)}`} title={<code>{ev.kind}</code>}>
      <Facts rows={rows} />
      <h4 className="wk-tp-h">What crossed</h4>
      {carried.rows.length ? (
        <Facts rows={carried.rows.map(([k, v]) => [k, <Plain key={k}>{v}</Plain>])} />
      ) : (
        <p className="wk-note">nothing recorded</p>
      )}
      {carried.body && (
        <div data-testid="hop-body">
          <small className="wk-note">{carried.body.label}{carried.body.truncated ? ' (the beginning only)' : ''}</small>
          <pre className="wk-tp-pre">{carried.body.text}</pre>
        </div>
      )}
      {carried.note && <p className="wk-note">{carried.note}</p>}
      <div className="wk-tp-actions">
        <button type="button" className="wk-btn" onClick={onBack}>← Back</button>
        <button type="button" className="wk-btn" onClick={onShow}>Show on graph</button>
        {onReplay && <button type="button" className="wk-btn primary" onClick={onReplay}>Replay conversation</button>}
      </div>
    </Card>
  )
}

export function Conversation({ name, list, current, onStep, onOpen }: ComponentProps<typeof ConversationPanel>) {
  const cur = list[current]?.ev
  const carried = cur ? hopContent(cur) : undefined
  return (
    <Card testId="conversation-panel" kicker="Conversation · replay" title={<Plain>{name}</Plain>}>
      <p className="wk-note">
        Every hop recorded for the latest run, in order. Everything off this conversation&apos;s route is dimmed.
      </p>
      {list.length ? (
        <HopList>
          {list.map((s, i) => (
            <HopRow key={`${s.ev.cursor}-${i}`} ev={s.ev} lead={`+${(s.offsetMs / 1000).toFixed(1)}s`} now={i === current} onOpen={() => onStep(i)} />
          ))}
        </HopList>
      ) : (
        <p className="wk-note">No recorded hops for this conversation in the buffer.</p>
      )}
      {cur && carried && (
        <>
          <h4 className="wk-tp-h">This hop carried</h4>
          <Facts rows={carried.rows.slice(0, 6).map(([k, v]) => [k, <Plain key={k}>{v}</Plain>])} />
          <button type="button" className="wk-btn link" onClick={() => onOpen(cur)}>Everything it carried</button>
        </>
      )}
    </Card>
  )
}
