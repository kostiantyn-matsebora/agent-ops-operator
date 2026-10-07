import { useEffect, useMemo, useRef, useState } from 'react'
import {
  Alert, Button, Card, CardBody, CardTitle, Checkbox, ClipboardCopy, DescriptionList,
  DescriptionListDescription, DescriptionListGroup, DescriptionListTerm, Label,
  Modal, ModalBody, ModalFooter, ModalHeader, TextArea, Tooltip,
} from '@patternfly/react-core'
import { Table, Tbody, Td, Th, Thead, Tr } from '@patternfly/react-table'
import { Link } from 'react-router-dom'
import { Empty, ErrorState, Loading } from '../../components/States'
import {
  useConversation, useConversationGraph, useConversations, useMarkRead, useSession,
  useTopology, useVocabulary,
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
import type { TimelineExtraItem } from './Timeline'
import { QuickChips } from './QuickChips'
import { formatBytes, formatDuration } from './format'
import { isCloseCommand, skipCloseConfirm, writeSkipCloseConfirm } from './closeConfirm'
import { Blocks } from '../../components/Blocks'
import { parse } from '../../api/blocks'
import type {
  ActivityEvent, ConversationDetail, ConversationSummary, Message, RecordedInput, Run, RunToolCall, RunTurn,
  VocabularyEntry,
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
        {/* A root's own ancestor chain is always empty — see `MemberCrumb` —
            so showing it there would be one static label pointing at
            nothing. Only a member, which HAS ancestors, gets the crumb. */}
        {isMember && <MemberCrumb conversation={c} />}
        <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
          {onBack && (
            <Button variant="plain" aria-label="back to the list" onClick={onBack}>
              ←
            </Button>
          )}
          {/* `display: block` + the truncation trio: a bare flex-child <span>
              still wraps at word boundaries when too narrow for its content
              (measured live at 375px — a long title wrapped one WORD per
              line, seven lines tall), since text-overflow only engages on a
              block-level box with a constrained width. */}
          <span
            title={title}
            style={{
              fontSize: '1.1em', fontWeight: 700, flex: 1, minWidth: 0,
              display: 'block', overflow: 'hidden', whiteSpace: 'nowrap', textOverflow: 'ellipsis',
            }}
          >
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
          {/* Runs shows a member its OWN turns/tool calls/stop reason — the
              same `RunTimeline` a root gets, reading the same `detail.
              conversation.runs` field, which the detail endpoint populates
              for a member exactly as it does for a root (item 16 QA: the
              backend always recorded this, the tab was the gap). Graph and
              Sequence stay root-only: both read the CONVERSATION's own
              topology/activity, which for a member is a single node a click
              into its parent's view already reaches with more context. */}
          <ViewButton active={view === 'runs'} onClick={() => setView('runs')}>Runs</ViewButton>
          {!isMember && (
            <>
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
  if (isRoot) return <CoordinatorBody conversation={conversation} detail={detail} onSentOffline={onSentOffline} />
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

/**
 * The parent chain, the uncaused root through every parent ABOVE this
 * conversation (console-conversation-tree). There is no "incident" entity in
 * this domain — the leading label names the Coordinator the chain belongs
 * to (the uncaused root's own `coordinator` field), never an invented noun.
 */
function MemberCrumb({ conversation }: Readonly<{ conversation: ConversationSummary }>) {
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
  // The conversation itself is never part of its OWN breadcrumb — its title
  // is already the pane's heading, one line below this. Showing it twice is
  // what made the crumb read as a second heading rather than a trail.
  const ancestors = chain.slice(0, -1)
  const coordinatorName = chain[0]?.coordinator
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 6, flexWrap: 'wrap', fontSize: '0.85em', color: 'var(--ao-text-subtle)' }}>
      <Icon icon="aops:agent" />
      {coordinatorName && (
        <strong style={{ color: 'var(--ao-accent)' }}>
          <PlainText>{coordinatorName}</PlainText>
        </strong>
      )}
      {ancestors.map((step, i) => (
        <span key={step.name} style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}>
          <span aria-hidden>›</span>
          <Link
            to={`/conversations/${step.name}`}
            style={i === ancestors.length - 1 ? { fontWeight: 700, color: 'var(--ao-text)' } : undefined}
          >
            <PlainText>{step.causedBy?.entry ?? stripLeadingIcon(step.title || step.name)}</PlainText>
          </Link>
        </span>
      ))}
    </div>
  )
}

