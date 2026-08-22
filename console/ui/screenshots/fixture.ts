// The install the site's screenshots are taken of.
//
// ONE curated namespace, entirely invented. Nothing here is copied from a real
// cluster — no host, no namespace, no identity, no image digest — because a
// published screenshot is the one place a stray production name is impossible
// to take back.
//
// It is written to be INTERESTING rather than empty: three pipelines, two
// runtimes, a live run, a queue with something waiting, and one condition that
// is not True. A tour of empty states teaches nobody what the console is for.
//
// Every age is a NUMBER of seconds and every timestamp is relative to NOW
// (below), which the capture freezes. That is what makes two runs produce
// byte-identical images.

import type {
  VocabularyResponse, ConversationDetail, ConversationGraph, ConversationPage, Detail,
  Finding, InventoryRow, KindInfo, Overview, Queues, Session, SourcesResponse,
  TopologyResponse,
} from '../src/api/types'

/** The clock the capture pins the browser to. */
export const NOW = new Date('2025-06-11T09:40:00Z')

const ago = (seconds: number) => new Date(NOW.getTime() - seconds * 1000).toISOString()

// ---- session -----------------------------------------------------------------

const session: Session = {
  authenticated: true,
  configured: true,
  identity: 'dana@example.com',
  identitySource: 'forward-auth',
  authMode: 'token',
  externalAuthenticator: '',
  writeEnabled: true,
  canWrite: true,
  canOriginate: true,
  metrics: false,
}

// ---- overview ----------------------------------------------------------------

const overview: Overview = {
  namespace: 'agent-ops',
  manager: {
    version: '0.14.0',
    leader: 'agentops-manager-6d4b8c9f7-w2xkq',
    runtimeSlots: { inUse: 2, max: 5, waiting: 1 },
    queues: [
      { adapter: 'console', queued: 0, claimed: 1 },
      { adapter: 'telegram', queued: 2, claimed: 0, oldestQueuedAgeSeconds: 34 },
    ],
    cooldowns: [{ source: 'prometheus-alerts', suppressed: 4, windowSeconds: 900 }],
  },
  stream: { connected: true, cursor: '18244', events: 1842, resyncs: 0 },
  workloads: [
    { name: 'agentops-manager', image: 'agentops-manager:0.14.0', desired: 1, ready: 1, restarts: 0 },
    { name: 'agentops-adapter-console', image: 'agentops-console:0.14.0', desired: 1, ready: 1, restarts: 0 },
    { name: 'agentops-adapter-telegram', image: 'agentops-channel-telegram:0.6.1', desired: 1, ready: 1, restarts: 0 },
    { name: 'agentops-signal-k8s-events', image: 'agentops-signal-k8s-events:0.4.2', desired: 1, ready: 1, restarts: 0 },
    { name: 'agentops-signal-alertmanager', image: 'agentops-signal-vmalertmanager:0.5.0', desired: 1, ready: 1, restarts: 1 },
    { name: 'agentops-signal-cron', image: 'agentops-signal-cron:0.4.0', desired: 1, ready: 1, restarts: 0 },
  ],
  runtimes: [
    { name: 'default', image: 'agentops-runtime-claude:0.5.1' },
    { name: 'sandbox', image: 'agentops-runtime-claude:0.5.1-sandbox' },
  ],
  adapters: [
    { kind: 'channeladapters', name: 'console', image: 'agentops-console:0.14.0', health: 'ok', serves: 1 },
    { kind: 'channeladapters', name: 'telegram', image: 'agentops-channel-telegram:0.6.1', health: 'ok', serves: 1 },
    { kind: 'signaladapters', name: 'k8s-events', image: 'agentops-signal-k8s-events:0.4.2', health: 'ok', serves: 1 },
    { kind: 'signaladapters', name: 'alertmanager', image: 'agentops-signal-vmalertmanager:0.5.0', health: 'ok', serves: 1 },
    { kind: 'signaladapters', name: 'cron', image: 'agentops-signal-cron:0.4.0', health: 'ok', serves: 2 },
  ],
  counts: {
    agentprofiles: 3, agentruntimes: 2, channels: 2, channeladapters: 2,
    conversations: 6, mcpconfigs: 2, mcptoolsets: 4, pipelines: 3,
    signaladapters: 3, signalsources: 5,
  },
  synced: {
    agentprofiles: true, agentruntimes: true, channels: true, channeladapters: true,
    conversations: true, mcpconfigs: true, mcptoolsets: true, pipelines: true,
    signaladapters: true, signalsources: true,
  },
  problems: [
    {
      kind: 'signalsources',
      name: 'bench-sensors',
      type: 'Wired',
      reason: 'NoPipeline',
      message: 'no Ready Pipeline lists this source, so signals posted to it are dropped',
      since: ago(5400),
      source: 'reported',
    },
  ],
}

