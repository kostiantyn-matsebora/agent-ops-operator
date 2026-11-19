import { describe, expect, it, vi } from 'vitest'
import { prefersReducedMotion } from './motion'

describe('prefersReducedMotion', () => {
  it('returns false when matchMedia is unavailable', () => {
    expect(prefersReducedMotion()).toBe(false)
  })

  it('returns true when the viewer prefers reduced motion', () => {
    const mockMatchMedia = vi.fn(() => ({ matches: true }))
    vi.stubGlobal('matchMedia', mockMatchMedia)
    expect(prefersReducedMotion()).toBe(true)
    expect(mockMatchMedia).toHaveBeenCalledWith('(prefers-reduced-motion: reduce)')
    vi.unstubAllGlobals()
  })

  it('returns false when the viewer does not prefer reduced motion', () => {
    const mockMatchMedia = vi.fn(() => ({ matches: false }))
    vi.stubGlobal('matchMedia', mockMatchMedia)
    expect(prefersReducedMotion()).toBe(false)
    vi.unstubAllGlobals()
  })
})
