import type { LookModule } from '../registry'
import { App } from './App'
import { ConfigDetailPage, ConfigKindPage, ConfigPage } from './pages/Config'
import { LoginPage } from './pages/Login'
import { OverviewPage } from './pages/Overview'
import { QueuesPage } from './pages/Queues'
import { TopologyPage } from './pages/Topology'
import { ChatView } from './pages/chat/ChatView'

// The classic look: the console as it was before looks existed, moved here
// with only its import paths changed.

export const Shell: LookModule['Shell'] = App

export const pages: LookModule['pages'] = {
  Overview: OverviewPage,
  Queues: QueuesPage,
  Config: ConfigPage,
  ConfigKind: ConfigKindPage,
  ConfigDetail: ConfigDetailPage,
  Topology: TopologyPage,
  Conversations: ChatView,
  Login: LoginPage,
}
