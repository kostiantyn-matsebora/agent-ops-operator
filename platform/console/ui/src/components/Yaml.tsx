import { useMemo, useState } from 'react'
import { Button, Card, CardBody, Split, SplitItem, Switch, Tooltip } from '@patternfly/react-core'

// A YAML viewer that preserves the document.
//
// The previous version handed the text to PatternFly's ClipboardCopy, which is a
// single-line INPUT: every newline collapsed and a Kubernetes object came out as
// one unreadable ribbon. YAML is whitespace-significant, so a viewer that eats
// the whitespace has destroyed the only thing that made it readable.
//
// Highlighting is a hand-written tokenizer rather than a syntax library:
// the input is YAML we generated ourselves from cluster JSON, the grammar we
// need is five token classes, and every token is rendered as a React text node —
// so there is no path where cluster-sourced text becomes markup. That property
// is worth more here than support for anchors and flow mappings.

type TokenKind = 'key' | 'string' | 'number' | 'literal' | 'comment' | 'punct' | 'text'

interface Token {
  kind: TokenKind
  text: string
}

// Both themes define these, so the highlighting follows the switcher without a
// second palette to keep in sync.
const COLORS: Record<TokenKind, string | undefined> = {
  key: 'var(--ao-brand)',
  string: 'var(--ao-success)',
  number: 'var(--ao-accent)',
  literal: 'var(--ao-accent)',
  comment: 'var(--ao-text-subtle)',
  punct: 'var(--ao-text-subtle)',
  text: undefined,
}

const KEY_LINE = /^(\s*)(-\s+)?([A-Za-z0-9_.\-/"']+)(:)(\s*)(.*)$/
const LITERALS = new Set(['true', 'false', 'null', '~', '{}', '[]'])

/** tokenizeLine splits one YAML line into display tokens. */
export function tokenizeLine(line: string): Token[] {
  const trimmed = line.trim()
  if (trimmed.startsWith('#')) return [{ kind: 'comment', text: line }]
  if (trimmed === '') return [{ kind: 'text', text: line }]

  const m = KEY_LINE.exec(line)
  if (m) {
    const [, indent, dash, key, colon, space, rest] = m
    const out: Token[] = []
    if (indent) out.push({ kind: 'text', text: indent })
    if (dash) out.push({ kind: 'punct', text: dash })
    out.push({ kind: 'key', text: key })
    out.push({ kind: 'punct', text: colon })
    if (space) out.push({ kind: 'text', text: space })
    if (rest) out.push(valueToken(rest))
    return out
  }

  // a bare sequence item: "- value"
  const seq = /^(\s*)(-\s+)(.*)$/.exec(line)
  if (seq) {
    const [, indent, dash, rest] = seq
    return [
      { kind: 'text', text: indent },
      { kind: 'punct', text: dash },
      valueToken(rest),
    ]
  }
  return [{ kind: 'text', text: line }]
}

function valueToken(raw: string): Token {
  const v = raw.trim()
  if (v.startsWith('#')) return { kind: 'comment', text: raw }
  if (LITERALS.has(v)) return { kind: 'literal', text: raw }
  if (v === '|' || v === '|-' || v === '>' || v === '>-') return { kind: 'punct', text: raw }
  if (/^-?\d+(\.\d+)?$/.test(v)) return { kind: 'number', text: raw }
  return { kind: 'string', text: raw }
}

export interface YamlProps {
  value: string
  /** Label used for the copy tooltip and the aria-label. */
  title?: string
}

export function Yaml({ value, title = 'YAML' }: YamlProps) {
  const [wrap, setWrap] = useState(false)
  const [copied, setCopied] = useState(false)
  const lines = useMemo(() => value.replace(/\n$/, '').split('\n'), [value])
  const gutter = String(lines.length).length

  async function copy() {
    try {
      await navigator.clipboard.writeText(value)
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    } catch {
      setCopied(false)
    }
  }

  return (
    <Card isCompact>
      <CardBody>
        <Split hasGutter style={{ marginBottom: 8, alignItems: 'center' }}>
          <SplitItem isFilled>
            <Switch
              id="yaml-wrap"
              label="Wrap long lines"
              isChecked={wrap}
              onChange={(_e, v) => setWrap(v)}
            />
          </SplitItem>
          <SplitItem>
            <Tooltip content={copied ? 'Copied' : `Copy ${title}`}>
              <Button variant="secondary" onClick={copy} aria-label={`copy ${title}`}>
                {copied ? 'Copied' : 'Copy'}
              </Button>
            </Tooltip>
          </SplitItem>
        </Split>
        <pre
          data-testid="yaml"
          aria-label={title}
          style={{
            margin: 0,
            padding: 12,
            maxHeight: '65vh',
            overflow: 'auto',
            borderRadius: 6,
            background: 'var(--ao-code-bg)',
            border: '1px solid var(--ao-border)',
            fontFamily: 'var(--pf-t--global--font--family--mono, monospace)',
            fontSize: 13,
            lineHeight: 1.5,
            whiteSpace: wrap ? 'pre-wrap' : 'pre',
            wordBreak: wrap ? 'break-word' : 'normal',
            tabSize: 2,
          }}
        >
          {lines.map((line, i) => (
            <div key={i} style={{ display: 'flex' }}>
              <span
                aria-hidden="true"
                style={{
                  userSelect: 'none',
                  textAlign: 'right',
                  minWidth: `${gutter + 1}ch`,
                  marginRight: 12,
                  color: 'var(--ao-text-subtle)',
                  opacity: 0.7,
                  flex: '0 0 auto',
                }}
              >
                {i + 1}
              </span>
              <span style={{ flex: '1 1 auto', whiteSpace: 'inherit' }}>
                {tokenizeLine(line).map((t, j) => (
                  <span key={j} style={{ color: COLORS[t.kind] }}>
                    {t.text}
                  </span>
                ))}
              </span>
            </div>
          ))}
        </pre>
      </CardBody>
    </Card>
  )
}
