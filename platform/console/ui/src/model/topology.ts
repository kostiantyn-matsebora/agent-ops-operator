import { useMemo } from 'react'
import { useActivityBuffer, useChart, useConversation, useConversationGraph, useTopology } from '../api/hooks'
import { useStream } from '../api/stream'
import { useDisplay } from '../graph/display'
import { mergeEvents } from '../graph/hops'

// THE TOPOLOGY PAGE'S FACTS: the wiring graph, the activity it animates, what
// the activity window covers, and the History card's charts. The graph itself
// is graph/, shared by every look; which events it is handed is decided here.

/**
 * The topology for the Display panel's window, the events to animate — what
 * the console held when the page opened, then the stream on top — and whether
 * the activity buffer reaches back across the whole window.
 */
export function useTopologyPage() {
  const windowSeconds = useDisplay((s) => s.windowSeconds)
  const historyOpen = useDisplay((s) => s.historyOpen)
  const query = useTopology(windowSeconds)
  const { data } = query
  const liveEvents = useStream((s) => s.events)
  const buffer = useActivityBuffer()
  const events = useMemo(() => mergeEvents(buffer.data?.events ?? [], liveEvents), [buffer.data, liveEvents])
  const bufferStart = data?.oldestEvent ? Date.parse(data.oldestEvent) : undefined

  // A window longer than the buffer holds is reported as such rather than
  // rendered as a quiet one — "we cannot see that far back" and "nothing
  // happened" are different claims.
  const bufferCovers = useMemo(() => {
    if (!data?.oldestEvent) return true
    const oldest = new Date(data.oldestEvent).getTime()
    return Date.now() - oldest >= windowSeconds * 1000
  }, [data?.oldestEvent, windowSeconds])

  return {
    ...query,
    events,
    bufferStart,
    bufferCovers,
    historyOpen,
    toggleHistory: () => useDisplay.getState().setHistoryOpen(!historyOpen),
  }
}

/**
 * One conversation's graph: the topology for the Display panel's window, this
 * conversation's recorded hops with the live stream on top, and whether the
 * pipeline was re-wired since it ran.
 */
export function useConversationGraphPage(name: string) {
  const windowSeconds = useDisplay((s) => s.windowSeconds)
  const graph = useConversationGraph(name)
  const topology = useTopology(windowSeconds)
  const live = useStream((s) => s.events)
  const events = useMemo(() => mergeEvents(graph.data?.events ?? [], live), [graph.data?.events, live])
  const oldest = topology.data?.oldestEvent
  return {
    graph,
    topology,
    events,
    bufferStart: oldest ? Date.parse(oldest) : undefined,
    loading: (graph.isLoading && !graph.data) || (topology.isLoading && !topology.data),
  }
}

/**
 * The History card's charts, from the metrics backend. Every one is an
 * AGGREGATE: the cardinality rule keeps ids out of metric labels, so no point
 * can identify a conversation.
 */
export const HISTORY_CHARTS: readonly { key: string; title: string; unit: string }[] = [
  { key: 'throughput', title: 'Conversations started', unit: '/min' },
  { key: 'runDurationP95', title: 'Run duration (p95)', unit: 's' },
  { key: 'queueDepth', title: 'Channel ops queued', unit: '' },
]

/**
 * One history chart. Charts deliberately ask for a LONGER range than the
 * graph's window: their whole purpose is the horizon the buffer cannot reach.
 */
export function useHistoryChart(chartKey: string) {
  const windowSeconds = useDisplay((s) => s.windowSeconds)
  const range = Math.max(windowSeconds, 6 * 3600)
  return useChart(chartKey, range, true)
}

/**
 * The conversation a hop belongs to, joined by id from its durable status — the
 * same query the conversation page reads, kept current by the stream. Idle when
 * the hop names none.
 */
export function useHopConversation(name?: string) {
  return useConversation(name ?? '', Boolean(name))
}
