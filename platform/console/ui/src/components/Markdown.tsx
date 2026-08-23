import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import remarkBreaks from 'remark-breaks'
import rehypeHighlight from 'rehype-highlight'
import bash from 'highlight.js/lib/languages/bash'
import go from 'highlight.js/lib/languages/go'
import ini from 'highlight.js/lib/languages/ini'
import java from 'highlight.js/lib/languages/java'
import json from 'highlight.js/lib/languages/json'
import python from 'highlight.js/lib/languages/python'
import sql from 'highlight.js/lib/languages/sql'
import typescript from 'highlight.js/lib/languages/typescript'
import xml from 'highlight.js/lib/languages/xml'
import yaml from 'highlight.js/lib/languages/yaml'
import { plain } from './Text'

// SYNTAX HIGHLIGHTING, LANGUAGE BY LANGUAGE.
//
// `rehype-highlight` runs lowlight, which produces an ELEMENT TREE that
// react-markdown renders as React nodes. That is the whole reason it is used
// instead of highlight.js directly: hljs returns an HTML STRING, and rendering
// one would need dangerouslySetInnerHTML — the single thing this app does not
// do anywhere (Text.tsx, and a test that scans every source file).
//
// Grammars are listed EXPLICITLY. The full set is ~200 languages and most of a
// megabyte, for readers who look at cluster payloads. What an ops agent
// actually emits decides this list: Kubernetes objects and events, commands,
// this repo's own sources, queries. Add one when an agent starts producing it.
const LANGUAGES = { bash, go, ini, java, json, python, sql, typescript, xml, yaml }

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
        // A SINGLE NEWLINE IS A LINE BREAK, as it is in every chat client.
        //
        // Standard markdown collapses one into a space, which turns anything
        // the agent wrote on consecutive lines into one running paragraph. A
        // three-line evidence list arrived as a single sentence for exactly
        // this reason.
        //
        // format.md now tells the agent to write real `- ` lists, and this is
        // the floor under that instruction: a model that writes plain lines,
        // or a stray `•`, still reads as lines rather than as prose.
        remarkPlugins={[remarkGfm, remarkBreaks]}
        // `detect: false` — colour a block only when the AGENT tagged it.
        // Guessing turns a plain log excerpt into a randomly coloured one, and
        // format.md already tells the agent to tag every fence.
        //
        // `subset: false` keeps an unknown tag from being auto-detected into
        // whichever grammar looks closest.
        rehypePlugins={[[rehypeHighlight, { languages: LANGUAGES, detect: false, subset: false }]]}
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
                // Sideways always — and DOWNWARD once it is tall enough to
                // dominate the thread. A signal payload is thirty lines of
                // machine document, and unbounded it pushes the message that
                // explains it off the screen.
                //
                // This is the console's answer to what Telegram does with an
                // expandable quote: bound the height, keep every line reachable.
                // A short block is under the limit and unaffected, so nothing
                // that already fitted gains a scrollbar.
                maxHeight: '22em',
                overflowY: 'auto',
                // WRAP, DO NOT SCROLL SIDEWAYS.
                //
                // `pre` defaults to `white-space: pre`, so one long line — a
                // JSON string field, a log message, a prose payload — became a
                // horizontal scrollbar. Reading a sentence by dragging a bar is
                // worse than reading it on two lines.
                //
                // `pre-wrap` keeps the indentation that makes a payload legible
                // and breaks only where a line is too long for the column.
                whiteSpace: 'pre-wrap',
                overflowWrap: 'anywhere',
                // Still available for what genuinely cannot break — a single
                // unspaced token wider than the message.
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