// ---- queues ------------------------------------------------------------------

const queues: Queues = {
  capacity: { inUse: 2, max: 5, waiting: 1 },
  work: [
    {
      conversation: 'cluster-events-7c1d4e',
      title: 'checkout-api is restarting',
      pipeline: 'k8s-observe',
      phase: 'Working',
      queued: 0,
      ageSeconds: 96,
      inflight: 'run-4',
      inflightAgeSeconds: 42,
    },
    {
      conversation: 'console-3f9a2b',
      title: 'Why is the payments deployment not rolling out?',
      pipeline: 'k8s-observe',
      phase: 'Working',
      queued: 1,
      ageSeconds: 61,
      inflight: 'run-2',
      inflightAgeSeconds: 18,
    },
    {
      conversation: 'prometheus-alerts-91b7fd',
      title: 'HighMemoryPressure on node-3',
      pipeline: 'alert-triage',
      phase: 'Pending',
      queued: 1,
      ageSeconds: 27,
      stuck: 'at-runtime-ceiling',
    },
  ],
  delivery: [
    { adapter: 'console', queued: 0, claimed: 1, oldestClaimedOpId: 'send:console-3f9a2b:console:run-2', oldestClaimedAgeSeconds: 3, oldestClaimedConversation: 'console-3f9a2b', adapterHealth: 'ok' },
    { adapter: 'telegram', queued: 2, claimed: 0, oldestQueuedOpId: 'send:cluster-events-7c1d4e:ops-chat:run-3', oldestQueuedAgeSeconds: 34, oldestQueuedConversation: 'cluster-events-7c1d4e', adapterHealth: 'ok' },
  ],
  cooldowns: [{ source: 'prometheus-alerts', suppressed: 4, windowSeconds: 900 }],
}

// ---- configuration -----------------------------------------------------------

const kinds: KindInfo[] = [
  { kind: 'pipelines', title: 'Pipelines', count: 3, synced: true },
  { kind: 'agentprofiles', title: 'Agent profiles', count: 3, synced: true },
  { kind: 'agentruntimes', title: 'Agent runtimes', count: 2, synced: true },
  { kind: 'signalsources', title: 'Signal sources', count: 5, synced: true },
  { kind: 'signaladapters', title: 'Signal adapters', count: 3, synced: true },
  { kind: 'channels', title: 'Channels', count: 2, synced: true },
  { kind: 'channeladapters', title: 'Channel adapters', count: 2, synced: true },
  { kind: 'mcptoolsets', title: 'MCP toolsets', count: 4, synced: true },
  { kind: 'mcpconfigs', title: 'MCP configs', count: 2, synced: true },
  { kind: 'conversations', title: 'Conversations', count: 6, synced: true },
]

