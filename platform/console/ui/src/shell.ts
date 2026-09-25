import { create } from 'zustand'
import { persist } from 'zustand/middleware'

// The shell's own layout: the navigation folded to its icons. A viewer's
// preference, like the theme and the topology's display — and kept the same
// way the display is (graph/display.ts), under a key of its own, so that a
// wide view keeps the width it was given across reloads.

export interface ShellState {
  navCollapsed: boolean
  setNavCollapsed: (v: boolean) => void
}

export const useShell = create<ShellState>()(
  persist(
    (set) => ({
      navCollapsed: false,
      setNavCollapsed: (navCollapsed) => set({ navCollapsed }),
    }),
    { name: 'agentops-console-shell', version: 1 },
  ),
)
