import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import {
  Alert, Button, Card, CardBody, CardTitle, ClipboardCopy,
  DescriptionList, DescriptionListDescription, DescriptionListGroup, DescriptionListTerm,
  Label, LabelGroup, PageSection, Stack, StackItem, Tab, TabTitleText, Tabs, TextArea,
  Title, Tooltip,
} from '@patternfly/react-core'
import { Table, Tbody, Td, Th, Thead, Tr } from '@patternfly/react-table'
import { useParams } from 'react-router-dom'
import { Empty, ErrorState, Loading } from '../App'
import { useConversation, useConversationGraph, useMarkRead, useSession } from '../api/hooks'
import { PlainText } from '../components/Text'
import { Graph } from '../graph/Graph'
import { api, ApiError } from '../api/client'
import { Crumbs } from '../components/Crumbs'
import { Yaml } from '../components/Yaml'
import { MetadataCard, age } from '../components/Metadata'
import type { ActivityEvent } from '../api/types'

export function ConversationPage() {
  const { name = '' } = useParams()
  const { data, isLoading, error, refetch } = useConversation(name)
  const [tab, setTab] = useState<string | number>(0)

  // Opening a conversation reports its CONSOLE thread read, and reports again
  // as activity arrives while the view stays open.
  //
  // The watermark is never generated here — the server reads it off the
  // conversation's own state, and `unread` is what says the report would
  // advance anything at all, so a re-opened, already-read conversation sends
  // nothing. Observed conversations are skipped: no console thread, no
  // watermark to move.
  const markRead = useMarkRead()
  const summary = data?.conversation
  const reported = useRef('')
  const activity = summary?.lastActivity ?? summary?.created ?? ''
  const joinedUnread = Boolean(summary?.joined && summary?.unread)
  useEffect(() => {
    if (!summary || !joinedUnread) return
    const stamp = `${summary.name}:${activity}`
    if (reported.current === stamp) return
    reported.current = stamp
    markRead.mutate({ names: [summary.name] })
    // Deliberately keyed on the conversation and its activity, not on the
    // mutation handle: re-running on the handle would report on every render.
  }, [summary?.name, joinedUnread, activity])

  if (isLoading && !data) return <Loading />
  if (error || !data) return <ErrorState title="Conversation not found">{String(error)}</ErrorState>

  const c = data.conversation
  return (
    <>
      <Crumbs
        items={[
          { label: 'Conversations', to: '/conversations' },
          { label: c.title || c.name },
        ]}
      />
      <PageSection>
      <Stack hasGutter>
        <StackItem>
          <Title headingLevel="h1">
            <PlainText>{c.title || c.name}</PlainText>
          </Title>
          {/* The whole identity of the run, as chips: phase, attribution,
              profile, the runtime pod, and the capabilities it MATERIALIZED. */}
          <LabelGroup numLabels={10}>
            <Label isCompact color="blue">{c.phase}</Label>
            {c.pipeline ? (
              <Label isCompact color="blue">pipeline {c.pipeline}</Label>
            ) : (
              <Tooltip content="a Conversation records no pipelineRef; attribution is inferred from its bindings and left blank when ambiguous">
                <Label isCompact color="grey">unattributed</Label>
              </Tooltip>
            )}
            {c.profile && <Label isCompact color="purple">profile {c.profile}</Label>}
            {c.runtimePod && <Label isCompact color="grey">pod {c.runtimePod}</Label>}
            {c.errored && <Label isCompact status="danger">last run failed</Label>}
            <Label isCompact color="grey">{c.runCount} run(s)</Label>
            <Label isCompact color="grey">age {age(c.created)}</Label>
            {(c.toolsets ?? []).map((t) => (
              <Label key={t} isCompact color="orange">{t}</Label>
            ))}
            {(c.mcpConfigs ?? []).map((m) => (
              <Label key={m} isCompact color="teal">{m}</Label>
            ))}
          </LabelGroup>
        </StackItem>
        <StackItem>
          <Tabs activeKey={tab} onSelect={(_e, k) => setTab(k)}>
            <Tab eventKey={0} title={<TabTitleText>Transcript</TabTitleText>}>
              {/* `active` is passed so the transcript can re-pin itself to the
                  newest message when this tab is shown again. Its message list
                  is unchanged by a tab switch, so nothing else would tell it
                  to. */}
              <Transcript detail={data} onSent={() => refetch()} active={tab === 0} />
            </Tab>
            <Tab eventKey={1} title={<TabTitleText>Runs</TabTitleText>}>
              <RunTimeline detail={data} />
            </Tab>
            <Tab eventKey={2} title={<TabTitleText>Graph</TabTitleText>}>
              <ConversationGraphTab name={name} />
            </Tab>
            <Tab eventKey={3} title={<TabTitleText>Sequence</TabTitleText>}>
              <Sequence events={data.events ?? []} />
            </Tab>
            <Tab eventKey={4} title={<TabTitleText>YAML</TabTitleText>}>
              <Stack hasGutter>
                <StackItem>
                  <MetadataCard meta={data.object.metadata} />
                </StackItem>
                <StackItem>
                  <Yaml value={data.yaml} title={`conversation ${c.name} YAML`} />
                </StackItem>
              </Stack>
            </Tab>
          </Tabs>
        </StackItem>
      </Stack>
      </PageSection>
    </>
  )
}

