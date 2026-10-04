import { useEffect, useMemo, useRef, useState } from 'react'
import {
  Alert, Button, Card, CardBody, CardTitle, ClipboardCopy, DescriptionList,
  DescriptionListDescription, DescriptionListGroup, DescriptionListTerm, Label,
  TextArea, Tooltip,
} from '@patternfly/react-core'
import { Table, Tbody, Td, Th, Thead, Tr } from '@patternfly/react-table'
import { Link } from 'react-router-dom'
import { Empty, ErrorState, Loading } from '../../components/States'
import {
  useConversation, useConversationGraph, useConversations, useMarkRead, useSession,
  useSources, useTopology, useVocabulary,
} from '../../api/hooks'
import { eventsFor, useStream } from '../../api/stream'
import { mergeEvents } from '../../graph/hops'
import { PlainText, RawText } from '../../components/Text'
import { Graph } from '../../graph/Graph'
import { useDisplay } from '../../graph/display'
import { api, ApiError } from '../../api/client'
import { PipelineName } from '../../components/PipelineName'
import { ComposerHint } from '../../components/ComposerHint'
import { Icon, stripLeadingIcon } from '../../components/Icon'
import { Yaml } from '../../components/Yaml'
import { MetadataCard } from '../../components/Metadata'
import { matchEntries } from '../NewConversation'
import { Timeline } from './Timeline'
import { QuickChips } from './QuickChips'
import type {
  ActivityEvent, ConversationDetail, ConversationSummary, Run, VocabularyEntry,
} from '../../api/types'

// `console-chat-layout` (secondary views) + `console-conversation-tree` (the
// incident timeline, the parent chain, read-only before escalation).
// Composition from `prototype/C-rail.html`'s header row and
// `prototype/D-incident.html`'s crumb + budget chip.

type SecondaryView = 'transcript' | 'runs' | 'graph' | 'sequence' | 'yaml'

export function ThreadPane({ name, onBack }: Readonly<{ name: string; onBack?: () => void }>) {
  const { data, isLoading, error, refetch } = useConversation(name)
  const [view, setView] = useState<SecondaryView>('transcript')
  const markRead = useMarkRead()
  // Called HERE, unconditionally — every hook below this line must run on
  // every render, loading or not, or React's hook count desyncs the moment
  // the query settles (rules of hooks).
  const vocabulary = useVocabulary()
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
  }, [summary?.name, joinedUnread, activity])

  // Switching conversations resets any secondary view back to the transcript
  // — a reader who left the Graph open on one conversation should not find
  // the next one opened there too.
  useEffect(() => setView('transcript'), [name])

  if (isLoading && !data) return <Loading />
  if (error || !data) return <ErrorState title="Conversation not found">{String(error)}</ErrorState>

  const c = data.conversation
  const pipelineIcon = vocabulary.data?.entries.find((e) => e.kind === 'pipeline' && e.name === c.pipeline)?.icon
  const title = stripLeadingIcon(c.title || c.name)
  const isRoot = Boolean(c.coordinator)
  const isMember = Boolean(c.causedBy)

  return (
    <div style={{ flex: 1, minWidth: 0, background: 'var(--ao-surface)', display: 'flex', flexDirection: 'column' }}>
      <div style={{ padding: '12px 24px 10px', borderBottom: '1px solid var(--ao-border)', display: 'flex', flexDirection: 'column', gap: 6 }}>
        {(isRoot || isMember) && <IncidentCrumb conversation={c} />}
        <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
          {onBack && (
            <Button variant="plain" aria-label="back to the list" onClick={onBack}>
              ←
            </Button>
          )}
          <span style={{ fontSize: '1.1em', fontWeight: 700, flex: 1, minWidth: 0 }}>
            <Icon icon={pipelineIcon} /> <PlainText>{title}</PlainText>
          </span>
          {c.presence && (
            <Label color="blue" icon={<Icon icon="aops:system" />}>
              {c.inflight ? `working · run ${c.inflight.runId}` : 'working'}
            </Label>
          )}
          <Tooltip content="every channel a reply here also reaches">
            <Label color="grey">
              <PlainText>{(c.threads ?? []).map((t) => t.channel).join(' · ') || 'no bound channel'}</PlainText>
            </Label>
          </Tooltip>
          {!isMember && (
            <>
              <ViewButton active={view === 'runs'} onClick={() => setView('runs')}>Runs</ViewButton>
              <ViewButton active={view === 'graph'} onClick={() => setView('graph')}>Graph</ViewButton>
              <ViewButton active={view === 'sequence'} onClick={() => setView('sequence')}>Sequence</ViewButton>
            </>
          )}
          <ViewButton active={view === 'yaml'} onClick={() => setView('yaml')}>YAML</ViewButton>
          {view !== 'transcript' && <ViewButton active onClick={() => setView('transcript')}>Back to transcript</ViewButton>}
        </div>
        {c.brief && (
          <p style={{ color: 'var(--ao-text-subtle)', margin: 0, fontSize: '0.9em' }}>
            <PlainText>{c.brief}</PlainText>
          </p>
        )}
      </div>
      <div style={{ flex: 1, minHeight: 0, display: 'flex', flexDirection: 'column' }}>
        {view === 'runs' && <RunTimeline detail={data} />}
        {view === 'graph' && <ConversationGraphTab name={name} />}
        {view === 'sequence' && <Sequence events={data.events ?? []} />}
        {view === 'yaml' && (
          <div style={{ padding: 16, overflowY: 'auto' }}>
            <MetadataCard meta={data.object.metadata} />
            <div style={{ marginTop: 12 }}>
              <Yaml value={data.yaml} title={`conversation ${c.name} YAML`} />
            </div>
          </div>
        )}
        {view === 'transcript' && (
          <TranscriptBody isRoot={isRoot} isMember={isMember} conversation={c} detail={data} onSentOffline={() => refetch()} />
        )}
      </div>
    </div>
  )
}

