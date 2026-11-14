import { useEffect, useMemo, useRef, useState } from 'react'
import {
  Alert, Button, Card, CardBody, CardTitle, Checkbox, ClipboardCopy, DescriptionList,
  DescriptionListDescription, DescriptionListGroup, DescriptionListTerm, Label,
  Modal, ModalBody, ModalFooter, ModalHeader, TextArea, Tooltip,
} from '@patternfly/react-core'
import { Table, Tbody, Td, Th, Thead, Tr } from '@patternfly/react-table'
import { Link } from 'react-router-dom'
import { Empty, ErrorState, Loading } from '../../components/States'
import { useConversationGraph, useTopology, useVocabulary } from '../../../../api/hooks'
import { useStream } from '../../../../api/stream'
import { mergeEvents } from '../../../../graph/hops'
import { PlainText, RawText } from '../../components/Text'
import { Graph } from '../../../../graph/Graph'
import { useDisplay } from '../../../../graph/display'
import { PipelineName } from '../../components/PipelineName'
import { ComposerHint } from '../../components/ComposerHint'
import { Icon } from '../../components/Icon'
import { Yaml } from '../../components/Yaml'
import { MetadataCard } from '../../components/Metadata'
import { matchEntries } from '../NewConversation'
import { Timeline } from './Timeline'
import type { TimelineExtraItem } from './Timeline'
import { QuickChips } from './QuickChips'
import { formatBytes, formatDuration } from './format'
import { isCloseCommand, skipCloseConfirm, writeSkipCloseConfirm } from './closeConfirm'
import { Blocks } from '../../components/Blocks'
import { parse } from '../../../../api/blocks'
import type {
  ActivityEvent, ConversationSummary, Run, RunToolCall, RunTurn, VocabularyEntry,
} from '../../../../api/types'
import {
  chainStepLabel, memberTranscript, useMemberChain, useMemberExchange, useThread, useThreadMembers,
  type Thread,
} from '../../../../model/thread'
import { sendText } from '../../../../model/verbs'
import { useCommands } from '../../../../model/commands'
import { TESTID, testid } from '../../../../model/testids'

// `console-chat-layout` (secondary views) + `console-conversation-tree` (the
// incident timeline, the parent chain, read-only before escalation).
// Composition from `prototype/C-rail.html`'s header row and
// `prototype/D-incident.html`'s crumb + budget chip.

type SecondaryView = 'transcript' | 'runs' | 'graph' | 'sequence' | 'yaml'

export function ThreadPane({ name, onBack }: Readonly<{ name: string; onBack?: () => void }>) {
  // The thread model owns the data, the stream, the mark-read on open and the
  // derived facts — called HERE, unconditionally, so every hook runs on every
  // render, loading or not (rules of hooks).
  const thread = useThread(name)
  const { detail: data, isLoading, error, refetch } = thread
  const [view, setView] = useState<SecondaryView>('transcript')

  // Switching conversations resets any secondary view back to the transcript
  // — a reader who left the Graph open on one conversation should not find
  // the next one opened there too.
  useEffect(() => setView('transcript'), [name])

  if (isLoading && !data) return <Loading />
  if (error || !data) return <ErrorState title="Conversation not found">{String(error)}</ErrorState>

  const c = data.conversation
  const { pipelineIcon, title, isRoot, isMember } = thread

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
        {view === 'runs' && <RunTimeline thread={thread} />}
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
          <TranscriptBody isRoot={isRoot} isMember={isMember} conversation={c} thread={thread} onSentOffline={() => void refetch?.()} />
        )}
      </div>
    </div>
  )
}