function Transcript({
  detail,
  onSent,
  active,
}: {
  detail: NonNullable<ReturnType<typeof useConversation>['data']>
  onSent: () => void
  active: boolean
}) {
  const session = useSession()
  const [text, setText] = useState('')
  const [error, setError] = useState<string>()
  const [busy, setBusy] = useState(false)
  const messages = detail.transcript ?? []
  // Stick to the NEWEST message — on open, and on every arrival after it.
  //
  // Both cases matter and they are the same effect: a thread that opens at its
  // oldest line is the wrong end of the only view whose purpose is answering
  // what was just said, and one that does not follow an incoming answer makes
  // the reader hunt for the thing they were waiting for.
  //
  // useLayoutEffect, not useEffect: it runs before paint, so the thread appears
  // already scrolled instead of visibly jumping. Keyed on the last message's id
  // as well as the count, because a replaced final message (a pending local
  // reply becoming confirmed) is new content to look at even when the count is
  // unchanged.
  const listRef = useRef<HTMLDivElement>(null)
  const lastID = messages.length ? messages[messages.length - 1].id : ''
  useLayoutEffect(() => {
    if (!active) return
    const el = listRef.current
    if (!el) return
    const pin = () => {
      el.scrollTop = el.scrollHeight
    }
    pin()
    // A second pass after paint. Coming back to this tab, the container can
    // still be laying out when the effect runs — a hidden element has no
    // height, so scrollHeight is whatever it was a moment ago and pinning
    // reads as "did nothing". One frame later the measurement is real.
    const raf = requestAnimationFrame(pin)
    return () => cancelAnimationFrame(raf)
  }, [messages.length, lastID, active])
  // canWrite from the session, not writeEnabled: a console whose fronting proxy
  // forwards no identity has writes ON and nothing to attribute them to, and
  // the composer must say so rather than accept text the server will refuse.
  const canWrite = (session.data?.canWrite ?? false) && detail.conversation.joined && !detail.archived
  const noIdentity =
    (session.data?.writeEnabled ?? false) && !(session.data?.canWrite ?? false)

  async function send() {
    setBusy(true)
    setError(undefined)
    try {
      await api.send(detail.conversation.name, text)
      setText('')
      onSent()
    } catch (e) {
      setError((e as ApiError).message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <Stack hasGutter>
      {detail.joinHint && (
        <StackItem>
          {/* No composer, and the reason plus the exact patch. The console never
              edits a Pipeline — showing the edit IS the answer. */}
          <Alert variant="info" isInline title="This conversation has no console thread">
            <p>
              <PlainText>{detail.joinHint.reason}</PlainText>
            </p>
            <ClipboardCopy isReadOnly>{detail.joinHint.fix}</ClipboardCopy>
            {detail.joinHint.note && (
              <small>
                <PlainText>{detail.joinHint.note}</PlainText>
              </small>
            )}
          </Alert>
        </StackItem>
      )}
      {detail.archived && (
        <StackItem>
          <Alert variant="info" isInline title="This thread was archived">
            The conversation was closed. The transcript stays readable; there is nothing left to
            reply to.
          </Alert>
        </StackItem>
      )}
      {/* The message list SCROLLS and the composer does not.
          A long thread used to push the reply box off the bottom of the page,
          so answering meant scrolling to the end first — on the one view whose
          entire purpose is answering. The list gets the overflow; the composer
          below it stays put. */}
      <StackItem isFilled style={{ minHeight: 0 }}>
        <Card style={{ height: '100%' }}>
          <CardBody>
            {/* A PLAIN div holds the ref. PatternFly's CardBody does not
                forward one to its DOM node, so scrolling it never worked —
                listRef.current was null and the thread opened at its oldest
                line every time. */}
            <div
              ref={listRef}
              style={{ maxHeight: '58vh', overflowY: 'auto', overscrollBehavior: 'contain' }}
            >
            {messages.length === 0 ? (
              <Empty title="No messages on the console thread yet" />
            ) : (
              messages.map((m) => (
                <div key={m.id} style={{ marginBottom: 12 }}>
                  <strong>
                    {/* `sender` is set only for relayed sibling-channel
                        messages; otherwise the kind names who spoke. */}
                    <PlainText>{m.sender || m.kind}</PlainText>
                  </strong>{' '}
                  <small>{new Date(m.at).toLocaleTimeString()}</small>
                  {m.pending && <Label color="grey">sending…</Label>}
                  <div>
                    {/* Cluster- and wire-sourced text, rendered as plain text. */}
                    <PlainText multiline>{m.text}</PlainText>
                  </div>
                </div>
              ))
            )}
            </div>
          </CardBody>
        </Card>
      </StackItem>
      {noIdentity && detail.conversation.joined && !detail.archived && (
        <StackItem>
          {/* Writes are on; nobody said who is making them. Saying it here is
              the difference between a read-only console and a broken one. */}
          <Alert variant="info" isInline title="Replying needs an identity">
            {session.data?.externalAuthenticator || 'The proxy'} in front of this console
            authenticated you but forwarded no identity header, and every write is logged with the
            identity that made it. Configure it to forward X-Forwarded-Email or
            X-Auth-Request-User.
          </Alert>
        </StackItem>
      )}
      {canWrite && (
        <StackItem>
          <TextArea
            aria-label="message"
            value={text}
            onChange={(_e, v) => setText(v)}
            rows={3}
            placeholder="Reply to the agent…"
          />
          {error && <Alert variant="danger" isInline title={error} />}
          <Button onClick={send} isDisabled={busy || !text.trim()}>
            Send
          </Button>
        </StackItem>
      )}
    </Stack>
  )
}

function RunTimeline({ detail }: { detail: NonNullable<ReturnType<typeof useConversation>['data']> }) {
  const runs = detail.conversation.runs ?? []
  return (
    <Stack hasGutter>
      <StackItem>
        <Card>
          <CardTitle>Runs</CardTitle>
          <CardBody>
            {runs.length === 0 ? (
              <Empty title="No completed runs" />
            ) : (
              <Table variant="compact" aria-label="runs">
                <Thead>
                  <Tr>
                    <Th>Run</Th>
                    <Th>Status</Th>
                    <Th>Exit</Th>
                    <Th>Finished</Th>
                    <Th>Result</Th>
                  </Tr>
                </Thead>
                <Tbody>
                  {runs.map((r) => (
                    <Tr key={r.runId}>
                      <Td dataLabel="Run">
                        <PlainText>{r.runId}</PlainText>
                      </Td>
                      <Td dataLabel="Status">
                        <Label status={r.status === 'succeeded' ? 'success' : 'danger'}>
                          <PlainText>{r.status}</PlainText>
                        </Label>
                      </Td>
                      <Td dataLabel="Exit">{r.exitCode ?? '—'}</Td>
                      <Td dataLabel="Finished">
                        {r.finishedAt ? new Date(r.finishedAt).toLocaleString() : '—'}
                      </Td>
                      <Td dataLabel="Result">
                        {r.result ? (
                          <PlainText multiline>{r.result}</PlainText>
                        ) : r.status !== 'succeeded' ? (
                          // A failure with NO output is the one an operator
                          // cannot act on, so say what it usually means rather
                          // than leaving the cell blank.
                          <Alert
                            variant="warning"
                            isInline
                            isPlain
                            title="failed before producing any output"
                          >
                            The agent process exited non-zero without a result. The common cause is
                            a session that could no longer be resumed — sessions live in the runtime
                            pod's <code>/data/home</code>, so they vanish when it is not backed by a
                            persistent volume. Check the runtime pod logs, and{' '}
                            <code>persistence.enabled</code> in the chart.
                          </Alert>
                        ) : (
                          <small>—</small>
                        )}
                      </Td>
                    </Tr>
                  ))}
                </Tbody>
              </Table>
            )}
          </CardBody>
        </Card>
      </StackItem>
      <StackItem>
        <Card>
          <CardTitle>Bindings and runtime</CardTitle>
          <CardBody>
            <DescriptionList isCompact>
              <DescriptionListGroup>
                <DescriptionListTerm>Inputs queued</DescriptionListTerm>
                <DescriptionListDescription>{detail.conversation.queued}</DescriptionListDescription>
              </DescriptionListGroup>
              <DescriptionListGroup>
                <DescriptionListTerm>Threads</DescriptionListTerm>
                <DescriptionListDescription>
                  {(detail.conversation.threads ?? []).map((t) => (
                    <Label key={t.channel} style={{ marginRight: 4 }}>
                      <PlainText>{`${t.channel}: ${t.threadId}`}</PlainText>
                    </Label>
                  ))}
                </DescriptionListDescription>
              </DescriptionListGroup>
              <DescriptionListGroup>
                <DescriptionListTerm>Runtime pod</DescriptionListTerm>
                <DescriptionListDescription>
                  <PlainText>{detail.conversation.runtimePod || '—'}</PlainText>
                  {detail.runtimePodStatus?.problem && (
                    <Label status="danger">
                      <PlainText>{detail.runtimePodStatus.problem}</PlainText>
                    </Label>
                  )}
                </DescriptionListDescription>
              </DescriptionListGroup>
              <DescriptionListGroup>
                <DescriptionListTerm>Toolsets it ran with</DescriptionListTerm>
                <DescriptionListDescription>
                  <PlainText>{(detail.conversation.toolsets ?? []).join(', ') || 'none'}</PlainText>
                </DescriptionListDescription>
              </DescriptionListGroup>
            </DescriptionList>
          </CardBody>
        </Card>
      </StackItem>
    </Stack>
  )
}

function ConversationGraphTab({ name }: { name: string }) {
  const { data, isLoading, error } = useConversationGraph(name)
  if (isLoading && !data) return <Loading />
  if (error || !data) return <ErrorState title="Could not build the graph">{String(error)}</ErrorState>
  return (
    <Stack hasGutter>
      {data.diverged && (
        <StackItem>
          {/* The graph shows what the run ACTUALLY had. Reading the live
              pipeline instead would silently rewrite history, and the forensic
              value of this view is precisely that it does not. */}
          <Alert variant="info" isInline title="The pipeline has been re-wired since this ran">
            This graph shows the bindings this conversation materialized, not{' '}
            <PlainText>{data.pipeline}</PlainText>'s current wiring.
            <ul>
              {(data.drift ?? []).map((d, i) => (
                <li key={i}>
                  <PlainText>{d}</PlainText>
                </li>
              ))}
            </ul>
          </Alert>
        </StackItem>
      )}
      <StackItem>
        <Graph
          topology={data}
          liveEvents={data.events ?? []}
          emptyMessage="This conversation involved no elements the Display panel is showing."
        />
      </StackItem>
    </Stack>
  )
}

/**
 * The waterfall — hops in time order with per-hop latency.
 *
 * This is where "why did that take 40 seconds" gets answered, and it is the view
 * a graph cannot replace: a graph shows that an edge was used, not when or for
 * how long.
 */
function Sequence({ events }: { events: ActivityEvent[] }) {
  if (events.length === 0) {
    return (
      <Empty title="No recorded hops for this conversation">
        The manager's activity buffer is bounded — hops older than it are gone, and{' '}
        <code>status.runs[]</code> stays the durable record.
      </Empty>
    )
  }
  // MIN/MAX, not first/last. Events arrive in CURSOR order — emission order —
  // which is usually chronological and is not guaranteed to be. Taking the ends
  // of the list as the bounds produced negative offsets (`+-944.0s` in the
  // column) and a span far smaller than the real one, which then made a long
  // hop's bar hundreds of percent wide and let it escape its cell across the
  // whole table.
  const times = events.map((e) => new Date(e.ts).getTime()).filter((t) => !Number.isNaN(t))
  const start = times.length ? Math.min(...times) : 0
  const end = times.length ? Math.max(...times) : 0
  const span = Math.max(end - start, 1)

  return (
    <Card>
      <CardBody>
        <Table variant="compact" aria-label="sequence">
          <Thead>
            <Tr>
              <Th>Hop</Th>
              <Th>From → To</Th>
              <Th>At</Th>
              <Th>Latency</Th>
              <Th>Timeline</Th>
            </Tr>
          </Thead>
          <Tbody>
            {events.map((e) => {
              const at = new Date(e.ts).getTime()
              // CLAMPED. A bar is a proportion of the span and can never be
              // more than the track, however odd the underlying timestamps
              // are. Without this an out-of-order event or an unusually long
              // hop overflows an absolutely-positioned div across the page.
              const offset = Math.min(Math.max(((at - start) / span) * 100, 0), 100)
              const rawWidth = e.latencyMs ? (e.latencyMs / span) * 100 : 1
              const width = Math.min(Math.max(rawWidth, 1), 100 - offset)
              return (
                <Tr key={e.cursor}>
                  <Td dataLabel="Hop">
                    <PlainText>{e.kind}</PlainText>
                    {e.status === 'error' && <Label status="danger">error</Label>}
                  </Td>
                  <Td dataLabel="From → To">
                    <small>
                      <PlainText>
                        {`${e.from ? `${e.from.kind}/${e.from.name}` : '∅'} → ${
                          e.to ? `${e.to.kind}/${e.to.name}` : '∅'
                        }`}
                      </PlainText>
                    </small>
                    {/* Detail belongs to ITS hop. It used to be concatenated
                        into one blob under the table, where a reader could see
                        the text and not which row produced it — useless for the
                        one question detail answers. Free text from the cluster,
                        so rendered plain and never as markup. */}
                    {e.detail && (
                      <div>
                        <small style={{ color: 'var(--ao-text-subtle)', wordBreak: 'break-word' }}>
                          <PlainText>{e.detail}</PlainText>
                        </small>
                      </div>
                    )}
                  </Td>
                  <Td dataLabel="At">{`+${((at - start) / 1000).toFixed(1)}s`}</Td>
                  <Td dataLabel="Latency">
                    {e.latencyMs ? `${(e.latencyMs / 1000).toFixed(2)}s` : '—'}
                  </Td>
                  <Td dataLabel="Timeline">
                    <div
                      style={{
                        position: 'relative',
                        height: 10,
                        background: 'var(--ao-surface-alt)',
                        // The clamp above is the fix; this is the guard that
                        // keeps any future arithmetic mistake inside the cell.
                        overflow: 'hidden',
                      }}
                    >
                      <div
                        style={{
                          position: 'absolute',
                          left: `${offset}%`,
                          width: `${width}%`,
                          height: '100%',
                          background: e.status === 'error' ? 'var(--ao-danger)' : 'var(--ao-brand)',
                        }}
                      />
                    </div>
                  </Td>
                </Tr>
              )
            })}
          </Tbody>
        </Table>
      </CardBody>
    </Card>
  )
}
