import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect } from 'react'
import { api } from './client'
import { connectStream, useStream } from './stream'
import type { CloseRequest, Health, MarkReadRequest } from './types'

// Query hooks. KEYS ARE STABLE — they name what a view shows and nothing about
// how many times it has changed.
//
// A change counter used to be folded into every key, so each event asked for a
// cache entry that had never been filled: `data` undefined, `isLoading` true,
// and the page swapped its content for a spinner until the refetch landed.
// Events are applied to the cache now (see apply.ts), which is why the counter
// has nothing left to do.
//
// A FETCH here therefore means one of four things, and each says which at its
// call site:
//
//   1. FIRST LOAD — the console holds nothing yet. Every hook below.
//   2. RESYNC — a reconnect or a reported activity gap, where the client has
//      provably missed events. Issued once, from the stream.
//   3. An EXPLICIT ACTION by the reader — the mutations, which re-read what
//      they changed.
//   4. A value that decays with TIME rather than with change. Two, both named.
//
// There is no fifth. A timed refresh that exists to observe CHANGE is the thing
// this file no longer does.

/** Opens the single stream for the app's lifetime. */
export function useLiveStream(): boolean {
  const client = useQueryClient()
  const connected = useStream((s) => s.connected)
  useEffect(() => connectStream(client), [client])
  return connected
}

export function useSession() {
  return useQuery({ queryKey: ['session'], queryFn: api.session, staleTime: 30_000 })
}

export function useOverview() {
  return useQuery({
    queryKey: ['overview'],
    queryFn: api.overview,
    // TIME-DECAYING: the manager's runtime slots, queue depths and cooldown
    // windows are rates and ages. An age is not wrong because something
    // changed, it is wrong because time passed, and no event announces that.
    refetchInterval: 15_000,
  })
}

export function useQueues() {
  // The stream pushes queue deltas every few seconds, so this only needs a cold
  // first read — polling on top would ask the manager the same question twice.
  const pushed = useStream((s) => s.queues)
  const query = useQuery({ queryKey: ['queues'], queryFn: api.queues })
  return { ...query, data: pushed ?? query.data }
}

export function useKinds() {
  return useQuery({ queryKey: ['kinds'], queryFn: api.kinds })
}

export function useInventory(kind: string) {
  return useQuery({ queryKey: ['inventory', kind], queryFn: () => api.inventory(kind) })
}

export function useDetail(kind: string, name: string) {
  return useQuery({ queryKey: ['detail', kind, name], queryFn: () => api.detail(kind, name) })
}

export function useFindings() {
  return useQuery({ queryKey: ['findings'], queryFn: api.findings })
}

export function useTopology(windowSeconds: number) {
  return useQuery({
    queryKey: ['topology', windowSeconds],
    queryFn: () => api.topology(windowSeconds),
    // Rates decay: a graph that never refetched would keep showing a burst that
    // ended ten minutes ago as current traffic.
    refetchInterval: 10_000,
  })
}

export function useConversations(params: URLSearchParams) {
  return useQuery({
    queryKey: ['conversations', params.toString()],
    queryFn: () => api.conversations(params),
    // KEPT, and this is the case the option is actually for: the key changes on
    // USER INPUT — paging, a filter, a search — so the new key genuinely has
    // never been filled and the table would blank while the next page loads.
    // Changes no longer move this key, so nothing else needs it.
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
  return useQuery({
    queryKey: ['conversationCount'],
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
  return useQuery({ queryKey: ['conversation', name], queryFn: () => api.conversation(name) })
}

export function useConversationGraph(name: string) {
  return useQuery({
    queryKey: ['conversationGraph', name],
    queryFn: () => api.conversationGraph(name),
  })
}

export function useSources() {
  return useQuery({ queryKey: ['sources'], queryFn: api.sources })
}

/** The Pipelines a message can address — Ready only, exactly like `/agents`. */
export function useVocabulary() {
  return useQuery({ queryKey: ['vocabulary'], queryFn: api.vocabulary })
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
