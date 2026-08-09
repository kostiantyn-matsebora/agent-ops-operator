import { describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/react'
import { PlainText, plain } from './Text'

// Everything the console renders comes from the cluster or the wire. None of it
// is trusted, and none of it is ours to interpret as markup.

describe('plain', () => {
  it('strips the chat HTML subset the adapters emit', () => {
    // Telegram wants <b>; the console is not that transport, and rendering a tag
    // because an adapter wrote one would be trusting a string from a channel.
    expect(plain('<b>Agents</b>: /k8s-ops')).toBe('Agents: /k8s-ops')
    expect(plain('a <i>very</i> <code>odd</code> message')).toBe('a very odd message')
  })

  it('removes anything script-shaped rather than rendering it', () => {
    expect(plain('<script>alert(1)</script>hello')).toBe('alert(1)hello')
    expect(plain('<img src=x onerror=alert(1)>')).toBe('')
  })

  it('decodes the entities adapters escape', () => {
    expect(plain('Usage: /&lt;agent&gt; &lt;task&gt;')).toBe('Usage: /<agent> <task>')
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
