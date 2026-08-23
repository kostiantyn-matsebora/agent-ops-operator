import { describe, expect, it } from 'vitest'
import { parse } from './blocks'

// THE ADVERSARIAL TABLE, kept in step with channels/telegram's Go copy.
// Two implementations of one grammar is the accepted cost of parsing in the
// surface; these cases are the contract they are both written against.
describe('parse', () => {
  it('yields one block for untagged prose', () => {
    const b = parse('All good.\n\nNothing to report.')
    expect(b).toEqual([{ role: 'section', text: 'All good.\n\nNothing to report.' }])
  })

  it.each([
    ['a less-than in prose', 'if x < y then scale up'],
    ['a generic', 'the Deployment<T> helper is fine'],
    ['a shell redirect', 'run it as:\ncmd < input.txt'],
    ['a mid-line tag', 'see <details> for more'],
    ['an inline code span', 'write `<details>` to fold'],
    ['an unpaired close', 'lead\n</evidence>\nmore'],
  ])('treats %s as text', (_name, input) => {
    expect(parse(input)).toEqual([{ role: 'section', text: input }])
  })

  it('treats a tag inside a fence as code', () => {
    const input = 'before\n```\n<details>\nnot a fold\n</details>\n```\nafter'
    expect(parse(input)).toEqual([{ role: 'section', text: input }])
  })

  it('accepts trailing whitespace on a tag', () => {
    expect(parse('<title>   \nDone\n</title>\t\nbody')).toEqual([
      { role: 'title', text: 'Done' },
      { role: 'section', text: 'body' },
    ])
  })

  it('keeps named sections in written order', () => {
    expect(parse('<root-cause>\nOOM\n</root-cause>\n<fix>\nraise it\n</fix>')).toEqual([
      { role: 'section', label: 'root-cause', text: 'OOM' },
      { role: 'section', label: 'fix', text: 'raise it' },
    ])
  })

  it('puts the title first wherever it was written', () => {
    expect(parse('<evidence>\n26 restarts\n</evidence>\n<title>\nLooping\n</title>')[0]).toEqual({
      role: 'title',
      text: 'Looping',
    })
  })

  it('closes an unpaired open at end of output rather than dropping it', () => {
    expect(parse('lead\n<evidence>\ntail one\ntail two')).toEqual([
      { role: 'section', text: 'lead' },
      { role: 'section', label: 'evidence', text: 'tail one\ntail two' },
    ])
  })

  it('treats a nested tag as literal inside its region', () => {
    expect(parse('<outer>\na\n<inner>\nb\n</inner>\nc\n</outer>')).toEqual([
      { role: 'section', label: 'outer', text: 'a\n<inner>\nb\n</inner>\nc' },
    ])
  })

  it('merges several folds into one, last', () => {
    expect(parse('<details>\nA\n</details>\nmid\n<details>\nB\n</details>')).toEqual([
      { role: 'section', text: 'mid' },
      { role: 'details', text: 'A\n\nB' },
    ])
  })

  it('matches reserved names case-insensitively', () => {
    expect(parse('<Title>\nUp\n</Title>')).toEqual([{ role: 'title', text: 'Up' }])
  })

  it('demotes a second title to a section', () => {
    expect(parse('<title>\nfirst\n</title>\n<title>\nsecond\n</title>')).toEqual([
      { role: 'title', text: 'first' },
      { role: 'section', label: 'title', text: 'second' },
    ])
  })

  // NOTHING IS TRIMMED. A length budget cut a markdown table from its header on
  // a live install; this fails if one ever comes back.
  it('keeps a long table and list whole', () => {
    const rows = '| a | b |\n|---|---|\n' + '| x | y |\n'.repeat(40)
    const list = '- something worth reading\n'.repeat(30)
    const [, table, items] = parse(
      `<title>\nT\n</title>\n<w>\n${rows}</w>\n<n>\n${list}</n>`,
    )
    expect(table.text.match(/\| x \| y \|/g)).toHaveLength(40)
    expect(table.text.startsWith('| a | b |')).toBe(true)
    expect(items.text.match(/- something/g)).toHaveLength(30)
  })

  it('is total', () => {
    expect(parse('')).toEqual([{ role: 'section', text: '' }])
  })
})
