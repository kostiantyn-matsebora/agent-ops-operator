import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import { Blocks, Fold, humanLabel } from './Blocks'
import type { Block } from '../api/types'

const answer: Block[] = [
  { role: 'title', text: 'Pod is looping' },
  { role: 'section', label: 'root-cause', text: 'OOM at **512Mi**.' },
  { role: 'section', label: 'fix', text: 'Raise the limit.' },
  { role: 'details', text: 'line one\nline two\nline three' },
]

describe('Blocks', () => {
  it('opens with the conclusion and hides the detail', () => {
    render(<Blocks blocks={answer} />)
    expect(screen.getByText('Pod is looping')).toBeInTheDocument()
    expect(screen.getByText('Root cause')).toBeInTheDocument()
    expect(screen.getByText('Fix')).toBeInTheDocument()
    // THE FOLD IS COLLAPSED BY DEFAULT. That is the whole point of the change:
    // a reader gets the conclusion, and the long tail only on request.
    expect(screen.queryByText(/line two/)).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: /details/i })).toHaveAttribute('aria-expanded', 'false')
  })

  it('expands the fold in place', async () => {
    render(<Blocks blocks={answer} />)
    await userEvent.click(screen.getByRole('button', { name: /details/i }))
    expect(screen.getByText(/line two/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /hide details/i })).toHaveAttribute('aria-expanded', 'true')
  })

  it('renders inline markdown inside a block', () => {
    render(<Blocks blocks={answer} />)
    expect(screen.getByText('512Mi').tagName).toBe('STRONG')
  })

  it('renders sections in the order the agent wrote them', () => {
    const { container } = render(<Blocks blocks={answer} />)
    const text = container.textContent ?? ''
    expect(text.indexOf('Root cause')).toBeLessThan(text.indexOf('Fix'))
    expect(text.indexOf('Pod is looping')).toBeLessThan(text.indexOf('Root cause'))
  })

  it('renders nothing when there are no blocks', () => {
    const { container } = render(<Blocks blocks={[]} />)
    expect(container).toBeEmptyDOMElement()
  })

  // MARKUP IN TEXT IS STILL NEVER MARKUP. An agent writing about tags must see
  // its characters, not have them interpreted.
  it('never interprets tag-shaped characters as markup', () => {
    const { container } = render(
      <Blocks blocks={[{ role: 'section', text: 'write <b>bold</b> or <script>x</script>' }]} />,
    )
    expect(container.querySelector('script')).toBeNull()
    expect(container.querySelector('b')).toBeNull()
  })
})

describe('humanLabel', () => {
  // GENERIC BY CONSTRUCTION: it transforms whatever it is given and recognises
  // nothing. The section vocabulary is open, so no agent's names live here.
  it.each([
    ['root-cause', 'Root cause'],
    ['what_i_changed', 'What i changed'],
    ['fix', 'Fix'],
    ['', ''],
  ])('presents %s as %s', (input, want) => {
    expect(humanLabel(input)).toBe(want)
  })
})

describe('code highlighting', () => {
  // A TAGGED FENCE IS COLOURED, and it stays ELEMENTS — rehype-highlight runs
  // lowlight, which emits a tree, so react-markdown renders real nodes and the
  // app's no-innerHTML rule is untouched.
  it('marks up a tagged fence as elements', () => {
    const { container } = render(
      <Blocks blocks={[{ role: 'details', text: '```json\n{"ready": true}\n```' }]} />,
    )
    // The fold has to be open before anything inside it exists.
    expect(container.querySelector('.hljs-attr, .hljs-string, .hljs-literal')).toBeNull()
  })

  it('colours JSON once the fold is expanded', async () => {
    render(<Blocks blocks={[{ role: 'details', text: '```json\n{"ready": true}\n```' }]} />)
    await userEvent.click(screen.getByRole('button', { name: /details/i }))
    const marked = document.querySelectorAll('[class*="hljs-"]')
    expect(marked.length).toBeGreaterThan(0)
  })

  // NOT GUESSED. An untagged block is left alone, so a log excerpt is not
  // randomly coloured as whichever grammar looked closest.
  it('leaves an untagged fence uncoloured', () => {
    const { container } = render(
      <Blocks blocks={[{ role: 'section', text: '```\nplain log line\n```' }]} />,
    )
    expect(container.querySelectorAll('[class*="hljs-"]').length).toBe(0)
    expect(container.textContent).toContain('plain log line')
  })

  // A language nobody registered must render as plain code, never as an error.
  it('renders an unregistered language as plain code', () => {
    const { container } = render(
      <Blocks blocks={[{ role: 'section', text: '```brainfuck\n+++.\n```' }]} />,
    )
    expect(container.textContent).toContain('+++.')
  })
})

describe('line breaks', () => {
  // A SINGLE NEWLINE IS A LINE BREAK. Standard markdown collapses one into a
  // space, which is how a three-line evidence list arrived as one sentence.
  it('keeps consecutive lines on separate lines', () => {
    const { container } = render(
      <Blocks blocks={[{ role: 'section', label: 'evidence', text: 'one\ntwo\nthree' }]} />,
    )
    expect(container.querySelectorAll('br').length).toBe(2)
  })

  // Even a stray literal bullet — the thing format.md now forbids — reads as
  // lines rather than as prose. The instruction is the fix, this is the floor.
  it('survives an agent that types the glyph anyway', () => {
    const { container } = render(
      <Blocks blocks={[{ role: 'section', text: '• first\n• second' }]} />,
    )
    expect(container.querySelectorAll('br').length).toBe(1)
  })

  // A REAL markdown list is still a real list, not br-separated text.
  it('renders a markdown list as a list', () => {
    const { container } = render(
      <Blocks blocks={[{ role: 'section', text: '- first\n- second' }]} />,
    )
    expect(container.querySelectorAll('li').length).toBe(2)
  })
})

describe('the payload fold', () => {
  // A SIGNAL's event document goes behind its own control, labelled for what it
  // is. Inline it is a wall of JSON between the card and the reply box.
  it('collapses a payload and names it', async () => {
    const { fence } = await import('../api/fence')
    render(<Fold text={fence('{\n  "reason": "BackOff"\n}')} label="Payload" />)
    const button = screen.getByRole('button', { name: /payload/i })
    expect(button).toHaveAttribute('aria-expanded', 'false')
    expect(screen.queryByText(/BackOff/)).not.toBeInTheDocument()
    await userEvent.click(button)
    expect(screen.getByText(/BackOff/)).toBeInTheDocument()
  })

  it('fences json so it is not reflowed as prose', async () => {
    const { fence } = await import('../api/fence')
    expect(fence('{"a":1}')).toBe('```json\n{"a":1}\n```')
    expect(fence('plain text')).toBe('```\nplain text\n```')
    // A payload carrying its own fence must not close ours early.
    expect(fence('see ```go\nx\n```').startsWith('````')).toBe(true)
  })
})
