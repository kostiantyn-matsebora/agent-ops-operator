import type { LookModule } from '../registry'
import { ConversationsPage } from './conversations/Conversations'
import { LoginPage } from './pages/Login'
import { ConfigDetailPage, ConfigKindPage, ConfigPage } from './pages/config'
import { OverviewPage } from './pages/Overview'
import { QueuesPage } from './pages/Queues'
import { TopologyPage } from './pages/topology'
import { Shell as WorkspaceShell } from './Shell'

// The workspace look: the conversations as one workspace — a navigation
// panel, a list and a thread — filling the window, drawn with its own plain
// CSS (workspace.css) over the model layer. It imports `model/` and
// `api/types`, never the API client and never another look.
//

export const Shell: LookModule['Shell'] = WorkspaceShell

export const pages: LookModule['pages'] = {
  Overview: OverviewPage,
  Queues: QueuesPage,
  Config: ConfigPage,
  ConfigKind: ConfigKindPage,
  ConfigDetail: ConfigDetailPage,
  Topology: TopologyPage,
  Conversations: ConversationsPage,
  Login: LoginPage,
}
