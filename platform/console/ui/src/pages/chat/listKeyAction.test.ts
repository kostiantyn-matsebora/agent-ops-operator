import { describe, expect, it } from 'vitest'
import { listKeyAction } from './ChatView'

describe('listKeyAction', () => {
  it('clears on Escape whatever the list holds', () => {
    expect(listKeyAction('Escape', 0, undefined)).toBe('clear')
  })
  it('does nothing on an empty list', () => {
    expect(listKeyAction('ArrowDown', 0, undefined)).toBe('none')
    expect(listKeyAction('Enter', 0, 'a')).toBe('none')
  })
  it('opens the highlighted row on Enter, nothing when none is highlighted', () => {
    expect(listKeyAction('Enter', 3, 'a')).toBe('open')
    expect(listKeyAction('Enter', 3, undefined)).toBe('none')
  })
  it('moves on the arrows', () => {
    expect(listKeyAction('ArrowDown', 3, undefined)).toBe('down')
    expect(listKeyAction('ArrowUp', 3, 'a')).toBe('up')
  })
  it('ignores other keys', () => {
    expect(listKeyAction('x', 3, 'a')).toBe('none')
  })
})
