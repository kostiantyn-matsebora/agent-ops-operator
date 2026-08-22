import { QueryClient } from '@tanstack/react-query'

/**
 * How long data no component is holding survives before it is released.
 *
 * Events are APPLIED to what is cached (see apply.ts), so a view stays correct
 * without ever being re-read — which also means it would stay RESIDENT for as
 * long as the tab is open if nothing released it. A conversation somebody
 * glanced at an hour ago should not still cost memory.
 *
 * This never collects what is on screen: react-query's `gcTime` applies only to
 * data no observer holds, so nothing is taken from under a reader.
 */
export const EVICT_UNUSED_AFTER_MS = 5 * 60_000

/**
 * How long applied data is trusted on a remount.
 *
 * Past it, returning to a view loads fresh rather than rendering whatever was
 * last applied to it. It never blanks: the previous data is either gone — in
 * which case this is a first load — or still held and shown while the read runs.
 */
export const FRESH_FOR_MS = 60_000

/**
 * The one place the cache's bounds are set, with the reasoning beside them.
 *
 * Five minutes and one minute: long enough that flicking between two pages
 * costs nothing, short enough that an afternoon of browsing does not carry
 * every conversation it touched. The bound is about MEMORY — correctness is the
 * resync rule's job, which replaces applied state wholesale whenever the client
 * may have missed an event.
 *
 * NOTHING IS PERSISTED. No localStorage, no IndexedDB, no service worker: the
 * console holds a snapshot of cluster state and a transcript of what agents
 * said, and writing either to a browser's disk is a durability promise this
 * component has no business making — it would also survive a logout.
 *
 * Retry stays low so a failing endpoint surfaces its reason rather than being
 * retried into a spinner.
 */
export function createQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: {
        retry: 1,
        refetchOnWindowFocus: false,
        staleTime: FRESH_FOR_MS,
        gcTime: EVICT_UNUSED_AFTER_MS,
      },
    },
  })
}