const inventory: Record<string, InventoryRow[]> = {
  pipelines: [
    {
      name: 'k8s-observe', created: ago(864000), health: 'ok', findings: 0,
      conditions: [{ type: 'Ready', status: 'True', reason: 'Wired' }],
      columns: {
        profile: 'k8s-engineer', sources: 'cluster-events, console',
        channels: 'console, ops-chat', toolsets: 'agentops-observe, k8s-observability',
        toolsMode: 'merge', mcpConfigs: 'k8s-api',
      },
    },
    {
      name: 'alert-triage', created: ago(604800), health: 'ok', findings: 0,
      conditions: [{ type: 'Ready', status: 'True', reason: 'Wired' }],
      columns: {
        profile: 'alert-investigator', sources: 'prometheus-alerts',
        channels: 'ops-chat', toolsets: 'agentops-observe, prometheus-query',
        toolsMode: 'merge', mcpConfigs: 'metrics',
      },
    },
    {
      name: 'nightly-report', created: ago(259200), health: 'ok', findings: 0,
      conditions: [{ type: 'Ready', status: 'True', reason: 'Wired' }],
      columns: {
        profile: 'release-scribe', sources: 'nightly',
        channels: 'ops-chat', toolsets: 'agentops-observe',
        toolsMode: 'merge', mcpConfigs: '',
      },
    },
  ],
  signalsources: [
    { name: 'cluster-events', created: ago(864000), health: 'ok', findings: 0, columns: { adapter: 'k8s-events', served: 'true', wired: 'true' } },
    { name: 'console', created: ago(864000), health: 'ok', findings: 0, columns: { adapter: 'console', served: 'true', wired: 'true' } },
    { name: 'prometheus-alerts', created: ago(604800), health: 'ok', findings: 0, columns: { adapter: 'alertmanager', served: 'true', wired: 'true' } },
    { name: 'nightly', created: ago(259200), health: 'ok', findings: 0, columns: { adapter: 'cron', served: 'true', wired: 'true' } },
    {
      name: 'bench-sensors', created: ago(5400), health: 'bad', findings: 1,
      conditions: [{ type: 'Wired', status: 'False', reason: 'NoPipeline', message: 'no Ready Pipeline lists this source' }],
      columns: { adapter: 'cron', served: 'true', wired: 'false' },
    },
  ],
}

const findings: Finding[] = [
  {
    kind: 'signalsources', name: 'bench-sensors', check: 'unwired-source',
    reason: 'NoPipeline',
    message: 'no Ready Pipeline lists this source, so signals posted to it are dropped',
  },
]

const pipelineDetail: Detail = {
  object: {
    kind: 'Pipeline',
    metadata: {
      name: 'k8s-observe', namespace: 'agent-ops',
      creationTimestamp: ago(864000),
      labels: { 'app.kubernetes.io/managed-by': 'Helm' },
    },
  },
  health: 'ok',
  conditions: [{ type: 'Ready', status: 'True', reason: 'Wired', lastTransitionTime: ago(864000) }],
  yaml: [
    'apiVersion: agentops.dev/v1alpha1',
    'kind: Pipeline',
    'metadata:',
    '  name: k8s-observe',
    '  namespace: agent-ops',
    'spec:',
    '  sources:',
    '    - name: cluster-events',
    '    - name: console',
    '  channels:',
    '    - name: console',
    '    - name: ops-chat',
    '  profileRef:',
    '    name: k8s-engineer',
    '  toolsets:',
    '    - name: agentops-observe',
    '    - name: k8s-observability',
    '  mcpConfigs:',
    '    - name: k8s-api',
    '',
  ].join('\n'),
  usedBy: null,
  findings: [],
  resolved: {
    pipeline: 'k8s-observe',
    profile: 'k8s-engineer',
    runtime: 'default',
    allowedTools: ['Read', 'Grep', 'Glob', 'Bash', 'mcp__kubernetes__resources_list'],
    toolsMode: 'merge',
    toolsets: ['agentops-observe', 'k8s-observability'],
    mcpConfigs: ['k8s-api'],
    mcpServers: ['kubernetes'],
  },
}

// ---- conversations -----------------------------------------------------------

