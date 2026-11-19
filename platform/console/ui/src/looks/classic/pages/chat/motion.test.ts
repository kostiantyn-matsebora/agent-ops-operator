import { afterEach, describe, expect, it, vi } from 'vitest'
import { prefersReducedMotion } from './motion'

describe('prefersReducedMotion', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('is false where matchMedia does not exist', () => {
    vi.stubGlobal('matchMedia', undefined)
    expect(prefersReducedMotion()).toBe(false)
  })

  it.each([true, false])('follows the media query (%s)', (matches) => {
    vi.stubGlobal('matchMedia', (query: string) => ({ matches, media: query }))
    expect(prefersReducedMotion()).toBe(matches)
  })
})
