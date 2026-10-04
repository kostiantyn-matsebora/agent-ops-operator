import { useCallback, useState } from 'react'

// The ONE thing the console persists in a browser (design D-B). Pane widths
// and the inbox's collapsed state are a per-viewer convenience, never
// conversation state — the "nothing is persisted" rule documented in
// `docs/console.md` is restated there to say so, not lifted.
//
// Guarded on BOTH sides: a private-browsing tab or a quota failure must
// still render the view, at the defaults, with no error reaching the
// operator (console-chat-layout: "Storage unavailable").

export const LAYOUT_STORAGE_KEY = 'agentops.console.layout'

export interface Layout {
  inboxWidth: number
  listWidth: number
  inboxCollapsed: boolean
}

// Defaults and the collapsed strip width are the prototype's
// (`prototype/C-rail.html`, `C-collapsed.html`) — read nowhere else.
export const DEFAULT_LAYOUT: Layout = { inboxWidth: 240, listWidth: 340, inboxCollapsed: false }
export const INBOX_COLLAPSED_WIDTH = 56

// Minimum widths below which a splitter cannot be dragged (design D-B).
export const MIN_INBOX_WIDTH = 200
export const MIN_LIST_WIDTH = 280
export const MIN_THREAD_WIDTH = 480

function isRecord(v: unknown): v is Record<string, unknown> {
  return typeof v === 'object' && v !== null
}

/** readLayout never throws. A browser that refuses storage gets the defaults. */
export function readLayout(): Layout {
  try {
    const raw = localStorage.getItem(LAYOUT_STORAGE_KEY)
    if (!raw) return DEFAULT_LAYOUT
    const parsed: unknown = JSON.parse(raw)
    if (!isRecord(parsed)) return DEFAULT_LAYOUT
    return {
      inboxWidth: typeof parsed.inboxWidth === 'number' ? parsed.inboxWidth : DEFAULT_LAYOUT.inboxWidth,
      listWidth: typeof parsed.listWidth === 'number' ? parsed.listWidth : DEFAULT_LAYOUT.listWidth,
      inboxCollapsed:
        typeof parsed.inboxCollapsed === 'boolean' ? parsed.inboxCollapsed : DEFAULT_LAYOUT.inboxCollapsed,
    }
  } catch {
    return DEFAULT_LAYOUT
  }
}

/** writeLayout never throws. A full or disabled store simply does not remember. */
export function writeLayout(layout: Layout): void {
  try {
    localStorage.setItem(LAYOUT_STORAGE_KEY, JSON.stringify(layout))
  } catch {
    // The view already rendered at whatever `layout` holds; the next session
    // tries the read again rather than being told about this one write.
  }
}

/** The layout, read once per mount and written through on every change. */
export function useLayout(): [Layout, (patch: Partial<Layout>) => void] {
  const [layout, setLayout] = useState<Layout>(() => readLayout())
  const update = useCallback((patch: Partial<Layout>) => {
    setLayout((prev) => {
      const next = { ...prev, ...patch }
      writeLayout(next)
      return next
    })
  }, [])
  return [layout, update]
}
