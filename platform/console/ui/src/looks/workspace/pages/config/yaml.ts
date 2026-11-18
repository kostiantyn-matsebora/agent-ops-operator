// The YAML highlighter's tokenizer for the workspace look: five token classes,
// every token rendered as a text node, so cluster-sourced text never becomes
// markup. (The classic look has its own copy; looks do not import each other.)

export type TokenKind = 'key' | 'string' | 'number' | 'literal' | 'comment' | 'punct' | 'text'
export interface Token {
  kind: TokenKind
  text: string
}

const INDENT = /^\s*/
const DASH = /^-\s+/
const KEY_CHARS = /^[A-Za-z0-9_.\-/"']+/
const LITERALS = new Set(['true', 'false', 'null', '~', '{}', '[]'])

function valueToken(raw: string): Token {
  const v = raw.trim()
  if (v.startsWith('#')) return { kind: 'comment', text: raw }
  if (LITERALS.has(v)) return { kind: 'literal', text: raw }
  if (v === '|' || v === '|-' || v === '>' || v === '>-') return { kind: 'punct', text: raw }
  if (/^-?\d+(\.\d+)?$/.test(v)) return { kind: 'number', text: raw }
  return { kind: 'string', text: raw }
}

/** Splits one YAML line into display tokens. */
export function tokenizeLine(line: string): Token[] {
  const trimmed = line.trim()
  if (trimmed.startsWith('#')) return [{ kind: 'comment', text: line }]
  if (trimmed === '') return [{ kind: 'text', text: line }]

  const indent = INDENT.exec(line)![0]
  let rest = line.slice(indent.length)
  const dash = DASH.exec(rest)?.[0] ?? ''
  rest = rest.slice(dash.length)

  const keyMatch = KEY_CHARS.exec(rest)
  if (keyMatch && rest[keyMatch[0].length] === ':') {
    const key = keyMatch[0]
    const afterColon = rest.slice(key.length + 1)
    const space = INDENT.exec(afterColon)![0]
    const value = afterColon.slice(space.length)
    const out: Token[] = []
    if (indent) out.push({ kind: 'text', text: indent })
    if (dash) out.push({ kind: 'punct', text: dash })
    out.push({ kind: 'key', text: key }, { kind: 'punct', text: ':' })
    if (space) out.push({ kind: 'text', text: space })
    if (value) out.push(valueToken(value))
    return out
  }
  if (dash) return [{ kind: 'text', text: indent }, { kind: 'punct', text: dash }, valueToken(rest)]
  return [{ kind: 'text', text: line }]
}
