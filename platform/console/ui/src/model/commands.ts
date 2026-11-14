import { useEffect, useMemo } from 'react'
import { create } from 'zustand'
import { useStream } from '../api/stream'
import type { ActivityEvent } from '../api/types'

// THE VIEWER'S OWN ACTIONS, AND WHERE EACH ONE IS (design D7,
// console-workspace-feedback: "The viewer's own actions show their state").
//
// Every action is tracked by its TARGET — a conversation's name, or a fixed
// key such as `new-conversation` — through four states:
//
//   sending   the request is out, nothing has answered
//   accepted  the server took it and the work happens later; waiting for the
//             event that confirms it
//   done      it happened
//   failed    it did not, and `error` says why
//
// The states are DERIVED, never guessed: `sending` → `accepted` or `done` from
// the response, `accepted` → `done` from the confirming event. `accepted` is
// OPTIONAL — a verb whose response already means "performed" goes straight
// from sending to done, which is every verb the console serves synchronously
// until `async-command-bus` reaches it, and a look shows that without change.
//
// The store is shared on purpose: the header counts what is in flight across
// every view, so one view's action must be visible to another. It holds plain
// data only; the request itself is the caller's, passed in as `perform`.

export type CommandState = 'sending' | 'accepted' | 'done' | 'failed'

export type Verb =
  | 'send'
  | 'start'
  | 'exit'
  | 'close'
  | 'reopen'
  | 'delete'
  | 'markRead'
  | 'markUnread'

/**
 * Whether repeating a verb after a failure is safe — the one test retry is
 * offered on.
 *
 * A failed request may still have reached the server: the connection can drop
 * after the write landed. So a verb is safe to repeat only when doing it twice
 * leaves the same state as doing it once.
 *
 * - SAFE. `close` (a second close of a closed conversation is a `skipped`
 *   outcome), `reopen` (reopening an open one changes nothing), `exit`
 *   (releasing an already released runtime is a no-op, and it is refused
 *   mid-run rather than repeated), `delete` (a deleted conversation is
 *   `skipped`), `markRead`/`markUnread` (a watermark set twice is set once).
 * - NOT SAFE. `send` (a reply posted twice is two messages to the agent, and
 *   two runs) and `start` (a second conversation, a second runtime). The
 *   viewer decides whether to send again, knowing what they see.
 */
export const RETRY_SAFE: Readonly<Record<Verb, boolean>> = {
  send: false,
  start: false,
  exit: true,
  close: true,
  reopen: true,
  delete: true,
  markRead: true,
  markUnread: true,
}

export interface Command {
  id: number
  target: string
  verb: Verb
  state: CommandState
  /** What did not happen, in the server's own words where it gave any. */
  error?: string
  /** What an accepted response named to wait for — a reply's pending message id. */
  ref?: string
  /** Offered only for a failed command of a verb safe to repeat. */
  retryable: boolean
  startedAt: number
  settledAt?: number
}

/**
 * How a response is read. Returns the reference to wait for when the server
 * only ACCEPTED the action, or nothing when the response means it is done.
 * A verb with no reader is synchronous.
 */
export type AcceptedReader<T> = (response: T) => string | undefined

export interface CommandSpec<T> {
  target: string
  verb: Verb
  perform: () => Promise<T>
  accepted?: AcceptedReader<T>
  /** The activity event that completes an accepted command, where one does. */
  confirmedBy?: (event: ActivityEvent, command: Command) => boolean
}

export type Outcome<T> = { ok: true; response: T; command: Command } | { ok: false; error: string; command: Command }

/** How many settled commands are kept for a look to show their result. */
const KEEP_SETTLED = 100

interface Pending {
  // Kept outside the published state: functions are not data a look renders.
  perform: () => Promise<unknown>
  accepted?: AcceptedReader<unknown>
  confirmedBy?: CommandSpec<unknown>['confirmedBy']
}

interface CommandStore {
  commands: Command[]
  /** The newest activity cursor already checked for confirmations. */
  observed: string
}

const specs = new Map<number, Pending>()
let nextId = 1

export const useCommandStore = create<CommandStore>(() => ({ commands: [], observed: '' }))

/** Clears every command. For tests, and for a sign-out. */
export function resetCommands() {
  specs.clear()
  nextId = 1
  useCommandStore.setState({ commands: [], observed: '' })
}

function patch(id: number, change: Partial<Command>) {
  useCommandStore.setState((s) => ({
    commands: s.commands.map((c) => (c.id === id ? { ...c, ...change } : c)),
  }))
}

function settle(id: number, state: 'done' | 'failed', error?: string) {
  const command = useCommandStore.getState().commands.find((c) => c.id === id)
  if (!command) return
  const retryable = state === 'failed' && RETRY_SAFE[command.verb]
  // A settled command keeps its spec only while a retry may still use it.
  if (!retryable) specs.delete(id)
  patch(id, { state, error, retryable, settledAt: Date.now() })
  prune()
}

function prune() {
  const { commands } = useCommandStore.getState()
  const settled = commands.filter((c) => c.state === 'done' || c.state === 'failed')
  if (settled.length <= KEEP_SETTLED) return
  const drop = new Set(settled.slice(0, settled.length - KEEP_SETTLED).map((c) => c.id))
  for (const id of drop) specs.delete(id)
  useCommandStore.setState({ commands: commands.filter((c) => !drop.has(c.id)) })
}

/** What a thrown request says did not happen — the server's explanation where it gave one. */
export function errorMessage(e: unknown): string {
  if (typeof e === 'string') return e
  const message = (e as { message?: unknown } | null | undefined)?.message
  return typeof message === 'string' ? message : 'the request failed'
}