const conversations: ConversationPage = {
  total: 6,
  unreadTotal: 2,
  offset: 0,
  limit: 25,
  facets: {
    phase: ['Working', 'Idle', 'Pending', 'Closed'],
    pipeline: ['k8s-observe', 'alert-triage', 'nightly-report'],
    profile: ['k8s-engineer', 'alert-investigator', 'release-scribe'],
  },
  items: [
    {
      name: 'cluster-events-7c1d4e', title: 'checkout-api is restarting',
      profile: 'k8s-engineer', pipeline: 'k8s-observe', phase: 'Working',
      inflight: { runId: 'run-4', dispatchedAt: ago(42) },
      runCount: 3, runtimePod: 'agentops-conv-cluster-events-7c1d4e',
      lastActivity: ago(42), created: ago(96), queued: 0, joined: true,
      consoleThread: 'console/cluster-events-7c1d4e', errored: false,
      unread: true, ageSeconds: 96, threads: [
        { channel: 'console', threadId: 'console/cluster-events-7c1d4e', readTracked: true },
        { channel: 'ops-chat', threadId: '2481', readTracked: true },
      ],
      closing: false,
    },
    {
      name: 'console-3f9a2b', title: 'Why is the payments deployment not rolling out?',
      profile: 'k8s-engineer', pipeline: 'k8s-observe', phase: 'Working',
      inflight: { runId: 'run-2', dispatchedAt: ago(18) },
      runCount: 1, runtimePod: 'agentops-conv-console-3f9a2b',
      lastActivity: ago(18), created: ago(61), queued: 1, joined: true,
      consoleThread: 'console/console-3f9a2b', errored: false,
      unread: true, ageSeconds: 61, threads: [
        { channel: 'console', threadId: 'console/console-3f9a2b', readTracked: true },
      ],
      closing: false,
    },
    {
      name: 'prometheus-alerts-91b7fd', title: 'HighMemoryPressure on node-3',
      profile: 'alert-investigator', pipeline: 'alert-triage', phase: 'Pending',
      runCount: 0, lastActivity: ago(27), created: ago(27), queued: 1,
      joined: false, errored: false, unread: false, ageSeconds: 27,
      threads: [{ channel: 'ops-chat', threadId: '2483', readTracked: true }],
      closing: false,
    },
    {
      name: 'console-b48e10', title: 'Which namespaces have no resource quota?',
      profile: 'k8s-engineer', pipeline: 'k8s-observe', phase: 'Idle',
      runCount: 2, lastActivity: ago(1860), created: ago(2100), queued: 0,
      joined: true, consoleThread: 'console/console-b48e10', errored: false,
      unread: false, readAt: ago(1800), ageSeconds: 2100,
      threads: [{ channel: 'console', threadId: 'console/console-b48e10', readTracked: true, readAt: ago(1800) }],
      closing: false,
    },
    {
      name: 'nightly-2f60c8', title: 'Nightly capacity report',
      profile: 'release-scribe', pipeline: 'nightly-report', phase: 'Idle',
      runCount: 1, lastActivity: ago(35400), created: ago(35700), queued: 0,
      joined: false, errored: false, unread: false, ageSeconds: 35700,
      threads: [{ channel: 'ops-chat', threadId: '2477', readTracked: true }],
      closing: false,
    },
    {
      name: 'cluster-events-d902a3', title: 'ingress-nginx admission webhook timed out',
      profile: 'k8s-engineer', pipeline: 'k8s-observe', phase: 'Closed',
      runCount: 4, lastActivity: ago(90000), created: ago(93600), queued: 0,
      joined: true, consoleThread: 'console/cluster-events-d902a3', errored: false,
      unread: false, readAt: ago(89000), ageSeconds: 93600,
      threads: [{ channel: 'console', threadId: 'console/cluster-events-d902a3', readTracked: true, readAt: ago(89000) }],
      closing: false,
    },
  ],
}

