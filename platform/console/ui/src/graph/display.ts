import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import type { EdgeLabel } from './hops'
import type { BoxBy } from './layout/boxes'
import type { LayoutId } from './layout'
import type { ViewId } from './types'
import type { Detail } from './views'

// The display control — Kiali's idiom, one per view.
//
// HIDING IS PRESENTATION ONLY. A hidden class still counts toward the health
// summary and the overview's problem rollup, and the control says when hidden
// elements include failures.
//
// EACH VIEW REMEMBERS ITS OWN classes, layout and boxes. Persisted across
// navigation and reload under a key of its own, so a selection saved by the
// lane graph is dropped rather than misapplied to a view that never had it.
// A scope is NOT kept here: a new visit opens unscoped.

export interface ViewDisplay {
  hidden: string[]
  layout: LayoutId
  box: BoxBy
}

export interface DisplayState {
  view: ViewId
  views: Record<ViewId, ViewDisplay>
  /** The Model's fold. */
  detail: Detail
  animate: boolean
  idleNodes: boolean
  idleEdges: boolean
  edgeLabels: EdgeLabel
  windowSeconds: number
  /** The routes shown, or null for every pipeline. */
  pipelines: string[] | null
  panelOpen: boolean
  setView: (v: ViewId) => void
  toggleClass: (cls: string) => void
  hideClass: (cls: string) => void
  setLayout: (l: LayoutId) => void
  setBox: (b: BoxBy) => void
  setDetail: (d: Detail) => void
  setAnimate: (v: boolean) => void
  setIdleNodes: (v: boolean) => void
  setIdleEdges: (v: boolean) => void
  setEdgeLabels: (v: EdgeLabel) => void
  setWindow: (v: number) => void
  setPipelines: (p: string[] | null) => void
  setPanelOpen: (v: boolean) => void
  reset: () => void
}

export const VIEW_DEFAULTS: Record<ViewId, ViewDisplay> = {
  model: { hidden: [], layout: 'cola', box: 'bundle' },
  components: { hidden: [], layout: 'concentric', box: 'none' },
  infrastructure: { hidden: [], layout: 'cola', box: 'node' },
}

const DEFAULTS = {
  view: 'model' as ViewId,
  views: VIEW_DEFAULTS,
  detail: 'full' as Detail,
  animate: true,
  idleNodes: true,
  idleEdges: true,
  edgeLabels: 'none' as EdgeLabel,
  windowSeconds: 300,
  pipelines: null as string[] | null,
  panelOpen: false,
}

export const useDisplay = create<DisplayState>()(
  persist(
    (set) => {
      const patch = (fn: (v: ViewDisplay) => Partial<ViewDisplay>) =>
        set((s) => ({ views: { ...s.views, [s.view]: { ...s.views[s.view], ...fn(s.views[s.view]) } } }))
      return {
        ...DEFAULTS,
        setView: (view) => set({ view }),
        toggleClass: (cls) =>
          patch((v) => ({ hidden: v.hidden.includes(cls) ? v.hidden.filter((c) => c !== cls) : [...v.hidden, cls] })),
        hideClass: (cls) => patch((v) => ({ hidden: v.hidden.includes(cls) ? v.hidden : [...v.hidden, cls] })),
        setLayout: (layout) => patch(() => ({ layout })),
        setBox: (box) => patch(() => ({ box })),
        setDetail: (detail) => set({ detail }),
        setAnimate: (animate) => set({ animate }),
        setIdleNodes: (idleNodes) => set({ idleNodes }),
        setIdleEdges: (idleEdges) => set({ idleEdges }),
        setEdgeLabels: (edgeLabels) => set({ edgeLabels }),
        setWindow: (windowSeconds) => set({ windowSeconds }),
        setPipelines: (pipelines) => set({ pipelines }),
        setPanelOpen: (panelOpen) => set({ panelOpen }),
        // Restores the display OPTIONS and keeps the view being looked at.
        reset: () => set((s) => ({ ...DEFAULTS, view: s.view, panelOpen: s.panelOpen })),
      }
    },
    { name: 'agentops-console-topology', version: 1 },
  ),
)