function TranscriptBody({
  isRoot, isMember, conversation, detail, onSentOffline,
}: Readonly<{
  isRoot: boolean
  isMember: boolean
  conversation: ConversationSummary
  detail: NonNullable<ReturnType<typeof useConversation>['data']>
  onSentOffline: () => void
}>) {
  if (isRoot) return <IncidentBody rootName={conversation.name} />
  if (isMember) return <MemberOwnBody conversation={conversation} />
  return <ConversationThread detail={detail} onSentOffline={onSentOffline} />
}

function ViewButton({ active, onClick, children }: Readonly<{ active: boolean; onClick: () => void; children: React.ReactNode }>) {
  return (
    <Button variant={active ? 'primary' : 'secondary'} size="sm" onClick={onClick}>
      {children}
    </Button>
  )
}

/** The parent chain, the uncaused root through every parent to this conversation (console-conversation-tree). */
function IncidentCrumb({ conversation }: Readonly<{ conversation: ConversationSummary }>) {
  const membersParams = useMemo(() => new URLSearchParams({ limit: '200' }), [])
  const members = useConversations(membersParams)
  const items = members.data?.items ?? []
  const chain: ConversationSummary[] = []
  const byName = new Map(items.map((c) => [c.name, c]))
  let cur: ConversationSummary | undefined = byName.get(conversation.name) ?? conversation
  const seen = new Set<string>()
  while (cur) {
    chain.unshift(cur)
    if (!cur.causedBy || seen.has(cur.name)) break
    seen.add(cur.name)
    cur = byName.get(cur.causedBy.parent)
  }
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 6, flexWrap: 'wrap', fontSize: '0.85em', color: 'var(--ao-text-subtle)' }}>
      <Icon icon="aops:agent" /> <strong style={{ color: 'var(--ao-accent)' }}>Incident</strong>
      {chain.map((step, i) => (
        <span key={step.name} style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}>
          <span aria-hidden>›</span>
          <Link
            to={`/conversations/${step.name}`}
            style={i === chain.length - 2 ? { fontWeight: 700, color: 'var(--ao-text)' } : undefined}
          >
            <PlainText>{step.causedBy?.entry ?? stripLeadingIcon(step.title || step.name)}</PlainText>
          </Link>
        </span>
      ))}
    </div>
  )
}