const conversationDetail: ConversationDetail = {
  conversation: conversations.items[0],
  object: {
    kind: 'Conversation',
    metadata: {
      name: 'cluster-events-7c1d4e', namespace: 'agent-ops',
      creationTimestamp: ago(96),
    },
  },
  yaml: [
    'apiVersion: agentops.dev/v1alpha1',
    'kind: Conversation',
    'metadata:',
    '  name: cluster-events-7c1d4e',
    'spec:',
    '  pipelineRef:',
    '    name: k8s-observe',
    '  profileRef:',
    '    name: k8s-engineer',
    '',
  ].join('\n'),
  archived: false,
  transcript: [
    // The console renders wire text PLAIN, deliberately (src/components/Text.tsx),
    // so the fixture writes prose. Markdown here would publish a screenshot of
    // asterisks and read as a rendering fault.
    {
      id: 'm1', thread: 'console/cluster-events-7c1d4e', kind: 'signal',
      text: 'Signal from cluster-events — checkout-api in namespace storefront has restarted 5 times in 10 minutes (BackOff).',
      at: ago(96),
    },
    {
      id: 'm2', thread: 'console/cluster-events-7c1d4e', kind: 'agent',
      text: [
        'checkout-api is being OOM-killed, not crashing on a bug.',
        '',
        'The last three restarts all ended OOMKilled with exit code 137. The',
        'container requests 256Mi and its working set sits at 244Mi before each',
        'restart, so it is running against the limit rather than leaking.',
        '',
        'Two things changed 20 minutes before the first restart. The deployment',
        'rolled to checkout-api:2.14.0, and CACHE_ENTRIES went from 5000 to',
        '50000 in the storefront-config ConfigMap.',
        '',
        'Raising the memory limit would hide it. The cache size is the change',
        'worth reverting first. I have changed nothing — this route grants',
        'observing tools only.',
      ].join('\n'),
      at: ago(58),
    },
    {
      id: 'm3', thread: 'console/cluster-events-7c1d4e', kind: 'relay',
      sender: 'ops-chat/dana',
      text: 'Does the same setting affect the other two services in that namespace?',
      at: ago(44),
    },
  ],
  events: [
    { cursor: '18240', ts: ago(96), kind: 'signal', from: { kind: 'signal-source', name: 'cluster-events' }, to: { kind: 'pipeline', name: 'k8s-observe' }, status: 'ok', conversation: 'cluster-events-7c1d4e', pipeline: 'k8s-observe' },
    { cursor: '18241', ts: ago(94), kind: 'dispatch', from: { kind: 'pipeline', name: 'k8s-observe' }, to: { kind: 'runtime', name: 'default' }, status: 'ok', conversation: 'cluster-events-7c1d4e', runId: 'run-3', latencyMs: 340 },
    { cursor: '18242', ts: ago(58), kind: 'answer', from: { kind: 'runtime', name: 'default' }, to: { kind: 'channel', name: 'console' }, status: 'ok', conversation: 'cluster-events-7c1d4e', runId: 'run-3', latencyMs: 36120 },
    { cursor: '18243', ts: ago(44), kind: 'inbound', from: { kind: 'channel', name: 'ops-chat' }, to: { kind: 'pipeline', name: 'k8s-observe' }, status: 'ok', conversation: 'cluster-events-7c1d4e' },
  ],
  runtimePodStatus: { phase: 'Running', problem: '', node: 'node-2' },
}

// ---- topology ----------------------------------------------------------------

const eventNodeKinds: Record<string, string> = {
  'signal-adapter': 'signaladapters',
  'signal-source': 'signalsources',
  pipeline: 'pipelines',
  profile: 'agentprofiles',
  runtime: 'agentruntimes',
  channel: 'channels',
  'channel-adapter': 'channeladapters',
  toolset: 'mcptoolsets',
  'mcp-config': 'mcpconfigs',
}

