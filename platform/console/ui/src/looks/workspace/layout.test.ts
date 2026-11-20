import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  DEFAULT_WORKSPACE_LAYOUT, MAX_PANE_WIDTH, WORKSPACE_LAYOUT_KEY, clampWidth, useWorkspaceLayout,
} from './layout'

beforeEach(() => {
  localStorage.clear()
  useWorkspaceLayout.setState(DEFAULT_WORKSPACE_LAYOUT)
})
afterEach(() => vi.restoreAllMocks())

describe('the workspace layout', () => {
  it('is stored under a key of this look, never classic\u2019s', () => {
    useWorkspaceLayout.getState().update({ listWidth: 400, navCollapsed: true })
    expect(WORKSPACE_LAYOUT_KEY).toBe('agentops-console-workspace-layout')
    expect(JSON.parse(localStorage.getItem(WORKSPACE_LAYOUT_KEY)!).state).toMatchObject({ listWidth: 400, navCollapsed: true })
    expect(localStorage.getItem('agentops.console.layout')).toBeNull()
  })

  it('survives a reload: pane sizes and collapsed states come back', async () => {
    useWorkspaceLayout.getState().update({ navWidth: 300, listWidth: 420, navCollapsed: true, listCollapsed: true })
    // A reload: the in-memory state is gone, the stored copy is read again.
    const stored = localStorage.getItem(WORKSPACE_LAYOUT_KEY)!
    useWorkspaceLayout.setState(DEFAULT_WORKSPACE_LAYOUT)
    expect(useWorkspaceLayout.getState().navWidth).toBe(DEFAULT_WORKSPACE_LAYOUT.navWidth)
    localStorage.setItem(WORKSPACE_LAYOUT_KEY, stored)
    await useWorkspaceLayout.persist.rehydrate()
    expect(useWorkspaceLayout.getState()).toMatchObject({ navWidth: 300, listWidth: 420, navCollapsed: true, listCollapsed: true })
  })

  it('ignores a stored showClosed value from an earlier version, and never throws', async () => {
    localStorage.setItem(WORKSPACE_LAYOUT_KEY, JSON.stringify({ state: { listWidth: 400, showClosed: true }, version: 1 }))
    await useWorkspaceLayout.persist.rehydrate()
    expect(useWorkspaceLayout.getState()).toMatchObject({ listWidth: 400 })
    expect(useWorkspaceLayout.getState()).not.toHaveProperty('showClosed')
  })

  it('drops a stored value of the wrong type rather than trusting it', async () => {
    localStorage.setItem(WORKSPACE_LAYOUT_KEY, JSON.stringify({ state: { listWidth: 'wide', navCollapsed: true }, version: 1 }))
    await useWorkspaceLayout.persist.rehydrate()
    expect(useWorkspaceLayout.getState()).toMatchObject({ listWidth: DEFAULT_WORKSPACE_LAYOUT.listWidth, navCollapsed: true })
  })

  it('still renders, at the defaults, where storage refuses', async () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('denied')
    })
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('quota')
    })
    await useWorkspaceLayout.persist.rehydrate()
    expect(() => useWorkspaceLayout.getState().update({ listWidth: 300 })).not.toThrow()
    expect(useWorkspaceLayout.getState().listWidth).toBe(300)
  })

  it('keeps a dragged width within its bounds', () => {
    expect(clampWidth(100, 200)).toBe(200)
    expect(clampWidth(9999, 200)).toBe(MAX_PANE_WIDTH)
    expect(clampWidth(321.6, 200)).toBe(322)
  })
})
