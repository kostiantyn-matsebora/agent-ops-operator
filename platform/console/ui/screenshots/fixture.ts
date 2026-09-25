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
  ActivityEvent, ActivityResponse, VocabularyResponse, ConversationDetail, ConversationGraph,
  ConversationPage, Detail, Finding, InventoryRow, KindInfo, Overview, Queues, Session,
  SourcesResponse, TopologyResponse,
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
    '  signalSourceRefs:',
    '    - name: cluster-events',
    '    - name: console',
    '  channelRefs:',
    '    - name: console',
    '    - name: ops-chat',
    '  profileRef:',
    '    name: k8s-engineer',
    '  toolsets:',
    '    mode: merge',
    '    refs:',
    '      - name: agentops-observe',
    '      - name: k8s-observability',
    '  mcpConfigs:',
    '    refs:',
    '      - name: k8s-api',
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
      // A published frame shows this card. Empty rows on it read as a broken
      // view rather than as an object that happens to carry no labels.
      uid: '7c1d4e28-5b90-4a11-9f3c-6d2e0b8ac114',
      resourceVersion: '184402',
    },
  },
  yaml: [
    'apiVersion: agentops.dev/v1alpha1',
    'kind: Conversation',
    'metadata:',
    '  name: cluster-events-7c1d4e',
    '  namespace: agent-ops',
    'spec:',
    '  pipelineRef:',
    '    name: k8s-observe',
    '  profileRef:',
    '    name: k8s-engineer',
    '  channelRefs:',
    '    - name: console',
    '    - name: ops-chat',
    '  toolsets:',
    '    mode: merge',
    '    refs:',
    '      - name: agentops-observe',
    '      - name: k8s-observability',
    '  mcpConfigs:',
    '    refs:',
    '      - name: k8s-api',
    'status:',
    '  phase: Working',
    '  runtimeContextId: ctx-9f31c0a4',
    '  threads:',
    '    - channel: console',
    '      threadId: console/cluster-events-7c1d4e',
    '      readTracked: true',
    '    - channel: ops-chat',
    '      threadId: "2481"',
    '      readTracked: true',
    '  runs:',
    '    - runId: run-1',
    '      status: Succeeded',
    '      result: diagnosed',
    '      deliveryTracked: true',
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
      // THE FORMAT THE PRODUCT MANDATES, not prose.
      //
      // `internal/dispatch/templates/format.md` is what an agent is told to
      // write when its profile declares `outputFormat: blocks`: a title, sections it
      // names for its own job, and `<details>` for the fold. A published
      // transcript showing a wall of text shows something the product asks its
      // agents not to produce.
      //
      // THE TAGS ARE HERE ON PURPOSE. The console parses them — the manager
      // passes an agent's text through untouched — so a fixture carrying the
      // raw output is exactly what a real conversation carries, and the
      // screenshot shows the fold a reader actually gets.
      text: [
        '<title>',
        '🔍 checkout-api is restarting — storefront/checkout-api · diagnosed',
        '</title>',
        '',
        '<root-cause>',
        'OOM-killed, not a bug. Requests 256Mi, working set reaches 244Mi',
        'before each restart — running against the limit, not leaking.',
        '</root-cause>',
        '',
        '<evidence>',
        '- Last 3 restarts ended `OOMKilled`, exit code 137',
        '- Rolled to `checkout-api:2.14.0` 20m before the first restart',
        '- `CACHE_ENTRIES` went 5000 → 50000 in `storefront-config`',
        '</evidence>',
        '',
        '<fix>',
        'Revert the cache size. Raising the memory limit would hide it.',
        '`kubectl -n storefront rollout undo deploy/checkout-api`',
        '</fix>',
        '',
        '<details>',
        'This route grants observing tools only, so nothing was changed.',
        '',
        'Ruled out: a node-pressure eviction (no pressure taints in the window),',
        'a bad image (the digest is unchanged since 2.14.0 rolled), and a',
        'liveness misconfiguration (the probe never fired).',
        '',
        '```json',
        '{"lastState":{"terminated":{"reason":"OOMKilled","exitCode":137}}}',
        '```',
        '</details>',
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
    { cursor: '18240', ts: ago(96), kind: 'signal.claimed', from: { kind: 'signal-source', name: 'cluster-events' }, to: { kind: 'pipeline', name: 'k8s-observe' }, status: 'ok', conversation: 'cluster-events-7c1d4e', pipeline: 'k8s-observe' },
    { cursor: '18241', ts: ago(94), kind: 'run.dispatched', from: { kind: 'pipeline', name: 'k8s-observe' }, to: { kind: 'runtime', name: 'default' }, status: 'ok', conversation: 'cluster-events-7c1d4e', pipeline: 'k8s-observe', runId: 'run-3', latencyMs: 340 },
    { cursor: '18242', ts: ago(58), kind: 'run.completed', from: { kind: 'runtime', name: 'default' }, to: { kind: 'pipeline', name: 'k8s-observe' }, status: 'ok', conversation: 'cluster-events-7c1d4e', pipeline: 'k8s-observe', runId: 'run-3', latencyMs: 36120 },
    { cursor: '18243', ts: ago(44), kind: 'channel.inbound', from: { kind: 'channel-adapter', name: 'telegram' }, to: { kind: 'conversation', name: 'cluster-events-7c1d4e' }, status: 'ok', conversation: 'cluster-events-7c1d4e', pipeline: 'k8s-observe' },
  ],
  runtimePodStatus: { phase: 'Running', problem: '', node: 'node-2' },
}

