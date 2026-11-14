import type { ComponentType } from 'react'

// The registry of looks.
//
// A look is the console's presentation: markup, styling, motion and layout
// over the one behaviour layer. "Theme" keeps its narrower meaning here —
// light, dark or automatic (theme/useTheme.ts) — and the two choices combine.
//
// Adding a look is a folder under looks/ and one entry below. Nothing else
// names a look, so the switcher, the resolution order and the lazy loading
// pick a new entry up with no other source change.

/** The pages every look provides. Each is a route's whole content. */
export interface LookPages {
  Overview: ComponentType
  Queues: ComponentType
  Config: ComponentType
  ConfigKind: ComponentType
  ConfigDetail: ComponentType
  Topology: ComponentType
  Conversations: ComponentType
  Login: ComponentType<{ configured: boolean; onLogin: () => void }>
}

/** What a look's folder exports from its index: the shell, and its pages. */
export interface LookModule {
  /** Renders the look's header, navigation and routes. */
  Shell: ComponentType
  pages: LookPages
}

export interface LookEntry {
  id: string
  /** What the look switcher shows. */
  label: string
  /**
   * A dynamic import, so a viewer downloads only the look in force. Called
   * only for the active look.
   */
  load: () => Promise<LookModule>
}

export const LOOKS: readonly LookEntry[] = [
  { id: 'classic', label: 'Classic', load: () => import('./classic') },
]

/**
 * The look for a viewer who chose none, on an install that named none.
 * `classic` until the new look is ready (design D12), so every step before
 * that is invisible to a viewer.
 */
export const BUILD_DEFAULT_LOOK = 'classic'
