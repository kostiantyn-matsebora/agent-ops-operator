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
  /**
   * When history was last lost, and why. Present means the activity window is
   * NOT a continuous record.
   *
   * It arrives on the health rather than on the stream deliberately: a gap
   * outlives the connection that reported it, and the browser that matters is
   * usually the one opened AFTER something went wrong.
   */
  lastGap?: { ts: string; detail?: string }
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
  /** Present while context storage is treated as unavailable install-wide.
   * Work is HELD, not failed, and no runtime pods are provisioned. */
  storageOutage?: { since?: string; forSeconds: number }
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
  /** How far this CHANNEL has read the thread — reading it elsewhere never clears it here. */
  readAt?: string
  /** A binding created after read reporting existed; one without it is treated as read. */
  readTracked?: boolean
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

export interface BlockedReason {
  reason: string
  detail?: string
  /** True for the subset that means "your volume is broken" rather than
   * "this pod had a bad day". */
  storage: boolean
}

export interface ConversationSummary {
  name: string
  uid?: string
  title?: string
  profile?: string
  pipeline?: string
  /**
   * The SignalSource that opened this conversation. Empty on one a channel
   * started, and on any created before the manager recorded it — render as
   * absent, never guessed.
   */
  source?: string
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
  /** Why the runtime could not start. Present means the conversation is not
   * merely queued — its pod could not come up, and this is the kubelet's own
   * reason for it. The 2026-08-20 outage showed a phase and nothing else. */
  blocked?: BlockedReason
  /** The CONSOLE's own thread has activity newer than its watermark. Observed
   * conversations — no console thread — are never unread. */
  unread: boolean
  /** The console thread's watermark, so a read is reported only when it advances. */
  readAt?: string
  ageSeconds: number
  toolsets?: string[]
  mcpConfigs?: string[]
  /** Deleted and held by its close-topics finalizer while threads are archived. */
  /** The conversation has a deletionTimestamp and is held by its close-topics
   * finalizer while its threads are archived. Named `closing` once, from when
   * /close deleted the conversation — those are two verbs now. */
  deleting: boolean
}

// ---- closing a batch ---------------------------------------------------------

/**
 * What a close batch asks for: NAMES the operator selected, and the opt-in that
 * abandons in-progress runs. No filter and no "everything matching" — what may
 * be closed is what was on screen.
 */
export interface CloseRequest {
  names: string[]
  includeWorking?: boolean
}

/** `closed` happened, `skipped` was declined and says why, `failed` was tried. */
export type CloseOutcome = 'closed' | 'deleted' | 'skipped' | 'failed'

export interface CloseResult {
  name: string
  outcome: CloseOutcome
  reason?: string
}

/** A mixed batch is the NORMAL outcome, so the result is per conversation. */
export interface CloseResponse {
  results: CloseResult[]
  closed: number
  skipped: number
  failed: number
}

/**
 * Deleting reclaims a conversation the manager has already CLOSED. `deleted`
 * replaces `closed` in the totals; the per-item shape is shared, because a
 * partial batch is the normal outcome of both.
 *
 * There is no bulk reopen: reopening re-materialises threads on every bound
 * channel, so it is a decision about one conversation.
 */
export interface DeleteResponse {
  results: CloseResult[]
  deleted: number
  skipped: number
  failed: number
}

export interface ConversationPage {
  items: ConversationSummary[]
  total: number
  /** Unread across ALL conversations, counted BEFORE any filter — a count that
   * moved because the view narrowed would let a filter hide a backlog. */
  unreadTotal: number
  offset: number
  limit: number
  facets: Record<string, string[]>
}

// ---- marking a batch read ----------------------------------------------------

/**
 * Marking read takes NAMES from the selection, like closing. The watermark is
 * not sent: the server reads each conversation's own activity, so the browser
 * can never mark activity it did not render.
 */
export interface MarkReadRequest {
  names: string[]
}

export type ReadOutcome = 'marked' | 'skipped' | 'failed'

export interface ReadResult {
  name: string
  outcome: ReadOutcome
  reason?: string
}