// ---- topology ----------------------------------------------------------------
//
// The BFF's real shape: the Model's objects and references, the COMPONENTS and
// PODS the other two views are drawn from, and the windowed hops. Every name is
// invented, and every image is the short form the overview above already uses.

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
  conversation: 'conversations',
}

const CLAUDE_IMAGE = 'agentops-runtime-claude:0.5.1'
const SANDBOX_IMAGE = 'agentops-runtime-claude:0.5.1-sandbox'
const MODEL = 'claude-sonnet-5'
const REPO = 'https://git.example.com/ops/agents.git'

type NodeExtra = Partial<TopologyResponse['topology']['nodes'][number]>
const node = (kind: string, name: string, extra: NodeExtra = {}) => ({
  id: `${kind}/${name}`, kind, name, health: 'ok' as const, active: 0, recent: 0, ...extra,
})
type Comp = NonNullable<TopologyResponse['topology']['components']>[number]
const comp = (role: string, name: string, extra: Partial<Comp> = {}): Comp => ({
  id: `${role}/${name}`, role, name, count: 0, health: 'none', ...extra,
})
type PodT = NonNullable<TopologyResponse['topology']['pods']>[number]
const pod = (name: string, clusterNode: string, component: string, extra: Partial<PodT> = {}): PodT => ({
  id: `pods/${name}`, name, clusterNode, component, phase: 'Running', health: 'ok',
  containers: [{ name: 'main', ready: true, restarts: 0 }], ...extra,
})
const convPod = (conversation: string, clusterNode: string, pipeline: string, image: string): PodT =>
  pod(`agentops-conv-${conversation}`, clusterNode, `runtime-image/${image}`, {
    conversation, pipeline,
    containers: [
      { name: 'agent', image, role: 'runtime-image', ready: true, restarts: 0 },
      { name: 'context-sync', image: 'agentops-context-sync:0.3.0', role: 'context-sync', ready: true, restarts: 0 },
      { name: 'egress-proxy', image: 'agentops-egress-proxy:0.2.1', role: 'egress-proxy', ready: true, restarts: 0 },
    ],
  })
const stat = (from: [string, string], to: [string, string], events: number, ratePerMin: number, p50LatencyMs?: number) => ({
  from: { kind: from[0], name: from[1] }, to: { kind: to[0], name: to[1] },
  events, errors: 0, ratePerMin, p50LatencyMs, unconfirmed: false,
})