const topology: TopologyResponse = {
  consoleChannel: 'console',
  unjoinedPipelines: null,
  synced: overview.synced,
  stream: overview.stream,
  oldestEvent: ago(900),
  metricsAvailable: false,
  topology: {
    windowSeconds: 900,
    eventNodeKinds,
    nodes: [
      { id: 'signaladapters/k8s-events', kind: 'signaladapters', name: 'k8s-events', health: 'ok', active: 0, recent: 6 },
      { id: 'signaladapters/alertmanager', kind: 'signaladapters', name: 'alertmanager', health: 'ok', active: 0, recent: 2 },
      { id: 'signaladapters/cron', kind: 'signaladapters', name: 'cron', health: 'ok', active: 0, recent: 1 },
      { id: 'signalsources/cluster-events', kind: 'signalsources', name: 'cluster-events', health: 'ok', active: 1, recent: 6 },
      { id: 'signalsources/console', kind: 'signalsources', name: 'console', health: 'ok', active: 1, recent: 3 },
      { id: 'signalsources/prometheus-alerts', kind: 'signalsources', name: 'prometheus-alerts', health: 'ok', active: 0, recent: 2 },
      { id: 'signalsources/nightly', kind: 'signalsources', name: 'nightly', health: 'ok', active: 0, recent: 1 },
      { id: 'signalsources/bench-sensors', kind: 'signalsources', name: 'bench-sensors', health: 'bad', reason: 'NoPipeline', message: 'no Ready Pipeline lists this source', active: 0, recent: 0 },
      { id: 'pipelines/k8s-observe', kind: 'pipelines', name: 'k8s-observe', health: 'ok', active: 2, recent: 9 },
      { id: 'pipelines/alert-triage', kind: 'pipelines', name: 'alert-triage', health: 'ok', active: 1, recent: 2 },
      { id: 'pipelines/nightly-report', kind: 'pipelines', name: 'nightly-report', health: 'ok', active: 0, recent: 1 },
      { id: 'agentprofiles/k8s-engineer', kind: 'agentprofiles', name: 'k8s-engineer', health: 'none', active: 2, recent: 9 },
      { id: 'agentprofiles/alert-investigator', kind: 'agentprofiles', name: 'alert-investigator', health: 'none', active: 1, recent: 2 },
      { id: 'agentprofiles/release-scribe', kind: 'agentprofiles', name: 'release-scribe', health: 'none', active: 0, recent: 1 },
      { id: 'agentruntimes/default', kind: 'agentruntimes', name: 'default', health: 'ok', active: 2, recent: 11 },
      { id: 'agentruntimes/sandbox', kind: 'agentruntimes', name: 'sandbox', health: 'ok', active: 0, recent: 0 },
      { id: 'channels/console', kind: 'channels', name: 'console', health: 'ok', active: 1, recent: 7 },
      { id: 'channels/ops-chat', kind: 'channels', name: 'ops-chat', health: 'ok', active: 0, recent: 5 },
      { id: 'channeladapters/console', kind: 'channeladapters', name: 'console', health: 'ok', active: 1, recent: 7 },
      { id: 'channeladapters/telegram', kind: 'channeladapters', name: 'telegram', health: 'ok', active: 0, recent: 5 },
      { id: 'mcptoolsets/agentops-observe', kind: 'mcptoolsets', name: 'agentops-observe', health: 'none', active: 0, recent: 0 },
      { id: 'mcptoolsets/k8s-observability', kind: 'mcptoolsets', name: 'k8s-observability', health: 'none', active: 0, recent: 0 },
      { id: 'mcpconfigs/k8s-api', kind: 'mcpconfigs', name: 'k8s-api', health: 'none', active: 0, recent: 0 },
    ],
    edges: [
      { from: 'signaladapters/k8s-events', to: 'signalsources/cluster-events', kind: 'served-by' },
      { from: 'signaladapters/alertmanager', to: 'signalsources/prometheus-alerts', kind: 'served-by' },
      { from: 'signaladapters/cron', to: 'signalsources/nightly', kind: 'served-by' },
      { from: 'signaladapters/cron', to: 'signalsources/bench-sensors', kind: 'served-by' },
      { from: 'channeladapters/console', to: 'channels/console', kind: 'served-by' },
      { from: 'channeladapters/telegram', to: 'channels/ops-chat', kind: 'served-by' },
      { from: 'signalsources/cluster-events', to: 'pipelines/k8s-observe', kind: 'feeds', traffic: { events: 6, errors: 0, ratePerMin: 0.4, p50LatencyMs: 210, maxLatencyMs: 480, lastTs: ago(96) } },
      { from: 'signalsources/console', to: 'pipelines/k8s-observe', kind: 'feeds', traffic: { events: 3, errors: 0, ratePerMin: 0.2, p50LatencyMs: 180, maxLatencyMs: 260, lastTs: ago(61) } },
      { from: 'signalsources/prometheus-alerts', to: 'pipelines/alert-triage', kind: 'feeds', traffic: { events: 2, errors: 0, ratePerMin: 0.1, p50LatencyMs: 240, maxLatencyMs: 300, lastTs: ago(27) } },
      { from: 'signalsources/nightly', to: 'pipelines/nightly-report', kind: 'feeds', traffic: { events: 1, errors: 0, ratePerMin: 0.1, lastTs: ago(35700) } },
      { from: 'pipelines/k8s-observe', to: 'agentprofiles/k8s-engineer', kind: 'answers', traffic: { events: 9, errors: 0, ratePerMin: 0.6, p50LatencyMs: 12400, maxLatencyMs: 41000, lastTs: ago(58) } },
      { from: 'pipelines/alert-triage', to: 'agentprofiles/alert-investigator', kind: 'answers', traffic: { events: 2, errors: 0, ratePerMin: 0.1, lastTs: ago(27) } },
      { from: 'pipelines/nightly-report', to: 'agentprofiles/release-scribe', kind: 'answers' },
      { from: 'agentprofiles/k8s-engineer', to: 'agentruntimes/default', kind: 'uses', traffic: { events: 9, errors: 0, ratePerMin: 0.6, p50LatencyMs: 340, maxLatencyMs: 900, lastTs: ago(58) } },
      { from: 'agentprofiles/alert-investigator', to: 'agentruntimes/default', kind: 'uses', traffic: { events: 2, errors: 0, ratePerMin: 0.1, lastTs: ago(27) } },
      { from: 'agentprofiles/release-scribe', to: 'agentruntimes/sandbox', kind: 'uses' },
      { from: 'pipelines/k8s-observe', to: 'channels/console', kind: 'posts', traffic: { events: 7, errors: 0, ratePerMin: 0.5, p50LatencyMs: 90, maxLatencyMs: 210, lastTs: ago(58) } },
      { from: 'pipelines/k8s-observe', to: 'channels/ops-chat', kind: 'posts', traffic: { events: 3, errors: 0, ratePerMin: 0.2, lastTs: ago(58), unconfirmed: true } },
      { from: 'pipelines/alert-triage', to: 'channels/ops-chat', kind: 'posts', traffic: { events: 2, errors: 0, ratePerMin: 0.1, lastTs: ago(27) } },
      { from: 'pipelines/nightly-report', to: 'channels/ops-chat', kind: 'posts' },
      { from: 'pipelines/k8s-observe', to: 'mcptoolsets/agentops-observe', kind: 'uses' },
      { from: 'pipelines/k8s-observe', to: 'mcptoolsets/k8s-observability', kind: 'uses' },
      { from: 'pipelines/k8s-observe', to: 'mcpconfigs/k8s-api', kind: 'uses' },
    ],
  },
}

