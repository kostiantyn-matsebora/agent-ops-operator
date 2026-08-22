import { create } from 'zustand'
import { persist } from 'zustand/middleware'

// The Display panel — Kiali's idiom, and the reason both graphs can be read as
// the wiring spine when you want orientation and as the full capability picture
// when you are debugging what an agent can reach.
//
// HIDING IS PRESENTATION ONLY. A hidden class still counts toward the graph's
// health summary and the overview's problem rollup, and the panel says when
// hidden classes contain failures. A filter that can conceal a broken component
// without saying so is the one way this view could mislead, so the honesty
// machinery lives with the toggles rather than being left to each view.

/** Node classes that can be shown or hidden independently. */
export const NODE_CLASSES = {
  signalsources: 'Signal sources',
  channels: 'Channels',
  signaladapters: 'Signal adapters',
  channeladapters: 'Channel adapters',
  pipelines: 'Pipelines',
  agentprofiles: 'Profiles',
  agentruntimes: 'Runtimes',
  mcptoolsets: 'Toolsets',
  mcpconfigs: 'MCP configs',
  pods: 'Runtime pods',
  conversations: 'Conversations',
} as const

export type NodeClass = keyof typeof NODE_CLASSES

export type EdgeLabel = 'none' | 'rate' | 'latency'

export interface DisplayState {
  /** Hidden classes; pipelines are deliberately not hideable — the graph would have no spine. */
  hidden: Record<string, boolean>
  animate: boolean
  showIdle: boolean
  edgeLabels: EdgeLabel
  windowSeconds: number
  /** Whether the panel itself is expanded. Collapsed gives the graph the width. */
  panelOpen: boolean
  toggleClass: (c: NodeClass) => void
  setAnimate: (v: boolean) => void
  setShowIdle: (v: boolean) => void
  setEdgeLabels: (v: EdgeLabel) => void
  setWindow: (v: number) => void
  setPanelOpen: (v: boolean) => void
  reset: () => void
}

const DEFAULTS = {
  // The capability layer starts folded away: it is what you bring forward when
  // debugging reach, not what you want between you and the wiring on first look.
  hidden: { mcptoolsets: true, mcpconfigs: true, pods: true } as Record<string, boolean>,
  animate: true,
  showIdle: true,
  edgeLabels: 'rate' as EdgeLabel,
  windowSeconds: 300,
  panelOpen: true,
}

export const useDisplay = create<DisplayState>()(
  persist(
    (set) => ({
      ...DEFAULTS,
      toggleClass: (c) => set((s) => ({ hidden: { ...s.hidden, [c]: !s.hidden[c] } })),
      setAnimate: (animate) => set({ animate }),
      setShowIdle: (showIdle) => set({ showIdle }),
      setEdgeLabels: (edgeLabels) => set({ edgeLabels }),
      setWindow: (windowSeconds) => set({ windowSeconds }),
      setPanelOpen: (panelOpen) => set({ panelOpen }),
      // reset restores the display OPTIONS, not the panel's own open state:
      // "reset display" means "undo my filtering", and closing the panel on
      // someone who just pressed it would read as the button breaking.
      reset: () => set({ ...DEFAULTS, panelOpen: true }),
    }),
    // Persisted across navigation AND reload: a Display selection is how an
    // operator has decided to read the graph, and losing it on every page change
    // makes the panel not worth using.
    { name: 'agentops-console-display' },
  ),
)

export function isHidden(hidden: Record<string, boolean>, kind: string): boolean {
  return Boolean(hidden[kind])
}
