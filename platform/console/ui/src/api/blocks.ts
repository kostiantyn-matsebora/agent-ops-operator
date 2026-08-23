import type { Block } from './types'

/**
 * The block grammar, parsed HERE.
 *
 * The manager passes an agent's text through untouched, exactly as it passes
 * markdown through untouched, and each adapter reads the grammar and renders it
 * to what its surface has. See design D1 in the structured-agent-output change.
 *
 * PARSING IN THE SURFACE IS WHAT MAKES HISTORY WORK. The tags live in
 * `status.runs[].result`, so a transcript rebuilt after a restart parses the
 * same characters a live message carried and renders identically. While the
 * manager parsed, a parsed structure travelled on the live message only and
 * every reopened conversation went flat.
 *
 * THIS FILE IS DUPLICATED, in Go, in channels/telegram/blocks.go. That is the
 * accepted cost of D1. The RECOGNITION RULES are stated once, in the capability
 * spec, and both implementations are written against them: change one, change
 * both, and keep the adversarial table in step.
 */

const TAG_LINE = /^<(\/?)([a-zA-Z][a-zA-Z0-9_-]*)>[ \t]*$/

const TITLE = 'title'
const DETAILS = 'details'

interface Tag {
  name: string
  closing: boolean
}

/**
 * parsedTag reports whether a line is a standalone block tag.
 *
 * No leading whitespace is allowed and trailing whitespace is: indentation means
 * the line is probably inside something, while trailing spaces are invisible and
 * models emit them constantly. Rejecting a tag over a character nobody can see
 * reads as a bug.
 */
function parsedTag(line: string): Tag | null {
  const m = TAG_LINE.exec(line)
  return m ? { closing: m[1] === '/', name: m[2] } : null
}

/**
 * fencedLines marks every line inside a ``` fence, INCLUDING the fence lines.
 * A fenced block is a machine document quoted verbatim, and a sample showing
 * `<details>` must survive being about this grammar.
 */
function fencedLines(lines: string[]): boolean[] {
  const out: boolean[] = []
  let open = false
  for (const l of lines) {
    if (l.trimStart().startsWith('```')) {
      out.push(true)
      open = !open
      continue
    }
    out.push(open)
  }
  return out
}

/** hasCloser reports whether a matching close tag appears later, outside fences. */
function hasCloser(lines: string[], fenced: boolean[], from: number, name: string): boolean {
  for (let i = from; i < lines.length; i++) {
    if (fenced[i]) continue
    const t = parsedTag(lines[i])
    if (t?.closing && t.name.toLowerCase() === name.toLowerCase()) return true
  }
  return false
}

/** oneLine collapses a title. A heading that wraps to three lines is not one. */
function oneLine(s: string): string {
  return s.split(/\s+/).filter(Boolean).join(' ')
}

/**
 * parse turns an agent's reported output into blocks. It is TOTAL: every input
 * yields blocks, and no input loses a character.
 *
 * A tag is recognized only when it stands alone on its own line at line start,
 * forms a well-formed open/close pair, and sits outside fenced code. Anything
 * else is literal text — which is what keeps an OPEN vocabulary safe, because
 * agent output is full of `<` in shell redirects, generics and code.
 */
export function parse(input: string): Block[] {
  const lines = input.split('\n')
  const fenced = fencedLines(lines)

  const out: Block[] = []
  let prose: string[] = []
  let region: string[] = []
  let curName = ''
  let inRegion = false

  const flushProse = () => {
    const t = prose.join('\n').trim()
    if (t) out.push({ role: 'section', text: t })
    prose = []
  }
  const closeRegion = () => {
    const text = region.join('\n').trim()
    const lower = curName.toLowerCase()
    if (lower === TITLE) out.push({ role: 'title', text: oneLine(text) })
    else if (lower === DETAILS) out.push({ role: 'details', text })
    else out.push({ role: 'section', label: curName, text })
    region = []
    curName = ''
    inRegion = false
  }

  lines.forEach((line, i) => {
    const tag = fenced[i] ? null : parsedTag(line)
    if (!tag) {
      ;(inRegion ? region : prose).push(line)
      return
    }
    if (inRegion && tag.closing && tag.name.toLowerCase() === curName.toLowerCase()) {
      closeRegion()
      return
    }
    if (inRegion) {
      // A TAG INSIDE AN OPEN REGION IS LITERAL. The model is flat, and a model
      // that forgot a close tag is far commoner than one nesting deliberately.
      region.push(line)
      return
    }
    if (tag.closing) {
      // A close with no open never formed a pair: literal text.
      prose.push(line)
      return
    }
    // An unpaired OPEN runs to end of output rather than being discarded —
    // losing an agent's words to a grammar slip is the worst failure available.
    void hasCloser(lines, fenced, i + 1, tag.name)
    flushProse()
    curName = tag.name
    inRegion = true
  })

  if (inRegion) closeRegion()
  flushProse()

  const normalized = normalize(out)
  if (normalized.length === 0) {
    // Total means total: empty in, one empty block out, so no caller downstream
    // has to branch.
    return [{ role: 'section', text: input.trim() }]
  }
  return normalized
}

/**
 * normalize applies the ordering rules: the title FIRST wherever it was written,
 * at most one, and the fold LAST.
 *
 * Named sections keep the order the agent wrote them in. With an open vocabulary
 * nothing here can tell which section is the conclusion, so reordering them
 * would be a guess — and NOTHING trims them either: a length budget cut a
 * markdown table from its header on a live install.
 */
function normalize(blocks: Block[]): Block[] {
  let title: Block | undefined
  const mid: Block[] = []
  const folded: string[] = []

  for (const b of blocks) {
    if (b.role === 'title') {
      if (!title) {
        title = b
        continue
      }
      // A SECOND TITLE IS A SECTION. Its words are kept under its own name
      // rather than becoming a heading no surface knows where to put.
      mid.push({ role: 'section', label: TITLE, text: b.text })
      continue
    }
    if (b.role === 'details') {
      if (b.text) folded.push(b.text)
      continue
    }
    if (b.text || b.label) mid.push(b)
  }

  const out: Block[] = []
  if (title) out.push(title)
  out.push(...mid)
  if (folded.length > 0) {
    // ONE FOLD. "The fold" is singular on every surface, and two disclosure
    // controls in one message is a presentation nobody asked for.
    out.push({ role: 'details', text: folded.join('\n\n') })
  }
  return out
}
