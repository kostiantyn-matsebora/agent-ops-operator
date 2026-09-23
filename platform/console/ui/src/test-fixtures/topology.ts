import type {
  ActivityEvent, Component, Container, ConversationDetail, GraphEdge, GraphNode, Pod, Topology,
} from '../api/types'

// One install, shaped like the mockup's: three routes, two runtimes on two
// images, a console adapter serving a channel AND a chat source, two cluster
// nodes. Every view, route and layout test reads the same picture.

export const CLAUDE_IMAGE = 'registry.example.com/agentops-runtime-claude:0.9.3'
export const OLLAMA_IMAGE = 'registry.example.com/agentops-runtime-ollama:0.4.1'
export const REPO = 'https://git.example.com/ops/agents.git'

const n = (kind: string, name: string, extra: Partial<GraphNode> = {}): GraphNode => ({
  id: `${kind}/${name}`, kind, name, health: 'ok', active: 0, recent: 0, ...extra,
})
const e = (from: string, to: string, kind: GraphEdge['kind'], extra: Partial<GraphEdge> = {}): GraphEdge => ({
  from, to, kind, ...extra,
})
const c = (role: string, name: string, extra: Partial<Component> = {}): Component => ({
  id: `${role}/${name}`, role, name, count: 0, health: 'none', ...extra,
})
const runtimeContainers = (image: string): Container[] => [
  { name: 'worker', image, role: 'runtime-image', ready: true, restarts: 0 },
  { name: 'context-sync', image: 'registry.example.com/agentops-context-sync:0.3.0', role: 'context-sync', ready: true, restarts: 0 },
  { name: 'egress-proxy', image: 'registry.example.com/agentops-egress-proxy:0.2.1', role: 'egress-proxy', ready: true, restarts: 0 },
]
const pod = (name: string, node: string, extra: Partial<Pod> = {}): Pod => ({
  id: `pods/${name}`, name, clusterNode: node, phase: 'Running', health: 'ok',
  containers: [{ name: 'main', ready: true, restarts: 0 }], ...extra,
})
const convPod = (conv: string, node: string, pipeline: string, image: string): Pod =>
  pod(`agentops-conv-${conv}`, node, {
    component: `runtime-image/${image}`, conversation: conv, pipeline, containers: runtimeContainers(image),
  })

