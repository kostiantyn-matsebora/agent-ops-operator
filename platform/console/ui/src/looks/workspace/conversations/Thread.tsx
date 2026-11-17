import { Fragment, type ReactElement, type ReactNode, useEffect, useLayoutEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import type { ActivityEvent, ConversationSummary, Message, Run, VocabularyEntry } from '../../../api/types'
import { useVocabulary } from '../../../model/actions'
import { skipCloseConfirm, writeSkipCloseConfirm } from '../../../model/closeConfirm'
import { useCommands } from '../../../model/commands'
import { budgetLines, escalation } from '../../../model/coordinator'
import { speakerOf } from '../../../model/message'
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
  onBack?: () => void
  onSend?: (text: string) => void
  onVerb?: (verb: '/exit' | '/close') => void
  busy?: boolean
}

type Item = { key: string; node: ReactNode; divider?: boolean }

/**
 * The conversation tab's entries, a day divider before each message that falls
 * on a later day than the one before it. The conversation's start is already
 * on the line under the tabs, so the first day needs no divider of its own.
 */
export function threadItems(p: Pick<ThreadViewProps, 'messages' | 'startedAt' | 'conversation' | 'runDetails' | 'invoked'>): Item[] {
  const rows: ({ divider: string; at: string } | { escalated: string } | { message: Message; speaker: ReturnType<typeof speakerOf> })[] = []
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
    rows.push({ message: m, speaker })
  }
  if (esc && !escPlaced) rows.push({ escalated: esc.label })
  return rows.map((r, i) => {
    if ('divider' in r) {
      return { key: `day-${r.divider}`, divider: true, node: <div className="wk-dv" role="separator">{day(r.at)}</div> }
    }
    if ('escalated' in r) {
      return { key: 'escalated', divider: true, node: <div className="wk-dv esc" role="separator" data-testid="escalated">{r.escalated}</div> }
    }
    // The entry before a divider, and the last one, close without their own rule.
    const next = rows[i + 1]
    const last = !next || 'divider' in next || 'escalated' in next
    return { key: r.message.id, node: entryFor(r.message, r.speaker, p, last) }
  })
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
          <pre className="wk-result">{run.result}</pre>
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
  const items = threadItems(p)

  // A thread opens at its newest message, and follows new ones while the
  // viewer is at the bottom.
  const stick = useRef(true)
  useLayoutEffect(() => {
    stick.current = true
  }, [c.name])
  useLayoutEffect(() => {
    const el = body.current
    if (el && stick.current && tab === 'conversation') el.scrollTop = el.scrollHeight
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
        <h2 title={p.title}>{p.title}</h2>
        {statusPill(c)}
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
          <div className="wk-card">
            <pre className="wk-yaml">{p.yaml || 'No YAML for this conversation.'}</pre>
          </div>
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
        <Composer onSend={p.onSend} onCommand={p.onVerb ? runCommand : undefined} vocabulary={p.vocabulary} disabled={p.busy} />
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
  const { members } = useThreadMembers(c, thread.detail?.transcript)
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
    vocabulary: vocabulary.data?.entries,
    canWrite: thread.canWrite,
    invoked,
    memberEntry: c.causedBy?.entry,
    onBack,
    onSend: (text) => void sendText(c.name, text),
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
