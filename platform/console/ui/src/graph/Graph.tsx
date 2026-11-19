import { classicChrome } from './classicChrome'
import { GraphView, type GraphProps } from './GraphView'

// The graph as the classic look draws it. A look with chrome of its own renders
// `GraphView` with that chrome instead (looks/workspace/pages/topology).

export type { GraphProps }

export function Graph(props: Readonly<GraphProps>) {
  return <GraphView chrome={classicChrome} {...props} />
}