const conversationGraph: ConversationGraph = {
  ...topology.topology,
  diverged: false,
  pipeline: 'k8s-observe',
  events: conversationDetail.events,
}

// ---- origination -------------------------------------------------------------

const sources: SourcesResponse = {
  canOriginate: true,
  writeEnabled: true,
  sources: [{ name: 'console', wired: true, pipeline: 'k8s-observe', profile: 'k8s-engineer' }],
}

const vocabulary: VocabularyResponse = {
  revision: 'fixture-1',
  entries: [
    { kind: 'builtin', name: 'pipelines', position: 'general',
      description: 'List the pipelines you can address' },
    { kind: 'builtin', name: 'help', position: 'general',
      description: 'Show the pipelines and how to address them' },
    { kind: 'builtin', name: 'exit', position: 'thread',
      description: "Release this conversation's runtime, keep the conversation" },
    { kind: 'builtin', name: 'close', position: 'thread',
      description: 'End this conversation and archive its thread' },
    { kind: 'pipeline', name: 'k8s-observe', position: 'general',
      description: 'k8s-engineer', profile: 'k8s-engineer', icon: 'aops:observe' },
    { kind: 'pipeline', name: 'alert-triage', position: 'general',
      description: 'alert-investigator', profile: 'alert-investigator', icon: 'aops:alert' },
    { kind: 'pipeline', name: 'nightly-report', position: 'general',
      description: 'release-scribe', profile: 'release-scribe', icon: 'aops:workload' },
  ],
}