// The activity buffer: one conversation's whole run in the current vocabulary,
// a second one mid-run, an alert claimed, and one drop. Each view maps these
// same hops onto its own nodes.
let seq = 18180
type Hop = ActivityEvent
const hop = (s: number, kind: string, from: [string, string] | null, to: [string, string] | null, extra: Partial<Hop> = {}): Hop => ({
  cursor: String(++seq), ts: ago(s), kind, status: 'ok',
  from: from ? { kind: from[0], name: from[1] } : undefined,
  to: to ? { kind: to[0], name: to[1] } : undefined,
  ...extra,
})
const C1 = { conversation: 'cluster-events-7c1d4e', pipeline: 'k8s-observe' }
const C2 = { conversation: 'console-3f9a2b', pipeline: 'k8s-observe' }
const A1 = { conversation: 'prometheus-alerts-91b7fd', pipeline: 'alert-triage' }
const activityEvents: Hop[] = [
  hop(100, 'signal.received', ['signal-adapter', 'k8s-events'], ['signal-source', 'cluster-events'], { conversation: C1.conversation }),
  hop(99, 'signal.claimed', ['signal-source', 'cluster-events'], ['pipeline', 'k8s-observe'], C1),
  hop(99, 'conversation.created', ['pipeline', 'k8s-observe'], ['conversation', C1.conversation], C1),
  hop(98, 'runtime.starting', ['conversation', C1.conversation], ['runtime', 'default'], { ...C1, latencyMs: 3400 }),
  hop(95, 'context.restored', ['runtime', 'default'], ['conversation', C1.conversation], { ...C1, code: 'start' }),
  hop(94, 'run.dispatched', ['pipeline', 'k8s-observe'], ['runtime', 'default'], { ...C1, runId: 'run-3', latencyMs: 340 }),
  hop(90, 'model.call', ['runtime-image', CLAUDE_IMAGE], ['model', MODEL], { ...C1, runId: 'run-3', detail: MODEL, data: { model: MODEL, tokensIn: '18422', tokensOut: '412', stopReason: 'tool_use' } }),
  hop(86, 'tool.call', ['runtime-image', CLAUDE_IMAGE], ['mcp-server', 'kubernetes'], { ...C1, runId: 'run-3', latencyMs: 820, detail: 'mcp__kubernetes__pods_log', data: { tool: 'mcp__kubernetes__pods_log', server: 'kubernetes', resultBytes: '6120' } }),
  hop(80, 'model.call', ['runtime-image', CLAUDE_IMAGE], ['model', MODEL], { ...C1, runId: 'run-3', detail: MODEL, data: { model: MODEL, tokensIn: '24906', tokensOut: '380', cacheReadTokens: '18000', stopReason: 'tool_use' } }),
  hop(76, 'tool.call', ['runtime-image', CLAUDE_IMAGE], ['mcp-server', 'kubernetes'], { ...C1, runId: 'run-3', latencyMs: 540, detail: 'mcp__kubernetes__resources_get', data: { tool: 'mcp__kubernetes__resources_get', server: 'kubernetes', resultBytes: '2210' } }),
  hop(62, 'model.call', ['runtime-image', CLAUDE_IMAGE], ['model', MODEL], { ...C1, runId: 'run-3', detail: MODEL, data: { model: MODEL, tokensIn: '27730', tokensOut: '1104', cacheReadTokens: '24500', stopReason: 'end_turn' } }),
  hop(58, 'run.completed', ['runtime', 'default'], ['pipeline', 'k8s-observe'], { ...C1, runId: 'run-3', latencyMs: 36120, code: 'succeeded', detail: 'succeeded (exit 0)' }),
  hop(58, 'context.checkpoint', ['conversation', C1.conversation], ['runtime', 'default'], { ...C1, code: 'work-done', data: { bytes: '48213', files: '3', quiesced: 'true' } }),
  hop(57, 'channel.op.enqueued', ['conversation', C1.conversation], ['channel', 'console'], { ...C1, opId: 'send:cluster-events-7c1d4e:console:run-3', code: 'send' }),
  hop(57, 'channel.op.enqueued', ['conversation', C1.conversation], ['channel', 'ops-chat'], { ...C1, opId: 'send:cluster-events-7c1d4e:ops-chat:run-3', code: 'send' }),
  hop(56, 'channel.op.completed', ['channel-adapter', 'console'], ['manager', 'manager'], { ...C1, opId: 'send:cluster-events-7c1d4e:console:run-3', latencyMs: 90, adapter: 'console' }),
  hop(61, 'signal.received', ['signal-adapter', 'console'], ['signal-source', 'console'], { conversation: C2.conversation }),
  hop(61, 'signal.claimed', ['signal-source', 'console'], ['pipeline', 'k8s-observe'], C2),
  hop(44, 'channel.inbound', ['channel-adapter', 'telegram'], ['conversation', C1.conversation], C1),
  hop(35, 'signal.dropped', ['signal-source', 'bench-sensors'], null, { status: 'error', code: 'unclaimed', detail: 'no Ready Pipeline lists this source' }),
  hop(27, 'signal.received', ['signal-adapter', 'alertmanager'], ['signal-source', 'prometheus-alerts'], { conversation: A1.conversation }),
  hop(27, 'signal.claimed', ['signal-source', 'prometheus-alerts'], ['pipeline', 'alert-triage'], A1),
  hop(18, 'run.dispatched', ['pipeline', 'k8s-observe'], ['runtime', 'default'], { ...C2, runId: 'run-2', latencyMs: 210 }),
  hop(12, 'model.call', ['runtime-image', CLAUDE_IMAGE], ['model', MODEL], { ...C2, runId: 'run-2', detail: MODEL, data: { model: MODEL, tokensIn: '15840', tokensOut: '296', stopReason: 'tool_use' } }),
  hop(8, 'tool.call', ['runtime-image', CLAUDE_IMAGE], ['mcp-server', 'kubernetes'], { ...C2, runId: 'run-2', latencyMs: 610, detail: 'mcp__kubernetes__resources_list', data: { tool: 'mcp__kubernetes__resources_list', server: 'kubernetes', resultBytes: '9034' } }),
  hop(4, 'tool.call', ['runtime-image', CLAUDE_IMAGE], null, { ...C2, runId: 'run-2', latencyMs: 40, detail: 'Read', data: { tool: 'Read', resultBytes: '1880' } }),
].sort((a, b) => a.ts.localeCompare(b.ts) || a.cursor.localeCompare(b.cursor))

