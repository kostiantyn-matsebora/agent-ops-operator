// Wire types, mirroring the Go BFF exactly.
//
// Hand-written rather than generated: the BFF and the SPA ship in one image
// from one repo, so a mismatch is caught by the build that produces both — and
// a generator would be a second toolchain for a surface one person maintains.

export type Health = 'ok' | 'bad' | 'unknown' | 'none'

export interface Condition {
  type: string
  status: string
  reason?: string
  message?: string
  lastTransitionTime?: string
}

export interface ObjectMeta {
  name: string
  namespace?: string
  uid?: string
  resourceVersion?: string
  creationTimestamp?: string
  labels?: Record<string, string>
  annotations?: Record<string, string>
}

export interface K8sObject {
  kind: string
  metadata: ObjectMeta
  spec?: unknown
  status?: unknown
}

// ---- topology ----------------------------------------------------------------

export interface EdgeTraffic {
  events: number
  errors: number
  ratePerMin: number
  p50LatencyMs?: number
  maxLatencyMs?: number
  lastTs?: string
  /** Manager enqueued, no adapter confirmed — rendered distinctly from success. */
  unconfirmed?: boolean
}

export interface GraphNode {
  id: string
  kind: string
  name: string
  health: Health
  reason?: string
  message?: string
  detached?: boolean
  active: number
  recent: number
}

export type EdgeKind = 'feeds' | 'answers' | 'posts' | 'served-by' | 'uses'

export interface GraphEdge {
  from: string
  to: string
  kind: EdgeKind
  dangling?: boolean
  traffic?: EdgeTraffic
}

export interface Topology {
  nodes: GraphNode[]
  edges: GraphEdge[]
  windowSeconds?: number
  eventNodeKinds: Record<string, string>
}

export interface TopologyResponse {
  topology: Topology
  consoleChannel: string
  unjoinedPipelines: string[] | null
  synced: Record<string, boolean>
  stream: StreamHealth
  oldestEvent: string
  metricsAvailable: boolean
}

// ---- activity ----------------------------------------------------------------

export interface NodeRef {
  kind: string
  name: string
}

export interface ActivityEvent {
  cursor: string
  ts: string
  kind: string
  from?: NodeRef
  to?: NodeRef
  status: string
  conversation?: string
  pipeline?: string
  runId?: string
  opId?: string
  inputId?: string
  latencyMs?: number
  code?: string
  detail?: string
  adapter?: string
}

export interface StreamHealth {
  connected: boolean
  cursor?: string
  events: number
  resyncs: number
  error?: string
}

// ---- overview ----------------------------------------------------------------

export interface WorkloadInfo {
  name: string
  image?: string
  digest?: string
  desired: number
  ready: number
  restarts: number
  problem?: string
}

export type ProblemSource = 'reported' | 'derived' | 'pod'

export interface Problem {
  kind: string
  name: string
  type: string
  reason?: string
  message?: string
  since?: string
  source: ProblemSource
}

export interface QueueStat {
  adapter: string
  queued: number
  claimed: number
  oldestQueuedOpId?: string
  oldestQueuedAgeSeconds?: number
  oldestClaimedOpId?: string
  oldestClaimedAgeSeconds?: number
}

export interface CooldownStat {
  source: string
  suppressed: number
  windowSeconds: number
}

export interface ManagerStatus {
  version?: string
  leader?: string
  now?: string
  runtimeSlots: { inUse: number; max: number; waiting: number }
  queues: QueueStat[] | null
  cooldowns: CooldownStat[] | null
}

export interface AdapterInfo {
  kind: string
  name: string
  image?: string
  servedBy?: string
  health: Health
  reason?: string
  serves: number
}

export interface Overview {
  namespace: string
  manager?: ManagerStatus
  managerError?: string
  stream: StreamHealth
  workloads: WorkloadInfo[]
  runtimes: { name: string; image?: string }[]
  adapters: AdapterInfo[]
  counts: Record<string, number>
  synced: Record<string, boolean>
  problems: Problem[] | null
}

// ---- queues ------------------------------------------------------------------

export type StuckReason =
  | 'nothing-claiming'
  | 'adapter-wedged'
  | 'at-runtime-ceiling'
  | 'runtime-hung'

export interface WorkRow {
  conversation: string
  title?: string
  pipeline?: string
  phase?: string
  queued: number
  ageSeconds: number
  inflight?: string
  inflightAgeSeconds?: number
  stuck?: StuckReason
}

