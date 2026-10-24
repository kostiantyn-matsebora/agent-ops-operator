import type { ReactNode } from 'react'
import {
  Button, Card, CardBody, CardExpandableContent, CardHeader, CardTitle, DescriptionList, DescriptionListDescription, DescriptionListGroup,
  DescriptionListTerm, Label, Stack, StackItem,
} from '@patternfly/react-core'
import { Link } from 'react-router-dom'
import { healthVariant, useConversation } from '../api/hooks'
import type { ActivityEvent, EdgeTraffic } from '../api/types'
import { PipelineName } from '../components/PipelineName'
import { PlainText } from '../components/Text'
import { hopContent, hopSummary } from './content'
import type { EdgeTone } from './hops'
import type { Step } from './replay'
import { plural, styleFor } from './shapes'
import type { ViewEdge, ViewNode } from './types'

// The side panel: the hop feed when nothing is selected, else what was.
//
// Reasons and messages are shown VERBATIM. An unclaimed source's whole value on
// this graph is the Wired=False message a reconciler wrote, and paraphrasing it
// would put the console's words in the cluster's mouth.

export const clock = (ts: string | number) =>
  new Date(ts).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })

const seconds = (ms?: number) => (ms ? `${(ms / 1000).toFixed(1)}s` : '')
const latency = (ms?: number) => (ms ? (ms >= 1000 ? `${(ms / 1000).toFixed(1)}s` : `${ms}ms`) : '—')

export function hopPath(ev: ActivityEvent): string {
  return `${ev.from?.name ?? '∅'} → ${ev.to?.name ?? '∅'}`
}

function Facts({ rows }: { rows: [string, ReactNode][] }) {
  return (
    <DescriptionList isCompact isHorizontal>
      {rows.map(([k, v], i) => (
        <DescriptionListGroup key={`${k}-${i}`}>
          <DescriptionListTerm>{k}</DescriptionListTerm>
          <DescriptionListDescription>{v}</DescriptionListDescription>
        </DescriptionListGroup>
      ))}
    </DescriptionList>
  )
}

export function HopRow({ ev, lead, now, onOpen }: { ev: ActivityEvent; lead?: string; now?: boolean; onOpen: () => void }) {
  const summary = hopSummary(ev)
  return (
    <div
      role="button"
      tabIndex={0}
      data-testid="hop-row"
      className="ao-hop"
      onClick={onOpen}
      onKeyDown={(e) => {
        if (e.key === 'Enter') onOpen()
      }}
      style={{
        display: 'grid', gridTemplateColumns: '8ch 1fr auto', gap: 8, padding: '5px 8px', fontSize: 12.5,
        borderBottom: '1px solid var(--ao-border)', cursor: 'pointer',
        background: now ? 'var(--ao-brand-soft)' : undefined,
      }}
    >
      <span style={{ color: 'var(--ao-text-subtle)', fontVariantNumeric: 'tabular-nums' }}>{lead ?? clock(ev.ts)}</span>
      <span>
        <code>{ev.kind}</code>
        {ev.status === 'error' && <span style={{ color: 'var(--ao-danger)', fontWeight: 500 }}> error{ev.code ? ` · ${ev.code}` : ''}</span>}
        <br />
        <span style={{ color: 'var(--ao-text-subtle)', fontSize: 12 }}>
          <PlainText>{hopPath(ev) + (summary ? ` · ${summary}` : '')}</PlainText>
        </span>
      </span>
      <span style={{ color: 'var(--ao-text-subtle)', fontVariantNumeric: 'tabular-nums' }}>{seconds(ev.latencyMs)}</span>
    </div>
  )
}

function HopList({ children }: { children: ReactNode }) {
  return <div style={{ maxHeight: 280, overflow: 'auto', border: '1px solid var(--ao-border)', borderRadius: 6 }}>{children}</div>
}

