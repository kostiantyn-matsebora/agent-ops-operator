import { Fragment, type ReactElement, type ReactNode, useEffect, useLayoutEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import type { ActivityEvent, ConversationSummary, Message, Run, VocabularyEntry } from '../../../api/types'
import { useVocabulary } from '../../../model/actions'
import { skipCloseConfirm, writeSkipCloseConfirm } from '../../../model/closeConfirm'
import { useCommands } from '../../../model/commands'
import { budgetLines, escalation } from '../../../model/coordinator'
import { firstUnreadId, speakerOf } from '../../../model/message'
import { resolveIcon } from '../../../model/icon'
import { runTokens, toolRows, turnRows } from '../../../model/runStats'
import type { RunDetails as RunDetailsFact, WorkingRun } from '../../../model/run'
import { GENERAL_FEED_NAME, useGeneralFeed } from '../../../model/session'
import { TESTID, testid } from '../../../model/testids'
import {
  invokedByReply, memberTranscript, useMemberChain, useThread, useThreadMembers,
} from '../../../model/thread'
import { rowTitle } from '../../../model/conversationList'
import { sendText } from '../../../model/verbs'
import { Chevron, Icon } from '../icons'
import { CopyButton } from './CopyButton'
import { dayKey, day, duration, started } from '../format'
import { CloseConfirm } from './CloseConfirm'
import { Composer } from './Composer'
import { Notice, PersonMessage, Reply, SignalCard, WorkingCard } from './Entries'
import { GraphTab, SequenceTab } from './Secondary'

// THE THREAD: one conversation, opened beside the list. The container reads
// the thread model; the view draws what it is given and nothing else.

export type ThreadTab = 'conversation' | 'runs' | 'graph' | 'sequence' | 'yaml'

export interface ThreadViewProps {
  conversation: ConversationSummary
  title: string
  messages: Message[]
  runs: Run[]
  runDetails: Map<string, RunDetailsFact>
  working?: WorkingRun
  startedAt?: string
  yaml?: string
  /** The conversation's recorded hops, for the Sequence tab. */
  events?: ActivityEvent[]
  /** The channel's vocabulary, for the command listing and the thread's own commands. */
  vocabulary?: VocabularyEntry[]
  canWrite: boolean
  /** Which members each reply opened, by reply id. */
  invoked: Map<string, ConversationSummary[]>
  /** The root a member belongs to, for its crumb and its reply control. */
  root?: { name: string; title: string }
  /** The entry the member was invoked as. */
  memberEntry?: string
  /** The first message the viewer had not read when the conversation opened. */
  unreadFrom?: string
  /** What an offered choice does: puts its command in the reply box. Absent for a viewer who cannot write. */
  onChoice?: (command: string) => void
  /** The Pipeline's own icon reference. */
  pipelineIcon?: string
  /** Why this conversation has no console thread, and the command that joins one. */
  joinHint?: { reason: string; fix: string }
  /** The thread was archived: the conversation closed and the transcript stays readable. */
  archived?: boolean
  /** A reply that did not reach the manager, with why — shown until dismissed. */
  sendFailure?: { error: string; onDismiss: () => void }
  /** A gap in the activity buffer — console-thread-live-cues: "Lost history is marked". */
  activityGap?: boolean
  /** The members could not be loaded: their invocations are missing from the transcript. */
  membersError?: string
  /** Object metadata for the YAML tab. */
  metadata?: { name: string; namespace?: string; uid?: string; resourceVersion?: string; creationTimestamp?: string; labels?: Record<string, string>; annotations?: Record<string, string> }
  onBack?: () => void
  onSend?: (text: string) => void
  onVerb?: (verb: '/exit' | '/close') => void
  onSentOffline?: () => void
  busy?: boolean
}

type Item = { key: string; node: ReactNode; divider?: boolean }

/**
 * The conversation tab's entries, a day divider before each message that falls
 * on a later day than the one before it. The conversation's start is already
 * on the line under the tabs, so the first day needs no divider of its own.
 */
export function threadItems(p: Pick<ThreadViewProps, 'messages' | 'startedAt' | 'conversation' | 'runDetails' | 'invoked'> & Partial<Pick<ThreadViewProps, 'unreadFrom' | 'onChoice'>>): Item[] {
  const rows: ({ divider: string; at: string } | { unread: true } | { escalated: string } | { message: Message; speaker: ReturnType<typeof speakerOf> })[] = []
  let lastDay = dayKey(p.startedAt) || dayKey(p.messages[0]?.at)
  // The moment a Coordinator root reached a person sits in the stream by time.
  const esc = escalation(p.conversation)
  let escPlaced = !esc
  for (const m of p.messages) {
    const speaker = speakerOf(m)
    if (speaker === 'hidden') continue
    if (esc && !escPlaced && (Date.parse(m.at) || 0) > esc.at) {
      rows.push({ escalated: esc.label })
      escPlaced = true
    }
    const k = dayKey(m.at)
    if (k && k !== lastDay) {
      rows.push({ divider: k, at: m.at })
      lastDay = k
    }
    if (m.id === p.unreadFrom) rows.push({ unread: true })
    rows.push({ message: m, speaker })
  }
  if (esc && !escPlaced) rows.push({ escalated: esc.label })
  return rows.map((r, i) => {
    if ('divider' in r) {
      return { key: `day-${r.divider}`, divider: true, node: <div className="wk-dv" role="separator">{day(r.at)}</div> }
    }
    if ('unread' in r) {
      return { key: 'unread', divider: true, node: <div className="wk-dv new" role="separator" data-testid="unread-divider">New messages</div> }
    }
    if ('escalated' in r) {
      const escTime = escalation(p.conversation)
      return {
        key: 'escalated',
        divider: true,
        node: (
          <div className="wk-dv esc" role="separator" data-testid="escalated">
            {r.escalated}
            {escTime && <> · {new Date(escTime.at).toLocaleString()}</>}
          </div>
        ),
      }
    }
    // The entry before a divider, and the last one, close without their own rule.
    const next = rows[i + 1]
    const last = !next || 'divider' in next || 'escalated' in next || 'unread' in next
    // Only the LATEST message's choices are actionable: an older prompt has moved on.
    const latest = r.message.id === p.messages.at(-1)?.id
    return { key: r.message.id, node: <>{entryFor(r.message, r.speaker, p, last)}<Choices choices={r.message.choices} interactive={latest && Boolean(p.onChoice)} onPick={p.onChoice} /></> }
  })
}

function Choices({ choices, interactive, onPick }: Readonly<{ choices?: Message['choices']; interactive: boolean; onPick?: (command: string) => void }>): ReactElement | null {
  if (!choices || choices.length === 0) return null
  if (interactive && onPick) {
    return (
      <div className="wk-choices" data-testid="choices">
        {choices.map((c) => (
          <button key={c.command} type="button" className="wk-btn" onClick={() => onPick(`${c.command} `)}>
            {c.label || c.command}
          </button>
        ))}
      </div>
    )
  }
  return (
    <ul className="wk-choices" aria-label="offered choices (no longer actionable)">
      {choices.map((c) => (
        <li key={c.command} className="stale">{c.label || c.command}</li>
      ))}
    </ul>
  )
}

function entryFor(m: Message, speaker: ReturnType<typeof speakerOf>, p: Pick<ThreadViewProps, 'conversation' | 'runDetails' | 'invoked'>, last: boolean) {
  switch (speaker) {
    case 'signal':
      return <SignalCard message={m} conversation={p.conversation} last={last} />
    case 'agent':
      return <Reply message={m} members={p.invoked.get(m.id)} details={p.runDetails.get(m.id)} last={last} />
    case 'notice':
      return <Notice message={m} />
    default:
      return <PersonMessage message={m} last={last} />
  }
}

function PipelineMark({ icon }: Readonly<{ icon?: string }>): ReactElement | null {
  const r = resolveIcon(icon)
  if (!r) return null
  if (r.kind === 'text') return <span className="wk-pmark" aria-hidden="true">{r.text}</span>
  if (r.kind === 'image') return <img className="wk-pmark" src={r.src} alt="" />
  return (
    <svg className="wk-pmark" viewBox="0 0 24 24" width="1em" height="1em" fill="none" stroke="currentColor" strokeWidth="1.8" aria-hidden="true">
      <path d={r.d} />
    </svg>
  )
}

function statusPill(c: ConversationSummary): ReactElement | null {
  if (c.presence) {
    return (
      <span className="wk-pill working">
        <span className="wk-spin" aria-hidden="true" />
        Working
      </span>
    )
  }
  if (c.errored) return <span className="wk-pill bad">Run failed</span>
  if (c.phase === 'Closed') return <span className="wk-pill">Closed</span>
  if (c.phase === 'Pending' || c.phase === 'Queued') return <span className="wk-pill warn">{c.phase}</span>
  return null
}

function RunCard({ run }: Readonly<{ run: Run }>) {
  const turns = turnRows(run)
  const tools = toolRows(run)
  const tokens = runTokens(run)
  return (
    <div className="wk-card" data-testid={TESTID.runsView}>
      <h4>Run</h4>
      <dl className="wk-kv">
        <dt>Run</dt>
        <dd>{run.runId}</dd>
        <dt>Status</dt>
        <dd>
          <span className={`wk-pill ${run.status === 'succeeded' ? 'ok' : 'bad'}`}>{run.status}</span>
        </dd>
        <dt>Exit</dt>
        <dd>{run.exitCode ?? '—'}</dd>
        {run.startedAt && run.finishedAt && (
          <>
            <dt>Took</dt>
            <dd className="wk-num">{duration(Date.parse(run.finishedAt) - Date.parse(run.startedAt))}</dd>
          </>
        )}
        <dt>Finished</dt>
        <dd>{run.finishedAt ? new Date(run.finishedAt).toLocaleString() : '—'}</dd>
        {tokens && (
          <>
            <dt>Tokens</dt>
            <dd className="wk-num" data-testid="run-tokens">{`${tokens.in} in · ${tokens.out} out`}</dd>
          </>
        )}
      </dl>
      {run.result && (
        <div className="wk-sec">
          <b>Result</b>
          <pre className="wk-result" >{run.result}</pre>
        </div>
      )}
      {turns.length > 0 && (
        <div className="wk-sec">
          <b>{`Turns – ${turns.length} model call${turns.length === 1 ? '' : 's'}`}</b>
          <div className="wk-tw">
            <table className="wk-tbl" aria-label="Turns">
              <thead>
                <tr><th>Model</th><th>In</th><th>Cached</th><th>Written</th><th>Fresh</th><th>Out</th><th>Stop reason</th></tr>
              </thead>
              <tbody>
                {turns.map((t) => (
                  <tr key={t.key}>
                    <td>{t.model}</td>
                    <td className="wk-num">{t.in}</td>
                    <td className="wk-num">{t.cached}</td>
                    <td className="wk-num">{t.written}</td>
                    <td className="wk-num">{t.fresh}</td>
                    <td className="wk-num">{t.out}</td>
                    <td>{t.stopReason}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}
      {tools.length > 0 && (
        <div className="wk-sec">
          <b>{`Tool calls – ${tools.length}`}</b>
          <div className="wk-tw">
            <table className="wk-tbl" aria-label="Tool calls">
              <thead>
                <tr><th>Tool</th><th>Server</th><th>Duration</th><th>Result size</th></tr>
              </thead>
              <tbody>
                {tools.map((t) => (
                  <tr key={t.key}>
                    <td>{t.tool}</td>
                    <td>{t.server}</td>
                    <td className="wk-num">{t.duration}</td>
                    <td className="wk-num">{t.size}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  )
}

function RunsTab({ runs, conversation }: Readonly<{ runs: Run[]; conversation: ConversationSummary }>) {
  return (
    <>
      {runs.length === 0 ? <div className="wk-card wk-note">No completed runs.</div> : runs.map((r) => <RunCard key={r.runId} run={r} />)}
      <div className="wk-card">
        <h4>Bindings and runtime</h4>
        <dl className="wk-kv">
          <dt>Inputs queued</dt>
          <dd>{conversation.queued}</dd>
          <dt>Runtime pod</dt>
          <dd>{conversation.runtimePod || '—'}</dd>
        </dl>
      </div>
    </>
  )
}

export function ThreadView(p: Readonly<ThreadViewProps>): ReactElement {
  const [tab, setTab] = useState<ThreadTab>('conversation')
  const [closing, setClosing] = useState(false)
  const [away, setAway] = useState(0)
  const body = useRef<HTMLDivElement>(null)
  const seen = useRef(0)
  const c = p.conversation
  const closed = c.phase === 'Closed'
  const isMember = Boolean(c.causedBy)
  const items = threadItems({ messages: p.messages, startedAt: p.startedAt, conversation: p.conversation, runDetails: p.runDetails, invoked: p.invoked, unreadFrom: p.unreadFrom, onChoice: p.canWrite && !closed && !isMember && p.onSend ? (command) => setInsert({ text: command, n: Date.now() }) : undefined })
  const fixRef = useRef<HTMLElement>(null)
  const [insert, setInsert] = useState<{ text: string; n: number }>()

  // A thread opens at its newest message, and follows new ones while the
  // viewer is at the bottom.
  const stick = useRef(true)
  useLayoutEffect(() => {
    stick.current = true
  }, [c.name])
  const openedAtUnread = useRef(false)
  useLayoutEffect(() => {
    const el = body.current
    if (!el || tab !== 'conversation') return
    // Opening with unread messages lands on what is new, never at the end.
    const divider = el.querySelector('[data-testid="unread-divider"]')
    if (divider && !openedAtUnread.current) {
      openedAtUnread.current = true
      stick.current = false
      ;(divider as HTMLElement).scrollIntoView?.({ block: 'start' })
      return
    }
    if (stick.current) el.scrollTop = el.scrollHeight
  }, [c.name, items.length, Boolean(p.working), tab])
  // Arrivals while the viewer is reading higher up are counted, never forced
  // into view: the pill says how many, and jumps on request.
  useEffect(() => {
    const grew = items.length - seen.current
    seen.current = items.length
    if (grew > 0 && !stick.current && tab === 'conversation') setAway((n) => n + grew)
  }, [items.length, tab])
  const jumpToEnd = () => {
    const el = body.current
    if (el) el.scrollTop = el.scrollHeight
    stick.current = true
    setAway(0)
  }
  // `/close` ends the conversation, so it asks first unless the viewer opted out. `/exit` never asks.
  const closeOrAsk = () => {
    if (!skipCloseConfirm()) setClosing(true)
    else p.onVerb?.('/close')
  }
  const runCommand = (command: '/exit' | '/close') => (command === '/close' ? closeOrAsk() : p.onVerb?.(command))
  const budget = budgetLines(c.budget)

  const tabs: { id: ThreadTab; label: string }[] = [
    { id: 'conversation', label: 'Conversation' },
    { id: 'runs', label: `Runs${p.runs.length ? ` · ${p.runs.length}` : ''}` },
    // Graph and Sequence read the conversation's own topology and hops: a root's. A member's single node is one click into its parent's view.
    ...(isMember ? [] : [{ id: 'graph' as const, label: 'Graph' }, { id: 'sequence' as const, label: 'Sequence' }]),
    { id: 'yaml', label: 'YAML' },
  ]

  return (
    <>
      <div className="wk-th">
        {isMember && p.root && (
          <span className="crumb">
            <Link className="wk-lnk" to={`/conversations/${encodeURIComponent(p.root.name)}`}>
              {p.root.title}
            </Link>
            <Chevron dir="right" />
            <span>{p.memberEntry}</span>
          </span>
        )}
        {p.onBack && (
          <button type="button" className="wk-icon-btn back" aria-label="Back to the list" onClick={p.onBack}>
            <Chevron dir="left" />
          </button>
        )}
        <h2 title={p.title}>
          <PipelineMark icon={p.pipelineIcon} />
          {p.title}
        </h2>
        {statusPill(c)}
        {(c.threads?.length ?? 0) > 0 && (
          <span className="wk-chip quiet" title="every channel a reply here also reaches" data-testid="bound-channels">
            {c.threads!.map((t) => t.channel).join(' · ')}
          </span>
        )}
        {p.canWrite && !closed && !isMember && p.onVerb && (
          <div className="acts">
            <button type="button" className="wk-btn" disabled={p.busy} onClick={() => p.onVerb?.('/exit')}>
              Exit runtime
            </button>
            <button type="button" className="wk-btn" disabled={p.busy} onClick={closeOrAsk}>
              Close
            </button>
          </div>
        )}
      </div>
      {c.brief && <p className="wk-brief">{c.brief}</p>}
      {p.activityGap && tab === 'conversation' && (
        <div className="wk-gap-notice" data-testid="activity-gap">
          — some activity here was not recorded —
        </div>
      )}
      <div className="wk-tabs" role="tablist" aria-label="conversation views">
        {tabs.map((t) => (
          <button
            key={t.id}
            type="button"
            role="tab"
            id={`wk-tab-${t.id}`}
            data-testid={testid.view(t.id)}
            aria-selected={tab === t.id}
            aria-controls="wk-thread-body"
            onClick={() => setTab(t.id)}
          >
            {t.label}
          </button>
        ))}
      </div>
      <div className="wk-dline">
        {tab === 'conversation' && p.startedAt && (
          <time dateTime={p.startedAt} data-testid="thread-started">
            {started(p.startedAt)}
          </time>
        )}
      </div>
      {tab === 'conversation' && (p.joinHint || p.archived || p.sendFailure || p.membersError) && (
        <div className="wk-notices">
          {p.joinHint && (
            <div className="wk-alert" data-testid="join-hint">
              <b>This conversation has no console thread</b>
              <div>{p.joinHint.reason}</div>
              <div className="wk-fix">
                <code ref={fixRef} data-testid={TESTID.joinHintFix}>{p.joinHint.fix}</code>
                <CopyButton text={p.joinHint.fix} target={fixRef} />
              </div>
            </div>
          )}
          {p.membersError && (
            <div className="wk-alert danger" role="alert" data-testid="members-error">
              <b>Could not load member conversations</b>
              <div>{p.membersError}</div>
            </div>
          )}
          {p.archived && (
            <div className="wk-alert" data-testid="archived">
              <b>This thread was archived</b>
              <div>The conversation was closed. The transcript stays readable.</div>
            </div>
          )}
          {p.sendFailure && (
            <div className="wk-alert bad" role="alert" data-testid="send-failure">
              <b>Not sent</b> — {p.sendFailure.error}{' '}
              <button type="button" className="wk-lnk" onClick={p.sendFailure.onDismiss}>Dismiss</button>
            </div>
          )}
        </div>
      )}
      {tab === 'conversation' && budget.length > 0 && (
        <div className="wk-bar" data-testid="budget">
          {budget.map((l) => (
            <span key={l.label}>
              {l.label} <b>{l.value}</b>
            </span>
          ))}
        </div>
      )}
      <div
        className={`wk-body${tab === 'graph' ? ' graph' : ''}`}
        id="wk-thread-body"
        role="tabpanel"
        aria-labelledby={`wk-tab-${tab}`}
        ref={body}
        data-testid={tab === 'conversation' ? TESTID.timeline : undefined}
        onScroll={(e) => {
          const el = e.currentTarget
          stick.current = el.scrollHeight - el.scrollTop - el.clientHeight < 90
          if (stick.current) setAway(0)
        }}
      >
        {tab === 'conversation' && (
          <>
            {items.map((it) => (
              <Fragment key={it.key}>{it.node}</Fragment>
            ))}
            {items.length === 0 && !p.working && (
              <div className="wk-empty">
                <h2>No messages yet</h2>
                {closed ? 'This conversation closed before anything was said.' : 'The first message will appear here.'}
              </div>
            )}
            {p.working && <WorkingCard working={p.working} />}
            {closed && <div className="wk-closed-note">This conversation is closed. Its history is kept.</div>}
            {away > 0 && (
              <button type="button" className="wk-btn primary wk-jump" data-testid={TESTID.jumpPill} onClick={jumpToEnd}>
                {`↓ ${away} new message${away === 1 ? '' : 's'}`}
              </button>
            )}
          </>
        )}
        {tab === 'runs' && <RunsTab runs={p.runs} conversation={c} />}
        {tab === 'graph' && <GraphTab name={c.name} />}
        {tab === 'sequence' && <SequenceTab events={p.events ?? []} />}
        {tab === 'yaml' && (
          <>
            {p.metadata && (
              <div className="wk-card" data-testid="metadata-card">
                <h4>Metadata</h4>
                <dl className="wk-kv">
                  <dt>Name</dt>
                  <dd>{p.metadata.name}</dd>
                  {p.metadata.namespace && (
                    <>
                      <dt>Namespace</dt>
                      <dd>{p.metadata.namespace}</dd>
                    </>
                  )}
                  {p.metadata.creationTimestamp && (
                    <>
                      <dt>Created</dt>
                      <dd>{new Date(p.metadata.creationTimestamp).toLocaleString()}</dd>
                    </>
                  )}
                  {p.metadata.uid && (
                    <>
                      <dt>UID</dt>
                      <dd><small>{p.metadata.uid}</small></dd>
                    </>
                  )}
                  {p.metadata.resourceVersion && (
                    <>
                      <dt>Resource version</dt>
                      <dd><small>{p.metadata.resourceVersion}</small></dd>
                    </>
                  )}
                  {p.metadata.labels && Object.keys(p.metadata.labels).length > 0 && (
                    <>
                      <dt>Labels</dt>
                      <dd><small>{Object.entries(p.metadata.labels).map(([k, v]) => `${k}=${v}`).join(', ')}</small></dd>
                    </>
                  )}
                  {p.metadata.annotations && Object.keys(p.metadata.annotations).length > 0 && (
                    <>
                      <dt>Annotations</dt>
                      <dd><small>{Object.entries(p.metadata.annotations).slice(0, 6).map(([k, v]) => `${k}=${v}`).join(', ')}</small></dd>
                    </>
                  )}
                </dl>
              </div>
            )}
            <div className="wk-card">
              <pre className="wk-yaml">{p.yaml || 'No YAML for this conversation.'}</pre>
            </div>
          </>
        )}
      </div>
      {tab === 'conversation' && isMember && p.root && (
        <div className="wk-foot">
          <Link className="wk-btn primary" to={`/conversations/${encodeURIComponent(p.root.name)}`}>
            Reply in {p.root.title}
          </Link>
        </div>
      )}
      {tab === 'conversation' && !isMember && !closed && p.canWrite && p.onSend && (
        <Composer onSend={p.onSend} onCommand={p.onVerb ? runCommand : undefined} vocabulary={p.vocabulary} disabled={p.busy} insert={insert} />
      )}
      {closing && (
        <CloseConfirm
          onCancel={() => setClosing(false)}
          onConfirm={(dontAskAgain) => {
            writeSkipCloseConfirm(dontAskAgain)
            setClosing(false)
            p.onVerb?.('/close')
          }}
        />
      )}
    </>
  )
}

// ---- the containers ---------------------------------------------------------------

function MemberRoot({ conversation, children }: Readonly<{ conversation: ConversationSummary; children: (root?: { name: string; title: string }) => ReactElement }>) {
  const chain = useMemberChain(conversation)
  const root = chain.ancestors[0]
  return children(root ? { name: root.name, title: rowTitle(root) } : conversation.causedBy ? { name: conversation.causedBy.parent, title: conversation.causedBy.parent } : undefined)
}

function LoadedThread({ thread, onBack }: Readonly<{ thread: ReturnType<typeof useThread> & { conversation: ConversationSummary }; onBack?: () => void }>) {
  const c = thread.conversation
  const commands = useCommands()
  const [failure, setFailure] = useState<string>()
  // Read once per open: marking the conversation read must not move the divider under the viewer.
  const [unreadFrom] = useState(() => firstUnreadId(thread.messages, c.readAt))
  const { members, error: membersError } = useThreadMembers(c, thread.detail?.transcript)
  const vocabulary = useVocabulary()
  // A member binds no channel of its own: its runs' records are its transcript.
  const messages = thread.isMember && thread.messages.length === 0 ? memberTranscript(c, thread.runs) : thread.messages
  const invoked = invokedByReply(messages, members)
  const props: Omit<ThreadViewProps, 'root'> = {
    conversation: c,
    title: thread.title,
    messages,
    runs: thread.runs,
    runDetails: thread.runDetails,
    working: thread.working,
    startedAt: thread.startedAt,
    yaml: thread.detail?.yaml,
    events: thread.detail?.events ?? [],
    activityGap: (thread.detail as any)?.activityGap,
    membersError: membersError ? String(membersError) : undefined,
    metadata: thread.detail?.object?.metadata,
    vocabulary: vocabulary.data?.entries,
    canWrite: thread.canWrite,
    invoked,
    memberEntry: c.causedBy?.entry,
    onBack,
    unreadFrom,
    pipelineIcon: thread.pipelineIcon,
    joinHint: thread.detail?.joinHint,
    archived: thread.detail?.archived,
    sendFailure: failure ? { error: failure, onDismiss: () => setFailure(undefined) } : undefined,
    onSend: async (text) => {
      setFailure(undefined)
      const outcome = await sendText(c.name, text)
      if (!outcome.ok) setFailure(outcome.error)
      // The stream is down, so nothing will announce the reply: read it again.
      else if (commands.connection !== 'connected') {
        void thread.refetch?.()
        props.onSentOffline?.()
      }
    },
    onVerb: (verb) => void sendText(c.name, verb),
    busy: commands.sending(c.name),
  }
  if (!thread.isMember) return <ThreadView {...props} />
  return <MemberRoot conversation={c}>{(root) => <ThreadView {...props} root={root} />}</MemberRoot>
}

export function ThreadPane({ name, onBack }: Readonly<{ name: string; onBack?: () => void }>): ReactElement {
  const thread = useThread(name)
  if (thread.isLoading && !thread.conversation) {
    return <div className="wk-empty" aria-busy="true">Loading the conversation…</div>
  }
  if (!thread.conversation) {
    return (
      <div className="wk-empty">
        <h2>Could not open this conversation</h2>
        {thread.error ? String(thread.error) : `Nothing is named ${name}.`}
      </div>
    )
  }
  return <LoadedThread key={name} thread={thread as typeof thread & { conversation: ConversationSummary }} onBack={onBack} />
}

/** The channel's general feed: thread-less sends, read-only. */
export function GeneralFeedPane({ onBack }: Readonly<{ onBack?: () => void }>): ReactElement {
  const feed = useGeneralFeed()
  const messages = feed.data?.messages ?? []
  return (
    <>
      <div className="wk-th">
        {onBack && (
          <button type="button" className="wk-icon-btn back" aria-label="Back to the list" onClick={onBack}>
            <Chevron dir="left" />
          </button>
        )}
        <h2>
          <span style={{ verticalAlign: '-3px', marginRight: 6, display: 'inline-flex' }}>
            <Icon name="hash" />
          </span>
          General feed
        </h2>
        {feed.data?.channel && <span className="wk-chip quiet">{feed.data.channel}</span>}
      </div>
      <div className="wk-body" data-testid={TESTID.timeline}>
        <div className="wk-card">
          <b>Channel-wide notices, not a conversation</b>
          <div className="wk-note">
            A send with no conversation behind it, like an <code>/agents</code> reply, lands here instead of being
            dropped. Read-only: there is nothing to reply into.
          </div>
        </div>
        {feed.isLoading && <div className="wk-empty" aria-busy="true">Loading…</div>}
        {!feed.isLoading && messages.length === 0 && (
          <div className="wk-empty">
            <h2>No messages on the console thread yet</h2>
          </div>
        )}
        {messages.map((m) =>
          speakerOf(m) === 'agent' ? <Reply key={m.id} message={m} /> : <Notice key={m.id} message={m} />,
        )}
      </div>
    </>
  )
}

export { GENERAL_FEED_NAME }