export function fixtureTopology(): Topology {
  return {
    eventNodeKinds: {
      'signal-adapter': 'signaladapters', 'signal-source': 'signalsources', pipeline: 'pipelines',
      profile: 'agentprofiles', runtime: 'agentruntimes', channel: 'channels',
      'channel-adapter': 'channeladapters', toolset: 'mcptoolsets', 'mcp-config': 'mcpconfigs',
      conversation: 'conversations',
    },
    windowSeconds: 300,
    nodes: [
      n('signaladapters', 'alertmanager', { bundle: 'prometheus' }),
      n('signaladapters', 'k8s-events', { bundle: 'kubernetes' }),
      n('signaladapters', 'cron', { bundle: 'release' }),
      n('signaladapters', 'console', { bundle: 'console', servedBy: 'channeladapters/console' }),
      n('signalsources', 'prometheus-alerts', { bundle: 'prometheus' }),
      n('signalsources', 'cluster-events', { bundle: 'kubernetes' }),
      n('signalsources', 'nightly', { bundle: 'release' }),
      n('signalsources', 'console', { bundle: 'console' }),
      n('signalsources', 'bench-sensors', {
        health: 'bad', reason: 'NoPipelineClaim', message: 'no Ready Pipeline lists this source', detached: true,
      }),
      n('pipelines', 'alert-triage', { bundle: 'prometheus', active: 1, recent: 1 }),
      n('pipelines', 'k8s-observe', { bundle: 'kubernetes', recent: 1 }),
      n('pipelines', 'nightly-report', { bundle: 'release', recent: 1 }),
      n('pipelines', 'chat-helper', { bundle: 'console' }),
      n('agentprofiles', 'alert-investigator', { health: 'none', bundle: 'prometheus' }),
      n('agentprofiles', 'k8s-engineer', { health: 'none', bundle: 'kubernetes' }),
      n('agentprofiles', 'release-scribe', { health: 'none', bundle: 'release' }),
      n('agentruntimes', 'default', { health: 'none', bundle: 'claude', image: CLAUDE_IMAGE, harness: 'Claude Code', vendor: 'Anthropic' }),
      n('agentruntimes', 'sandbox', { health: 'none', bundle: 'ollama', image: OLLAMA_IMAGE, harness: 'agent-ops', vendor: 'Ollama' }),
      n('mcptoolsets', 'agentops-observe', { health: 'none' }),
      n('mcptoolsets', 'agentops-shell', { health: 'none' }),
      n('mcpconfigs', 'kubernetes', { health: 'none', bundle: 'kubernetes' }),
      n('mcpconfigs', 'prometheus', { health: 'none', bundle: 'prometheus' }),
      n('channels', 'console', { bundle: 'console' }),
      n('channels', 'ops-chat', { bundle: 'telegram' }),
      n('channeladapters', 'console', { bundle: 'console', externals: [{ name: 'kubernetes-api', kind: 'kubernetes' }] }),
      n('channeladapters', 'telegram', { bundle: 'telegram', externals: [{ name: 'telegram-api', kind: 'api' }] }),
      n('conversations', 'prometheus-alerts-a1', { health: 'none', phase: 'Working', runtimePod: 'pods/agentops-conv-prometheus-alerts-a1' }),
      n('conversations', 'cluster-events-b7', { health: 'none', phase: 'Idle', runtimePod: 'pods/agentops-conv-cluster-events-b7' }),
      n('conversations', 'nightly-d2', { health: 'none', phase: 'Idle', runtimePod: 'pods/agentops-conv-nightly-d2' }),
    ],
    edges: [
      e('signalsources/prometheus-alerts', 'signaladapters/alertmanager', 'served-by'),
      e('signalsources/cluster-events', 'signaladapters/k8s-events', 'served-by'),
      e('signalsources/nightly', 'signaladapters/cron', 'served-by'),
      e('signalsources/console', 'signaladapters/console', 'served-by'),
      e('signalsources/prometheus-alerts', 'pipelines/alert-triage', 'feeds'),
      e('signalsources/cluster-events', 'pipelines/k8s-observe', 'feeds'),
      e('signalsources/nightly', 'pipelines/nightly-report', 'feeds'),
      e('signalsources/console', 'pipelines/chat-helper', 'feeds'),
      e('pipelines/alert-triage', 'agentprofiles/alert-investigator', 'answers'),
      e('pipelines/k8s-observe', 'agentprofiles/k8s-engineer', 'answers'),
      e('pipelines/nightly-report', 'agentprofiles/release-scribe', 'answers'),
      e('pipelines/chat-helper', 'agentprofiles/k8s-engineer', 'answers'),
      e('pipelines/alert-triage', 'agentruntimes/default', 'runs-on'),
      e('pipelines/k8s-observe', 'agentruntimes/default', 'runs-on'),
      e('pipelines/nightly-report', 'agentruntimes/sandbox', 'runs-on'),
      e('pipelines/chat-helper', 'agentruntimes/default', 'runs-on'),
      e('pipelines/alert-triage', 'mcptoolsets/agentops-observe', 'uses'),
      e('pipelines/k8s-observe', 'mcptoolsets/agentops-observe', 'uses'),
      e('pipelines/k8s-observe', 'mcptoolsets/agentops-shell', 'uses'),
      e('pipelines/alert-triage', 'mcpconfigs/prometheus', 'uses'),
      e('pipelines/k8s-observe', 'mcpconfigs/kubernetes', 'uses'),
      e('pipelines/alert-triage', 'channels/console', 'posts'),
      e('pipelines/alert-triage', 'channels/ops-chat', 'posts'),
      e('pipelines/k8s-observe', 'channels/console', 'posts'),
      e('pipelines/nightly-report', 'channels/console', 'posts'),
      e('pipelines/chat-helper', 'channels/ops-chat', 'posts'),
      e('channels/console', 'channeladapters/console', 'served-by'),
      e('channels/ops-chat', 'channeladapters/telegram', 'served-by'),
      e('pipelines/alert-triage', 'conversations/prometheus-alerts-a1', 'opened'),
      e('pipelines/k8s-observe', 'conversations/cluster-events-b7', 'opened'),
      e('pipelines/nightly-report', 'conversations/nightly-d2', 'opened'),
    ],
    components: [
      c('manager', 'manager', { count: 1, workload: 'deployments/agentops-manager' }),
      c('signal-adapter', 'alertmanager', { health: 'ok', count: 1, implements: ['signaladapters/alertmanager'], bundle: 'prometheus' }),
      c('signal-adapter', 'k8s-events', { health: 'ok', count: 1, implements: ['signaladapters/k8s-events'], bundle: 'kubernetes' }),
      c('signal-adapter', 'cron', { health: 'ok', count: 1, implements: ['signaladapters/cron'], bundle: 'release' }),
      c('signal-adapter', 'console', { health: 'ok', implements: ['signaladapters/console'], servedBy: 'channel-adapter/console' }),
      c('channel-adapter', 'console', { health: 'ok', count: 1, implements: ['channeladapters/console'] }),
      c('channel-adapter', 'telegram', { health: 'ok', count: 1, implements: ['channeladapters/telegram'] }),
      c('gateway', 'telegram', { count: 1 }),
      c('runtime-image', CLAUDE_IMAGE, { count: 2, image: CLAUDE_IMAGE, implements: ['agentruntimes/default'], harness: 'Claude Code', vendor: 'Anthropic' }),
      c('runtime-image', OLLAMA_IMAGE, { count: 1, image: OLLAMA_IMAGE, implements: ['agentruntimes/sandbox'], harness: 'agent-ops', vendor: 'Ollama' }),
      c('context-sync', 'context-sync', { count: 3 }),
      c('egress-proxy', 'egress-proxy', { count: 3 }),
      c('housekeeping', 'housekeeping'),
      c('model', 'claude-sonnet-5'),
      c('model', 'qwen3:14b'),
      c('mcp-server', 'kubernetes', { count: 1, implements: ['mcpconfigs/kubernetes'], workload: 'deployments/kubernetes-mcp' }),
      c('mcp-server', 'prometheus', { implements: ['mcpconfigs/prometheus'], url: 'http://prometheus-mcp.example.com/mcp' }),
      c('repository', REPO, { url: REPO, implements: ['agentprofiles/alert-investigator', 'agentprofiles/k8s-engineer', 'agentprofiles/release-scribe'] }),
      c('external', 'alertmanager', { externalKind: 'sender' }),
      c('external', 'kubernetes-api', { externalKind: 'kubernetes' }),
      c('external', 'telegram-api', { externalKind: 'api' }),
    ],
    componentEdges: [
      e('external/alertmanager', 'signal-adapter/alertmanager', 'sends'),
      e('signal-adapter/k8s-events', 'external/kubernetes-api', 'calls'),
      e('channel-adapter/console', 'external/kubernetes-api', 'calls'),
      e('channel-adapter/telegram', 'external/telegram-api', 'calls'),
    ],
    pods: [
      pod('agentops-manager-7c9d', 'node-a', { component: 'manager/manager' }),
      pod('agentops-signal-alertmanager-6b7d', 'node-a', { component: 'signal-adapter/alertmanager' }),
      pod('agentops-signal-k8s-events-2f0a', 'node-a', { component: 'signal-adapter/k8s-events' }),
      pod('agentops-signal-cron-9c21', 'node-b', { component: 'signal-adapter/cron' }),
      pod('agentops-adapter-console-4d1c', 'node-a', { component: 'channel-adapter/console' }),
      pod('agentops-adapter-telegram-b3e9', 'node-b', { component: 'channel-adapter/telegram' }),
      pod('agentops-gateway-telegram-e21f', 'node-b', { component: 'gateway/telegram' }),
      pod('kubernetes-mcp-5a8f', 'node-a', { component: 'mcp-server/kubernetes' }),
      pod('agentops-housekeeping-29473120-x8k2', 'node-b', { component: 'housekeeping/housekeeping', phase: 'Succeeded' }),
      convPod('prometheus-alerts-a1', 'node-b', 'alert-triage', CLAUDE_IMAGE),
      convPod('cluster-events-b7', 'node-a', 'k8s-observe', CLAUDE_IMAGE),
      convPod('nightly-d2', 'node-b', 'nightly-report', OLLAMA_IMAGE),
    ],
    hops: [
      { from: { kind: 'runtime-image', name: CLAUDE_IMAGE }, to: { kind: 'model', name: 'claude-sonnet-5' }, events: 4, errors: 0, ratePerMin: 0.8, unconfirmed: false },
      { from: { kind: 'runtime-image', name: OLLAMA_IMAGE }, to: { kind: 'model', name: 'qwen3:14b' }, events: 2, errors: 0, ratePerMin: 0.4, unconfirmed: false },
    ],
  }
}

