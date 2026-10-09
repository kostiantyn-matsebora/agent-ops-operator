import { useLayoutEffect, useMemo, useRef, useState } from 'react'
import { Button } from '@patternfly/react-core'
import { Empty } from '../../components/States'
import { Icon } from '../../components/Icon'
import { PlainText } from '../../components/Text'
import { Markdown } from '../../components/Markdown'
import { Blocks, Fold, agentText } from '../../components/Blocks'
import { parse } from '../../api/blocks'
import { fence } from '../../api/fence'
import type { ActivityEvent, Message } from '../../api/types'

// Ported from `prototype/C-rail.html` and `States.html` ("In the thread") —
// the divider, the jump pill and the run-event lines read from there.
// Behaviour from `console-thread-live-cues` and `console-unread`.

// Mirrors `countedKinds` on the console's own Go side (`transcript.go`) — a
// SECOND statement of the same rule, which `invariants.md`'s "parse markdown
// twice" note already accepts as the cost of a wire that carries no parsed
// structure.
const COUNTED_KINDS = new Set(['signal', 'agent', 'relay'])

const RUN_EVENT_KINDS: Record<string, string> = {
  'run.dispatched': 'run dispatched',
  'runtime.starting': 'runtime starting',
  'context.restored': 'context restored',
  'context.checkpoint': 'context checkpoint',
  'context.skipped': 'context skipped',
  'context.failed': 'context failed',
  'run.completed': 'run completed',
}

const SPEAKERS: Record<string, string> = { local: 'you', relay: 'user', agent: 'agent', ack: 'agent-ops', signal: 'signal' }
function speaker(kind: string): string {
  return SPEAKERS[kind] ?? kind
}
const SPEAKER_STYLE: Record<string, { icon: string; tint: string }> = {
  local: { icon: 'aops:user', tint: 'var(--ao-accent)' },
  relay: { icon: 'aops:user', tint: 'var(--ao-accent)' },
  agent: { icon: 'aops:agent', tint: 'var(--ao-text)' },
  signal: { icon: 'aops:alert', tint: 'var(--ao-danger)' },
}
function speakerStyle(kind: string) {
  return SPEAKER_STYLE[kind] ?? { icon: 'aops:system', tint: 'var(--ao-text)' }
}

function Avatar({ kind, icon }: Readonly<{ kind: string; icon?: string }>) {
  const style = speakerStyle(kind)
  return (
    <span
      aria-hidden
      style={{
        display: 'inline-flex', alignItems: 'center', justifyContent: 'center', width: '2em', height: '2em',
        borderRadius: '50%', color: style.tint, background: 'var(--ao-surface-alt)', border: `1px solid ${style.tint}`,
      }}
    >
      <Icon icon={kind === 'agent' ? icon || style.icon : style.icon} size="1.1em" />
    </span>
  )
}

/** One run's events folded to a single summary line, expandable. */
interface RunGroup {
  runId: string
  events: ActivityEvent[]
  at: number
}

/** Groups consecutive run-shaped events sharing one `runId` (design D-G). */
function groupRunEvents(events: ActivityEvent[]): RunGroup[] {
  const groups = new Map<string, RunGroup>()
  const order: string[] = []
  for (const e of events) {
    if (!RUN_EVENT_KINDS[e.kind]) continue
    const key = e.runId || e.cursor
    if (!groups.has(key)) {
      groups.set(key, { runId: e.runId || '', events: [], at: Date.parse(e.ts) || 0 })
      order.push(key)
    }
    groups.get(key)!.events.push(e)
  }
  return order.map((k) => groups.get(k)!)
}

function RunEventLine({ group }: Readonly<{ group: RunGroup }>) {
  const [expanded, setExpanded] = useState(false)
  const summary = group.events.map((e) => RUN_EVENT_KINDS[e.kind] || e.kind).join(' · ')
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 8, padding: '3px 0 3px 48px', fontSize: '0.8em', color: 'var(--ao-text-subtle)' }}>
      <span aria-hidden style={{ width: 14, height: 1, background: 'var(--ao-border)' }} />
      {expanded ? (
        <span>
          {group.events.map((e) => (
            <span key={e.cursor} style={{ marginRight: 10 }}>
              <PlainText>{RUN_EVENT_KINDS[e.kind] || e.kind}</PlainText>
              {e.latencyMs ? ` (${(e.latencyMs / 1000).toFixed(1)}s)` : ''}
            </span>
          ))}
        </span>
      ) : (
        <PlainText>{summary}</PlainText>
      )}
      {group.events.length > 1 && (
        <Button variant="link" isInline onClick={() => setExpanded((v) => !v)}>
          {expanded ? 'collapse' : `${group.events.length} events`}
        </Button>
      )}
    </div>
  )
}

