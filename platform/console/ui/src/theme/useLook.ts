import { create } from 'zustand'
import { persist } from 'zustand/middleware'

// Look selection. The colour scheme (useTheme.ts) is a separate choice, and
// the two combine.
//
// The look in force is resolved in this order:
//
//   1. a `look` query parameter, for this page load, never stored,
//   2. the viewer's stored choice,
//   3. the install's named default (`Session.defaultLook`),
//   4. the build's default.
//
// A step naming no registered look is skipped. The store holds the CHOICE,
// not the resolved look, so a viewer who chose nothing keeps following the
// install's default when it changes.

export const LOOK_STORAGE_KEY = 'agentops-console-look'

interface LookState {
  /** The viewer's stored choice. Absent means they chose nothing. */
  choice?: string
  /** The `look` query parameter of this page load. Never persisted. */
  forced?: string
  /** A choice in the switcher: stored, and it ends the URL's override. */
  choose: (id: string) => void
  force: (id: string | undefined) => void
  /** Drops a stored choice this build no longer registers. */
  forget: () => void
}

export const useLookStore = create<LookState>()(
  persist(
    (set) => ({
      choice: undefined,
      forced: undefined,
      choose: (choice) => set({ choice, forced: undefined }),
      force: (forced) => set({ forced }),
      forget: () => set({ choice: undefined }),
    }),
    {
      name: LOOK_STORAGE_KEY,
      version: 1,
      partialize: (s) => ({ choice: s.choice }),
    },
  ),
)

/** The `look` query parameter, if the URL carries one. */
export function lookFromQuery(search: string): string | undefined {
  return new URLSearchParams(search).get('look') || undefined
}

/** Reads the page's `look` query parameter into the store, before first render. */
export function startLookFromUrl(): void {
  if (typeof window === 'undefined') return
  useLookStore.getState().force(lookFromQuery(window.location.search))
}

export type LookSource = 'query' | 'stored' | 'install' | 'build'

export interface LookInputs {
  query?: string
  stored?: string
  installDefault?: string
  registered: readonly string[]
  buildDefault: string
}

export interface LookResolution {
  look: string
  source: LookSource
  /**
   * The stored choice names a look this build does not register. The caller
   * clears it rather than storing the fallback: a stored fallback would read
   * as a choice and outrank a later install default.
   */
  staleStored: boolean
}

export function resolveLook({ query, stored, installDefault, registered, buildDefault }: LookInputs): LookResolution {
  const known = (id: string | undefined): id is string => !!id && registered.includes(id)
  const staleStored = !!stored && !known(stored)
  if (known(query)) return { look: query, source: 'query', staleStored }
  if (known(stored)) return { look: stored, source: 'stored', staleStored }
  if (known(installDefault)) return { look: installDefault, source: 'install', staleStored }
  return { look: buildDefault, source: 'build', staleStored }
}

/** Puts the look in force on <html>, beside `data-theme`. */
export function applyLook(id: string): void {
  if (typeof document === 'undefined') return
  document.documentElement.setAttribute('data-look', id)
}
