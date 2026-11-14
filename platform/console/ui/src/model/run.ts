import { useEffect, useMemo, useState } from 'react'
import type { ActivityEvent, ConversationSummary, Message, Run, RunToolCall } from '../api/types'

// RUN DETAIL IS SHAPED HERE (design D8). A look never parses events itself:
//
//   - while a run works, the model exposes it as `{ elapsed, latest, count }`
//     — one stable fact, however many events arrive;
//   - when the reply lands, the run's tools and events are ATTACHED to that
//     reply, so a look can fold them under it (`workspace`) or keep drawing
//     them inline (`classic`, console-thread-live-cues). Both read the same
//     data.
//
// The same holds whether the viewer started the run with a reply or a signal
// started it: a run is keyed by its run id, never by what caused it.

/** The lifecycle events of a run, and how each reads. */
export const RUN_EVENT_LABELS: Readonly<Record<string, string>> = {
  'run.dispatched': 'run dispatched',
  'runtime.starting': 'runtime starting',
  'context.restored': 'context restored',
  'context.checkpoint': 'context checkpoint',
  'context.skipped': 'context skipped',
  'context.failed': 'context failed',
  'run.completed': 'run completed',
}

export function isRunEvent(e: ActivityEvent): boolean {
  return Object.hasOwn(RUN_EVENT_LABELS, e.kind)
}

export function runEventLabel(kind: string): string {
  return Object.hasOwn(RUN_EVENT_LABELS, kind) ? RUN_EVENT_LABELS[kind] : kind
}

/** One run's lifecycle events, in arrival order. */
export interface RunGroup {
  runId: string
  events: ActivityEvent[]
  /** When the run's first event happened, epoch ms. */
  at: number
}

/** Groups run-shaped events sharing one `runId` (design D-G). An event with no run id is its own group. */
export function groupRunEvents(events: ActivityEvent[]): RunGroup[] {
  const groups = new Map<string, RunGroup>()
  const order: string[] = []
  for (const e of events) {
    if (!isRunEvent(e)) continue
    const key = e.runId || e.cursor
    let group = groups.get(key)
    if (!group) {
      group = { runId: e.runId || '', events: [], at: Date.parse(e.ts) || 0 }
      groups.set(key, group)
      order.push(key)
    }
    group.events.push(e)
  }
  return order.map((k) => groups.get(k) as RunGroup)
}

/**
 * The run a reply answers, read from its id where the console derived the id
 * from the run: `run:<runId>` for a reply rebuilt from the run record,
 * `send:<conversation>:<channel>:<runId>` for a delivered one (`internal/chat/ops.go`). Undefined for any other id.
 */
export function runIdOfMessage(id: string): string | undefined {
  if (id.startsWith('run:')) return id.slice('run:'.length) || undefined
  const parts = id.split(':')
  if (parts.length >= 4 && parts[0] === 'send') return parts.slice(3).join(':') || undefined
  return undefined
}

/** A run in flight, as one fact. */
export interface WorkingRun {
  runId?: string
  /** Milliseconds since the run was dispatched — zero when nothing says when. */
  elapsed: number
  /** The most recent lifecycle event, already labelled. Absent before the first. */
  latest?: { kind: string; label: string; at: string }
  /** How many lifecycle events the run has produced so far. */
  count: number
}

/**
 * The run working right now, or undefined when none is. `now` is passed in so
 * the same inputs always give the same answer.
 */
export function workingRun(conversation: ConversationSummary, events: ActivityEvent[], now: number): WorkingRun | undefined {
  if (!conversation.presence) return undefined
  const runId = conversation.inflight?.runId
  const groups = groupRunEvents(events)
  // The inflight run's own group; with no run id named, the newest group that
  // has not completed.
  const group = runId
    ? groups.find((g) => g.runId === runId)
    : groups.filter((g) => !g.events.some((e) => e.kind === 'run.completed')).at(-1)
  const own = group?.events ?? []
  const last = own.at(-1)
  const dispatched = Date.parse(conversation.inflight?.dispatchedAt ?? '') || group?.at || 0
  return {
    runId: runId ?? (group?.runId || undefined),
    elapsed: dispatched ? Math.max(0, now - dispatched) : 0,
    latest: last ? { kind: last.kind, label: runEventLabel(last.kind), at: last.ts } : undefined,
    count: own.length,
  }
}

/** What a finished run did, attached to the reply it produced. */
export interface RunDetails {
  runId: string
  /** The run's durable record, where the conversation still holds it (the last ten). */
  run?: Run
  /** The tools the run called — empty when the runtime reported none. */
  tools: RunToolCall[]
  /** The run's lifecycle events still in the activity window — empty once they scrolled out. */
  events: ActivityEvent[]
}

const isReply = (m: Message) => m.kind === 'agent'

/**
 * Attaches each finished run's tools and events to the reply it produced,
 * keyed by that reply's message id. A run with no reply yet attaches to
 * nothing — it is still the working run, and its details arrive with the
 * reply.
 *
 * The reply is found by the run id its message id carries; failing that, it
 * is the first reply at or after the run started that no other run claimed
 * (runs are strictly serial per conversation, so there is one candidate).
 */
export function attachRunDetails(messages: Message[], runs: Run[] | undefined, events: ActivityEvent[]): Map<string, RunDetails> {
  const out = new Map<string, RunDetails>()
  const groups = new Map(groupRunEvents(events).filter((g) => g.runId).map((g) => [g.runId, g]))
  const records = new Map((runs ?? []).map((r) => [r.runId, r]))
  const runIds = [...new Set([...records.keys(), ...groups.keys()])]
  const replies = messages.filter(isReply)
  const claimed = new Set<string>()
  const byId = new Map<string, Message>()
  for (const m of replies) {
    const runId = runIdOfMessage(m.id)
    if (runId) byId.set(runId, m)
  }
  const startOf = (runId: string) =>
    Date.parse(records.get(runId)?.startedAt ?? '') || groups.get(runId)?.at || 0
  // Earliest run first, so the time fallback hands each reply to the run that
  // began before it.
  runIds.sort((a, b) => startOf(a) - startOf(b))
  for (const runId of runIds) {
    let reply = byId.get(runId)
    if (!reply) {
      const start = startOf(runId)
      if (!start) continue
      reply = replies.find((m) => !claimed.has(m.id) && !runIdOfMessage(m.id) && (Date.parse(m.at) || 0) >= start)
    }
    if (!reply || claimed.has(reply.id)) continue
    claimed.add(reply.id)
    const run = records.get(runId)
    out.set(reply.id, { runId, run, tools: run?.toolCalls ?? [], events: groups.get(runId)?.events ?? [] })
  }
  return out
}

/** How often a working run's elapsed time advances, in ms. */
export const ELAPSED_TICK_MS = 1000

/** The working run, its elapsed time kept current while it works. */
export function useWorkingRun(conversation: ConversationSummary | undefined, events: ActivityEvent[]): WorkingRun | undefined {
  const working = Boolean(conversation?.presence)
  const [now, setNow] = useState(() => Date.now())
  // A display clock, never a refresh: it re-reads nothing, it only lets the
  // elapsed time advance. One timer per tick, re-armed by the tick itself and
  // dropped the moment the run stops working — the replay's own pattern.
  useEffect(() => {
    if (!working) return
    const timer = setTimeout(() => setNow(Date.now()), ELAPSED_TICK_MS)
    return () => clearTimeout(timer)
  }, [working, now])
  return useMemo(
    () => (conversation ? workingRun(conversation, events, now) : undefined),
    [conversation, events, now],
  )
}
