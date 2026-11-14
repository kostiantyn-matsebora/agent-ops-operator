import { describe, expect, it } from 'vitest'
import { BUILD_DEFAULT_LOOK, LOOKS, type LookPages } from './registry'

describe('the look registry', () => {
  it('lists classic and workspace', () => {
    expect(LOOKS.map((l) => [l.id, l.label])).toEqual([['classic', 'Classic'], ['workspace', 'Workspace']])
  })

  it('defaults to a registered look, classic until the flip (task 7.4)', () => {
    expect(BUILD_DEFAULT_LOOK).toBe('classic')
    expect(LOOKS.map((l) => l.id)).toContain(BUILD_DEFAULT_LOOK)
  })

  it('registers each identifier once', () => {
    const ids = LOOKS.map((l) => l.id)
    expect(new Set(ids).size).toBe(ids.length)
  })

  it('loads each look as a shell and every page of the contract', async () => {
    const contract: (keyof LookPages)[] = [
      'Overview', 'Queues', 'Config', 'ConfigKind', 'ConfigDetail', 'Topology', 'Conversations', 'Login',
    ]
    for (const look of LOOKS) {
      const mod = await look.load()
      expect(typeof mod.Shell, look.id).toBe('function')
      expect(Object.keys(mod.pages).sort(), look.id).toEqual([...contract].sort())
      for (const page of contract) expect(typeof mod.pages[page], `${look.id}.${page}`).toBe('function')
    }
  })
})