function TranscriptBody({
  isRoot, isMember, conversation, thread, onSentOffline,
}: Readonly<{
  isRoot: boolean
  isMember: boolean
  conversation: ConversationSummary
  thread: Thread
  onSentOffline: () => void
}>) {
  if (isRoot) return <CoordinatorBody conversation={conversation} thread={thread} onSentOffline={onSentOffline} />
  if (isMember) return <MemberOwnBody conversation={conversation} />
  return <ConversationThread thread={thread} onSentOffline={onSentOffline} />
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
  // The conversation itself is never part of its OWN breadcrumb — its title
  // is already the pane's heading, one line below this.
  const { coordinator: coordinatorName, ancestors } = useMemberChain(conversation)
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
            <PlainText>{chainStepLabel(step)}</PlainText>
          </Link>
        </span>
      ))}
    </div>
  )
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
  const exchange = useMemberExchange(member.name, expanded)
  const entry = member.causedBy?.entry ?? member.name
  const done = member.phase === 'Closed'
  return (
    <details
      data-testid={testid.memberInvocation(member.name)}
      data-expanded={expanded}
      style={{
        background: 'var(--ao-surface-alt)', border: '1px solid var(--ao-border)', borderRadius: 8,
        padding: '8px 14px',
      }}
      onToggle={(e) => setExpanded((e.target as HTMLDetailsElement).open)}
    >
      <summary data-testid={TESTID.memberInvocationToggle} style={{ cursor: 'pointer', display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
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
        {expanded && <ExpandedInvocation exchange={exchange} entry={entry} />}
        <div>
          <Link to={`/conversations/${member.name}`}>Open {member.name} on its own →</Link>
        </div>
      </div>
    </details>
  )
}

/** The expanded half of one `MemberInvocation` — its own loading/error states,
 * kept apart so the collapsed summary above never pays for them. */
function ExpandedInvocation({ exchange, entry }: Readonly<{ exchange: ReturnType<typeof useMemberExchange>; entry: string }>) {
  if (exchange.isLoading && !exchange.loaded) return <Loading />
  if (exchange.error || !exchange.loaded) return <ErrorState title="Could not load this member">{String(exchange.error)}</ErrorState>
  return <MemberInvocationExchange exchange={exchange} entry={entry} />
}

/** The two labelled sub-blocks a member invocation expands to. */
function MemberInvocationExchange({ exchange, entry }: Readonly<{ exchange: ReturnType<typeof useMemberExchange>; entry: string }>) {
  const { task, result } = exchange
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
  conversation, thread, onSentOffline,
}: Readonly<{
  conversation: ConversationSummary
  thread: Thread
  onSentOffline: () => void
}>) {
  const members = useThreadMembers(conversation, thread.detail?.transcript)
  const budget = conversation.budget
  if (members.isLoading && !members.loaded) return <Loading />
  if (members.error || !members.loaded) return <ErrorState title="Could not load member conversations">{String(members.error)}</ErrorState>

  const extraItems = members.members.map(({ member: m, at }) => ({
    key: `invoke-${m.name}`,
    at,
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
      <ConversationThread thread={thread} onSentOffline={onSentOffline} extraItems={extraItems} />
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
  const exchange = useMemberExchange(conversation.name)
  if (exchange.isLoading && !exchange.loaded) return <Loading />
  if (exchange.error || !exchange.conversation) return <ErrorState title="Could not load this member">{String(exchange.error)}</ErrorState>
  const messages = memberTranscript(conversation, exchange.conversation.runs)
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
  thread, onSentOffline, extraItems,
}: Readonly<{
  thread: Thread
  onSentOffline: () => void
  /** Rows to interleave into the transcript by time — a root's member
   * invocations. Absent for an ordinary pipeline conversation. */
  extraItems?: TimelineExtraItem[]
}>) {
  const detail = thread.detail!
  const name = detail.conversation.name
  const commands = useCommands()
  const connected = commands.connection === 'connected'
  const [text, setText] = useState('')
  const vocabulary = useVocabulary()
  const [dismissed, setDismissed] = useState(false)
  const [cursor, setCursor] = useState(0)
  const textRef = useRef<HTMLTextAreaElement>(null)
  const typeahead = dismissed ? null : matchEntries(text, vocabulary.data?.entries ?? [], 'thread')
  const activeCommand = typeahead ? Math.min(cursor, typeahead.length - 1) : 0
  const [error, setError] = useState<string>()
  // Sending is the command's state, the model's — not a flag this view keeps.
  const busy = commands.sending(name)
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
    if (!typeahead) return
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      setCursor((c) => (c + 1) % typeahead.length)
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      setCursor((c) => (c - 1 + typeahead.length) % typeahead.length)
    } else if (e.key === 'Enter' || e.key === 'Tab') {
      e.preventDefault()
      chooseCommand(typeahead[activeCommand])
    } else if (e.key === 'Escape') {
      e.preventDefault()
      setDismissed(true)
    }
  }

  const { canWrite, messages, pipelineIcon, events } = thread
  // The composer's own insert path — now ALSO how an interactive offered
  // choice lands text, wired straight into `Timeline` (B2: a message's
  // choices render WITH it, only the latest message's set interactive).
  function insertIntoComposer(cmd: string) {
    setText(cmd)
    requestAnimationFrame(() => textRef.current?.focus())
  }

  async function postMessage(toSend: string) {
    setError(undefined)
    const outcome = await sendText(name, toSend)
    if (outcome.ok) {
      setText('')
      if (!connected) onSentOffline()
    } else {
      setError(outcome.error)
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
            <ClipboardCopy isReadOnly data-testid={TESTID.joinHintFix}>{detail.joinHint.fix}</ClipboardCopy>
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
        onChoiceSelect={canWrite ? insertIntoComposer : undefined}
      />
      {canWrite && (
        <div style={{ padding: '10px 24px 14px', borderTop: '1px solid var(--ao-border)', display: 'flex', flexDirection: 'column', gap: 8 }}>
          <QuickChips
            canWrite={canWrite}
            onRunCommand={(cmd) => void send(cmd)}
          />
          <div style={{ position: 'relative' }}>
            {typeahead && (
              <ul
                aria-label="commands"
                data-testid={TESTID.commandTypeahead}
                style={{
                  position: 'absolute', bottom: '100%', left: 0, right: 0, zIndex: 200, marginBottom: 4,
                  maxHeight: '40vh', overflowY: 'auto', background: 'var(--ao-surface)', border: '1px solid var(--ao-border)',
                  borderRadius: 8, listStyle: 'none', padding: 4,
                }}
              >
                {typeahead.map((cmd, i) => (
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
    <Modal isOpen onClose={onCancel} variant="small" aria-label="confirm close" data-testid={TESTID.closeConfirmModal}>
      <ModalHeader title="Close this conversation?" />
      <ModalBody>
        <p>
          The agent says goodbye and the thread is archived. The conversation itself stays — its
          answers and its workspace are kept — and it can be reopened.
        </p>
        <Checkbox
          id="close-confirm-skip"
          data-testid={TESTID.closeConfirmSkip}
          label="Don't ask again"
          isChecked={dontAskAgain}
          onChange={(_e, v) => setDontAskAgain(v)}
        />
      </ModalBody>
      <ModalFooter>
        <Button variant="danger" onClick={() => onConfirm(dontAskAgain)} data-testid={TESTID.closeConfirmOk}>
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

/**
 * freshTokens is the input the call actually paid full price for: total in,
 * minus what the provider served from its prompt cache, minus what the call
 * wrote INTO that cache. Derived rather than reported — the work contract
 * carries the three facts and this is their arithmetic. Undefined while the
 * total is, so a turn with no usage renders dashes, never zeros.
 */
function freshTokens(t: RunTurn): number | undefined {
  if (t.tokensIn === undefined) return undefined
  return t.tokensIn - (t.cacheReadTokens ?? 0) - (t.cacheCreationTokens ?? 0)
}

function TurnsTable({ turns }: Readonly<{ turns: RunTurn[] }>) {
  return (
    <div>
      <SubTableLabel>{`turns — ${turns.length} model call${turns.length === 1 ? '' : 's'}`}</SubTableLabel>
      <Table variant="compact" aria-label="turns">
        <Thead>
          {/* Short on purpose: seven columns truncate long headers, and two
              adjacent "Cache …" stubs were indistinguishable at 1400px. */}
          <Tr>
            <Th>Model</Th><Th>In</Th><Th>Cached</Th><Th>Written</Th>
            <Th>Fresh</Th><Th>Out</Th><Th>Stop reason</Th>
          </Tr>
        </Thead>
        <Tbody>
          {withKeys(turns, (t) => `${t.model}:${t.tokensIn}:${t.tokensOut}:${t.cacheReadTokens}:${t.cacheCreationTokens}:${t.stopReason}`).map(([key, t]) => (
            <Tr key={key}>
              <Td dataLabel="Model"><PlainText>{t.model || '—'}</PlainText></Td>
              <Td dataLabel="In">{t.tokensIn ?? '—'}</Td>
              <Td dataLabel="Cached">{t.cacheReadTokens ?? '—'}</Td>
              <Td dataLabel="Written">{t.cacheCreationTokens ?? '—'}</Td>
              <Td dataLabel="Fresh">{freshTokens(t) ?? '—'}</Td>
              <Td dataLabel="Out">{t.tokensOut ?? '—'}</Td>
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

function RunTimeline({ thread }: Readonly<{ thread: Thread }>) {
  const { runs } = thread
  const detail = thread.detail!
  return (
    <div data-testid={TESTID.runsView} style={{ padding: 16, overflowY: 'auto', flex: 1 }}>
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
