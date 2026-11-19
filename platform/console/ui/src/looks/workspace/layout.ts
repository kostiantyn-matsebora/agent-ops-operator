import { useEffect, useState } from 'react'
import { create } from 'zustand'
import { createJSONStorage, persist } from 'zustand/middleware'
import type { Scope } from '../../model/conversationList'

// The workspace's own layout: pane widths, what is collapsed, what the list
// shows. A per-viewer convenience kept in the browser, under a key of THIS
// look — classic keeps its own (`agentops.console.layout`), and switching
// looks must not hand one look's widths to the other's different panes.

export const WORKSPACE_LAYOUT_KEY = 'agentops-console-workspace-layout'

// The prototype's geometry.
export const DEFAULT_NAV_WIDTH = 240
export const DEFAULT_LIST_WIDTH = 340
export const MIN_NAV_WIDTH = 200
export const MIN_LIST_WIDTH = 280
export const MAX_PANE_WIDTH = 560
/** One arrow-key press on a splitter, in px. */
export const SPLITTER_STEP = 16
export const NAV_COLLAPSED_WIDTH = 56
export const LIST_COLLAPSED_WIDTH = 64

export interface WorkspaceLayout {
  navWidth: number
  listWidth: number
  /** The navigation panel folded to its icons. */
  navCollapsed: boolean
  /** The navigation panel put away altogether, from the header's menu button, on a wide window. */
  navHidden: boolean
  listCollapsed: boolean
  /** Every row its own line, instead of members under their root. */
  flatten: boolean
}

export const DEFAULT_WORKSPACE_LAYOUT: WorkspaceLayout = {
  navWidth: DEFAULT_NAV_WIDTH,
  listWidth: DEFAULT_LIST_WIDTH,
  navCollapsed: false,
  navHidden: false,
  listCollapsed: false,
  flatten: false,
}

interface LayoutStore extends WorkspaceLayout {
  update: (patch: Partial<WorkspaceLayout>) => void
}

export function clampWidth(width: number, min: number): number {
  return Math.round(Math.min(MAX_PANE_WIDTH, Math.max(min, width)))
}

/**
 * Storage that never throws: a private tab or a full quota still renders the
 * workspace, at the defaults, and simply does not remember.
 */
const safeStorage = createJSONStorage(() => ({
  getItem: (k: string) => {
    try {
      return localStorage.getItem(k)
    } catch {
      return null
    }
  },
  setItem: (k: string, v: string) => {
    try {
      localStorage.setItem(k, v)
    } catch {
      /* not remembered, nothing else lost */
    }
  },
  removeItem: (k: string) => {
    try {
      localStorage.removeItem(k)
    } catch {
      /* nothing to forget */
    }
  },
}))

export const useWorkspaceLayout = create<LayoutStore>()(
  persist(
    (set) => ({
      ...DEFAULT_WORKSPACE_LAYOUT,
      update: (patch) => set(patch),
    }),
    {
      name: WORKSPACE_LAYOUT_KEY,
      version: 1,
      storage: safeStorage,
      partialize: ({ update: _update, ...layout }) => layout,
      // A stored value of the wrong type is dropped, never trusted. A
      // `showClosed` an earlier version wrote under this same key is
      // likewise never restored: it is no longer a key of
      // `DEFAULT_WORKSPACE_LAYOUT`, so the loop below never looks at it
      // (console-inbox-open-closed: "a value left by an earlier version
      // SHALL be ignored").
      merge: (stored, current) => {
        const s = (stored ?? {}) as Record<string, unknown>
        const out = { ...current }
        for (const key of Object.keys(DEFAULT_WORKSPACE_LAYOUT) as (keyof WorkspaceLayout)[]) {
          if (typeof s[key] === typeof DEFAULT_WORKSPACE_LAYOUT[key]) (out as Record<string, unknown>)[key] = s[key]
        }
        return out
      },
    },
  ),
)

/**
 * The scope the list shows. Shared by the navigation panel, which picks it,
 * and the conversations page, which reads it — so it is a store of this look,
 * not either component's state. Not persisted: a scope is where the viewer is
 * looking now.
 */
export const useWorkspaceScope = create<{ scope: Scope; setScope: (s: Scope) => void }>((set) => ({
  scope: { kind: 'open' },
  setScope: (scope) => set({ scope }),
}))

// The breakpoints are the prototype's.
/** At or below this the navigation is a drawer. */
export const DRAWER_MAX = 1180
/** At or below this the workspace shows one pane at a time. */
export const SINGLE_PANE_MAX = 900

export type Arrangement = 'full' | 'drawer' | 'single'

function arrangementOf(width: number): Arrangement {
  if (width <= SINGLE_PANE_MAX) return 'single'
  if (width <= DRAWER_MAX) return 'drawer'
  return 'full'
}

/** How the window arranges the workspace, kept current as it resizes. */
export function useArrangement(): Arrangement {
  const read = () => (typeof window === 'undefined' ? 'full' : arrangementOf(window.innerWidth))
  const [arrangement, setArrangement] = useState<Arrangement>(read)
  useEffect(() => {
    const onResize = () => setArrangement(read())
    window.addEventListener('resize', onResize)
    return () => window.removeEventListener('resize', onResize)
  }, [])
  return arrangement
}