export function FeedPanel({
  title, feed, conversations, onHop, onConversation, expanded, onToggle,
}: {
  title: string
  feed: ActivityEvent[]
  conversations: string[]
  onHop: (ev: ActivityEvent) => void
  onConversation: (name: string) => void
  /** Collapsed to its title, the feed stays where it is and reopens from the same chevron. */
  expanded: boolean
  onToggle: () => void
}) {
  return (
    <Card isCompact isExpanded={expanded} data-testid="hop-feed">
      <CardHeader onExpand={onToggle} toggleButtonProps={{ 'aria-label': 'Toggle hop feed', 'aria-expanded': expanded }}>
        <CardTitle>{title}</CardTitle>
      </CardHeader>
      <CardExpandableContent>
      <CardBody>
        <Stack hasGutter>
          <StackItem>
            {feed.length ? (
              <HopList>
                {feed.map((ev) => <HopRow key={`${ev.cursor}-${ev.ts}-${ev.kind}`} ev={ev} onOpen={() => onHop(ev)} />)}
              </HopList>
            ) : (
              <small>quiet</small>
            )}
          </StackItem>
          <StackItem>
            <strong>Conversations in window</strong>
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, marginTop: 6 }}>
              {conversations.length ? (
                conversations.map((c) => (
                  <Button key={c} size="sm" variant="secondary" onClick={() => onConversation(c)} aria-label={`replay ${c}`}>
                    <PlainText>{c}</PlainText>
                  </Button>
                ))
              ) : (
                <small>none</small>
              )}
            </div>
          </StackItem>
        </Stack>
      </CardBody>
      </CardExpandableContent>
    </Card>
  )
}

export interface Neighbour {
  edge: ViewEdge
  other?: ViewNode
  traffic?: EdgeTraffic
}

