import type {
  AgentsResponse, ChartResponse, ConversationDetail, ConversationGraph, ConversationPage, Detail,
  Finding, KindInfo, InventoryRow, Overview, Queues, Session, SourcesResponse, TopologyResponse,
} from './types'

/** Raised for a non-2xx response, carrying the server's own explanation. */
export class ApiError extends Error {
  constructor(
    readonly status: number,
    message: string,
    readonly body: Record<string, unknown> = {},
  ) {
    super(message)
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    ...init,
    headers: { 'Content-Type': 'application/json', ...(init?.headers ?? {}) },
    credentials: 'same-origin',
  })
  if (res.status === 204) return undefined as T
  const text = await res.text()
  const body = text ? (JSON.parse(text) as Record<string, unknown>) : {}
  if (!res.ok) {
    // The server's message is the useful one — it names the Wired=False reason,
    // the missing grant, or the patch to apply. Replacing it with a generic
    // string is how a diagnosable failure becomes "something went wrong".
    throw new ApiError(res.status, (body.error as string) ?? res.statusText, body)
  }
  return body as T
}

export const api = {
  session: () => request<Session>('/api/session'),
  login: (token: string) =>
    request<void>('/api/login', { method: 'POST', body: JSON.stringify({ token }) }),
  logout: () => request<void>('/api/logout', { method: 'POST' }),

  overview: () => request<Overview>('/api/overview'),
  queues: () => request<Queues>('/api/queues'),

  kinds: () => request<KindInfo[]>('/api/config'),
  inventory: (kind: string) => request<InventoryRow[]>(`/api/config/${kind}`),
  detail: (kind: string, name: string) => request<Detail>(`/api/config/${kind}/${name}`),
  findings: () => request<Finding[]>('/api/findings'),

  topology: (windowSeconds: number) =>
    request<TopologyResponse>(`/api/topology?windowSeconds=${windowSeconds}`),

  conversations: (params: URLSearchParams) =>
    request<ConversationPage>(`/api/conversations?${params.toString()}`),
  conversation: (name: string) => request<ConversationDetail>(`/api/conversations/${name}`),
  conversationGraph: (name: string) =>
    request<ConversationGraph>(`/api/conversations/${name}/graph`),

  sources: () => request<SourcesResponse>('/api/sources'),
  agents: () => request<AgentsResponse>('/api/agents'),
  start: (task: string, source?: string) =>
    request<{ source: string; note: string }>('/api/conversations', {
      method: 'POST',
      body: JSON.stringify({ task, source }),
    }),
  send: (name: string, text: string) =>
    request<unknown>(`/api/conversations/${name}/messages`, {
      method: 'POST',
      body: JSON.stringify({ text }),
    }),

  charts: () => request<{ available: boolean; charts: string[] }>('/api/charts'),
  chart: (name: string, windowSeconds: number) =>
    request<ChartResponse>(`/api/charts/${name}?windowSeconds=${windowSeconds}`),
}
