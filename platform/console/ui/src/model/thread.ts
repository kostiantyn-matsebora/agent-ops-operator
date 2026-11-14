import { useEffect, useMemo, useRef } from 'react'
import {
  useConversation, useConversations, useMarkRead, useSession, useVocabulary,
} from '../api/hooks'
import { eventsFor, useStream } from '../api/stream'
import { fence } from '../api/fence'
import { mergeEvents } from '../graph/hops'
import type {
  ActivityEvent, ConversationDetail, ConversationSummary, Message, RecordedInput, Run,
} from '../api/types'
import { confirmRef } from './commands'
import { attachRunDetails, useWorkingRun } from './run'
import { stripLeadingIcon } from './titles'

// THE THREAD MODEL: one conversation's messages — each with the markdown it was
// sent as — its signals, runs, members and start time, and what the viewer may
// do in it. Every look reads these from here.

/** A message as the thread holds it, with the markdown it was sent as. */
export interface ThreadMessage extends Message {
  /**
   * The SOURCE markdown — what a copy control copies, never the rendered HTML.
   * A signal's raw event document is appended fenced, as the card shows it.
   */
  source: string
}

/** The markdown a message was sent as. */
export function messageSource(m: Message): string {
  return m.payload ? `${m.text}\n\n${fence(m.payload)}` : m.text
}

/** Every input any run recorded a non-empty text for, oldest first — the durable half of a conversation's questions. */
export function recordedInputs(runs: Run[] | undefined): RecordedInput[] {
  return (runs ?? []).flatMap((r) => (r.inputs ?? []).filter((i) => i.text))
}

/** The text a conversation was actually given — its first recorded input. For a member this IS its task. */
export function firstTaskInput(runs: Run[] | undefined): RecordedInput | undefined {
  return recordedInputs(runs)[0]
}

/** The most recent run that actually answered. */
export function lastResult(runs: Run[] | undefined): { text: string; at?: string } | undefined {
  const r = (runs ?? []).filter((x) => x.result).at(-1)
  return r ? { text: r.result as string, at: r.finishedAt || r.startedAt } : undefined
}

/**
 * The coordinator's OWN run that was executing when it invoked a member.
 * `invoke` is a synchronous tool call made mid-run, and runs are strictly
 * serial (invariants.md), so at most one run can have been in flight at the
 * member's creation instant.
 */
export function invokingRun(runs: Run[] | undefined, memberCreatedAt: number): Run | undefined {
  if (!memberCreatedAt) return undefined
  return (runs ?? []).find((r) => {
    const start = Date.parse(r.startedAt || '') || 0
    if (!start || memberCreatedAt < start) return false
    if (!r.finishedAt) return true // still running when this member was created
    return memberCreatedAt <= (Date.parse(r.finishedAt) || 0)
  })
}

/**
 * Where a member's invoke card belongs in transcript time order.
 *
 * `member.created` is mid-run, well before the coordinator's own reasoning for
 * that run is recorded, so sorting by it reverses decide-then-act. Neither is
 * `run.finishedAt` the reasoning's clock: the message's own `at` is stamped by
 * the console's buffer at RECEIPT, measured two seconds apart on an ordinary
 * run. The card anchors to the reasoning message's OWN timestamp: the first
 * agent message at or after the invoking run's start is that run's result,
 * by the strictly-serial invariant.
 *
 * Falls back to the run's own `finishedAt`/`startedAt`, then the member's
 * creation, when no run's window covers it — `status.runs[]` keeps only the
 * last ten.
 */
