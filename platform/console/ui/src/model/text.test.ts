import { describe, expect, it } from 'vitest'
import { plain, preview } from './text'

describe('preview', () => {
  it('reads structured output as its title', () => {
    expect(preview('<title>\nRolled back checkout-api\n</title>\n\n<root-cause>OOM</root-cause>')).toBe('Rolled back checkout-api')
  })
  it('takes a title cut off before its closing tag as it stands', () => {
    expect(preview('<title>❓ Escalated to yo')).toBe('❓ Escalated to yo')
  })
  it('drops tags from text that has no title, and folds it to one line', () => {
    expect(preview('<root-cause>OOM\nkilled</root-cause>')).toBe('OOM killed')
    expect(preview('plain &amp; simple')).toBe('plain & simple')
  })
  it('leaves plain alone', () => {
    expect(plain('<b>x</b>')).toBe('x')
  })
})