/** The root's incident timeline — fetches its own detail and the page's conversations to find members (design D-F). */
function IncidentBody({ rootName }: Readonly<{ rootName: string }>) {
  const root = useConversation(rootName)
  const membersParams = useMemo(() => new URLSearchParams({ limit: '200' }), [])
  const members = useConversations(membersParams)
  if ((root.isLoading && !root.data) || (members.isLoading && !members.data)) return <Loading />
  if (root.error || !root.data) return <ErrorState title="Could not load this conversation">{String(root.error)}</ErrorState>
  if (members.error || !members.data) return <ErrorState title="Could not load member conversations">{String(members.error)}</ErrorState>
  return <CoordinatorTimeline rootDetail={root.data} allConversations={members.data.items} depth={0} />
}

interface TimelineEntry {
  at: number
  run?: Run
  member?: ConversationSummary
}

function buildIncidentTimeline(rootDetail: ConversationDetail, allConversations: ConversationSummary[]): TimelineEntry[] {
  const rootName = rootDetail.conversation.name
  const entries: TimelineEntry[] = (rootDetail.conversation.runs ?? []).map((run) => ({
    at: Date.parse(run.startedAt || run.finishedAt || '') || 0,
    run,
  }))
  for (const member of allConversations) {
    if (member.causedBy?.parent === rootName) {
      entries.push({ at: Date.parse(member.created || '') || 0, member })
    }
  }
  entries.sort((a, b) => a.at - b.at)
  return entries
}

function counted(used: number | undefined, max: number | undefined): string {
  const count = used ?? 0
  return max ? `${count} of ${max}` : String(count)
}

function CoordinatorTimeline({
  rootDetail, allConversations, depth,
}: Readonly<{ rootDetail: ConversationDetail; allConversations: ConversationSummary[]; depth: number }>) {
  const entries = useMemo(() => buildIncidentTimeline(rootDetail, allConversations), [rootDetail, allConversations])
  const c = rootDetail.conversation
  const budget = c.budget
  const notEscalatedYet = !c.escalatedAt && c.phase !== 'Closed'
  return (
    <div style={{ flex: 1, minHeight: 0, display: 'flex', flexDirection: 'column' }}>
      <div style={{ padding: '12px 24px', overflowY: 'auto', flex: 1 }}>
        {budget && (
          <DescriptionList isCompact isHorizontal style={{ marginBottom: 12 }}>
            <DescriptionListGroup>
              <DescriptionListTerm>Agents invoked</DescriptionListTerm>
              <DescriptionListDescription>{counted(budget.agentsInvoked, budget.maxAgents)}</DescriptionListDescription>
            </DescriptionListGroup>
            <DescriptionListGroup>
              <DescriptionListTerm>Turns</DescriptionListTerm>
              <DescriptionListDescription>{counted(budget.turns, budget.maxTurns)}</DescriptionListDescription>
            </DescriptionListGroup>
            {budget.deadline && (
              <DescriptionListGroup>
                <DescriptionListTerm>Deadline</DescriptionListTerm>
                <DescriptionListDescription>{new Date(budget.deadline).toLocaleString()}</DescriptionListDescription>
              </DescriptionListGroup>
            )}
          </DescriptionList>
        )}
        {entries.length === 0 ? (
          <Empty title="Nothing has happened on this incident yet" />
        ) : (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
            {entries.map((e) => {
              if (e.run) return <RunEntry key={`run-${e.run.runId}`} run={e.run} />
              if (e.member) {
                return (
                  <MemberEntry
                    key={`member-${e.member.name}`}
                    member={e.member}
                    allConversations={allConversations}
                    depth={depth}
                  />
                )
              }
              return null
            })}
            {c.escalatedAt && (
              <div style={{ display: 'flex', alignItems: 'center', gap: 12, margin: '8px 0 0', color: 'var(--ao-accent)', fontSize: '0.8em', fontWeight: 700 }}>
                <span aria-hidden style={{ flex: 1, height: 1, background: 'var(--ao-accent)' }} />
                {`Escalated · ${new Date(c.escalatedAt).toLocaleString()}`}
                <span aria-hidden style={{ flex: 1, height: 1, background: 'var(--ao-accent)' }} />
              </div>
            )}
          </div>
        )}
      </div>
      <TimelineFooter c={c} notEscalatedYet={notEscalatedYet} rootDetail={rootDetail} />
    </div>
  )
}