/** Every input any of `runs` recorded a non-empty text for, oldest first — the
 * durable half of a conversation's questions (`Run.inputs`, invariants.md: "A
 * CONVERSATION'S MESSAGES ARE KUBERNETES-API STATE"). */
function recordedInputs(runs: Run[] | undefined): RecordedInput[] {
  return (runs ?? []).flatMap((r) => (r.inputs ?? []).filter((i) => i.text))
}

/** The text a conversation was actually given — its first recorded input.
 * For a member this IS its task: `invoke` addresses no human channel, so this
 * is the only place that text lives (see `RecordedInput.surface`). */
function firstTaskInput(runs: Run[] | undefined): RecordedInput | undefined {
  return recordedInputs(runs)[0]
}

/** The most recent run that actually answered, oldest-finished-last. */
function lastResult(runs: Run[] | undefined): { text: string; at?: string } | undefined {
  const withResult = (runs ?? []).filter((r) => r.result)
  const r = withResult.at(-1)
  return r ? { text: r.result!, at: r.finishedAt || r.startedAt } : undefined
}

/** `text`, parsed and rendered exactly as an agent's own message would be
 * (`Timeline`'s `agentText` branch) — the same characters, the same renderer,
 * whether this is a coordinator's task handoff or a member's answer. */
function AgentText({ text }: Readonly<{ text: string }>) {
  return <Blocks blocks={parse(text)} />
}

/**
 * One member a coordinator root invoked, inline in its own transcript —
 * collapsed by default (console-conversation-tree), expanding to the task it
 * was actually given and what it answered, each a full-width block at
 * ordinary message type scale rather than a cramped side-card (the
 * transcript-concept mockup).
 */