export interface MarkReadResponse {
  results: ReadResult[]
  marked: number
  skipped: number
  failed: number
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
  /**
   * Actions this message OFFERS. Structured, not prose — the manager states
   * what is on offer and says nothing about how it looks.
   *
   * Additive: the message body already names every choice and its addressed
   * form, for surfaces that cannot render controls. These save the typing.
   */
  choices?: Choice[]
  /**
   * A SIGNAL's raw event document, carried apart from `text` so it can be put
   * behind a disclosure control.
   *
   * It is the tallest thing in an event card and the least often read. Left
   * inside the text it is a wall of JSON between the card and the reply box.
   */
  payload?: string
}

/**
 * One block of parsed agent output. Produced by `api/blocks.ts` IN THE BROWSER,
 * never received from the wire — the manager sends the agent's text as written.
 *
 * The section vocabulary is OPEN — every agent names its own sections for its
 * own job — so a label is rendered generically and this app carries no
 * knowledge of any particular agent's names.
 */
export interface Block {
  /** "title" — the heading. "details" — THE FOLD. Anything else is a section. */
  role: 'title' | 'section' | 'details'
  /** The agent's own name for a section, absent on title and details. */
  label?: string
  text: string
}

/** One offered action. */
export interface Choice {
  label: string
  /** The addressed text this choice stands for. */
  command: string
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

// ---- the stream --------------------------------------------------------------

/**
 * One CR change, carrying the object in the shapes the snapshots serve.
 *
 * A DISCRIMINATED UNION on purpose: a delete has no object to carry, so the
 * type system refuses a handler that reads one off it. The event used to be
 * `{type, kind, name}` for every case, and every consumer answered it with a
 * request for what it had just been told had changed.
 */
export type DeltaEvent =
  | {
      type: 'ADDED' | 'MODIFIED'
      kind: string
      name: string
      /** The row this object's listing shows. Absent for kinds with no listing. */
      row?: InventoryRow
      /** The kind page. Absent for pipelines, whose resolved capabilities are
       * the manager's answer and are fetched rather than streamed. */
      detail?: Detail
      /** Conversations only: the row its own listing shows. */
      conversationRow?: ConversationSummary
      /** Conversations only: the page, minus the transcript and the events —
       * both arrive on streams of their own. */
      conversationView?: Omit<ConversationDetail, 'transcript' | 'events'>
    }
  | { type: 'DELETED'; kind: string; name: string }
  | { type: 'RESYNC'; kind: string }

/** The stream's opening event, and its answer to a cursor it cannot serve. */
export interface ResyncEvent {
  reason: string
  cursor?: string
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

/**
 * One thing a person may type, as the composer offers it.
 *
 * NAMED FOR WHAT IT IS. It used to be `Agent`, which was wrong twice: an agent
 * in this project is a DEFINITION inside a profile's repository, and this list
 * has never held one. What a message addresses is a PIPELINE.
 */
export interface VocabularyEntry {
  /** `builtin` for a manager command, `pipeline` for an addressable Pipeline. */
  kind: 'builtin' | 'pipeline'
  name: string
  /** Menu text. For a pipeline, the profile answering for it. */
  description?: string
  /**
   * How this entry is RECOGNISED in a list — an emoji, or nothing.
   *
   * Declared on the Pipeline and published as-is. Drawing it is this surface's
   * decision: Telegram cannot put one in a command name and leads the
   * description with it instead.
   */
  icon?: string
  /**
   * Where the entry is valid: `general` is the composer that STARTS a
   * conversation, `thread` the one attached to an existing one. The two take
   * disjoint sets — addressing a Pipeline inside a thread is input for the
   * agent, and the commands that end or release a conversation have nothing to
   * act on outside one.
   */
  position: 'general' | 'thread'
  /** What tells two pipelines apart when their names do not. */
  profile?: string
}

export interface VocabularyResponse {
  entries: VocabularyEntry[]
  revision?: string
}

export interface Session {
  authenticated: boolean
  configured: boolean
  identity: string
  /** Where `identity` came from: a proxy, the shared token, or nobody. */
  identitySource: 'forward-auth' | 'token' | ''
  /** `token` = this console authenticates; `external` = something in front does. */
  authMode: 'token' | 'external'
  /** What the release named as the thing authenticating instead. */
  externalAuthenticator: string
  writeEnabled: boolean
  /** writeEnabled AND an identity to attribute the write to. */
  canWrite: boolean
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
