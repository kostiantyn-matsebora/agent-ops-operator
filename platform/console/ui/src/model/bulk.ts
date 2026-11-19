import type { CloseResponse, ConversationSummary, DeleteResponse } from '../api/types'
import { descendantsOf } from './tree'

// THE BATCH VERBS' FACTS: which rows on the current page a close or a delete
// may select, how many of a selection are working, and which of a finished
// batch did not land. Every look's confirmation draws these; none recomputes
// them.
//
// Three properties are load-bearing:
//
//   - the selection can never escape the current page (each function takes the
//     rows the server returned, which IS one page), so a mis-set filter cannot
//     close hundreds across pages;
//   - deleting is narrower than closing: only a CLOSED conversation is a
//     candidate, because the two-step is the safety property, not a nag;
//   - a MEMBER is never a candidate, for either verb — the manager's own
//     cascade off its root already reaches it (console-conversation-tree: "A
//     member SHALL NOT be selectable in this view"), and the server now
//     refuses a member named directly anyway (`causedBy.parent` guard).

/**
 * The ROOT rows on the CURRENT PAGE that may be selected for a close. A
 * conversation already held by its close-topics finalizer is on its way out,
 * and offering it again would only produce a second `/close` with nowhere to
 * land. A member carries no selection control at all, so it is never a
 * candidate here either.
 */
export function selectableNames(items: ConversationSummary[]): string[] {
  return items.filter((c) => !c.deleting && !c.causedBy).map((c) => c.name)
}

/** How many of the selection are working — stated before anything is abandoned. */
export function workingCount(items: ConversationSummary[], selected: Set<string>): number {
  return items.filter((c) => selected.has(c.name) && c.phase === 'Working').length
}

/**
 * How many of the cascade's MEMBERS — not the selected roots themselves —
 * are working, across every selected root's descendants at any depth. Named
 * separately from `workingCount` (console-inbox-open-closed design: "The
 * member line separately names how many of the cascade's members are
 * working, so a run in progress is not abandoned unseen when its root
 * closes").
 */
export function workingMemberCount(items: ConversationSummary[], roots: string[]): number {
  return roots.reduce((n, root) => n + descendantsOf(items, root).filter((m) => m.phase === 'Working').length, 0)
}

/** The conversations a close batch did not close, with the server's own reason. */
export function notClosed(result: CloseResponse): CloseResponse['results'] {
  return result.results.filter((r) => r.outcome !== 'closed')
}

/** The rows on the CURRENT PAGE that may be deleted: closed, and not already on their way out. */
export function deletableNames(items: ConversationSummary[]): string[] {
  return items.filter((c) => !c.deleting && c.phase === 'Closed').map((c) => c.name)
}

/** The conversations a delete batch did not delete, with the server's own reason. */
export function notDeleted(result: DeleteResponse): DeleteResponse['results'] {
  return result.results.filter((r) => r.outcome !== 'deleted')
}
