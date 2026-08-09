import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect } from 'react'
import { api } from './client'
import { connectStream, useStream } from './stream'
import type { Health } from './types'

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