export function invokeCardAt(runs: Run[] | undefined, member: ConversationSummary, transcript: Message[] | null | undefined): number {
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
 * A member's own transcript, opened directly: the task it was given, then its
 * result. It binds no human channel (invariants.md), so its runs' records are
 * the only place either lives.
 */
export function memberTranscript(conversation: ConversationSummary, runs: Run[] | undefined): Message[] {
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
  return messages
}

/**
 * The parent chain of a member, the uncaused root first, the member itself
 * excluded — its title is already the heading. There is no "incident" entity
 * in this domain: the chain is labelled by the Coordinator the root names.
 */
export function memberChain(conversation: ConversationSummary, items: ConversationSummary[]) {
  const byName = new Map(items.map((c) => [c.name, c]))
  const chain: ConversationSummary[] = []
  let cur: ConversationSummary | undefined = byName.get(conversation.name) ?? conversation
  const seen = new Set<string>()
  while (cur) {
    chain.unshift(cur)
    if (!cur.causedBy || seen.has(cur.name)) break
    seen.add(cur.name)
    cur = byName.get(cur.causedBy.parent)
  }
  return { coordinator: chain[0]?.coordinator, ancestors: chain.slice(0, -1) }
}

/** How a step of a member's chain is named: the entry it was invoked as, else its title. */
export function chainStepLabel(step: ConversationSummary): string {
  return step.causedBy?.entry ?? stripLeadingIcon(step.title || step.name)
}

/**
 * One conversation, everything a thread view shows of it.
 *
 * Opening a conversation the viewer joined and has unread activity in marks it
 * read, once per new activity — a look does not decide when.
 */
export function useThread(name: string) {
  const { data, isLoading, error, refetch } = useConversation(name)
  const vocabulary = useVocabulary()
  const session = useSession()
  const live = useStream((s) => s.events)
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
    // markRead is a fresh object every render; the stamp is what decides.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [summary?.name, joinedUnread, activity])

  const events: ActivityEvent[] = useMemo(
    () => (data ? mergeEvents(data.events ?? [], eventsFor(data.conversation.name, live)) : []),
    [data, live],
  )
  const messages: ThreadMessage[] = useMemo(
    () => (data?.transcript ?? []).map((m) => ({ ...m, source: messageSource(m) })),
    [data?.transcript],
  )
  const runDetails = useMemo(
    () => attachRunDetails(messages, summary?.runs, events),
    [messages, summary?.runs, events],
  )
  const working = useWorkingRun(summary, events)

  // A reply the viewer posted is ACCEPTED until its message turns durable —
  // that is the event that confirms it (console-workspace-feedback: "A reply
  // settles on its event").
  useEffect(() => {
    for (const m of messages) if (!m.pending) confirmRef(name, m.id)
  }, [messages, name])

  const pipelineIcon = vocabulary.data?.entries.find(
    (e) => e.kind === 'pipeline' && e.name === summary?.pipeline,
  )?.icon

  return {
    isLoading,
    error,
    refetch,
    /** The conversation detail as served — for what the thread model does not shape (YAML, metadata). */
    detail: data as ConversationDetail | undefined,
    conversation: summary,
    title: summary ? stripLeadingIcon(summary.title || summary.name) : '',
    pipelineIcon,
    /** A Coordinator's own, uncaused conversation. */
    isRoot: Boolean(summary?.coordinator),
    /** A conversation a Coordinator invoked. */
    isMember: Boolean(summary?.causedBy),
    /** When the conversation began. */
    startedAt: summary?.created,
    messages,
    signals: messages.filter((m) => m.kind === 'signal'),
    runs: summary?.runs ?? [],
    /** The conversation's activity: the window the page opened with, then the stream, once each. */
    events,
    /** The run in flight, as one fact; undefined when none is. */
    working,
    /** Each finished run's tools and events, keyed by the id of the reply it produced. */
    runDetails,
    /** The viewer may write here: the install allows it, the console holds a thread, and it is not archived. */
    canWrite: Boolean(session.data?.canWrite && summary?.joined && !data?.archived),
  }
}

export type Thread = ReturnType<typeof useThread>

/** How many conversations a thread reads to find its members and ancestors. */
const MEMBER_SCAN_LIMIT = '200'

function useMemberScan() {
  const params = useMemo(() => new URLSearchParams({ limit: MEMBER_SCAN_LIMIT }), [])
  return useConversations(params)
}

/**
 * The members a Coordinator root invoked directly, each with the time its
 * invocation belongs at in the root's transcript.
 */
export function useThreadMembers(conversation: ConversationSummary, transcript: Message[] | null | undefined) {
  const scan = useMemberScan()
  const members = useMemo(
    () => (scan.data?.items ?? [])
      .filter((m) => m.causedBy?.parent === conversation.name)
      .map((member) => ({ member, at: invokeCardAt(conversation.runs, member, transcript) })),
    [scan.data, conversation.name, conversation.runs, transcript],
  )
  return { isLoading: scan.isLoading, error: scan.error, loaded: Boolean(scan.data), members }
}

/** A member's ancestors and the Coordinator its chain belongs to. */
export function useMemberChain(conversation: ConversationSummary) {
  const scan = useMemberScan()
  return useMemo(() => memberChain(conversation, scan.data?.items ?? []), [conversation, scan.data])
}

/**
 * What one member was asked and what it answered — read only once `enabled`,
 * so a folded invocation costs nothing.
 */
export function useMemberExchange(name: string, enabled = true) {
  const detail = useConversation(name, enabled)
  const runs = detail.data?.conversation.runs
  return {
    isLoading: detail.isLoading,
    error: detail.error,
    loaded: Boolean(detail.data),
    conversation: detail.data?.conversation,
    task: firstTaskInput(runs),
    result: lastResult(runs),
  }
}
