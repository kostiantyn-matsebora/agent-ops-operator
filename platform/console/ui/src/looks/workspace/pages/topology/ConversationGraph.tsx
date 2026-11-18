import { GraphView, type GraphProps } from '../../../../graph/GraphView'
import { workspaceChrome } from './chrome'
import './topology.css'

/** One conversation's graph, drawn in the workspace look: the topology page's canvas and chrome, opened on a replay. */
export function ConversationGraph(props: Readonly<GraphProps>) {
  return <GraphView chrome={workspaceChrome} {...props} />
}