/** One row to interleave into the transcript by time, alongside messages and
 * run-event groups — currently a Coordinator root's member-invocation blocks
 * (console-conversation-tree: these belong IN the transcript, not a separate
 * structural view). Timeline places it by `at` and renders `node` full-width,
 * without knowing anything about what it is. */
export interface TimelineExtraItem {
  key: string
  at: number
  node: React.ReactNode
}

export interface TimelineProps {
  messages: Message[]
  events: ActivityEvent[]
  presence: boolean
  /** The reader's watermark — the first counted message after this gets the "New messages" divider. */
  readAt?: string
  pipelineIcon?: string
  pipelineName?: string
  /** A gap in the activity buffer — console-thread-live-cues: "Lost history is marked". */
  activityGap?: boolean
  extraItems?: TimelineExtraItem[]
}

export function Timeline({
  messages, events, presence, readAt, pipelineIcon, pipelineName, activityGap, extraItems,
}: Readonly<TimelineProps>) {
  const bodyMessages = messages.filter((m) => m.kind !== 'ack')
  const runGroups = useMemo(() => groupRunEvents(events), [events])
  const watermark = readAt ? Date.parse(readAt) : undefined
  const firstUnreadId = useMemo(() => {
    if (watermark === undefined) return undefined
    const hit = bodyMessages.find((m) => COUNTED_KINDS.has(m.kind) && Date.parse(m.at) > watermark)
    return hit?.id
  }, [bodyMessages, watermark])

  const listRef = useRef<HTMLDivElement>(null)
  const dividerRef = useRef<HTMLDivElement>(null)
  const [atBottom, setAtBottom] = useState(true)
  const [arrivedSinceScrolledUp, setArrivedSinceScrolledUp] = useState(0)
  const lastCount = useRef(bodyMessages.length)

  // Opening with unread messages scrolls to the DIVIDER, never the end
  // (console-thread-live-cues: "Opening lands on what is new"). Otherwise —
  // and on every later arrival while already at the bottom — it pins to the
  // newest message.
  useLayoutEffect(() => {
    const el = listRef.current
    if (!el) return
    const grew = bodyMessages.length > lastCount.current
    lastCount.current = bodyMessages.length
    if (dividerRef.current && firstUnreadId) {
      dividerRef.current.scrollIntoView?.({ block: 'start' })
      return
    }
    if (atBottom || !grew) {
      const pin = () => {
        el.scrollTop = el.scrollHeight
      }
      pin()
      const raf = requestAnimationFrame(pin)
      return () => cancelAnimationFrame(raf)
    }
    if (grew) setArrivedSinceScrolledUp((n) => n + 1)
    // firstUnreadId is read once per conversation open and is deliberately
    // excluded: re-scrolling to a now-stale divider on every later arrival
    // would fight the autoscroll rule it hands off to.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [bodyMessages.length])

  function onScroll() {
    const el = listRef.current
    if (!el) return
    const nowAtBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 24
    setAtBottom(nowAtBottom)
    if (nowAtBottom) setArrivedSinceScrolledUp(0)
  }

  function jumpToEnd() {
    const el = listRef.current
    if (!el) return
    el.scrollTop = el.scrollHeight
    setArrivedSinceScrolledUp(0)
    setAtBottom(true)
  }

  // Interleave messages, run-event groups and any extra (invocation) rows by
  // time — design D-G.
  type Item = { at: number; message?: Message; events?: RunGroup; extra?: TimelineExtraItem }
  const items: Item[] = [
    ...bodyMessages.map((m) => ({ at: Date.parse(m.at) || 0, message: m })),
    ...runGroups.map((g) => ({ at: g.at, events: g })),
    ...(extraItems ?? []).map((e) => ({ at: e.at, extra: e })),
  ].sort((a, b) => a.at - b.at)

  return (
    <div style={{ position: 'relative', flex: 1, minHeight: 0, display: 'flex', flexDirection: 'column' }}>
      <div ref={listRef} onScroll={onScroll} data-testid="timeline" style={{ flex: 1, minHeight: 0, overflowY: 'auto', overflowX: 'hidden', padding: '0 24px' }}>
        {activityGap && (
          <div style={{ textAlign: 'center', fontSize: '0.8em', color: 'var(--ao-text-subtle)', padding: '6px 0' }}>
            — some activity here was not recorded —
          </div>
        )}
        {items.length === 0 ? (
          <Empty title="No messages on the console thread yet" />
        ) : (
          items.map((item, i) => {
            if (item.events) return <RunEventLine key={`events-${item.events.events[0].cursor}`} group={item.events} />
            if (item.extra) return <div key={item.extra.key} style={{ padding: '0.5em 0' }}>{item.extra.node}</div>
            const m = item.message!
            const prev = items[i - 1]?.message
            const sameSpeaker = prev?.kind === m.kind && (prev.sender ?? '') === (m.sender ?? '')
            const within = prev && Date.parse(m.at) - Date.parse(prev.at) < 60_000
            const startsGroup = !sameSpeaker || !within
            return (
              <article key={m.id} style={{ display: 'grid', gridTemplateColumns: '2em 1fr', columnGap: '0.75em', padding: startsGroup ? '0.85em 0 0.15em' : '0.15em 0' }}>
                {m.id === firstUnreadId && (
                  <div
                    ref={dividerRef}
                    style={{ gridColumn: '1 / -1', display: 'flex', alignItems: 'center', gap: 12, margin: '8px 0 4px', color: 'var(--ao-brand-strong)', fontSize: '0.8em', fontWeight: 700 }}
                  >
                    <span aria-hidden style={{ flex: 1, height: 1, background: 'var(--ao-brand)' }} />
                    <span>New messages</span>
                    <span aria-hidden style={{ flex: 1, height: 1, background: 'var(--ao-brand)' }} />
                  </div>
                )}
                <div style={{ gridColumn: 1 }}>{startsGroup && <Avatar kind={m.kind} icon={pipelineIcon} />}</div>
                <div style={{ gridColumn: 2, minWidth: 0 }}>
                  {startsGroup && (
                    <header style={{ display: 'flex', alignItems: 'baseline', justifyContent: 'space-between', gap: '0.75em', marginBottom: '0.2em' }}>
                      <strong style={{ color: speakerStyle(m.kind).tint }}>
                        <PlainText>{m.sender || speaker(m.kind)}</PlainText>
                      </strong>
                      <time style={{ color: 'var(--ao-text-subtle)', fontSize: '0.85em' }}>{new Date(m.at).toLocaleTimeString()}</time>
                    </header>
                  )}
                  {agentText(m.kind) ? <Blocks blocks={parse(m.text)} /> : <Markdown>{m.text}</Markdown>}
                  {m.payload && <Fold text={fence(m.payload)} label="Payload" />}
                </div>
              </article>
            )
          })
        )}
        {presence && (
          <div style={{ display: 'grid', gridTemplateColumns: '2em 1fr', columnGap: '0.75em', padding: '0.5em 0' }}>
            <Avatar kind="agent" icon={pipelineIcon} />
            <div>
              {pipelineName && (
                <div style={{ fontSize: '0.85em', color: 'var(--ao-brand-strong)' }}>
                  <PlainText>{pipelineName}</PlainText> is working…
                </div>
              )}
              <span
                data-testid="typing"
                aria-label="working"
                style={{ display: 'inline-flex', gap: 4, padding: '8px 12px', borderRadius: 14, background: 'var(--ao-surface-alt)', border: '1px solid var(--ao-border)' }}
              >
                <i className="ao-typing-dot" /><i className="ao-typing-dot" /><i className="ao-typing-dot" />
              </span>
            </div>
          </div>
        )}
      </div>
      {!atBottom && arrivedSinceScrolledUp > 0 && (
        <Button
          onClick={jumpToEnd}
          data-testid="jump-pill"
          style={{ position: 'absolute', right: 28, bottom: 16, borderRadius: 16 }}
        >
          {`↓ ${arrivedSinceScrolledUp} new message${arrivedSinceScrolledUp === 1 ? '' : 's'}`}
        </Button>
      )}
    </div>
  )
}
