import { createServer, type Server, type ServerResponse } from 'node:http'
import { readFile } from 'node:fs/promises'
import { extname, join, normalize } from 'node:path'
import type { AddressInfo } from 'node:net'

// The mocked API both browser specs serve the built bundle against: one
// Coordinator root with a member, a working, a pending and an errored
// conversation, and an open stream so the connection settles on live. A spec
// may add rows and vocabulary entries of its own.

export const T0 = Date.parse('2025-06-11T09:00:00Z')
export const at = (s: number) => new Date(T0 + s * 1000).toISOString()

export const SESSION = {
  authenticated: true, configured: true, identity: 'e2e@example.com', identitySource: 'forward-auth',
  authMode: 'token', externalAuthenticator: '', writeEnabled: true, canWrite: true, canOriginate: true, metrics: false,
}

export const base = { runCount: 1, queued: 0, joined: true, errored: false, unread: false, ageSeconds: 0, deleting: false }

export const ROOT = {
  ...base, name: 'incident-root', title: 'Elevated 5xx on checkout', coordinator: 'incident', phase: 'Idle',
  created: at(-60), lastActivity: at(40), unread: true, unreadCount: 1, source: 'cluster-events',
  lastMessage: { kind: 'agent', text: 'Rolled back.' },
  runs: [{
    runId: 'run-1', status: 'succeeded', startedAt: at(0), finishedAt: at(25),
    toolCalls: [{ tool: 'mcp__k8s__get_pods', durationMs: 1500 }, { tool: 'mcp__aops__invoke', durationMs: 900 }],
  }],
}
export const MEMBER = {
  ...base, name: 'incident-root-triage', title: 'triage: confirm the restarts', phase: 'Idle', joined: false,
  created: at(5), lastActivity: at(20), causedBy: { parent: 'incident-root', entry: 'triage' },
}
export const ROWS = [
  ROOT,
  MEMBER,
  { ...base, name: 'working-1', title: 'payments rollout', pipeline: 'k8s-observe', phase: 'Working', presence: true, inflight: { runId: 'run-w' }, lastActivity: at(30), lastMessage: { kind: 'agent', text: 'Checking the rollout status for the payments deployment, which has a long name' } },
  { ...base, name: 'pending-1', title: 'HighMemoryPressure', pipeline: 'alert-triage', phase: 'Pending', runCount: 0, lastActivity: at(10) },
  { ...base, name: 'errored-1', title: 'nightly report', pipeline: 'nightly-report', phase: 'Idle', errored: true, lastActivity: at(-600) },
  { ...base, name: 'idle-1', title: 'namespaces without quotas', pipeline: 'k8s-observe', phase: 'Idle', lastActivity: at(-3600) },
]

export const REPLY_TEXT = [
  '<title>', 'Rolled back checkout-api', '</title>', '',
  '<root-cause>', 'OOM-killed: `CACHE_ENTRIES` went 5000 → 50000.', '</root-cause>', '',
  '<fix>', '`kubectl -n storefront rollout undo deploy/checkout-api`', '</fix>',
].join('\n')
export const OWN_TEXT = 'Does it affect **the other two** services?'
export const SIGNAL_TEXT = 'checkout-api restarted 5 times in 10 minutes (BackOff).'

export const DETAIL = {
  conversation: ROOT,
  object: { kind: 'Conversation', metadata: { name: ROOT.name } },
  yaml: 'kind: Conversation',
  archived: false,
  transcript: [
    { id: 'm1', thread: 'console/incident-root', kind: 'signal', text: SIGNAL_TEXT, severity: 'warning', payload: '{"reason":"BackOff"}', at: at(-60) },
    { id: 'run:run-1', thread: 'console/incident-root', kind: 'agent', text: REPLY_TEXT, at: at(25) },
    { id: 'm3', thread: 'console/incident-root', kind: 'local', text: OWN_TEXT, at: at(40) },
  ],
  events: [
    { cursor: '1', ts: at(0), kind: 'run.dispatched', runId: 'run-1', status: 'ok', conversation: ROOT.name },
    { cursor: '2', ts: at(26), kind: 'context.checkpoint', runId: 'run-1', status: 'ok', conversation: ROOT.name },
    { cursor: '3', ts: at(26), kind: 'run.completed', runId: 'run-1', status: 'ok', conversation: ROOT.name },
  ],
}

/** The stream-health shape every page that joins live events reads. */
export const STREAM_HEALTH = { connected: true, events: 3, resyncs: 0 }

/** `/api/overview`'s shape — the installation, its capacity and what reports a problem. */
export const OVERVIEW = {
  namespace: 'agent-ops',
  manager: {
    version: 'v0.1.0', leader: 'agentops-manager-0', now: at(0),
    runtimeSlots: { inUse: 1, max: 5, waiting: 0 }, queues: [], cooldowns: [],
  },
  stream: STREAM_HEALTH,
  workloads: [{ name: 'agentops-manager', desired: 1, ready: 1, restarts: 0 }],
  runtimes: [{ name: 'default' }],
  adapters: [{ kind: 'channeladapters', name: 'console', health: 'ok', serves: 1 }],
  counts: { conversations: 2 },
  synced: { conversations: true },
  problems: [],
}

