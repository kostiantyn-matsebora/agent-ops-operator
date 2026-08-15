import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect } from 'react'
import { api } from './client'
import { connectStream, useStream } from './stream'
import type { CloseRequest, Health, MarkReadRequest } from './types'

// Query hooks. Each names the resource kinds it depends on, so a CR delta for
// that kind invalidates exactly the queries showing it — the alternative,
// refetching everything on every delta, turns a busy namespace into a refetch
// storm.

/** revisionKey folds the relevant kind revisions into a query key. */
function useRevision(kinds: string[]): number {
  const revisions = useStream((s) => s.revisions)
  const resyncs = useStream((s) => s.resyncs)
  return kinds.reduce((sum, k) => sum + (revisions[k] ?? 0), resyncs)
}

/** Opens the single stream for the app's lifetime and wires resync. */
export function useLiveStream(): boolean {
  const client = useQueryClient()
  const connected = useStream((s) => s.connected)
  useEffect(() => connectStream(() => client.invalidateQueries()), [client])
  return connected
}

export function useSession() {
  return useQuery({ queryKey: ['session'], queryFn: api.session, staleTime: 30_000 })
}

const ALL_KINDS = [
  'agentprofiles', 'agentruntimes', 'channels', 'channeladapters', 'conversations',
  'mcpconfigs', 'mcptoolsets', 'pipelines', 'signaladapters', 'signalsources',
  'deployments', 'pods',
]

export function useOverview() {
  const rev = useRevision(ALL_KINDS)
  return useQuery({ queryKey: ['overview', rev], queryFn: api.overview, refetchInterval: 15_000 })
}

export function useQueues() {
  // The stream pushes queue deltas every few seconds, so this only needs a cold
  // first read — polling on top would ask the manager the same question twice.
  const pushed = useStream((s) => s.queues)
  const query = useQuery({ queryKey: ['queues'], queryFn: api.queues })
  return { ...query, data: pushed ?? query.data }
}

export function useKinds() {
  const rev = useRevision(ALL_KINDS)
  return useQuery({ queryKey: ['kinds', rev], queryFn: api.kinds })
}

export function useInventory(kind: string) {
  const rev = useRevision([kind])
  return useQuery({ queryKey: ['inventory', kind, rev], queryFn: () => api.inventory(kind) })
}

export function useDetail(kind: string, name: string) {
  const rev = useRevision([kind])
  return useQuery({ queryKey: ['detail', kind, name, rev], queryFn: () => api.detail(kind, name) })
}

export function useFindings() {
  const rev = useRevision(ALL_KINDS)
  return useQuery({ queryKey: ['findings', rev], queryFn: api.findings })
}

export function useTopology(windowSeconds: number) {
  const rev = useRevision(ALL_KINDS)
  return useQuery({
    queryKey: ['topology', windowSeconds, rev],
    queryFn: () => api.topology(windowSeconds),
    // Rates decay: a graph that never refetched would keep showing a burst that
    // ended ten minutes ago as current traffic.
    refetchInterval: 10_000,
  })
}

export function useConversations(params: URLSearchParams) {
  const rev = useRevision(['conversations'])
  return useQuery({
    queryKey: ['conversations', params.toString(), rev],
    queryFn: () => api.conversations(params),
    // The revision is part of the KEY, so every delta asks for a cache entry
    // that has never been filled — `data` undefined, `isLoading` true, and the
    // page swaps its table for a spinner. Keeping the previous page on screen
    // is what makes a live list update instead of blink; without it a batch
    // close (fifty deltas) strobes.
    placeholderData: keepPreviousData,
  })
}

/**
 * Closing a batch of conversations.
 *
 * Invalidated on SETTLE rather than on success: a partially applied batch still
 * changed the cluster, so the list must be re-read either way. The closed
 * conversations do not vanish immediately — the close-topics finalizer holds
 * them while the threads are archived — so what the refetch shows is them
 * turning `closing`, which is the honest state.
 */
export function useCloseConversations() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (req: CloseRequest) => api.closeConversations(req),
    onSettled: () => client.invalidateQueries({ queryKey: ['conversations'] }),
  })
}

/**
 * Marking a selection read.
 *
 * Invalidated on SETTLE like every other batch: a partly applied batch still
 * moved watermarks, so the list has to be re-read either way.
 */
export function useMarkRead() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (req: MarkReadRequest) => api.markRead(req),
    onSettled: () => client.invalidateQueries({ queryKey: ['conversations'] }),
  })
}

/**
 * The unread count for the navigation, without the rows.
 *
 * count=1 asks for the totals only — the badge wants a number, and a page of
 * conversations it would throw away is the wrong thing to fetch on every route.
 * The count is computed server-side BEFORE any filter, so it is the same number
 * whatever the list page is currently showing.
 */
export function useUnreadCount() {
  const rev = useRevision(['conversations'])
  return useQuery({
    queryKey: ['conversationCount', rev],
    queryFn: () => api.conversations(new URLSearchParams({ count: '1' })),
  })
}

export function useDeleteConversations() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (req: CloseRequest) => api.deleteConversations(req),
    onSettled: () => client.invalidateQueries({ queryKey: ['conversations'] }),
  })
}

// Per conversation, and no bulk equivalent: reopening re-materialises threads
// on every bound channel, so a batch would announce itself on surfaces nobody
// is watching.
export function useReopenConversation() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (name: string) => api.reopenConversation(name),
    onSettled: () => client.invalidateQueries({ queryKey: ['conversations'] }),
  })
}

export function useConversation(name: string) {
  const rev = useRevision(['conversations'])
  const messageRevision = useStream((s) => s.messageRevision)
  return useQuery({
    queryKey: ['conversation', name, rev, messageRevision],
    queryFn: () => api.conversation(name),
  })
}

export function useConversationGraph(name: string) {
  const rev = useRevision(['conversations'])
  return useQuery({
    queryKey: ['conversationGraph', name, rev],
    queryFn: () => api.conversationGraph(name),
  })
}

export function useSources() {
  const rev = useRevision(['signalsources', 'pipelines'])
  return useQuery({ queryKey: ['sources', rev], queryFn: api.sources })
}

/** The Pipelines a message can address — Ready only, exactly like `/agents`. */
export function useAgents() {
  const rev = useRevision(['pipelines'])
  return useQuery({ queryKey: ['agents', rev], queryFn: api.agents })
}

/** Which historical charts the backend can answer, and whether one exists. */
export function useCharts() {
  return useQuery({ queryKey: ['charts'], queryFn: api.charts, staleTime: 30_000 })
}

export function useChart(name: string, windowSeconds: number, enabled: boolean) {
  return useQuery({
    queryKey: ['chart', name, windowSeconds],
    queryFn: () => api.chart(name, windowSeconds),
    enabled,
    retry: false,
  })
}

/**
 * PatternFly Label status for a health value.
 *
 * `none` maps to `custom` rather than a status colour: a kind that asserts no
 * health has nothing to be green or red about, and colouring it would put a
 * verdict on an object no reconciler judged.
 */
export function healthVariant(h: Health): 'success' | 'danger' | 'warning' | 'custom' {
  switch (h) {
    case 'ok':
      return 'success'
    case 'bad':
      return 'danger'
    case 'unknown':
      return 'warning'
    default:
      return 'custom'
  }
}