let cursor = 0

/** One hop in the activity vocabulary. `from`/`to` are `<kind>/<name>`. */
export function hop(kind: string, from: string | null, to: string | null, extra: Partial<ActivityEvent> = {}): ActivityEvent {
  const ref = (id: string | null) => {
    if (!id) return undefined
    const i = id.indexOf('/')
    return { kind: id.slice(0, i), name: id.slice(i + 1) }
  }
  cursor++
  return {
    cursor: String(cursor).padStart(8, '0'), ts: new Date(Date.UTC(2026, 8, 23, 10, 0, 0)).toISOString(),
    kind, status: 'ok', from: ref(from), to: ref(to), ...extra,
  }
}

/** A conversation whose status records run r7: its input, its result, its delivery. */
export const RESULT = '<title>checkout restarts on OOM</title>\nContainer `checkout` is killed at 512Mi every ~40s.'

export function detailWithRun(): ConversationDetail {
  return {
    conversation: {
      name: 'cluster-events-b7', runCount: 1, queued: 0, joined: true, errored: false, unread: false,
      ageSeconds: 10, deleting: false, toolsets: ['agentops-observe'], mcpConfigs: ['kubernetes'],
      runs: [{ runId: 'r7', status: 'succeeded', exitCode: 0, result: RESULT }],
    },
    object: {
      kind: 'conversations', metadata: { name: 'cluster-events-b7' },
      spec: { inputs: [{ id: 'in-q', type: 'task', payload: 'queued, not yet run' }] },
      status: {
        runtimeContextId: 'ctx-b7-4',
        runs: [{
          runId: 'r7', status: 'succeeded', exitCode: 0, result: RESULT, delivered: ['console'],
          inputs: [{ id: 'in-7', text: 'why is checkout restarting?', sender: 'operator', surface: 'console' }],
        }],
      },
    },
    yaml: '', transcript: [], archived: false, events: [],
  }
}
