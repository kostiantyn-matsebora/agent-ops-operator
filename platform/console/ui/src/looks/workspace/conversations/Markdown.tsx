import type { ReactElement, ReactNode } from 'react'
import ReactMarkdown from 'react-markdown'
import remarkBreaks from 'remark-breaks'
import remarkGfm from 'remark-gfm'

// Markdown as the workspace draws it.
//
// No raw HTML reaches the DOM: react-markdown drops it (`skipHtml`), and no
// string is ever handed over as markup.
//
// INLINE CODE IS QUIET (console-workspace-look): monospace and a faint ground,
// styled in workspace.css. A command long enough to wrap is set as its own
// block instead, so it does not break mid-flag across a sentence.

/** Inline code at least this long is a command block — the prototype's measure of a line that wraps. */
export const COMMAND_BLOCK_CHARS = 44

function textOf(node: ReactNode): string {
  if (typeof node === 'string' || typeof node === 'number') return String(node)
  if (Array.isArray(node)) return node.map(textOf).join('')
  return ''
}

export function isCommandBlock(text: string): boolean {
  return !text.includes('\n') && text.length >= COMMAND_BLOCK_CHARS
}

export function Markdown({ children }: Readonly<{ children?: string | null }>): ReactElement | null {
  if (!children) return null
  return (
    <div className="wk-md">
      <ReactMarkdown
        skipHtml
        remarkPlugins={[remarkGfm, remarkBreaks]}
        components={{
          table: ({ children: rows }) => (
            <div className="wk-tw">
              <table>{rows}</table>
            </div>
          ),
          code: ({ children: code, className }) => {
            const text = textOf(code)
            if (className || text.includes('\n')) return <code className={className}>{code}</code>
            return <code className={isCommandBlock(text) ? 'cmd' : undefined}>{code}</code>
          },
          a: ({ children: label, href }) => (
            <a href={href} target="_blank" rel="noreferrer noopener">
              {label}
            </a>
          ),
        }}
      >
        {children}
      </ReactMarkdown>
    </div>
  )
}