/** `/api/queues`'s shape — the two admission pools, work and delivery. */
export const QUEUES = {
  capacity: { inUse: 1, max: 5, waiting: 0 },
  members: { inUse: 0, max: 5, waiting: 0 },
  work: [],
  delivery: [],
  cooldowns: [],
}

/** `/api/config`'s shape — every kind the console watches, with its count. */
export const CONFIG_KINDS = [
  { kind: 'pipelines', title: 'Pipelines', count: 1, synced: true },
  { kind: 'coordinators', title: 'Coordinators', count: 1, synced: true },
]

export const TOPOLOGY = {
  windowSeconds: 300,
  eventNodeKinds: { pipeline: 'pipelines', channel: 'channels', runtime: 'agentruntimes' },
  nodes: [
    { id: 'pipelines/ops', kind: 'pipelines', name: 'ops', health: 'ok', active: 1, recent: 4 },
    { id: 'agentruntimes/default', kind: 'agentruntimes', name: 'default', health: 'none', active: 0, recent: 0 },
    { id: 'channels/console', kind: 'channels', name: 'console', health: 'ok', active: 0, recent: 0 },
  ],
  edges: [
    { from: 'pipelines/ops', to: 'agentruntimes/default', kind: 'uses' },
    { from: 'pipelines/ops', to: 'channels/console', kind: 'posts' },
  ],
}

export const MIME: Record<string, string> = {
  '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css',
  '.svg': 'image/svg+xml', '.woff2': 'font/woff2', '.json': 'application/json',
}

type Extra = { rows?: object[]; vocabulary?: object[] }

export function answer(path: string, query: URLSearchParams, extra: Extra = {}): unknown {
  const rows = [...ROWS, ...(extra.rows ?? [])]
  if (path === '/api/session') return SESSION
  if (path === '/api/overview') return OVERVIEW
  if (path === '/api/queues') return QUEUES
  if (path === '/api/config') return CONFIG_KINDS
  if (path === '/api/topology') {
    return {
      topology: TOPOLOGY, oldestEvent: at(-120), unjoinedPipelines: [], synced: {}, metricsAvailable: false,
      stream: STREAM_HEALTH, consoleChannel: 'console',
    }
  }
  if (path === `/api/conversations/${ROOT.name}/graph`) return { ...TOPOLOGY, events: DETAIL.events, diverged: false, pipeline: 'incident' }
  if (path === '/api/sources') return { sources: [{ name: 'console', wired: true, pipeline: 'k8s-observe' }], canOriginate: true, writeEnabled: true }
  if (path === '/api/vocabulary') return { entries: [{ kind: 'coordinator', name: 'incident', position: 'general' }, { kind: 'pipeline', name: 'k8s-observe', position: 'general' }, ...(extra.vocabulary ?? [])] }
  if (path === '/api/conversations') {
    const page = { total: rows.length, unreadTotal: 1, offset: 0, limit: 100, scopes: { open: 1, working: 1, mine: 0, errored: 1, closed: 0, incident: 1 } }
    return query.get('count') === '1' ? { ...page, items: [] } : { ...page, items: rows }
  }
  if (path === `/api/conversations/${ROOT.name}`) return DETAIL
  const m = /^\/api\/conversations\/([^/]+)$/.exec(path)
  const row = m && rows.find((r) => r.name === m[1])
  if (row) {
    const hint = (row as { joinHint?: object }).joinHint
    return { ...DETAIL, conversation: row, transcript: [], events: [], ...(hint ? { joinHint: hint } : {}) }
  }
  return {}
}

export interface Harness {
  url: string
  /** Writes one server-sent event to every open stream. */
  push(event: string, data: unknown): void
  stop(): Promise<void>
}

export async function startHarness(extra: { rows?: object[]; vocabulary?: object[] } = {}): Promise<Harness> {
  const live = new Set<ServerResponse>()
  const dist = join(process.cwd(), 'dist')
  const server: Server = createServer(async (req, res) => {
    const [path, rawQuery] = (req.url ?? '/').split('?')
    if (path === '/api/stream') {
      res.writeHead(200, { 'content-type': 'text/event-stream', 'cache-control': 'no-cache', connection: 'keep-alive' })
      res.write('event: resync\ndata: {"reason":"connected"}\n\n')
      live.add(res)
      req.on('close', () => live.delete(res))
      return
    }
    if (path.startsWith('/api/')) {
      res.writeHead(200, { 'content-type': 'application/json' })
      res.end(JSON.stringify(answer(path, new URLSearchParams(rawQuery ?? ''), extra)))
      return
    }
    const rel = normalize(path).replace(/^([/\\])+/, '')
    for (const file of [rel, 'index.html']) {
      try {
        const body = await readFile(join(dist, file))
        res.writeHead(200, { 'content-type': MIME[extname(file)] ?? 'application/octet-stream' })
        res.end(body)
        return
      } catch {
        /* SPA fallback */
      }
    }
    res.writeHead(404)
    res.end()
  })
  await new Promise<void>((resolve) => server.listen(0, '127.0.0.1', resolve))
  const { port } = server.address() as AddressInfo
  return {
    url: `http://127.0.0.1:${port}`,
    push(event, data) {
      for (const res of live) res.write(`event: ${event}\ndata: ${JSON.stringify(data)}\n\n`)
    },
    async stop() {
      for (const res of live) res.end()
      live.clear()
      server.closeAllConnections()
      await new Promise<void>((resolve) => server.close(() => resolve()))
    },
  }
}