function TimelineFooter({
  c, notEscalatedYet, rootDetail,
}: Readonly<{ c: ConversationSummary; notEscalatedYet: boolean; rootDetail: ConversationDetail }>) {
  if (notEscalatedYet) {
    return (
      <div style={{ padding: '12px 24px', borderTop: '1px solid var(--ao-border)' }}>
        <Alert variant="info" isInline title="Read-only — the coordinator has not asked for a person">
          This root has no bound channel until it escalates. Replies cannot be sent yet.
        </Alert>
      </div>
    )
  }
  if (c.phase === 'Closed') {
    return (
      <div style={{ padding: '12px 24px', borderTop: '1px solid var(--ao-border)' }}>
        <Alert
          variant="info"
          isInline
          title={c.escalatedAt ? 'This incident is closed' : 'Closed without escalating — nobody was notified'}
        >
          {c.closeReason && <PlainText>{c.closeReason}</PlainText>}
        </Alert>
      </div>
    )
  }
  return <ConversationThread detail={rootDetail} onSentOffline={() => undefined} />
}

function RunEntry({ run }: Readonly<{ run: Run }>) {
  const when = run.finishedAt || run.startedAt
  return (
    <div style={{ display: 'flex', alignItems: 'baseline', gap: '0.5em', padding: '0.35em 0' }}>
      <Label isCompact color="grey" icon={<Icon icon="aops:observe" />}>run</Label>
      <PlainText>{run.runId}</PlainText>
      <Label isCompact status={run.status === 'succeeded' ? 'success' : 'danger'}>
        <PlainText>{run.status}</PlainText>
      </Label>
      <small style={{ color: 'var(--ao-text-subtle)' }}>{when ? new Date(when).toLocaleString() : ''}</small>
    </div>
  )
}

function MemberBody({
  loading, error, data, allConversations, depth,
}: Readonly<{ loading: boolean; error: unknown; data: ConversationDetail | undefined; allConversations: ConversationSummary[]; depth: number }>) {
  if (loading && !data) return <Loading />
  if (error || !data) return <ErrorState title="Could not load this member">{String(error)}</ErrorState>
  if (data.conversation.coordinator) {
    return <CoordinatorTimeline rootDetail={data} allConversations={allConversations} depth={depth + 1} />
  }
  const runs = data.conversation.runs ?? []
  if (runs.length === 0) return <Empty title="No completed runs" />
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
      {runs.map((r) => <RunEntry key={r.runId} run={r} />)}
    </div>
  )
}

/** One member, expandable and collapsed by default — a nested card (console-conversation-tree). */
function MemberEntry({
  member, allConversations, depth,
}: Readonly<{ member: ConversationSummary; allConversations: ConversationSummary[]; depth: number }>) {
  const [expanded, setExpanded] = useState(false)
  const detail = useConversation(member.name, expanded)
  const isEscalation = Boolean(member.coordinator && member.phase === 'Closed' && member.escalatedAt)
  return (
    <details
      style={{
        marginLeft: depth * 20, borderLeft: depth ? '3px solid var(--ao-accent)' : '3px solid var(--ao-accent-soft)',
        paddingLeft: '0.75em', background: 'var(--ao-surface-alt)', borderRadius: 6, padding: '6px 0.75em',
      }}
      onToggle={(e) => setExpanded((e.target as HTMLDetailsElement).open)}
    >
      <summary style={{ cursor: 'pointer', display: 'flex', alignItems: 'baseline', gap: '0.5em', flexWrap: 'wrap' }}>
        <Icon icon="aops:agent" />
        <strong>{stripLeadingIcon(member.brief || member.title || member.name)}</strong>
        {member.causedBy?.entry && (
          <Label isCompact color="purple"><PlainText>{member.causedBy.entry}</PlainText></Label>
        )}
        <Label isCompact color={member.phase === 'Closed' ? 'grey' : 'blue'}>
          <PlainText>{member.phase}</PlainText>
        </Label>
        {isEscalation && <Label isCompact color="purple">escalation</Label>}
        {member.coordinator && <Label isCompact color="teal">coordinates its own</Label>}
      </summary>
      <div style={{ padding: '0.5em 0 0.75em 1.75em' }}>
        {expanded && (
          <MemberBody loading={detail.isLoading} error={detail.error} data={detail.data} allConversations={allConversations} depth={depth} />
        )}
        <div style={{ marginTop: '0.5em' }}>
          <Link to={`/conversations/${member.name}`}>Open full transcript →</Link>
        </div>
      </div>
    </details>
  )
}

