import { api } from '../api/client'
import type { Message } from '../api/types'
import { runCommand, type AcceptedReader, type Outcome, type Verb } from './commands'

// The console's write verbs, each run as a tracked command (commands.ts). A
// look calls these and never the API client — design D2, rule 2.

/** The target a start is tracked under: it has no conversation name until it exists. */
export const NEW_CONVERSATION = 'new-conversation'

/**
 * A message post is ACCEPTED, not performed: the console answers with the
 * viewer's own message marked pending, and the message turns durable once the
 * manager has delivered it. The pending message's id is what to wait for.
 * A response that is not a pending message means the post is already done.
 */
export const pendingMessage: AcceptedReader<unknown> = (response) => {
  const m = response as Partial<Message> | undefined
  return m?.pending && typeof m.id === 'string' ? m.id : undefined
}

/**
 * Which verb a typed message is. `/exit` and `/close` are thread commands
 * posted as messages, and they keep their own verb so retry follows the verb,
 * not the transport.
 */
export function verbOfText(text: string): Verb {
  const command = text.trim().split(/\s+/)[0]
  if (command === '/exit') return 'exit'
  if (command === '/close') return 'close'
  return 'send'
}

/** Posts a message — a reply, or a thread command — into a conversation. */
export function sendText(name: string, text: string): Promise<Outcome<unknown>> {
  return runCommand({ target: name, verb: verbOfText(text), perform: () => api.send(name, text), accepted: pendingMessage })
}

/**
 * Releases a conversation's runtime from outside its thread — the row's menu.
 *
 * Read as SYNCHRONOUS: the list does not watch the thread its confirming
 * message lands on, so an accepted reading would sit in flight with nothing to
 * settle it. The response is the outcome until the list can observe one.
 * `/exit` is fully recoverable (invariants.md), so it needs no confirmation
 * either way.
 */
export function exitRuntime(name: string): Promise<Outcome<unknown>> {
  return runCommand({ target: name, verb: 'exit', perform: () => api.send(name, '/exit') })
}

/** Starts a conversation on a source. Synchronous: the response is the outcome. */
export function startConversation(task: string, source?: string) {
  return runCommand({ target: NEW_CONVERSATION, verb: 'start', perform: () => api.start(task, source) })
}