// ---- the install -------------------------------------------------------------

/**
 * Everything the fixture serves, as ONE value.
 *
 * It is exported so a second producer can LAYER over it rather than invent a
 * second install: `demo/story.ts` clones this and patches it beat by beat, so
 * the recording on the landing page and the screenshots on the console page
 * show the same made-up namespace and cannot drift apart.
 */
export interface Install {
  session: Session
  overview: Overview
  queues: Queues
  kinds: KindInfo[]
  findings: Finding[]
  inventory: Record<string, InventoryRow[]>
  pipelineDetail: Detail
  conversations: ConversationPage
  conversationDetail: ConversationDetail
  conversationGraph: ConversationGraph
  topology: TopologyResponse
  sources: SourcesResponse
  vocabulary: VocabularyResponse
}

export const install: Install = {
  session, overview, queues, kinds, findings, inventory, pipelineDetail,
  conversations, conversationDetail, conversationGraph, topology, sources,
  vocabulary,
}

// ---- routing -----------------------------------------------------------------

/**
 * Answers one `/api/*` path from a given install, or null when it has nothing
 * for it — the server then falls back to `{}`, which is what an unexercised
 * endpoint should return rather than a crash mid-capture.
 *
 * ONE routing table, over whichever install is passed. A producer that walked a
 * story would otherwise write a second one, and the day an endpoint moved only
 * one of them would follow.
 */
export function responder(state: Install) {
  return function answer(path: string, query: URLSearchParams): unknown {
    switch (path) {
      case '/api/session': return state.session
      case '/api/overview': return state.overview
      case '/api/queues': return state.queues
      case '/api/config': return state.kinds
      case '/api/findings': return state.findings
      case '/api/topology': return state.topology
      case '/api/sources': return state.sources
      case '/api/vocabulary': return state.vocabulary
      case '/api/charts': return { available: false, charts: [] }
    }

    if (path === '/api/conversations') {
      // The navigation badge asks for the totals only.
      return query.get('count') === '1'
        ? { ...state.conversations, items: [] }
        : state.conversations
    }

    const detail = /^\/api\/conversations\/([^/]+)(\/graph)?$/.exec(path)
    if (detail && detail[1] === state.conversationDetail.conversation.name) {
      return detail[2] ? state.conversationGraph : state.conversationDetail
    }

    const inv = /^\/api\/config\/([a-z]+)$/.exec(path)
    if (inv) return state.inventory[inv[1]] ?? []
    if (path === '/api/config/pipelines/k8s-observe') return state.pipelineDetail

    return null
  }
}

/** The frozen install the console's screenshots are taken of. */
export const answer = responder(install)
