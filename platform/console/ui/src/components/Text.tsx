import type { ReactElement } from 'react'

// PLAIN TEXT, ALWAYS.
//
// Every string the console renders comes from the cluster or the wire: a
// condition message a reconciler wrote, an agent's own output, a chat message a
// person typed, an opaque config block. None of it is trusted, and none of it is
// ours to interpret as markup.
//
// React escapes by default, so this component's job is not escaping — it is
// making the ONE dangerous alternative impossible to reach by accident. There is
// no dangerouslySetInnerHTML anywhere in this app; if a view wants formatting,
// it composes elements instead of handing a string to the DOM.
//
// The manager now sends SEMANTIC messages and each adapter renders its own
// surface, so this text should arrive as markdown rather than Telegram HTML.
// The stripping stays anyway: an agent's own output is free text, a Kubernetes
// object can carry anything, and "no tag reaches the DOM" is a property worth
// keeping independent of what any upstream promises this week.

const TAG = /<\/?[a-zA-Z][^>]*>/g
const ENTITIES: Record<string, string> = {
  '&amp;': '&',
  '&lt;': '<',
  '&gt;': '>',
  '&quot;': '"',
  '&#39;': "'",
  '&nbsp;': ' ',
}

/**
 * plain strips the chat HTML subset and decodes the entities the adapters
 * escape, so text arrives as the person actually typed it.
 */
export function plain(input: string): string {
  const withoutTags = input.replace(TAG, '')
  return withoutTags.replace(/&(amp|lt|gt|quot|#39|nbsp);/g, (m) => ENTITIES[m] ?? m)
}

export interface PlainTextProps {
  children?: string | null
  /** Render newlines as line breaks — transcripts and messages need it. */
  multiline?: boolean
  className?: string
}

/** PlainText renders cluster- and wire-sourced text, never as markup. */
export function PlainText({ children, multiline, className }: PlainTextProps): ReactElement | null {
  if (!children) return null
  const text = plain(children)
  if (!multiline) return <span className={className}>{text}</span>
  return (
    <span className={className} style={{ whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}>
      {text}
    </span>
  )
}

/**
 * RawText renders a string EXACTLY as it was recorded, tags included.
 *
 * PlainText strips anything tag-shaped, which is right for a rendered view and
 * WRONG for a record. `status.runs[].result` is what the agent printed — the
 * block grammar is part of that text, and stripping it made the runs view show
 * a version of the answer nobody ever produced. Silent deletion, in the one
 * place whose whole purpose is fidelity.
 *
 * IT IS STILL SAFE, and by the same mechanism as everything else here: React
 * escapes a text node. Nothing is handed to the DOM as markup, no
 * innerHTML is involved, and the app-wide rule is untouched. The stripping in
 * PlainText was belt-and-braces on top of that — useful where markup would be
 * noise, harmful where it is the content.
 */
export function RawText({ children, className }: PlainTextProps): ReactElement | null {
  if (!children) return null
  return (
    <span
      className={className}
      style={{ whiteSpace: 'pre-wrap', wordBreak: 'break-word', fontFamily: 'var(--pf-t--global--font--family--mono)' }}
    >
      {children}
    </span>
  )
}