export interface DeliveryRow {
  adapter: string
  queued: number
  claimed: number
  oldestQueuedOpId?: string
  oldestQueuedAgeSeconds?: number
  oldestQueuedConversation?: string
  oldestClaimedOpId?: string
  oldestClaimedAgeSeconds?: number
  oldestClaimedConversation?: string
  stuck?: StuckReason
  adapterHealth?: Health
}

export interface Queues {
  capacity: { inUse: number; max: number; waiting: number }
  work: WorkRow[]
  delivery: DeliveryRow[]
  cooldowns: CooldownStat[]
  error?: string
}

// ---- configuration -----------------------------------------------------------

export interface KindInfo {
  kind: string
  title: string
  count: number
  synced: boolean
}

export interface InventoryRow {
  name: string
  uid?: string
  created?: string
  labels?: Record<string, string>
  annotations?: Record<string, string>
  health: Health
  conditions?: Condition[]
  summary?: string
  columns?: Record<string, string>
  findings: number
}

export interface Finding {
  kind: string
  name: string
  check: string
  reason: string
  message: string
  ref?: string
}

export interface InboundRef {
  kind: string
  name: string
  field: string
}

export interface ResolvedCapabilities {
  pipeline: string
  profile: string
  runtime?: string
  allowedTools: string[]
  toolsMode: string
  toolsets: string[]
  mcpConfigs: string[]
  mcpServers: string[]
  unresolved?: string[]
}

export interface Detail {
  object: K8sObject
  health: Health
  conditions?: Condition[]
  yaml: string
  usedBy: InboundRef[] | null
  findings: Finding[]
  resolved?: ResolvedCapabilities
  resolvedError?: string
}

// ---- conversations -----------------------------------------------------------

export interface ThreadBinding {
  channel: string
  threadId: string
}

export interface Run {
  runId: string
  jobKind?: string
  status: string
  exitCode?: number
  result?: string
  startedAt?: string
  finishedAt?: string
}

export interface ConversationSummary {
  name: string
  uid?: string
  title?: string
  profile?: string
  pipeline?: string
  phase?: string
  inflight?: { runId: string; dispatchedAt?: string }
  runs?: Run[]
  runCount: number
  threads?: ThreadBinding[]
  runtimePod?: string
  lastActivity?: string
  created?: string
  queued: number
  joined: boolean
  consoleThread?: string
  errored: boolean
  ageSeconds: number
  toolsets?: string[]
  mcpConfigs?: string[]
}

export interface ConversationPage {
  items: ConversationSummary[]
  total: number
  offset: number
  limit: number
  facets: Record<string, string[]>
}

export interface Message {
  id: string
  thread: string
  /** "agent" | "user" | "notice" — who the bubble belongs to. */
  kind: string
  /** Set only for relayed sibling-channel messages ("<channel>/<user>"). */
  sender?: string
  text: string
  at: string
  /** A locally-typed message the manager has not confirmed yet. */
  pending?: boolean
}

export interface ConversationDetail {
  conversation: ConversationSummary
  object: K8sObject
  yaml: string
  transcript: Message[] | null
  archived: boolean
  events: ActivityEvent[] | null
  runtimePodStatus?: { phase: string; problem: string; node: string }
  joinHint?: { reason: string; fix: string; note?: string }
}

export interface ConversationGraph extends Topology {
  events: ActivityEvent[] | null
  diverged: boolean
  pipeline?: string
  drift?: string[]
}

// ---- origination + session ---------------------------------------------------

export interface OriginationSource {
  name: string
  wired: boolean
  reason?: string
  message?: string
  pipeline?: string
  profile?: string
  patch?: string
}

export interface SourcesResponse {
  sources: OriginationSource[]
  canOriginate: boolean
  writeEnabled: boolean
}

export interface Session {
  authenticated: boolean
  configured: boolean
  identity: string
  writeEnabled: boolean
  canOriginate: boolean
  metrics: boolean
}

// ---- charts ------------------------------------------------------------------

export interface ChartSeries {
  labels: Record<string, string>
  points: { ts: number; value: number }[]
}

export interface ChartResponse {
  chart: string
  windowSeconds: number
  series: ChartSeries[]
  aggregate: boolean
  available: boolean
  error?: string
  fix?: string
}
