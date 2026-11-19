import { ConversationGraph } from './ConversationGraph'
import { graphContract } from '../../../../graph/contract'

// The workspace look's chrome, held to the same contract as classic's: every
// view, filter, scope, replay, panel and fold the classic graph has.
graphContract(ConversationGraph)