const activity: ActivityResponse = {
  events: activityEvents,
  cursor: activityEvents[activityEvents.length - 1].cursor,
  stream: overview.stream,
}

const topology: TopologyResponse = {
  consoleChannel: 'console',
  unjoinedPipelines: null,
  synced: overview.synced,
  stream: overview.stream,
  oldestEvent: ago(900),
  metricsAvailable: false,
  topology: {
    windowSeconds: 300,
    eventNodeKinds,
    nodes: [
      node('signaladapters', 'k8s-events', { bundle: 'kubernetes', recent: 6, externals: [{ name: 'Kubernetes API', kind: 'kubernetes' }] }),
      node('signaladapters', 'alertmanager', { bundle: 'prometheus', recent: 2, externals: [{ name: 'Alertmanager', kind: 'sender' }] }),
      node('signaladapters', 'cron', { recent: 1 }),
      node('signaladapters', 'console', { servedBy: 'channeladapters/console', recent: 3 }),
      node('signalsources', 'cluster-events', { bundle: 'kubernetes', active: 1, recent: 6 }),
      node('signalsources', 'console', { active: 1, recent: 3 }),
      node('signalsources', 'prometheus-alerts', { bundle: 'prometheus', recent: 2 }),
      node('signalsources', 'nightly', { recent: 1 }),
      node('signalsources', 'bench-sensors', { health: 'bad', reason: 'NoPipeline', message: 'no Ready Pipeline lists this source', detached: true }),
      node('pipelines', 'k8s-observe', { bundle: 'kubernetes', active: 2, recent: 9, icon: 'aops:observe' }),
      node('pipelines', 'alert-triage', { bundle: 'prometheus', active: 1, recent: 2, icon: 'aops:alert' }),
      node('pipelines', 'nightly-report', { recent: 1, icon: 'aops:workload' }),
      node('agentprofiles', 'k8s-engineer', { health: 'none', bundle: 'kubernetes' }),
      node('agentprofiles', 'alert-investigator', { health: 'none', bundle: 'prometheus' }),
      node('agentprofiles', 'release-scribe', { health: 'none' }),
      node('agentruntimes', 'default', { bundle: 'claude', image: CLAUDE_IMAGE, harness: 'Claude Code', vendor: 'Anthropic' }),
      node('agentruntimes', 'sandbox', { image: SANDBOX_IMAGE, harness: 'Claude Code', vendor: 'Anthropic' }),
      node('mcptoolsets', 'agentops-observe', { health: 'none' }),
      node('mcptoolsets', 'k8s-observability', { health: 'none', bundle: 'kubernetes' }),
      node('mcpconfigs', 'k8s-api', { health: 'none', bundle: 'kubernetes' }),
      node('channels', 'console', { active: 1, recent: 7 }),
      node('channels', 'ops-chat', { bundle: 'telegram', recent: 5 }),
      node('channeladapters', 'console', { recent: 7, externals: [{ name: 'Browser', kind: 'sender' }, { name: 'Kubernetes API', kind: 'kubernetes' }] }),
      node('channeladapters', 'telegram', { bundle: 'telegram', recent: 5, externals: [{ name: 'Telegram Bot API', kind: 'api' }] }),
      node('conversations', 'cluster-events-7c1d4e', { health: 'none', phase: 'Working', runtimePod: 'pods/agentops-conv-cluster-events-7c1d4e' }),
      node('conversations', 'console-3f9a2b', { health: 'none', phase: 'Working', runtimePod: 'pods/agentops-conv-console-3f9a2b' }),
      node('conversations', 'prometheus-alerts-91b7fd', { health: 'none', phase: 'Pending' }),
    ],
    edges: [
      { from: 'signalsources/cluster-events', to: 'signaladapters/k8s-events', kind: 'served-by' },
      { from: 'signalsources/prometheus-alerts', to: 'signaladapters/alertmanager', kind: 'served-by' },
      { from: 'signalsources/nightly', to: 'signaladapters/cron', kind: 'served-by' },
      { from: 'signalsources/bench-sensors', to: 'signaladapters/cron', kind: 'served-by' },
      { from: 'signalsources/console', to: 'signaladapters/console', kind: 'served-by' },
      { from: 'channels/console', to: 'channeladapters/console', kind: 'served-by' },
      { from: 'channels/ops-chat', to: 'channeladapters/telegram', kind: 'served-by' },
      { from: 'signalsources/cluster-events', to: 'pipelines/k8s-observe', kind: 'feeds', traffic: { events: 6, errors: 0, ratePerMin: 1.2, p50LatencyMs: 210, maxLatencyMs: 480, lastTs: ago(99) } },
      { from: 'signalsources/console', to: 'pipelines/k8s-observe', kind: 'feeds', traffic: { events: 3, errors: 0, ratePerMin: 0.6, p50LatencyMs: 180, maxLatencyMs: 260, lastTs: ago(61) } },
      { from: 'signalsources/prometheus-alerts', to: 'pipelines/alert-triage', kind: 'feeds', traffic: { events: 2, errors: 0, ratePerMin: 0.4, p50LatencyMs: 240, maxLatencyMs: 300, lastTs: ago(27) } },
      { from: 'signalsources/nightly', to: 'pipelines/nightly-report', kind: 'feeds' },
      { from: 'pipelines/k8s-observe', to: 'agentprofiles/k8s-engineer', kind: 'answers' },
      { from: 'pipelines/alert-triage', to: 'agentprofiles/alert-investigator', kind: 'answers' },
      { from: 'pipelines/nightly-report', to: 'agentprofiles/release-scribe', kind: 'answers' },
      { from: 'pipelines/k8s-observe', to: 'agentruntimes/default', kind: 'runs-on', traffic: { events: 4, errors: 0, ratePerMin: 0.8, p50LatencyMs: 340, maxLatencyMs: 36120, lastTs: ago(18) } },
      { from: 'pipelines/alert-triage', to: 'agentruntimes/default', kind: 'runs-on' },
      { from: 'pipelines/nightly-report', to: 'agentruntimes/sandbox', kind: 'runs-on' },
      { from: 'pipelines/k8s-observe', to: 'channels/console', kind: 'posts', traffic: { events: 2, errors: 0, ratePerMin: 0.4, p50LatencyMs: 90, maxLatencyMs: 210, lastTs: ago(56) } },
      { from: 'pipelines/k8s-observe', to: 'channels/ops-chat', kind: 'posts', traffic: { events: 2, errors: 0, ratePerMin: 0.4, lastTs: ago(44), unconfirmed: true } },
      { from: 'pipelines/alert-triage', to: 'channels/ops-chat', kind: 'posts' },
      { from: 'pipelines/nightly-report', to: 'channels/ops-chat', kind: 'posts' },
      { from: 'pipelines/k8s-observe', to: 'mcptoolsets/agentops-observe', kind: 'uses' },
      { from: 'pipelines/k8s-observe', to: 'mcptoolsets/k8s-observability', kind: 'uses' },
      { from: 'pipelines/alert-triage', to: 'mcptoolsets/agentops-observe', kind: 'uses' },
      { from: 'pipelines/k8s-observe', to: 'mcpconfigs/k8s-api', kind: 'uses' },
      { from: 'pipelines/k8s-observe', to: 'conversations/cluster-events-7c1d4e', kind: 'opened' },
      { from: 'pipelines/k8s-observe', to: 'conversations/console-3f9a2b', kind: 'opened' },
      { from: 'pipelines/alert-triage', to: 'conversations/prometheus-alerts-91b7fd', kind: 'opened' },
    ],
    components: [
      comp('manager', 'manager', { count: 1, health: 'ok', image: 'agentops-manager:0.14.0', workload: 'deployments/agentops-manager' }),
      comp('signal-adapter', 'k8s-events', { count: 1, health: 'ok', bundle: 'kubernetes', image: 'agentops-signal-k8s-events:0.4.2', implements: ['signaladapters/k8s-events'] }),
      comp('signal-adapter', 'alertmanager', { count: 1, health: 'ok', bundle: 'prometheus', image: 'agentops-signal-alertmanager:0.5.0', implements: ['signaladapters/alertmanager'] }),
      comp('signal-adapter', 'cron', { count: 1, health: 'ok', image: 'agentops-signal-cron:0.4.0', implements: ['signaladapters/cron'] }),
      comp('signal-adapter', 'console', { health: 'ok', implements: ['signaladapters/console'], servedBy: 'channel-adapter/console' }),
      comp('channel-adapter', 'console', { count: 1, health: 'ok', image: 'agentops-console:0.14.0', implements: ['channeladapters/console'] }),
      comp('channel-adapter', 'telegram', { count: 1, health: 'ok', bundle: 'telegram', image: 'agentops-channel-telegram:0.6.1', implements: ['channeladapters/telegram'] }),
      comp('gateway', 'telegram', { count: 1, health: 'ok', bundle: 'telegram', image: 'agentops-gateway-telegram:0.6.0', workload: 'deployments/agentops-gateway-telegram' }),
      comp('runtime-image', CLAUDE_IMAGE, { count: 2, image: CLAUDE_IMAGE, implements: ['agentruntimes/default'], harness: 'Claude Code', vendor: 'Anthropic' }),
      comp('runtime-image', SANDBOX_IMAGE, { image: SANDBOX_IMAGE, implements: ['agentruntimes/sandbox'], harness: 'Claude Code', vendor: 'Anthropic' }),
      comp('context-sync', 'context-sync', { count: 2, image: 'agentops-context-sync:0.3.0' }),
      comp('egress-proxy', 'egress-proxy', { count: 2, image: 'agentops-egress-proxy:0.2.1' }),
      comp('housekeeping', 'housekeeping', { image: 'agentops-housekeeping:0.2.0', workload: 'cronjobs/agentops-housekeeping' }),
      comp('model', MODEL),
      comp('mcp-server', 'kubernetes', { count: 1, health: 'ok', bundle: 'kubernetes', implements: ['mcpconfigs/k8s-api'], workload: 'deployments/kubernetes-mcp' }),
      comp('repository', REPO, { url: REPO, implements: ['agentprofiles/alert-investigator', 'agentprofiles/k8s-engineer', 'agentprofiles/release-scribe'] }),
      comp('external', 'Alertmanager', { externalKind: 'sender' }),
      comp('external', 'Browser', { externalKind: 'sender' }),
      comp('external', 'Kubernetes API', { externalKind: 'kubernetes' }),
      comp('external', 'Telegram Bot API', { externalKind: 'api' }),
    ],
    componentEdges: [
      { from: 'external/Alertmanager', to: 'signal-adapter/alertmanager', kind: 'sends' },
      { from: 'external/Browser', to: 'channel-adapter/console', kind: 'sends' },
      { from: 'signal-adapter/k8s-events', to: 'external/Kubernetes API', kind: 'calls' },
      { from: 'channel-adapter/console', to: 'external/Kubernetes API', kind: 'calls' },
      { from: 'channel-adapter/telegram', to: 'external/Telegram Bot API', kind: 'calls' },
    ],
    pods: [
      pod('agentops-manager-6d4b8c9f7-w2xkq', 'node-1', 'manager/manager'),
      pod('agentops-adapter-console-5c8d7b6f9-h4mzt', 'node-1', 'channel-adapter/console'),
      pod('agentops-adapter-telegram-7f9c6d5b8-q2wxr', 'node-3', 'channel-adapter/telegram'),
      pod('agentops-gateway-telegram-6b5d4c8f7-k9plm', 'node-3', 'gateway/telegram'),
      pod('agentops-signal-k8s-events-8d7c6b5f4-t3vbn', 'node-1', 'signal-adapter/k8s-events'),
      pod('agentops-signal-alertmanager-5f4d3c2b1-r8xzq', 'node-3', 'signal-adapter/alertmanager'),
      pod('agentops-signal-cron-4c3b2a1f9-m7nkw', 'node-1', 'signal-adapter/cron'),
      pod('kubernetes-mcp-9a8b7c6d5-p5jhs', 'node-3', 'mcp-server/kubernetes'),
      convPod('cluster-events-7c1d4e', 'node-2', 'k8s-observe', CLAUDE_IMAGE),
      convPod('console-3f9a2b', 'node-2', 'k8s-observe', CLAUDE_IMAGE),
    ],
    hops: [
      stat(['runtime-image', CLAUDE_IMAGE], ['model', MODEL], 4, 0.8),
      stat(['runtime-image', CLAUDE_IMAGE], ['mcp-server', 'kubernetes'], 3, 0.6, 610),
      stat(['signal-adapter', 'k8s-events'], ['signal-source', 'cluster-events'], 1, 0.2),
      stat(['pipeline', 'k8s-observe'], ['runtime', 'default'], 2, 0.4, 340),
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
  activity: ActivityResponse
  sources: SourcesResponse
  vocabulary: VocabularyResponse
}

export const install: Install = {
  session, overview, queues, kinds, findings, inventory, pipelineDetail,
  conversations, conversationDetail, conversationGraph, topology, activity, sources,
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
      case '/api/activity': return state.activity
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
