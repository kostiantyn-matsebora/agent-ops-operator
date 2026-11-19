import type { ConversationSummary } from '../api/types'

// Derived client-side by following `causedBy` to the uncaused root (design
// D-F) — the snapshot the list already fetched holds every conversation it
// shows, so a chain walk over it is linear and needs no server-side tree
// endpoint.

/** One row of the rendered list: the conversation, its indent, and the root it (or its own root) belongs to. */
export interface TreeRow {
  row: ConversationSummary
  depth: number
  rootName: string
  /** `causedBy.parent` names a conversation this snapshot does not hold. */
  parentMissing: boolean
  /** Every live descendant at any depth — present only on a row that is itself a Coordinator's root. */
  memberCount: number
  /** How many of those members have a run underway or queued — what the root is WAITING on. */
  membersActive: number
  /** The entries those members were invoked as, for saying who is being waited on. */
  activeEntries: string[]
}

/** Whether a conversation has a run underway or waiting for one. */
export function isActive(c: ConversationSummary): boolean {
  return Boolean(c.presence) || c.phase === 'Working' || c.phase === 'Pending' || c.phase === 'Queued'
}

/** The members of `name`, at any depth, that are working or queued. */
export function activeMembersOf(items: ConversationSummary[], name: string): ConversationSummary[] {
  return descendantsOf(items, name).filter((m) => m.phase !== 'Closed' && isActive(m))
}

/**
 * Every row's descendants and what they are waiting on, in ONE pass over the
 * snapshot. A walk per row rebuilt the parent map for each of them — quadratic
 * in the list, paid on every stream event while a person is clicking.
 */
function summaries(items: ConversationSummary[]) {
  const children = childrenMap(items)
  const memo = new Map<string, { memberCount: number; membersActive: number; activeEntries: string[] }>()
  const visit = (name: string, trail: Set<string>): { memberCount: number; membersActive: number; activeEntries: string[] } => {
    const cached = memo.get(name)
    if (cached) return cached
    let memberCount = 0
    let membersActive = 0
    const entries = new Set<string>()
    if (!trail.has(name)) {
      trail.add(name)
      for (const child of children.get(name) ?? []) {
        const below = visit(child.name, trail)
        memberCount += 1 + below.memberCount
        membersActive += below.membersActive
        below.activeEntries.forEach((e) => entries.add(e))
        if (child.phase !== 'Closed' && isActive(child)) {
          membersActive += 1
          entries.add(child.causedBy?.entry ?? child.name)
        }
      }
      trail.delete(name)
    }
    const out = { memberCount, membersActive, activeEntries: [...entries] }
    memo.set(name, out)
    return out
  }
  return (name: string) => visit(name, new Set())
}

function byName(items: ConversationSummary[]): Map<string, ConversationSummary> {
  return new Map(items.map((c) => [c.name, c]))
}

function childrenMap(items: ConversationSummary[]): Map<string, ConversationSummary[]> {
  const names = byName(items)
  const children = new Map<string, ConversationSummary[]>()
  for (const c of items) {
    if (!c.causedBy || !names.has(c.causedBy.parent)) continue
    children.set(c.causedBy.parent, [...(children.get(c.causedBy.parent) ?? []), c])
  }
  return children
}

/** Every live descendant of `name`, at any depth, from the snapshot alone. */
export function descendantsOf(items: ConversationSummary[], name: string): ConversationSummary[] {
  const children = childrenMap(items)
  const out: ConversationSummary[] = []
  const walk = (n: string) => {
    for (const child of children.get(n) ?? []) {
      out.push(child)
      walk(child.name)
    }
  }
  walk(name)
  return out
}

export interface SkippedMember {
  name: string
  parentName: string
}

/**
 * Splits a selection into what a bulk action actually SENDS and what it
 * skips LOCALLY, never touching the server for the second half.
 *
 * A member reached DIRECTLY — no ancestor of it also selected — is skipped
 * and names its parent (console-conversation-tree: "a member cannot be
 * closed/deleted directly"). A member whose root IS also selected is simply
 * left out of the request: the manager's own cascade off that root already
 * reaches it, and sending it again would ask the server to act on a
 * conversation with no channel binding of its own.
 */
export function partitionSelection(
  items: ConversationSummary[],
  selected: Set<string>,
): { send: string[]; skipped: SkippedMember[] } {
  const names = byName(items)
  const send: string[] = []
  const skipped: SkippedMember[] = []
  for (const name of selected) {
    const row = names.get(name)
    if (!row?.causedBy) {
      send.push(name)
      continue
    }
    let cur = row
    let ancestorSelected = false
    const seen = new Set<string>()
    while (cur.causedBy && names.has(cur.causedBy.parent) && !seen.has(cur.name)) {
      seen.add(cur.name)
      const parent = names.get(cur.causedBy.parent)!
      if (selected.has(parent.name)) {
        ancestorSelected = true
        break
      }
      cur = parent
    }
    if (!ancestorSelected) skipped.push({ name, parentName: row.causedBy.parent })
  }
  return { send, skipped }
}

/**
 * Walks `causedBy.parent` to the uncaused root, stopping the moment a link
 * leaves the snapshot — which is also how a row with a missing parent ends
 * up naming itself as its own "root" (never dropped, see `buildTree`).
 */
export function rootNameOf(items: ConversationSummary[], c: ConversationSummary): string {
  const names = byName(items)
  let cur = c
  const seen = new Set<string>()
  while (cur.causedBy && names.has(cur.causedBy.parent) && !seen.has(cur.name)) {
    seen.add(cur.name)
    cur = names.get(cur.causedBy.parent)!
  }
  return cur.name
}

/**
 * Builds the rows the list renders.
 *
 * Grouped (the default — proposal.md: "Grouped by incident by default"), a
 * member nests directly under its root, depth-first, so a grandchild sits
 * under its own parent rather than back under the root. Flattened, the
 * server's own ordering (newest activity first) is untouched and every
 * conversation is its own row at depth 0.
 */
export function buildTree(items: ConversationSummary[], grouped: boolean): TreeRow[] {
  const names = byName(items)
  const isNested = (c: ConversationSummary) => Boolean(c.causedBy && names.has(c.causedBy.parent))
  const parentMissing = (c: ConversationSummary) => Boolean(c.causedBy && !names.has(c.causedBy.parent))
  const sum = summaries(items)

  if (!grouped) {
    return items.map((row) => ({
      row,
      depth: 0,
      rootName: row.name,
      parentMissing: parentMissing(row),
      ...sum(row.name),
    }))
  }

  const children = childrenMap(items)
  const out: TreeRow[] = []
  const walk = (c: ConversationSummary, depth: number, rootName: string) => {
    out.push({ row: c, depth, rootName, parentMissing: false, ...sum(c.name) })
    for (const child of children.get(c.name) ?? []) walk(child, depth + 1, rootName)
  }
  for (const c of items) {
    // A row with a missing parent is NEITHER nested (its parent is not in
    // the snapshot) NOR an ordinary root — it is handled once, below, with
    // the marker. Treating it as an ordinary root here would push it twice.
    if (isNested(c) || parentMissing(c)) continue
    walk(c, 0, c.name)
  }
  // A member whose parent the snapshot does not hold sits at the TOP LEVEL
  // with a marker, never dropped (console-conversation-tree spec).
  for (const c of items) {
    if (!parentMissing(c)) continue
    out.push({ row: c, depth: 0, rootName: c.name, parentMissing: true, ...sum(c.name) })
  }
  return out
}
