import { create } from 'zustand'
import { persist } from 'zustand/middleware'

// Theme selection: light, dark, or follow the system.
//
// SYSTEM IS THE DEFAULT, and it is a live subscription rather than a read at
// startup — an operator whose OS flips to dark in the evening should not have to
// reload a console they left open. That is also why the store holds the CHOICE
// ("system") and derives the applied theme, instead of collapsing the two: a
// resolved value would forget that the user asked to follow.

export type ThemeChoice = 'light' | 'dark' | 'system'
export type ResolvedTheme = 'light' | 'dark'

/** PatternFly 6 switches on these classes at the document root. */
const CLASS: Record<ResolvedTheme, string> = {
  light: 'pf-v6-theme-light',
  dark: 'pf-v6-theme-dark',
}

const QUERY = '(prefers-color-scheme: dark)'

export function systemTheme(): ResolvedTheme {
  return typeof window !== 'undefined' && window.matchMedia?.(QUERY).matches ? 'dark' : 'light'
}

export function resolve(choice: ThemeChoice): ResolvedTheme {
  return choice === 'system' ? systemTheme() : choice
}

/** apply puts the resolved theme on <html>, where PatternFly's tokens read it. */
export function applyTheme(theme: ResolvedTheme): void {
  if (typeof document === 'undefined') return
  const root = document.documentElement
  root.classList.remove(CLASS.light, CLASS.dark)
  root.classList.add(CLASS[theme])
  // Also expose it as an attribute: the SVG and any plain CSS can key off one
  // selector without knowing PatternFly's class names.
  root.setAttribute('data-theme', theme)
  // Tells the browser which scrollbar and form-control colours to use, so the
  // chrome around the app matches the app.
  root.style.colorScheme = theme
}

interface ThemeState {
  choice: ThemeChoice
  setChoice: (c: ThemeChoice) => void
}

export const useThemeStore = create<ThemeState>()(
  persist(
    (set) => ({
      choice: 'system',
      setChoice: (choice) => {
        applyTheme(resolve(choice))
        set({ choice })
      },
    }),
    {
      name: 'agentops-console-theme',
      // Apply on rehydration too: the persisted choice must win before first
      // paint, or the console flashes the wrong theme on every load.
      onRehydrateStorage: () => (state) => {
        if (state) applyTheme(resolve(state.choice))
      },
    },
  ),
)

/**
 * startThemeSync applies the current choice and keeps `system` following the OS.
 * Returns an unsubscribe.
 */
export function startThemeSync(): () => void {
  applyTheme(resolve(useThemeStore.getState().choice))
  if (typeof window === 'undefined' || !window.matchMedia) return () => undefined

  const mq = window.matchMedia(QUERY)
  const onSystemChange = () => {
    if (useThemeStore.getState().choice === 'system') applyTheme(systemTheme())
  }
  mq.addEventListener('change', onSystemChange)
  return () => mq.removeEventListener('change', onSystemChange)
}