/**
 * The full explanation of a refusal: the message plus the server's own
 * `message` and `fix` where the body carries them (a start refused for an
 * unwired source names the patch to apply).
 */
export function describeError(e: unknown): string {
  const body = (e as { body?: Record<string, unknown> } | undefined)?.body ?? {}
  const extra = [body.message, body.fix].filter((v): v is string => typeof v === 'string' && v !== '')
  return [errorMessage(e), ...extra].join(' — ')
}

async function execute<T>(id: number): Promise<Outcome<T>> {
  const spec = specs.get(id) as Pending | undefined
  const current = () => useCommandStore.getState().commands.find((c) => c.id === id) as Command
  if (!spec) return { ok: false, error: 'nothing to run', command: current() }
  try {
    const response = (await spec.perform()) as T
    const ref = spec.accepted?.(response)
    if (ref !== undefined) {
      patch(id, { state: 'accepted', ref })
    } else {
      settle(id, 'done')
    }
    return { ok: true, response, command: current() }
  } catch (e) {
    const error = describeError(e)
    settle(id, 'failed', error)
    return { ok: false, error, command: current() }
  }
}

/** Starts an action. Never throws: a failure is an outcome, and the command says why. */
export function runCommand<T>(spec: CommandSpec<T>): Promise<Outcome<T>> {
  const id = nextId++
  specs.set(id, spec as CommandSpec<unknown>)
  const command: Command = {
    id, target: spec.target, verb: spec.verb, state: 'sending', retryable: false, startedAt: Date.now(),
  }
  useCommandStore.setState((s) => ({ commands: [...s.commands, command] }))
  return execute<T>(id)
}

/**
 * Runs a failed command again, as the same command. Refused — returns
 * undefined and sends nothing — for a verb not safe to repeat, or a command
 * that has not failed.
 */
export function retryCommand(id: number): Promise<Outcome<unknown>> | undefined {
  const command = useCommandStore.getState().commands.find((c) => c.id === id)
  if (!command?.retryable || !specs.has(id)) return undefined
  patch(id, { state: 'sending', error: undefined, retryable: false, settledAt: undefined })
  return execute(id)
}

/**
 * Settles every accepted command of a target and verb as done — the confirming
 * event arrived. With `ref`, only the command that named it.
 */
export function confirmCommand(target: string, verb: Verb, ref?: string) {
  for (const c of useCommandStore.getState().commands) {
    if (c.state !== 'accepted' || c.target !== target || c.verb !== verb) continue
    if (ref !== undefined && c.ref !== ref) continue
    settle(c.id, 'done')
  }
}

/** Settles the accepted command on a target that named `ref` — its confirming message arrived. */
export function confirmRef(target: string, ref: string) {
  for (const c of useCommandStore.getState().commands) {
    if (c.state === 'accepted' && c.target === target && c.ref === ref) settle(c.id, 'done')
  }
}

/** Forgets one settled command — a look dismissing a result it has shown. */
export function dismissCommand(id: number) {
  specs.delete(id)
  useCommandStore.setState((s) => ({ commands: s.commands.filter((c) => c.id !== id) }))
}

/**
 * Checks arriving activity events against every accepted command that names a
 * confirming event. Events at or before the last cursor checked are skipped,
 * so every caller may feed the whole buffer.
 */
export function observeEvents(events: ActivityEvent[]) {
  const { observed, commands } = useCommandStore.getState()
  const fresh = events.filter((e) => e.cursor && e.cursor > observed)
  if (fresh.length === 0) return
  const newest = fresh.reduce((max, e) => (e.cursor > max ? e.cursor : max), observed)
  useCommandStore.setState({ observed: newest })
  for (const c of commands) {
    if (c.state !== 'accepted') continue
    const confirmedBy = specs.get(c.id)?.confirmedBy
    if (confirmedBy && fresh.some((e) => confirmedBy(e, c))) settle(c.id, 'done')
  }
}

/** Whether a command is still awaiting its outcome. */
export function inFlight(c: Command): boolean {
  return c.state === 'sending' || c.state === 'accepted'
}

export type Connection = 'connected' | 'disconnected'

/**
 * The viewer's actions and the live stream they settle on.
 *
 * While the stream is down nothing confirms an accepted command, so it holds
 * where it is and resumes when the stream returns — the console keeps showing
 * what it has and says the connection is lost (console-workspace-feedback).
 */
export function useCommands() {
  const commands = useCommandStore((s) => s.commands)
  const connected = useStream((s) => s.connected)
  const events = useStream((s) => s.events)
  useEffect(() => observeEvents(events), [events])
  return useMemo(() => {
    const latest = (target: string, verb?: Verb): Command | undefined => {
      for (let i = commands.length - 1; i >= 0; i--) {
        const c = commands[i]
        if (c.target === target && (verb === undefined || c.verb === verb)) return c
      }
      return undefined
    }
    return {
      commands,
      /** How many of the viewer's actions are awaiting their outcome. Zero shows nothing. */
      inFlight: commands.filter(inFlight).length,
      connection: (connected ? 'connected' : 'disconnected') as Connection,
      /** The newest command on a target, of one verb or any. */
      latest,
      /** Whether anything on a target is still being sent. */
      sending: (target: string) => commands.some((c) => c.target === target && c.state === 'sending'),
      run: runCommand,
      retry: retryCommand,
      confirm: confirmCommand,
      confirmRef,
      dismiss: dismissCommand,
    }
  }, [commands, connected])
}
