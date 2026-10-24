import type { ViewId } from '../types'

// Each view lists only the classes it can draw, and names the one it cannot do
// without. The display control reads these, so a class never offered on a view
// is never a toggle that does nothing there.

export const VIEW_CLASSES: Record<ViewId, string[]> = {
  model: [
    'signaladapters', 'signalsources', 'pipelines', 'coordinators', 'agentprofiles',
    'agentcapabilities', 'agentruntimes', 'mcptoolsets', 'mcpconfigs', 'channels',
    'channeladapters', 'conversations',
  ],
  components: [
    'signal-adapter', 'manager', 'channel-adapter', 'gateway', 'runtime-image', 'sidecar',
    'housekeeping', 'workload', 'model', 'mcp-server', 'repository', 'external',
  ],
  infrastructure: ['pod', 'container', 'model', 'mcp-server', 'repository', 'external'],
}

export const SPINE_CLASS: Record<ViewId, string> = {
  model: 'pipelines',
  components: 'manager',
  infrastructure: 'pod',
}

/** The Model classes the routes-only fold keeps around each pipeline. A
 * Coordinator is a route too — it claims sources and posts to channels
 * exactly as a Pipeline does — so it is kept here. AgentCapability is not: it
 * is a leaf/reach object, referenced rather than routing anything. */
export const ROUTE_CLASSES = [
  'signaladapters', 'signalsources', 'pipelines', 'coordinators', 'channels', 'channeladapters',
]

export const MANAGER = 'manager/manager'

/** The activity vocabulary's names for Model kinds, when the BFF serves none. */
export const DEFAULT_EVENT_NODE_KINDS: Record<string, string> = {
  'signal-adapter': 'signaladapters',
  'signal-source': 'signalsources',
  pipeline: 'pipelines',
  profile: 'agentprofiles',
  runtime: 'agentruntimes',
  channel: 'channels',
  'channel-adapter': 'channeladapters',
  toolset: 'mcptoolsets',
  'mcp-config': 'mcpconfigs',
  conversation: 'conversations',
}
