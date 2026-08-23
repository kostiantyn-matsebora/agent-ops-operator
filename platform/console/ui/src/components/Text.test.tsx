import { describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/react'
import { PlainText, plain, RawText } from './Text'

// Everything the console renders comes from the cluster or the wire. None of it
// is trusted, and none of it is ours to interpret as markup.

describe('plain', () => {
  it('strips the chat HTML subset the adapters emit', () => {
    // Telegram wants <b>; the console is not that transport, and rendering a tag
    // because an adapter wrote one would be trusting a string from a channel.
    expect(plain('<b>Pipelines</b>: /k8s-ops')).toBe('Pipelines: /k8s-ops')
    expect(plain('a <i>very</i> <code>odd</code> message')).toBe('a very odd message')
  })

  it('removes anything script-shaped rather than rendering it', () => {
    expect(plain('<script>alert(1)</script>hello')).toBe('alert(1)hello')
    expect(plain('<img src=x onerror=alert(1)>')).toBe('')
  })

  it('decodes the entities adapters escape', () => {
    expect(plain('Usage: /&lt;pipeline&gt; &lt;task&gt;')).toBe('Usage: /<pipeline> <task>')
    expect(plain('a &amp; b')).toBe('a & b')
  })
})

describe('PlainText', () => {
  it('renders text content, never markup', () => {
    const { container } = render(<PlainText>{'<b>bold</b>'}</PlainText>)
    expect(screen.getByText('bold')).toBeInTheDocument()
    expect(container.querySelector('b')).toBeNull()
  })

  it('renders nothing for empty input rather than an empty element', () => {
    const { container } = render(<PlainText>{''}</PlainText>)
    expect(container.firstChild).toBeNull()
  })

  it('preserves newlines when multiline', () => {
    const { container } = render(<PlainText multiline>{'one\ntwo'}</PlainText>)
    expect(container.textContent).toBe('one\ntwo')
    expect((container.firstChild as HTMLElement).style.whiteSpace).toBe('pre-wrap')
  })
})

// THE APP-WIDE RULE, ASSERTED RATHER THAN STATED.
//
// Two components' comments promise no string is ever handed to the DOM as
// markup. A promise in a comment is undone by one convenient call, and the
// block grammar is exactly the pressure that invites it: an agent's output now
// has STRUCTURE, and reaching for innerHTML to render it is the shortcut this
// design exists to refuse. Blocks are already elements, so there is no need.
describe('the no-innerHTML rule', () => {
  it('holds across every source file', () => {
    // Vite reads the sources at build time, so this needs no Node types and
    // runs in the same browser-ish environment as every other test here.
    const sources = import.meta.glob('../**/*.{ts,tsx}', { eager: true, query: '?raw', import: 'default' })
    // Assembled from parts so this scanner never matches ITSELF — the literal
    // must not appear in any source file, this one included.
    const needle = 'dangerously' + 'SetInnerHTML'

    // A glob that matched nothing would make this assertion vacuous — which is
    // the failure mode of every "no occurrences" test.
    expect(Object.keys(sources).length).toBeGreaterThan(20)

    const offenders: string[] = []
    for (const [path, body] of Object.entries(sources as Record<string, string>)) {
      for (const [i, line] of body.split('\n').entries()) {
        // The two documented MENTIONS are in comments promising it is absent.
        if (line.includes(needle) && !/^\s*(\*|\/\/)/.test(line)) {
          offenders.push(`${path}:${i + 1}`)
        }
        if (/\.innerHTML\s*=/.test(line)) offenders.push(`${path}:${i + 1} (innerHTML)`)
      }
    }
    expect(offenders).toEqual([])
  })
})

// THE RECORD SHOWS WHAT WAS RECORDED.
//
// `status.runs[].result` is what the agent printed, and the block grammar is
// part of that text. PlainText strips anything tag-shaped — right for a
// rendered view, wrong for a record, where it silently deleted content.
describe('RawText', () => {
  it('shows tags instead of eating them', () => {
    render(<RawText>{'<title>\nDisk filling\n</title>'}</RawText>)
    expect(screen.getByText(/<title>/)).toBeInTheDocument()
  })

  it('is still never markup', () => {
    const { container } = render(<RawText>{'<b>bold</b><script>x</script>'}</RawText>)
    expect(container.querySelector('b')).toBeNull()
    expect(container.querySelector('script')).toBeNull()
    expect(container.textContent).toBe('<b>bold</b><script>x</script>')
  })

  // The contrast that makes both correct: one renders, one records.
  it('differs from PlainText exactly in what it keeps', () => {
    const tagged = '<details>kept</details>'
    const { container: raw } = render(<RawText>{tagged}</RawText>)
    const { container: plainOut } = render(<PlainText>{tagged}</PlainText>)
    expect(raw.textContent).toBe(tagged)
    expect(plainOut.textContent).toBe('kept')
  })
})
