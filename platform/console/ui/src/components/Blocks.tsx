import { useState } from 'react'
import type { ReactElement } from 'react'
import type { Block } from '../api/types'
import { Markdown } from './Markdown'

/**
 * Agent output, AS STRUCTURE.
 *
 * The manager parses an answer into blocks and this renders them as elements:
 * the title as a heading, each named section under its own label, and the fold
 * behind a disclosure control that starts closed.
 *
 * The parse lives in `api/blocks.ts` and runs HERE IN THE BROWSER, on the
 * agent's own text — a live message and a transcript rebuilt from
 * `status.runs[].result` carry the same characters, which is why history renders
 * identically to live.
 *
 * Still no sanitizer: blocks become elements, and no string is ever handed to
 * the DOM as markup. That property is unchanged from PlainText and Markdown.
 *
 * The section vocabulary is OPEN. Every label below is rendered generically
 * from whatever the agent called it, and this file contains no agent's section
 * names.
 */
export function Blocks({ blocks }: { blocks?: Block[] | null }): ReactElement | null {
  if (!blocks || blocks.length === 0) return null
  return (
    <div style={{ minWidth: 0, maxWidth: '100%' }}>
      {blocks.map((b, i) => (
        <BlockView key={i} block={b} />
      ))}
    </div>
  )
}

function BlockView({ block }: { block: Block }): ReactElement | null {
  switch (block.role) {
    case 'title':
      // A heading, not an <h1>: this sits inside a message bubble in a
      // transcript, and a document-level heading would outrank the page's own.
      return (
        <div style={{ fontWeight: 600, fontSize: '1.05em', margin: '0.1rem 0 0.35rem' }}>
          <Markdown>{block.text}</Markdown>
        </div>
      )
    case 'details':
      return <Fold text={block.text} />
    default:
      return (
        <section style={{ margin: '0.35rem 0' }}>
          {block.label && (
            <div
              style={{
                fontWeight: 600,
                color: 'var(--ao-text-subtle)',
                textTransform: 'uppercase',
                fontSize: '0.78em',
                letterSpacing: '0.04em',
                marginBottom: '0.1rem',
              }}
            >
              {humanLabel(block.label)}
            </div>
          )}
          <Markdown>{block.text}</Markdown>
        </section>
      )
  }
}

/**
 * THE FOLD, COLLAPSED BY DEFAULT.
 *
 * A real control rather than a <details> element so it inherits the console's
 * own styling and focus behaviour, and so the summary line can say how much is
 * behind it — the thing that makes a reader decide whether to open it.
 */
export function Fold({ text, label = 'Details' }: { text: string; label?: string }): ReactElement {
  const [open, setOpen] = useState(false)
  const lines = text ? text.split('\n').length : 0
  return (
    <div style={{ margin: '0.5rem 0 0.2rem' }}>
      <button
        type="button"
        aria-expanded={open}
        onClick={() => setOpen((v) => !v)}
        style={{
          display: 'inline-flex',
          alignItems: 'center',
          gap: '0.4em',
          background: 'none',
          border: 'none',
          padding: '0.15rem 0',
          color: 'var(--ao-text-subtle)',
          cursor: 'pointer',
          font: 'inherit',
          fontSize: '0.85em',
        }}
      >
        <span aria-hidden style={{ display: 'inline-block', transform: open ? 'rotate(90deg)' : 'none' }}>
          ▸
        </span>
        {open ? `Hide ${label.toLowerCase()}` : `${label} (${lines} ${lines === 1 ? 'line' : 'lines'})`}
      </button>
      {open && (
        <div
          style={{
            borderLeft: '2px solid var(--ao-border)',
            paddingLeft: '0.75em',
            marginTop: '0.25rem',
          }}
        >
          <Markdown>{text}</Markdown>
        </div>
      )}
    </div>
  )
}

/**
 * humanLabel presents an agent's own tag name — `root-cause` reads as
 * "Root cause". Generic by construction: it transforms whatever it is given
 * and recognises nothing.
 */
export function humanLabel(label: string): string {
  const words = label.replace(/[-_]+/g, ' ').trim()
  if (!words) return ''
  return words.charAt(0).toUpperCase() + words.slice(1)
}

/**
 * agentText reports whether a transcript entry's text is an AGENT's, and so
 * carries the block grammar.
 *
 * The exclusions are the point. A RELAY and a locally-typed message are
 * somebody's words: parsing them would consume characters a person deliberately
 * wrote, and somebody asking why `<details>` will not render in their docs must
 * see their own text. A SIGNAL is not prose at all — it is a card.
 */
export function agentText(kind: string): boolean {
  return kind === 'agent' || kind === 'ack'
}
