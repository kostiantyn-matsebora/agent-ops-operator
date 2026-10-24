import type { Topology } from '../../api/types'
import type { ViewGraph, ViewId } from '../types'
import { componentsView } from './components'
import { foldToRoutes } from './fold'
import { infrastructureView } from './infrastructure'
import { modelView } from './model'

export type Detail = 'routes' | 'full'

export interface ViewOptions {
  /** The Model's fold. Absent on the other views. */
  detail?: Detail
  /** Conversation pods opened into their containers. */
  expanded?: ReadonlySet<string>
}

export function buildView(view: ViewId, topo: Topology, o: ViewOptions = {}): ViewGraph {
  switch (view) {
    case 'components':
      return componentsView(topo)
    case 'infrastructure':
      return infrastructureView(topo, o.expanded)
    default: {
      const g = modelView(topo)
      return o.detail === 'routes' ? foldToRoutes(g) : g
    }
  }
}

export { componentsView, foldToRoutes, infrastructureView, modelView }
