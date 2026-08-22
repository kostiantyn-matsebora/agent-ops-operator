import { describe, expect, it, vi } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClientProvider, useQuery } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { EVICT_UNUSED_AFTER_MS, FRESH_FOR_MS, createQueryClient } from './queryClient'

// The cache is BOUNDED IN TIME, not held for the tab's lifetime.
//
// Applying events keeps a view correct without re-reading it, which is also
// what would keep it resident forever if nothing evicted it. These are memory
// bounds: correctness is the resync rule's job, which replaces applied state
// wholesale whenever the client may have missed an event.

function wrapper(client: ReturnType<typeof createQueryClient>) {
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  )
}

describe('the cache is bounded', () => {
  it('releases data for a view that is no longer on screen', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    const client = createQueryClient()
    const view = renderHook(
      () => useQuery({ queryKey: ['inventory', 'channels'], queryFn: async () => ['a'] }),
      { wrapper: wrapper(client) },
    )
    await waitFor(() => expect(client.getQueryData(['inventory', 'channels'])).toEqual(['a']))

    view.unmount() // the reader navigated away
    expect(client.getQueryData(['inventory', 'channels'])).toEqual(['a'])

    vi.advanceTimersByTime(EVICT_UNUSED_AFTER_MS + 1_000)
    expect(client.getQueryData(['inventory', 'channels'])).toBeUndefined()
    vi.useRealTimers()
  })

  it('loads fresh when a view is re-opened after the bound', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    const client = createQueryClient()
    const queryFn = vi.fn(async () => ['a'])
    const first = renderHook(() => useQuery({ queryKey: ['kinds'], queryFn }), {
      wrapper: wrapper(client),
    })
    await waitFor(() => expect(queryFn).toHaveBeenCalledTimes(1))
    first.unmount()

    vi.advanceTimersByTime(FRESH_FOR_MS + 1_000)
    const second = renderHook(() => useQuery({ queryKey: ['kinds'], queryFn }), {
      wrapper: wrapper(client),
    })
    // Fresh, rather than whatever was last applied to it. It does not blank:
    // the previous data is either gone — a first load — or shown while the
    // read runs.
    await waitFor(() => expect(queryFn).toHaveBeenCalledTimes(2))
    second.unmount()
    vi.useRealTimers()
  })

  it('never re-reads a view that is held on screen', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    const client = createQueryClient()
    const queryFn = vi.fn(async () => ['a'])
    const view = renderHook(() => useQuery({ queryKey: ['kinds'], queryFn }), {
      wrapper: wrapper(client),
    })
    await waitFor(() => expect(queryFn).toHaveBeenCalledTimes(1))

    vi.advanceTimersByTime(EVICT_UNUSED_AFTER_MS * 2)
    // Staleness decides what a REMOUNT does. A view being watched stays current
    // from events, and the bound alone must never turn it into a request.
    expect(queryFn).toHaveBeenCalledTimes(1)
    expect(view.result.current.data).toEqual(['a'])
    view.unmount()
    vi.useRealTimers()
  })
})

// Nothing outlives the tab. The console holds a snapshot of cluster state and a
// transcript of what agents said; writing either to a browser's disk is a
// durability promise this component has no business making, and it would
// survive a logout.
describe('nothing is persisted', () => {
  it('writes no browser storage while caching and applying', async () => {
    const local = vi.spyOn(Storage.prototype, 'setItem')
    const client = createQueryClient()
    client.setQueryData(['conversation', 'conv-1'], { yaml: 'secret' })
    await waitFor(() => expect(client.getQueryData(['conversation', 'conv-1'])).toBeDefined())

    expect(local).not.toHaveBeenCalled()
    local.mockRestore()
  })

  it('imports no persistence anywhere in the app', () => {
    // Vite reads the sources, so this needs no filesystem access and no node
    // types — the console UI keeps its dependencies to what it ships.
    const sources = import.meta.glob('../**/*.{ts,tsx}', {
      query: '?raw', import: 'default', eager: true,
    }) as Record<string, string>

    const offenders: string[] = []
    for (const [path, raw] of Object.entries(sources)) {
      if (/\.test\.(ts|tsx)$/.test(path)) continue
      // The theme switch is the ONE thing a viewer may keep, and it is a
      // preference rather than cluster state — so it is excluded by name
      // rather than by loosening the rule.
      if (path.includes('theme')) continue
      // Comments are stripped first: this file and queryClient.ts both SAY
      // localStorage in prose explaining why they do not use it, and a rule
      // its own reasoning trips is a rule people delete.
      const code = raw.replace(/\/\*[\s\S]*?\*\//g, '').replace(/\/\/.*$/gm, '')
      const uses = /localStorage|sessionStorage|indexedDB|serviceWorker|persistQueryClient/.exec(code)
      if (uses) offenders.push(`${path}: ${uses[0]}`)
    }
    expect(offenders).toEqual([])
  })
})
