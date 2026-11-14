import { Fragment, type FormEvent, type KeyboardEvent, type ReactElement, type ReactNode, useEffect, useLayoutEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import type { ConversationSummary, Message, Run } from '../../../api/types'
import { useCommands } from '../../../model/commands'
import { speakerOf } from '../../../model/message'
import type { RunDetails as RunDetailsFact, WorkingRun } from '../../../model/run'
import { GENERAL_FEED_NAME, useGeneralFeed } from '../../../model/session'
import { TESTID } from '../../../model/testids'
import {
  invokedByReply, memberTranscript, useMemberChain, useThread, useThreadMembers,
} from '../../../model/thread'
import { rowTitle } from '../../../model/conversationList'
import { sendText } from '../../../model/verbs'
import { Chevron, Icon } from '../icons'
import { dayKey, day, duration, started, toolDuration } from '../format'
import { Notice, PersonMessage, Reply, SignalCard, WorkingCard } from './Entries'

// THE THREAD: one conversation, opened beside the list. The container reads
// the thread model; the view draws what it is given and nothing else.

export type ThreadTab = 'conversation' | 'runs' | 'yaml'

export interface ThreadViewProps {
  conversation: ConversationSummary
  title: string
  messages: Message[]
  runs: Run[]
  runDetails: Map<string, RunDetailsFact>
  working?: WorkingRun
  startedAt?: string
  yaml?: string
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
  const rows: ({ divider: string; at: string } | { message: Message; speaker: ReturnType<typeof speakerOf> })[] = []
  let lastDay = dayKey(p.startedAt) || dayKey(p.messages[0]?.at)
  for (const m of p.messages) {
    const speaker = speakerOf(m)
    if (speaker === 'hidden') continue
    const k = dayKey(m.at)
    if (k && k !== lastDay) {
      rows.push({ divider: k, at: m.at })
      lastDay = k
    }
    rows.push({ message: m, speaker })
  }
  return rows.map((r, i) => {
    if ('divider' in r) {
      return { key: `day-${r.divider}`, divider: true, node: <div className="wk-dv" role="separator">{day(r.at)}</div> }
    }
    // The entry before a divider, and the last one, close without their own rule.
    const next = rows[i + 1]
    const last = !next || 'divider' in next
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

function Composer({ onSend, disabled }: Readonly<{ onSend: (text: string) => void; disabled?: boolean }>) {
  const [text, setText] = useState('')
  function submit(e?: FormEvent) {
    e?.preventDefault()
    const t = text.trim()
    if (!t || disabled) return
    onSend(t)
    setText('')
  }
  function onKeyDown(e: KeyboardEvent<HTMLTextAreaElement>) {
    if (e.key === 'Enter' && e.shiftKey) submit(e as unknown as FormEvent)
  }
  return (
    <form className="wk-compose" onSubmit={submit}>
      <textarea
        aria-label="message"
        placeholder="Reply to the agent…"
        value={text}
        onChange={(e) => setText(e.target.value)}
        onKeyDown={onKeyDown}
      />
      <span className="h">
        <span><kbd>/</kbd> commands</span>
        <span><kbd>Shift</kbd> + <kbd>Enter</kbd> send</span>
      </span>
      <button className="wk-btn primary" type="submit" disabled={disabled || !text.trim()}>
        Send
      </button>
    </form>
  )
}

function RunsTab({ runs }: Readonly<{ runs: Run[] }>) {
  if (runs.length === 0) return <div className="wk-card wk-note">No runs yet.</div>
  return (
    <>
      {runs.map((r) => (
        <div key={r.runId} className="wk-card" data-testid={TESTID.runsView}>
          <dl className="wk-kv">
            <dt>Run</dt>
            <dd>{r.runId}</dd>
            <dt>Status</dt>
            <dd>{r.status}</dd>
            {r.startedAt && r.finishedAt && (
              <>
                <dt>Took</dt>
                <dd className="wk-num">{duration(Date.parse(r.finishedAt) - Date.parse(r.startedAt))}</dd>
              </>
            )}
            {r.finishedAt && (
              <>
                <dt>Finished</dt>
                <dd>{new Date(r.finishedAt).toLocaleString()}</dd>
              </>
            )}
            {(r.toolCalls?.length ?? 0) > 0 && (
              <>
                <dt>Tool calls</dt>
                <dd>
                  {r.toolCalls!.map((t, i) => (
                    <div key={i}>
                      <code>{t.tool}</code> <span className="wk-num wk-note">{toolDuration(t.durationMs)}</span>
                    </div>
                  ))}
                </dd>
              </>
            )}
          </dl>
        </div>
      ))}
    </>
  )
}

export function ThreadView(p: Readonly<ThreadViewProps>): ReactElement {
  const [tab, setTab] = useState<ThreadTab>('conversation')
  const [confirmClose, setConfirmClose] = useState(false)
  const body = useRef<HTMLDivElement>(null)
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
  useEffect(() => {
    if (!confirmClose) return
    const t = setTimeout(() => setConfirmClose(false), 4000)
    return () => clearTimeout(t)
  }, [confirmClose])

  const tabs: { id: ThreadTab; label: string }[] = [
    { id: 'conversation', label: 'Conversation' },
    { id: 'runs', label: `Runs${p.runs.length ? ` · ${p.runs.length}` : ''}` },
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
            <button
              type="button"
              className={`wk-btn${confirmClose ? ' danger' : ''}`}
              disabled={p.busy}
              onClick={() => {
                if (!confirmClose) {
                  setConfirmClose(true)
                  return
                }
                setConfirmClose(false)
                p.onVerb?.('/close')
              }}
            >
              {confirmClose ? 'Confirm close' : 'Close'}
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
      <div
        className="wk-body"
        id="wk-thread-body"
        role="tabpanel"
        aria-labelledby={`wk-tab-${tab}`}
        ref={body}
        data-testid={tab === 'conversation' ? TESTID.timeline : undefined}
        onScroll={(e) => {
          const el = e.currentTarget
          stick.current = el.scrollHeight - el.scrollTop - el.clientHeight < 90
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
          </>
        )}
        {tab === 'runs' && <RunsTab runs={p.runs} />}
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
        <Composer onSend={p.onSend} disabled={p.busy} />
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