export function NodePanel({
  node, events, scoped, spine, inbound, outbound, canReplay, podToggle, onScope, onReplay, onTogglePod, onHide,
}: {
  node: ViewNode
  events: number
  scoped: boolean
  spine: boolean
  inbound: Neighbour[]
  outbound: Neighbour[]
  canReplay?: string
  podToggle?: 'expand' | 'collapse'
  onScope: () => void
  onReplay: () => void
  onTogglePod: () => void
  onHide: () => void
}) {
  const style = styleFor(node.cls)
  const rows: [string, ReactNode][] = [
    ['Health', node.health === 'none' ? 'reports none' : <Label status={healthVariant(node.health)}>{node.health}</Label>],
  ]
  if (node.reason) rows.push(['Reason', <PlainText key="r">{node.reason}</PlainText>])
  if (node.message) rows.push(['Message', <PlainText key="m" multiline>{node.message}</PlainText>])
  if (node.detached) rows.push(['Detached', 'nothing in the wiring references this'])
  if (node.count !== undefined) rows.push(['Instances', `${node.count} running`])
  for (const [k, v] of node.facts) rows.push([k, <PlainText key={k}>{v}</PlainText>])
  rows.push(['Events', `${events} in window`])
  if (node.config) rows.push(['Object', <Link key="o" to={`/config/${node.config}`}>open</Link>])
  const table = (title: string, list: Neighbour[]) => (
    <StackItem>
      <strong>{title}</strong>
      {list.length ? (
        <table style={{ width: '100%', fontSize: 12.5, borderCollapse: 'collapse' }}>
          <thead>
            <tr style={{ color: 'var(--ao-text-subtle)', textAlign: 'left' }}>
              <th>element</th><th style={{ textAlign: 'right' }}>/min</th><th style={{ textAlign: 'right' }}>err</th><th style={{ textAlign: 'right' }}>p50</th>
            </tr>
          </thead>
          <tbody>
            {list.map((n) => (
              <tr key={n.edge.id}>
                <td><PlainText>{n.other?.label ?? n.other?.name ?? '?'}</PlainText></td>
                <td style={{ textAlign: 'right' }}>{(n.traffic?.ratePerMin ?? 0).toFixed(1)}</td>
                <td style={{ textAlign: 'right' }}>{n.traffic?.errors ?? 0}</td>
                <td style={{ textAlign: 'right' }}>{latency(n.traffic?.p50LatencyMs)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      ) : (
        <div><small>none</small></div>
      )}
    </StackItem>
  )
  return (
    <Card isCompact data-testid="node-panel">
      <CardTitle>
        <small style={{ textTransform: 'uppercase', letterSpacing: '.06em', color: 'var(--ao-text-subtle)' }}>{style.label}</small>
        <div>{node.icon ? <PipelineName name={node.name} icon={node.icon} /> : <PlainText>{node.name}</PlainText>}</div>
      </CardTitle>
      <CardBody>
        <Stack hasGutter>
          <StackItem><Facts rows={rows} /></StackItem>
          <StackItem>
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
              <Button size="sm" variant="secondary" onClick={onScope}>{scoped ? 'Clear scope' : 'Scope to route'}</Button>
              {canReplay && <Button size="sm" variant="primary" onClick={onReplay}>Replay conversation</Button>}
              {podToggle && <Button size="sm" variant="secondary" onClick={onTogglePod}>{podToggle === 'expand' ? 'Expand pod' : 'Collapse pod'}</Button>}
              {!spine && <Button size="sm" variant="link" onClick={onHide}>Hide {plural(style.label).toLowerCase()}</Button>}
            </div>
          </StackItem>
          {table('Inbound', inbound)}
          {table('Outbound', outbound)}
        </Stack>
      </CardBody>
    </Card>
  )
}

export function EdgePanel({
  edge, from, to, tone, traffic, history, onHop,
}: {
  edge: ViewEdge
  from?: ViewNode
  to?: ViewNode
  tone: EdgeTone
  traffic?: EdgeTraffic
  history: ActivityEvent[]
  onHop: (ev: ActivityEvent) => void
}) {
  const name = (n?: ViewNode) => (n ? `${styleFor(n.cls).label} ${n.label ?? n.name}` : '?')
  return (
    <Card isCompact data-testid="edge-panel">
      <CardTitle>
        <small style={{ textTransform: 'uppercase', color: 'var(--ao-text-subtle)' }}>edge · {edge.kind}</small>
        <div><PlainText>{`${from?.label ?? from?.name ?? edge.from} → ${to?.label ?? to?.name ?? edge.to}`}</PlainText></div>
      </CardTitle>
      <CardBody>
        <Stack hasGutter>
          <StackItem>
            <Facts rows={[
              ['From', <PlainText key="f">{name(from)}</PlainText>],
              ['To', <PlainText key="t">{name(to)}</PlainText>],
              ['Tone', tone],
              ['Rate', traffic ? `${traffic.ratePerMin.toFixed(2)}/min` : 'idle'],
              ['Events', String(traffic?.events ?? 0)],
              ['Errors', String(traffic?.errors ?? 0)],
              ['p50 / max', traffic?.p50LatencyMs ? `${latency(traffic.p50LatencyMs)} / ${latency(traffic.maxLatencyMs)}` : '—'],
              ['Last', traffic?.lastTs ? clock(traffic.lastTs) : '—'],
            ]} />
          </StackItem>
          <StackItem>
            <strong>Recent hops on this edge</strong>
            {history.length ? (
              <HopList>{history.map((ev) => <HopRow key={`${ev.cursor}-${ev.kind}`} ev={ev} onOpen={() => onHop(ev)} />)}</HopList>
            ) : (
              <div><small>none in window</small></div>
            )}
          </StackItem>
        </Stack>
      </CardBody>
    </Card>
  )
}

export function HopPanel({
  ev, onBack, onShow, onReplay,
}: {
  ev: ActivityEvent
  onBack: () => void
  onShow: () => void
  onReplay?: () => void
}) {
  // Joined by id from the conversation's durable status — the same query the
  // conversation page reads, kept current by the stream.
  const { data } = useConversation(ev.conversation ?? '', Boolean(ev.conversation))
  const carried = hopContent(ev, data)
  const rows: [string, ReactNode][] = [
    ['Path', <PlainText key="p">{hopPath(ev)}</PlainText>],
    ['Status', ev.status === 'error'
      ? <Label key="s" status="danger">error{ev.code ? ` · ${ev.code}` : ''}</Label>
      : <Label key="s" status="success">ok</Label>],
  ]
  if (ev.latencyMs) rows.push(['Latency', `${(ev.latencyMs / 1000).toFixed(2)}s`])
  if (ev.conversation) rows.push(['Conversation', <PlainText key="c">{ev.conversation}</PlainText>])
  if (ev.pipeline) rows.push(['Pipeline', <PipelineName key="pl" name={ev.pipeline} />])
  return (
    <Card isCompact data-testid="hop-panel">
      <CardTitle>
        <small style={{ textTransform: 'uppercase', color: 'var(--ao-text-subtle)' }}>hop · {clock(ev.ts)}</small>
        <div><code>{ev.kind}</code></div>
      </CardTitle>
      <CardBody>
        <Stack hasGutter>
          <StackItem><Facts rows={rows} /></StackItem>
          <StackItem>
            <strong>What crossed</strong>
            {carried.rows.length ? (
              <Facts rows={carried.rows.map(([k, v]) => [k, <PlainText key={k}>{v}</PlainText>])} />
            ) : (
              <div><small>nothing recorded</small></div>
            )}
            {carried.body && (
              <div data-testid="hop-body" style={{ marginTop: 8 }}>
                <small>{carried.body.label}{carried.body.truncated ? ' (the beginning only)' : ''}</small>
                <pre style={{ whiteSpace: 'pre-wrap', wordBreak: 'break-word', fontSize: 12, margin: 0 }}>{carried.body.text}</pre>
              </div>
            )}
            {carried.note && <div><small>{carried.note}</small></div>}
          </StackItem>
          <StackItem>
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
              <Button size="sm" variant="secondary" onClick={onBack}>← Back</Button>
              <Button size="sm" variant="secondary" onClick={onShow}>Show on graph</Button>
              {onReplay && <Button size="sm" variant="primary" onClick={onReplay}>Replay conversation</Button>}
            </div>
          </StackItem>
        </Stack>
      </CardBody>
    </Card>
  )
}

export function ConversationPanel({
  name, list, current, onStep, onOpen,
}: {
  name: string
  list: Step[]
  current: number
  onStep: (i: number) => void
  onOpen: (ev: ActivityEvent) => void
}) {
  const cur = list[current]?.ev
  const carried = cur ? hopContent(cur) : undefined
  return (
    <Card isCompact data-testid="conversation-panel">
      <CardTitle>
        <small style={{ textTransform: 'uppercase', color: 'var(--ao-text-subtle)' }}>Conversation · replay</small>
        <div><PlainText>{name}</PlainText></div>
      </CardTitle>
      <CardBody>
        <Stack hasGutter>
          <StackItem>
            <small>Every hop recorded for the latest run, in order. Everything off this conversation&apos;s route is dimmed.</small>
          </StackItem>
          <StackItem>
            {list.length ? (
              <HopList>
                {list.map((s, i) => (
                  <HopRow key={`${s.ev.cursor}-${i}`} ev={s.ev} lead={`+${(s.offsetMs / 1000).toFixed(1)}s`} now={i === current} onOpen={() => onStep(i)} />
                ))}
              </HopList>
            ) : (
              <small>No recorded hops for this conversation in the buffer.</small>
            )}
          </StackItem>
          {cur && carried && (
            <StackItem>
              <strong>This hop carried</strong>
              <Facts rows={carried.rows.slice(0, 6).map(([k, v]) => [k, <PlainText key={k}>{v}</PlainText>])} />
              <Button size="sm" variant="link" isInline onClick={() => onOpen(cur)}>Everything it carried</Button>
            </StackItem>
          )}
        </Stack>
      </CardBody>
    </Card>
  )
}