/** A member opened DIRECTLY (not via its root): its own runs, read-only, since it binds no human channel. */
function MemberOwnBody({ conversation }: Readonly<{ conversation: ConversationSummary }>) {
  const detail = useConversation(conversation.name)
  if (detail.isLoading && !detail.data) return <Loading />
  if (detail.error || !detail.data) return <ErrorState title="Could not load this member">{String(detail.error)}</ErrorState>
  return (
    <div style={{ padding: '12px 24px', overflowY: 'auto', flex: 1 }}>
      <Alert variant="info" isInline title="A member holds no channel of its own">
        Its result reaches the root as an input and is never unread on its own. Open{' '}
        <Link to={`/conversations/${conversation.causedBy?.parent}`}>the incident</Link> for the full timeline.
      </Alert>
      <div style={{ marginTop: 12, display: 'flex', flexDirection: 'column', gap: 4 }}>
        {(detail.data.conversation.runs ?? []).map((r) => <RunEntry key={r.runId} run={r} />)}
      </div>
    </div>
  )
}

/** The ordinary thread: transcript + composer + quick chips. */
function ConversationThread({
  detail, onSentOffline,
}: Readonly<{ detail: NonNullable<ReturnType<typeof useConversation>['data']>; onSentOffline: () => void }>) {
  const session = useSession()
  const sources = useSources()
  const connected = useStream((s) => s.connected)
  const live = useStream((s) => s.events)
  const [text, setText] = useState('')
  const vocabulary = useVocabulary()
  const [dismissed, setDismissed] = useState(false)
  const [cursor, setCursor] = useState(0)
  const textRef = useRef<HTMLTextAreaElement>(null)
  const commands = dismissed ? null : matchEntries(text, vocabulary.data?.entries ?? [], 'thread')
  const activeCommand = commands ? Math.min(cursor, commands.length - 1) : 0
  const [error, setError] = useState<string>()
  const [busy, setBusy] = useState(false)

  function chooseCommand(entry: VocabularyEntry) {
    const next = '/' + entry.name
    setText(next)
    setDismissed(true)
    setCursor(0)
  }

  function onKeyDown(e: React.KeyboardEvent<HTMLTextAreaElement>) {
    if (e.key === 'Enter' && e.shiftKey) {
      e.preventDefault()
      if (!busy && text.trim()) void send()
      return
    }
    if (!commands) return
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      setCursor((c) => (c + 1) % commands.length)
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      setCursor((c) => (c - 1 + commands.length) % commands.length)
    } else if (e.key === 'Enter' || e.key === 'Tab') {
      e.preventDefault()
      chooseCommand(commands[activeCommand])
    } else if (e.key === 'Escape') {
      e.preventDefault()
      setDismissed(true)
    }
  }

  const canWrite = (session.data?.canWrite ?? false) && detail.conversation.joined && !detail.archived
  const canStart = Boolean(session.data?.canOriginate) && (sources.data?.sources ?? []).some((s) => s.wired)
  const messages = detail.transcript ?? []
  const pipelineIcon = vocabulary.data?.entries.find(
    (e) => e.kind === 'pipeline' && e.name === detail.conversation.pipeline,
  )?.icon
  const events = useMemo(
    () => mergeEvents(detail.events ?? [], eventsFor(detail.conversation.name, live)),
    [detail.events, detail.conversation.name, live],
  )
  const choices = messages.at(-1)?.choices

  async function send() {
    setBusy(true)
    setError(undefined)
    try {
      await api.send(detail.conversation.name, text)
      setText('')
      if (!connected) onSentOffline()
    } catch (e) {
      setError((e as ApiError).message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div style={{ flex: 1, minHeight: 0, display: 'flex', flexDirection: 'column' }}>
      {detail.joinHint && (
        <div style={{ padding: '8px 24px' }}>
          <Alert variant="info" isInline title="This conversation has no console thread">
            <p><PlainText>{detail.joinHint.reason}</PlainText></p>
            <ClipboardCopy isReadOnly>{detail.joinHint.fix}</ClipboardCopy>
          </Alert>
        </div>
      )}
      {detail.archived && (
        <div style={{ padding: '8px 24px' }}>
          <Alert variant="info" isInline title="This thread was archived">
            The conversation was closed. The transcript stays readable.
          </Alert>
        </div>
      )}
      <Timeline
        messages={messages}
        events={events}
        presence={Boolean(detail.conversation.presence)}
        readAt={detail.conversation.readAt}
        pipelineIcon={pipelineIcon}
        pipelineName={detail.conversation.pipeline}
      />
      {canWrite && (
        <div style={{ padding: '10px 24px 14px', borderTop: '1px solid var(--ao-border)', display: 'flex', flexDirection: 'column', gap: 8 }}>
          <QuickChips
            canWrite={canWrite}
            canStart={canStart}
            choices={choices}
            onInsertCommand={(cmd) => {
              setText(cmd)
              requestAnimationFrame(() => textRef.current?.focus())
            }}
          />
          <div style={{ position: 'relative' }}>
            {commands && (
              <ul
                aria-label="commands"
                data-testid="command-typeahead"
                style={{
                  position: 'absolute', bottom: '100%', left: 0, right: 0, zIndex: 200, marginBottom: 4,
                  maxHeight: '40vh', overflowY: 'auto', background: 'var(--ao-surface)', border: '1px solid var(--ao-border)',
                  borderRadius: 8, listStyle: 'none', padding: 4,
                }}
              >
                {commands.map((cmd, i) => (
                  <li key={cmd.name}>
                    <button
                      type="button"
                      onClick={() => chooseCommand(cmd)}
                      style={{
                        all: 'unset', display: 'block', width: '100%', padding: '6px 10px', cursor: 'pointer',
                        background: i === activeCommand ? 'var(--ao-brand-soft)' : 'transparent',
                      }}
                    >
                      <Icon icon={cmd.icon} /> /{cmd.name}
                    </button>
                  </li>
                ))}
              </ul>
            )}
            <TextArea
              aria-label="message"
              ref={textRef}
              value={text}
              onChange={(_e, v) => {
                setText(v)
                setDismissed(false)
                setCursor(0)
              }}
              onKeyDown={onKeyDown}
              rows={3}
              placeholder="Reply to the agent…"
            />
          </div>
          {error && <Alert variant="danger" isInline title={error} />}
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', flexWrap: 'wrap', gap: 12 }}>
            <ComposerHint shortcuts={[{ keys: ['/'], does: 'commands' }, { keys: ['Shift', 'Enter'], does: 'send' }]} />
            <Button onClick={send} isDisabled={busy || !text.trim()}>Send</Button>
          </div>
        </div>
      )}
    </div>
  )
}

function RunTimeline({ detail }: Readonly<{ detail: NonNullable<ReturnType<typeof useConversation>['data']> }>) {
  const runs = detail.conversation.runs ?? []
  return (
    <div style={{ padding: 16, overflowY: 'auto', flex: 1 }}>
      <Card>
        <CardTitle>Runs</CardTitle>
        <CardBody>
          {runs.length === 0 ? (
            <Empty title="No completed runs" />
          ) : (
            <Table variant="compact" aria-label="runs">
              <Thead>
                <Tr><Th>Run</Th><Th>Status</Th><Th>Exit</Th><Th>Finished</Th><Th>Result</Th></Tr>
              </Thead>
              <Tbody>
                {runs.map((r) => (
                  <Tr key={r.runId}>
                    <Td dataLabel="Run"><PlainText>{r.runId}</PlainText></Td>
                    <Td dataLabel="Status">
                      <Label status={r.status === 'succeeded' ? 'success' : 'danger'}><PlainText>{r.status}</PlainText></Label>
                    </Td>
                    <Td dataLabel="Exit">{r.exitCode ?? '—'}</Td>
                    <Td dataLabel="Finished">{r.finishedAt ? new Date(r.finishedAt).toLocaleString() : '—'}</Td>
                    <Td dataLabel="Result">{r.result ? <RawText>{r.result}</RawText> : <small>—</small>}</Td>
                  </Tr>
                ))}
              </Tbody>
            </Table>
          )}
        </CardBody>
      </Card>
      <Card style={{ marginTop: 12 }}>
        <CardTitle>Bindings and runtime</CardTitle>
        <CardBody>
          <DescriptionList isCompact>
            <DescriptionListGroup>
              <DescriptionListTerm>Inputs queued</DescriptionListTerm>
              <DescriptionListDescription>{detail.conversation.queued}</DescriptionListDescription>
            </DescriptionListGroup>
            <DescriptionListGroup>
              <DescriptionListTerm>Runtime pod</DescriptionListTerm>
              <DescriptionListDescription><PlainText>{detail.conversation.runtimePod || '—'}</PlainText></DescriptionListDescription>
            </DescriptionListGroup>
          </DescriptionList>
        </CardBody>
      </Card>
    </div>
  )
}

function ConversationGraphTab({ name }: Readonly<{ name: string }>) {
  const { data, isLoading, error } = useConversationGraph(name)
  const windowSeconds = useDisplay((s) => s.windowSeconds)
  const topology = useTopology(windowSeconds)
  const live = useStream((s) => s.events)
  const events = useMemo(() => mergeEvents(data?.events ?? [], live), [data?.events, live])
  if ((isLoading && !data) || (topology.isLoading && !topology.data)) return <Loading />
  if (error || !data) return <ErrorState title="Could not build the graph">{String(error)}</ErrorState>
  if (topology.error || !topology.data) return <ErrorState title="Could not load the topology">{String(topology.error)}</ErrorState>
  const oldest = topology.data.oldestEvent
  return (
    <div style={{ padding: 16, overflow: 'hidden', flex: 1, display: 'flex', flexDirection: 'column' }}>
      {data.diverged && (
        <Alert variant="info" isInline title="The pipeline has been re-wired since this ran" style={{ marginBottom: 12 }}>
          This conversation materialized different bindings from <PipelineName name={data.pipeline ?? 'the pipeline'} />'s current wiring.
        </Alert>
      )}
      <div style={{ flex: 1, minHeight: 0 }}>
        <Graph
          topology={topology.data.topology}
          events={events}
          bufferStart={oldest ? Date.parse(oldest) : undefined}
          conversation={name}
          emptyMessage="This conversation involved no elements the Display panel is showing."
        />
      </div>
    </div>
  )
}

function endpoint(ref: { kind: string; name: string } | undefined): string {
  return ref ? `${ref.kind}/${ref.name}` : '∅'
}

function Sequence({ events }: Readonly<{ events: ActivityEvent[] }>) {
  if (events.length === 0) {
    return (
      <Empty title="No recorded hops for this conversation">
        The manager's activity buffer is bounded — hops older than it are gone, and <code>status.runs[]</code> stays the durable record.
      </Empty>
    )
  }
  const times = events.map((e) => new Date(e.ts).getTime()).filter((t) => !Number.isNaN(t))
  const start = times.length ? Math.min(...times) : 0
  return (
    <div style={{ padding: 16, overflowY: 'auto', flex: 1 }}>
      <Card>
        <CardBody>
          <Table variant="compact" aria-label="sequence">
            <Thead><Tr><Th>Hop</Th><Th>From → To</Th><Th>At</Th><Th>Latency</Th></Tr></Thead>
            <Tbody>
              {events.map((e) => {
                const at = new Date(e.ts).getTime()
                return (
                  <Tr key={e.cursor}>
                    <Td dataLabel="Hop">
                      <PlainText>{e.kind}</PlainText>
                      {e.status === 'error' && <Label status="danger">error</Label>}
                    </Td>
                    <Td dataLabel="From → To">
                      <small><PlainText>{`${endpoint(e.from)} → ${endpoint(e.to)}`}</PlainText></small>
                    </Td>
                    <Td dataLabel="At">{`+${((at - start) / 1000).toFixed(1)}s`}</Td>
                    <Td dataLabel="Latency">{e.latencyMs ? `${(e.latencyMs / 1000).toFixed(2)}s` : '—'}</Td>
                  </Tr>
                )
              })}
            </Tbody>
          </Table>
        </CardBody>
      </Card>
    </div>
  )
}
