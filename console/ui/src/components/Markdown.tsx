import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { plain } from './Text'

/**
 * Agent output, RENDERED.
 *
 * The contract says prose is markdown and each surface renders what it can.
 * Telegram can do a little; a browser can do all of it — and a namespace table
 * arriving as thirty lines of pipes is the case that proves the difference.
 *
 * SAFETY IS UNCHANGED. Raw HTML is never enabled, so no `dangerouslySetInnerHTML`
 * exists here any more than it does in PlainText, and the input is passed
 * through the same tag-stripping first: an agent's output is free text and "no
 * tag reaches the DOM" is a property this app keeps whatever upstream promises.
 *
 * Tables go through PatternFly so they inherit the console's own styling rather
 * than arriving unstyled, and every block that can be too wide scrolls INSIDE
 * itself — a transcript must never make the page scroll sideways.
 */
export function Markdown({ children }: { children?: string | null }) {
  if (!children) return null
  return (
    // ISOLATED FROM THE PAGE. Nothing inside may size the layout around it:
    // the block is at most as wide as what holds it, and anything larger
    // scrolls within itself.
    <div style={{ minWidth: 0, maxWidth: '100%', overflowWrap: 'anywhere' }}>
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        components={{
          // SIDEWAYS ONLY.
          //
          // A long table is just a long message, and the transcript already
          // scrolls — giving it a second, vertical scrollbar of its own is the
          // "which one am I in" problem one level down.
          //
          // Width is the exception: the page must never scroll sideways, so a
          // table wider than the message scrolls within itself instead.
          //
          // A plain <table>, not the console's page-level one: that component
          // brings page layout with it, which is the opposite of contained.
          table: ({ children }) => (
            <div
              style={{
                maxWidth: '100%',
                overflowX: 'auto',
                margin: '0.5em 0',
                border: '1px solid var(--ao-border)',
                borderRadius: '0.25em',
              }}
            >
              <table style={{ borderCollapse: 'collapse', width: '100%' }}>{children}</table>
            </div>
          ),
          th: ({ children }) => (
            <th
              style={{
                textAlign: 'left',
                padding: '0.35em 0.6em',
                borderBottom: '1px solid var(--ao-border)',
                background: 'var(--ao-surface-alt)',
                whiteSpace: 'nowrap',
              }}
            >
              {children}
            </th>
          ),
          td: ({ children }) => (
            <td style={{ padding: '0.3em 0.6em', borderBottom: '1px solid var(--ao-border)', verticalAlign: 'top' }}>
              {children}
            </td>
          ),
          // Fenced code is a machine document: monospaced, and scrolling in its
          // own box rather than widening the thread.
          pre: ({ children }) => (
            <pre
              style={{
                // Sideways only, for the same reason a table is: a long code
                // block is a long message, and the transcript scrolls already.
                overflowX: 'auto',
                background: 'var(--ao-surface-alt)',
                border: '1px solid var(--ao-border)',
                borderRadius: '0.25em',
                padding: '0.6em 0.75em',
                margin: '0.5em 0',
              }}
            >
              {children}
            </pre>
          ),
          code: ({ children, className }) =>
            className ? (
              <code className={className}>{children}</code>
            ) : (
              <code
                style={{
                  background: 'var(--ao-surface-alt)',
                  borderRadius: 3,
                  padding: '0.05rem 0.3rem',
                }}
              >
                {children}
              </code>
            ),
          // Links open away from the console, and cannot reach back into it.
          a: ({ children, href }) => (
            <a href={href} target="_blank" rel="noreferrer noopener">
              {children}
            </a>
          ),
          p: ({ children }) => <p style={{ margin: '0.4rem 0' }}>{children}</p>,
          ul: ({ children }) => <ul style={{ margin: '0.4rem 0', paddingLeft: '1.5rem' }}>{children}</ul>,
          ol: ({ children }) => <ol style={{ margin: '0.4rem 0', paddingLeft: '1.5rem' }}>{children}</ol>,
        }}
      >
        {plain(children)}
      </ReactMarkdown>
    </div>
  )
}
