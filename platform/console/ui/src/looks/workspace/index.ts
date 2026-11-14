import type { LookModule } from '../registry'
import { ConversationsPage } from './conversations/Conversations'
import { LoginPage } from './pages/Login'
import {
  ConfigDetailPage, ConfigKindPage, ConfigPage, OverviewPage, QueuesPage, TopologyPage,
} from './pages/Placeholders'
import { Shell as WorkspaceShell } from './Shell'

// The workspace look: the conversations as one workspace — a navigation
// panel, a list and a thread — filling the window, drawn with its own plain
// CSS (workspace.css) over the model layer. It imports `model/` and
// `api/types`, never the API client and never another look.
//
// The pages other than Conversations and Login are TODO(4.x) markers until
// section 4 of console-visual-refresh restyles them.

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
