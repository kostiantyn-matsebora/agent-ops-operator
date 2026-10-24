import { afterEach, describe, expect, it } from 'vitest'
import { useComposerIntent } from './composerIntent'

describe('useComposerIntent', () => {
  afterEach(() => {
    useComposerIntent.getState().clear()
  })

  it('starts with nothing requested', () => {
    expect(useComposerIntent.getState().requestedTask).toBeNull()
  })

  it('holds the requested task until cleared', () => {
    useComposerIntent.getState().openWith('/ops check the pods')
    expect(useComposerIntent.getState().requestedTask).toBe('/ops check the pods')
    useComposerIntent.getState().clear()
    expect(useComposerIntent.getState().requestedTask).toBeNull()
  })
})