function MemberInvocation({ member }: Readonly<{ member: ConversationSummary }>) {
  const [expanded, setExpanded] = useState(false)
  const detail = useConversation(member.name, expanded)
  const entry = member.causedBy?.entry ?? member.name
  const done = member.phase === 'Closed'
  return (
    <details
      style={{
        background: 'var(--ao-surface-alt)', border: '1px solid var(--ao-border)', borderRadius: 8,
        padding: '8px 14px',
      }}
      onToggle={(e) => setExpanded((e.target as HTMLDetailsElement).open)}
    >
      <summary style={{ cursor: 'pointer', display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
        <Icon icon="aops:agent" />
        <span>
          {'↳ invoked '}
          <strong><PlainText>{entry}</PlainText></strong>
          {' as '}
          <code><PlainText>{member.name}</PlainText></code>
        </span>
        <Label isCompact color={done ? 'grey' : 'blue'}>
          <PlainText>{done ? 'done' : member.phase || 'working'}</PlainText>
        </Label>
        {member.errored && <Label isCompact color="red">run failed</Label>}
      </summary>
      <div style={{ padding: '10px 2px 2px', display: 'flex', flexDirection: 'column', gap: 12 }}>
        {expanded && <ExpandedInvocation detail={detail} entry={entry} />}
        <div>
          <Link to={`/conversations/${member.name}`}>Open {member.name} on its own →</Link>
        </div>
      </div>
    </details>
  )
}

/** The expanded half of one `MemberInvocation` — its own loading/error states,
 * kept apart so the collapsed summary above never pays for them. */
function ExpandedInvocation({ detail, entry }: Readonly<{ detail: ReturnType<typeof useConversation>; entry: string }>) {
  if (detail.isLoading && !detail.data) return <Loading />
  if (detail.error || !detail.data) return <ErrorState title="Could not load this member">{String(detail.error)}</ErrorState>
  return <MemberInvocationExchange detail={detail.data} entry={entry} />
}

/** The two labelled sub-blocks a member invocation expands to. */
function MemberInvocationExchange({ detail, entry }: Readonly<{ detail: ConversationDetail; entry: string }>) {
  const task = firstTaskInput(detail.conversation.runs)
  const result = lastResult(detail.conversation.runs)
  return (
    <>
      <div>
        <div style={{ fontSize: '0.78em', fontWeight: 700, textTransform: 'uppercase', letterSpacing: '0.04em', color: 'var(--ao-text-subtle)', marginBottom: 4 }}>
          Task sent
        </div>
        {task?.text ? <AgentText text={task.text} /> : <small style={{ color: 'var(--ao-text-subtle)' }}>No recorded input</small>}
      </div>
      <div>
        <div style={{ fontSize: '0.78em', fontWeight: 700, textTransform: 'uppercase', letterSpacing: '0.04em', color: 'var(--ao-text-subtle)', marginBottom: 4 }}>
          <PlainText>{entry}</PlainText> responded
        </div>
        {result?.text ? <AgentText text={result.text} /> : <small style={{ color: 'var(--ao-text-subtle)' }}>No result yet</small>}
      </div>
    </>
  )
}

/**
 * The coordinator's OWN run that was executing when it invoked a member.
 * `invoke` is a synchronous tool call made mid-run, and runs are strictly
 * serial (invariants.md: "Strictly serial per conversation"), so at most one
 * of the coordinator's own runs can have been in flight at the member's
 * creation instant.
 */
function invokingRun(runs: Run[] | undefined, memberCreatedAt: number): Run | undefined {
  if (!memberCreatedAt) return undefined
  return (runs ?? []).find((r) => {
    const start = Date.parse(r.startedAt || '') || 0
    if (!start || memberCreatedAt < start) return false
    if (!r.finishedAt) return true // still running when this member was created
    return memberCreatedAt <= (Date.parse(r.finishedAt) || 0)
  })
}

/**
 * Where a member's invoke card belongs in transcript time order (item #15 QA,
 * bug 1). `member.created` is when the MEMBER OBJECT was created — which
 * happens mid-run, as soon as `invoke` is called — well before the
 * coordinator's OWN reasoning for that same run is recorded
 * (`status.runs[].result`, written only once the whole run finishes).
 * Sorting the card by its own creation time therefore always placed it
 * BEFORE the reasoning that explains it, reversing the mockup's
 * decide-then-act order: measured live, a member created 90 seconds into a
 * two-minute run sorted ahead of that run's own reasoning text by over a
 * minute.
 *
 * A first attempt anchored the card to `run.finishedAt` on the theory that it
 * is "the same clock the reasoning text sorts by" — it is NOT. Measured
 * live: `run.finishedAt` (the manager's own recorded stamp) and the
 * reasoning message's own `at` (timestamped by the console's live buffer at
 * RECEIPT, which this conversation's transcript keeps using even on a fresh
 * reload, since the buffer lives in the console's own process, not the
 * browser) differed by two real seconds on an otherwise ordinary run —
 * `run.finishedAt` sorted the invoke card BEFORE the very reasoning message
 * it was supposed to tie with.
 *
 * The fix anchors to the reasoning message's OWN rendered timestamp
 * directly, not a second computation of it: the first `agent`-kind
 * transcript message at or after this run's `startedAt` is that run's own
 * result, by the strictly-serial-runs invariant above, so there is exactly
 * one candidate and no guessing which one it is.
 *
 * Falls back to the run's own `finishedAt`/`startedAt`, then to the member's
 * own creation time, when no run's window covers it — `status.runs[]` keeps
 * only the last 10 (api/v1alpha1/conversation_types.go), so an old member's
 * invoking run may have scrolled out of both the run list and the
 * transcript's live buffer alike.
 */
function invokeCardAt(runs: Run[] | undefined, member: ConversationSummary, transcript: Message[] | null | undefined): number {
  const createdAt = Date.parse(member.created || '') || 0
  const run = invokingRun(runs, createdAt)
  if (!run) return createdAt
  const startedAt = Date.parse(run.startedAt || '') || 0
  const fallback = Date.parse(run.finishedAt || run.startedAt || '') || createdAt
  if (!startedAt) return fallback
  const reasoningAt = (transcript ?? [])
    .filter((m) => m.kind === 'agent')
    .map((m) => Date.parse(m.at) || 0)
    .filter((at) => at >= startedAt)
    .sort((a, b) => a - b)[0]
  return reasoningAt ?? fallback
}

/**
 * A Coordinator root's own transcript — the SAME `ConversationThread` an
 * ordinary pipeline conversation gets (its own messages, composer and
 * QuickChips, never a separate structural view by default), with each direct
 * member it invoked woven in as a collapsed, expandable exchange at the time
 * it was invoked (console-conversation-tree, the transcript-concept mockup).
 *
 * No `channelBound` override here (item 6): `console-conversation-tree`'s
 * "the composer follows channel binding, not escalation" is satisfied by
 * `detail.conversation.joined` ALONE, now that `coordination-escalation`
 * binds the Coordinator's `channelRefs` into the root unconditionally at
 * creation (`platform/manager/internal/chat/claimant.go`'s
 * `coordinatorClaimant.BoundChannelRefs`) — the same materialization a
 * Pipeline's own channels get. `joined` already means exactly "the console
 * channel holds a thread binding", so it goes live the moment that binding
 * is reconciled rather than waiting for `escalate`. A root predating that
 * binding, or one whose Coordinator never named the console at all, stays
 * read-only — spec's other permitted case, not a bug.
 */
function CoordinatorBody({
  conversation, detail, onSentOffline,
}: Readonly<{
  conversation: ConversationSummary
  detail: NonNullable<ReturnType<typeof useConversation>['data']>
  onSentOffline: () => void
}>) {
  const membersParams = useMemo(() => new URLSearchParams({ limit: '200' }), [])
  const members = useConversations(membersParams)
  const budget = conversation.budget
  if (members.isLoading && !members.data) return <Loading />
  if (members.error || !members.data) return <ErrorState title="Could not load member conversations">{String(members.error)}</ErrorState>

  const directMembers = members.data.items.filter((m) => m.causedBy?.parent === conversation.name)
  const extraItems = directMembers.map((m) => ({
    key: `invoke-${m.name}`,
    at: invokeCardAt(conversation.runs, m, detail.transcript),
    node: <MemberInvocation member={m} />,
  }))
  if (conversation.escalatedAt) {
    extraItems.push({
      key: 'escalated',
      at: Date.parse(conversation.escalatedAt) || 0,
      node: (
        <div style={{ display: 'flex', alignItems: 'center', gap: 12, color: 'var(--ao-accent)', fontSize: '0.8em', fontWeight: 700 }}>
          <span aria-hidden style={{ flex: 1, height: 1, background: 'var(--ao-accent)' }} />
          {`Escalated · ${new Date(conversation.escalatedAt).toLocaleString()}`}
          <span aria-hidden style={{ flex: 1, height: 1, background: 'var(--ao-accent)' }} />
        </div>
      ),
    })
  }

  return (
    <div style={{ flex: 1, minHeight: 0, display: 'flex', flexDirection: 'column' }}>
      {budget && (
        <div style={{ padding: '8px 24px 0' }}>
          <DescriptionList isCompact isHorizontal>
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
        </div>
      )}
      <ConversationThread detail={detail} onSentOffline={onSentOffline} extraItems={extraItems} />
    </div>
  )
}

function counted(used: number | undefined, max: number | undefined): string {
  const count = used ?? 0
  return max ? `${count} of ${max}` : String(count)
}

/**
 * A member opened DIRECTLY (not through its root): the SAME kind of real
 * transcript — the task it was given, then its result, ordinary-looking
 * messages — just with no composer, since it binds no human channel of its
 * own (invariants.md: "A CAUSED CONVERSATION BINDS NO HUMAN CHANNEL").
 */
function MemberOwnBody({ conversation }: Readonly<{ conversation: ConversationSummary }>) {
  const detail = useConversation(conversation.name)
  if (detail.isLoading && !detail.data) return <Loading />
  if (detail.error || !detail.data) return <ErrorState title="Could not load this member">{String(detail.error)}</ErrorState>
  const runs = detail.data.conversation.runs
  const task = firstTaskInput(runs)
  const result = lastResult(runs)
  const messages: Message[] = []
  if (task?.text) {
    messages.push({
      id: 'task', thread: conversation.name, kind: 'signal', text: task.text,
      at: task.receivedAt || conversation.created || '',
    })
  }
  if (result?.text) {
    messages.push({ id: 'result', thread: conversation.name, kind: 'agent', text: result.text, at: result.at || '' })
  }
  return (
    <div style={{ flex: 1, minHeight: 0, display: 'flex', flexDirection: 'column' }}>
      <div style={{ padding: '8px 24px 0' }}>
        <Alert variant="info" isInline title="A member holds no channel of its own">
          Its result reaches its parent as an input. This transcript is read-only.
        </Alert>
      </div>
      <Timeline messages={messages} events={[]} presence={Boolean(conversation.presence)} />
    </div>
  )
}

/** The ordinary thread: transcript + composer + quick chips — reused for a
 * plain pipeline conversation AND a Coordinator root's own transcript. */
function ConversationThread({
  detail, onSentOffline, extraItems,
}: Readonly<{
  detail: NonNullable<ReturnType<typeof useConversation>['data']>
  onSentOffline: () => void
  /** Rows to interleave into the transcript by time — a root's member
   * invocations. Absent for an ordinary pipeline conversation. */
  extraItems?: TimelineExtraItem[]
}>) {
  const session = useSession()
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
  // `/close` gets a confirm dialog (item 22) — `/exit` never does, since it
  // only releases the runtime and nothing about the conversation is lost.
  const [confirmingClose, setConfirmingClose] = useState(false)

  function chooseCommand(entry: VocabularyEntry) {
    const next = '/' + entry.name
    setText(next)
    setDismissed(true)
    setCursor(0)
  }

  function onKeyDown(e: React.KeyboardEvent<HTMLTextAreaElement>) {
    if (e.key === 'Enter' && e.shiftKey) {
      e.preventDefault()
      if (!busy && text.trim()) void send(text)
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
  const messages = detail.transcript ?? []
  const pipelineIcon = vocabulary.data?.entries.find(
    (e) => e.kind === 'pipeline' && e.name === detail.conversation.pipeline,
  )?.icon
  const events = useMemo(
    () => mergeEvents(detail.events ?? [], eventsFor(detail.conversation.name, live)),
    [detail.events, detail.conversation.name, live],
  )
  const choices = messages.at(-1)?.choices

  async function postMessage(toSend: string) {
    setBusy(true)
    setError(undefined)
    try {
      await api.send(detail.conversation.name, toSend)
      setText('')
      if (!connected) onSentOffline()
    } catch (e) {
      setError((e as ApiError).message)
    } finally {
      setBusy(false)
    }
  }

  // `/close` can be destructive once the retention window has run
  // (invariants.md: "`/exit` RELEASES THE RUNTIME — `/close` ENDS THE
  // CONVERSATION"), so it gets an ordinary confirm dialog first — unless the
  // operator already opted out. `/exit` and every ordinary reply send at once.
  //
  // `toSend` is explicit rather than read off `text`, because a thread
  // command chip (QuickChips' `onRunCommand`) runs `/exit`/`/close`
  // DIRECTLY — never by writing into the composer for the user to send
  // themselves — and must not pick up whatever unrelated draft happens to be
  // sitting in the box at the time.
  const [pendingSend, setPendingSend] = useState('')

  async function send(toSend: string) {
    if (isCloseCommand(toSend) && !skipCloseConfirm()) {
      setPendingSend(toSend)
      setConfirmingClose(true)
      return
    }
    await postMessage(toSend)
  }

  function confirmClose(dontAskAgain: boolean) {
    writeSkipCloseConfirm(dontAskAgain)
    setConfirmingClose(false)
    void postMessage(pendingSend)
  }

  return (
    <div style={{ flex: 1, minHeight: 0, display: 'flex', flexDirection: 'column' }}>
      {confirmingClose && (
        <CloseConfirmModal onConfirm={confirmClose} onCancel={() => setConfirmingClose(false)} />
      )}
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
        extraItems={extraItems}
      />
      {canWrite && (
        <div style={{ padding: '10px 24px 14px', borderTop: '1px solid var(--ao-border)', display: 'flex', flexDirection: 'column', gap: 8 }}>
          <QuickChips
            canWrite={canWrite}
            choices={choices}
            onRunCommand={(cmd) => void send(cmd)}
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
            <Button onClick={() => void send(text)} isDisabled={busy || !text.trim()}>Send</Button>
          </div>
        </div>
      )}
    </div>
  )
}

/**
 * An ordinary confirm dialog for `/close` (item 22) — never `/exit`, which is
 * fully recoverable and needs no gate. The "don't ask again" box is a
 * per-browser convenience (`closeConfirm.ts`), re-defaulted unchecked every
 * time the dialog opens: an opt-in that remembers itself is not one — the
 * same rule `CloseSelectedModal`'s "include working" switch follows.
 */
function CloseConfirmModal({
  onConfirm, onCancel,
}: Readonly<{ onConfirm: (dontAskAgain: boolean) => void; onCancel: () => void }>) {
  const [dontAskAgain, setDontAskAgain] = useState(false)
  return (
    <Modal isOpen onClose={onCancel} variant="small" aria-label="confirm close" data-testid="close-confirm-modal">
      <ModalHeader title="Close this conversation?" />
      <ModalBody>
        <p>
          The agent says goodbye and the thread is archived. The conversation itself stays — its
          answers and its workspace are kept — and it can be reopened.
        </p>
        <Checkbox
          id="close-confirm-skip"
          label="Don't ask again"
          isChecked={dontAskAgain}
          onChange={(_e, v) => setDontAskAgain(v)}
        />
      </ModalBody>
      <ModalFooter>
        <Button variant="danger" onClick={() => onConfirm(dontAskAgain)} data-testid="close-confirm-ok">
          Close
        </Button>
        <Button variant="link" onClick={onCancel}>Cancel</Button>
      </ModalFooter>
    </Modal>
  )
}

function SubTableLabel({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <div style={{ fontSize: '0.78em', fontWeight: 700, textTransform: 'uppercase', letterSpacing: '0.04em', color: 'var(--ao-text-subtle)' }}>
      {children}
    </div>
  )
}

/** Pairs each item with a content key, suffixed by its occurrence so repeats stay unique. */
function withKeys<T>(items: T[], keyOf: (item: T) => string): Array<[string, T]> {
  const seen = new Map<string, number>()
  return items.map((item) => {
    const base = keyOf(item)
    const n = (seen.get(base) ?? 0) + 1
    seen.set(base, n)
    return [`${base}#${n}`, item]
  })
}

function TurnsTable({ turns }: Readonly<{ turns: RunTurn[] }>) {
  return (
    <div>
      <SubTableLabel>{`turns — ${turns.length} model call${turns.length === 1 ? '' : 's'}`}</SubTableLabel>
      <Table variant="compact" aria-label="turns">
        <Thead>
          <Tr><Th>Model</Th><Th>Tokens in</Th><Th>Tokens out</Th><Th>Stop reason</Th></Tr>
        </Thead>
        <Tbody>
          {withKeys(turns, (t) => `${t.model}:${t.tokensIn}:${t.tokensOut}:${t.stopReason}`).map(([key, t]) => (
            <Tr key={key}>
              <Td dataLabel="Model"><PlainText>{t.model || '—'}</PlainText></Td>
              <Td dataLabel="Tokens in">
                {t.tokensIn ?? '—'}
                {t.cacheReadTokens !== undefined && (
                  <span style={{ color: 'var(--ao-text-subtle)' }}> ({t.cacheReadTokens} cached)</span>
                )}
              </Td>
              <Td dataLabel="Tokens out">{t.tokensOut ?? '—'}</Td>
              <Td dataLabel="Stop reason"><PlainText>{t.stopReason || '—'}</PlainText></Td>
            </Tr>
          ))}
        </Tbody>
      </Table>
    </div>
  )
}

function ToolCallsTable({ calls }: Readonly<{ calls: RunToolCall[] }>) {
  return (
    <div>
      <SubTableLabel>{`tool calls — ${calls.length}`}</SubTableLabel>
      <Table variant="compact" aria-label="tool calls">
        <Thead>
          <Tr><Th>Tool</Th><Th>Server</Th><Th>Duration</Th><Th>Result size</Th></Tr>
        </Thead>
        <Tbody>
          {withKeys(calls, (c) => `${c.server}:${c.tool}:${c.durationMs}:${c.resultBytes}`).map(([key, c]) => (
            <Tr key={key}>
              <Td dataLabel="Tool"><PlainText>{c.tool || '—'}</PlainText></Td>
              <Td dataLabel="Server">
                {/* Empty means a built-in tool, never missing data — say so rather
                    than leave a blank cell. */}
                <PlainText>{c.server || 'built-in'}</PlainText>
              </Td>
              <Td dataLabel="Duration">{formatDuration(c.durationMs)}</Td>
              <Td dataLabel="Result size">{formatBytes(c.resultBytes)}</Td>
            </Tr>
          ))}
        </Tbody>
      </Table>
    </div>
  )
}

/**
 * One run: its facts, its raw result text kept exactly as it was before this
 * change (QA finding: "keep the raw input/output exactly as it is today —
 * that's what makes investigating a prompt-engineering issue possible"),
 * then the turns and tool-calls tables the runtime may additionally report.
 *
 * Each run is its own full-width block rather than a row of a shared table:
 * a `turns`/`toolCalls` table needs more columns than a "Result" table
 * column can give it without clipping, and a Card's body is the width the
 * mockup's layout assumes. Either sub-section is omitted entirely when its
 * array is absent or empty, never rendered as an empty table.
 */
function RunEntry({ run }: Readonly<{ run: Run }>) {
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
      <DescriptionList isCompact isHorizontal>
        <DescriptionListGroup>
          <DescriptionListTerm>Run</DescriptionListTerm>
          <DescriptionListDescription><PlainText>{run.runId}</PlainText></DescriptionListDescription>
        </DescriptionListGroup>
        <DescriptionListGroup>
          <DescriptionListTerm>Status</DescriptionListTerm>
          <DescriptionListDescription>
            <Label status={run.status === 'succeeded' ? 'success' : 'danger'}><PlainText>{run.status}</PlainText></Label>
          </DescriptionListDescription>
        </DescriptionListGroup>
        <DescriptionListGroup>
          <DescriptionListTerm>Exit</DescriptionListTerm>
          <DescriptionListDescription>{run.exitCode ?? '—'}</DescriptionListDescription>
        </DescriptionListGroup>
        <DescriptionListGroup>
          <DescriptionListTerm>Finished</DescriptionListTerm>
          <DescriptionListDescription>{run.finishedAt ? new Date(run.finishedAt).toLocaleString() : '—'}</DescriptionListDescription>
        </DescriptionListGroup>
      </DescriptionList>
      {run.result ? <RawText>{run.result}</RawText> : <small>—</small>}
      {run.turns && run.turns.length > 0 && <TurnsTable turns={run.turns} />}
      {run.toolCalls && run.toolCalls.length > 0 && <ToolCallsTable calls={run.toolCalls} />}
    </div>
  )
}

function RunTimeline({ detail }: Readonly<{ detail: NonNullable<ReturnType<typeof useConversation>['data']> }>) {
  const runs = detail.conversation.runs ?? []
  return (
    <div data-testid="runs-view" style={{ padding: 16, overflowY: 'auto', flex: 1 }}>
      <Card>
        <CardTitle>Runs</CardTitle>
        <CardBody>
          {runs.length === 0 ? (
            <Empty title="No completed runs" />
          ) : (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 20 }}>
              {runs.map((r, i) => (
                <div key={r.runId} style={i > 0 ? { borderTop: '1px solid var(--ao-border)', paddingTop: 20 } : undefined}>
                  <RunEntry run={r} />
                </div>
              ))}
            </div>
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
