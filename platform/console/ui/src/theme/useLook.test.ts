import { beforeEach, describe, expect, it } from 'vitest'
import { LOOK_STORAGE_KEY, applyLook, lookFromQuery, resolveLook, useLookStore } from './useLook'

const registered = ['classic', 'workspace']
const base = { registered, buildDefault: 'workspace' }

describe('the resolution order', () => {
  it('takes a registered query parameter over everything', () => {
    expect(resolveLook({ ...base, query: 'classic', stored: 'workspace', installDefault: 'workspace' })).toEqual({
      look: 'classic', source: 'query', staleStored: false,
    })
  })

  it('takes the stored choice over the install default', () => {
    expect(resolveLook({ ...base, stored: 'workspace', installDefault: 'classic' })).toMatchObject({
      look: 'workspace', source: 'stored',
    })
  })

  it('takes the install default when the viewer chose nothing', () => {
    expect(resolveLook({ ...base, installDefault: 'classic' })).toMatchObject({ look: 'classic', source: 'install' })
  })

  it('falls to the build default when nothing names a look', () => {
    expect(resolveLook(base)).toEqual({ look: 'workspace', source: 'build', staleStored: false })
  })
})

describe('a step naming no registered look is skipped', () => {
  it('ignores an unknown query parameter and resolves from the rest, in order', () => {
    expect(resolveLook({ ...base, query: 'sparkly', stored: 'classic' })).toMatchObject({ look: 'classic', source: 'stored' })
    expect(resolveLook({ ...base, query: 'sparkly', installDefault: 'classic' })).toMatchObject({ look: 'classic', source: 'install' })
    expect(resolveLook({ ...base, query: 'sparkly' })).toMatchObject({ look: 'workspace', source: 'build' })
  })

  it('falls back from a stored look this build no longer registers, and marks it for replacing', () => {
    expect(resolveLook({ ...base, stored: 'retired', installDefault: 'classic' })).toEqual({
      look: 'classic', source: 'install', staleStored: true,
    })
    expect(resolveLook({ ...base, stored: 'retired' })).toEqual({ look: 'workspace', source: 'build', staleStored: true })
  })

  it('still marks a stale stored look when a query parameter wins', () => {
    expect(resolveLook({ ...base, query: 'classic', stored: 'retired' }).staleStored).toBe(true)
  })

  it('ignores an install default naming no registered look', () => {
    expect(resolveLook({ ...base, installDefault: 'sparkly' })).toMatchObject({ look: 'workspace', source: 'build' })
  })
})

describe('the look store', () => {
  beforeEach(() => {
    localStorage.clear()
    useLookStore.setState({ choice: undefined, forced: undefined })
  })

  it('persists the choice under its own key, and never the query override', () => {
    useLookStore.getState().force('classic')
    expect(localStorage.getItem(LOOK_STORAGE_KEY) ?? '').not.toContain('forced')
    useLookStore.getState().choose('workspace')
    const stored = JSON.parse(localStorage.getItem(LOOK_STORAGE_KEY)!)
    expect(stored.state).toEqual({ choice: 'workspace' })
  })

  it('ends the query override when the viewer chooses', () => {
    useLookStore.getState().force('classic')
    useLookStore.getState().choose('workspace')
    expect(useLookStore.getState()).toMatchObject({ choice: 'workspace', forced: undefined })
  })

  it('forgets a stored choice', () => {
    useLookStore.getState().choose('retired')
    useLookStore.getState().forget()
    expect(useLookStore.getState().choice).toBeUndefined()
    expect(JSON.parse(localStorage.getItem(LOOK_STORAGE_KEY)!).state.choice).toBeUndefined()
  })
})

describe('the query parameter and the root attribute', () => {
  it('reads look from the query string', () => {
    expect(lookFromQuery('?look=classic&x=1')).toBe('classic')
    expect(lookFromQuery('?look=')).toBeUndefined()
    expect(lookFromQuery('')).toBeUndefined()
  })

  it('puts the look on <html> as data-look', () => {
    applyLook('classic')
    expect(document.documentElement.getAttribute('data-look')).toBe('classic')
  })
})
