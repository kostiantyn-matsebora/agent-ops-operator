import { parse } from '../api/blocks'
import { fence } from '../api/fence'
import type { Block, ConversationSummary, Message } from '../api/types'

// ONE MESSAGE, AS EVERY LOOK READS IT: who is speaking, whether its text
// carries the block grammar, a signal's severity and attributes, and the text a
// copy control copies. A look draws these and decides none of them.

/** A signal's urgency, the contract's closed vocabulary. */
export const SEVERITIES = ['critical', 'error', 'warning', 'info'] as const
export type Severity = (typeof SEVERITIES)[number]

/**
 * A signal's severity, or undefined when it is unrated. A value outside the
 * contract's vocabulary is unrated too: guessing a tint for a word nobody
 * defined would rate it.
 */
export function signalSeverity(m: Pick<Message, 'severity'>): Severity | undefined {
  return (SEVERITIES as readonly string[]).includes(m.severity ?? '') ? (m.severity as Severity) : undefined
}

/** Who a message belongs to, for drawing. `hidden` is an acknowledgement no thread shows. */
export type Speaker = 'signal' | 'agent' | 'person' | 'notice' | 'hidden'

export function speakerOf(m: Pick<Message, 'kind'>): Speaker {
  switch (m.kind) {
    case 'signal':
      return 'signal'
    case 'agent':
      return 'agent'
    case 'notice':
      return 'notice'
    // The manager's own receipt that it took a message — a delivery fact, never
    // something a reader is told twice.
    case 'ack':
      return 'hidden'
    default:
      // `local` (the viewer), `relay` (a person on another channel), `user`.
      return 'person'
  }
}

/** Whether a person's message is the viewer's own, rather than a relay from another channel. */
export function isOwn(m: Pick<Message, 'kind'>): boolean {
  return m.kind === 'local' || m.kind === 'user'
}

/**
 * The blocks an agent's text parses into, or undefined for any other speaker.
 * A person's words are never parsed: a tag they typed is their text.
 */
export function replyBlocks(m: Pick<Message, 'kind' | 'text'>): Block[] | undefined {
  return m.kind === 'agent' ? parse(m.text) : undefined
}

/** A signal's attributes, as label and value, the severity first. */
export function signalAttributes(m: Message, conversation?: Pick<ConversationSummary, 'source'>): [string, string][] {
  const rows: [string, string][] = [['severity', signalSeverity(m) ?? 'unrated']]
  if (conversation?.source) rows.push(['source', conversation.source])
  if (m.sender) rows.push(['sender', m.sender])
  return rows
}

/**
 * What copying a message puts on the clipboard: the markdown it was written
 * as, never rendered HTML. A signal copies its body, its attributes as
 * readable lines, and its raw event document fenced.
 */
export function copyText(m: Message, conversation?: Pick<ConversationSummary, 'source'>): string {
  if (m.kind !== 'signal') return m.text
  const attributes = signalAttributes(m, conversation).map(([k, v]) => `${k}: ${v}`).join('\n')
  const parts = [m.text.trim(), attributes]
  if (m.payload) parts.push(fence(m.payload))
  return parts.filter(Boolean).join('\n\n')
}

/** The kinds that count toward unread: what a person has to read. */
export const COUNTED_KINDS: ReadonlySet<string> = new Set(['signal', 'agent', 'relay'])

/**
 * The first message the viewer has not read: the first counted one after their
 * read watermark. Absent when there is no watermark (a shared token keeps
 * none) or everything was read.
 */
export function firstUnreadId(messages: Message[], readAt: string | undefined): string | undefined {
  const watermark = readAt ? Date.parse(readAt) : undefined
  if (watermark === undefined || Number.isNaN(watermark)) return undefined
  return messages.find((m) => m.kind !== 'ack' && COUNTED_KINDS.has(m.kind) && Date.parse(m.at) > watermark)?.id
}
