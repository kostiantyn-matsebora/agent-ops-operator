import type { VocabularyEntry } from '../api/types'

// THE SLASH-TYPED COMPOSER'S LISTING: what a composer shows for the text typed
// so far. Every look draws the result and none recomputes it.

/** The one place the addressed form is spelled in the UI. */
export const ADDRESS_PREFIX = '/'

/**
 * matchEntries answers what a SLASH-TYPED composer should show for the
 * current text, and `null` for "show nothing".
 *
 * Still used by the in-thread composer (`ThreadPane.tsx`, `position:
 * 'thread'`), which addresses a running conversation and has no card picker
 * of its own. This modal no longer calls it for `'general'` — see the module
 * comment above — but the function stays exported and tested for the caller
 * that still needs it.
 *
 * The listing opens only on a prefix at the very START of the message,
 * because that is the only position that addresses anyone — a slash
 * mid-sentence is a path or a date, and popping a menu over it would fight
 * the person typing.
 *
 * An empty result is `null` rather than an empty list: a popup saying nothing
 * is worse than no popup, and a surface with no Ready pipelines has nothing to
 * offer in the first place.
 */
export function matchEntries(
  text: string,
  entries: VocabularyEntry[],
  position: 'general' | 'thread',
): VocabularyEntry[] | null {
  if (!text.startsWith(ADDRESS_PREFIX)) return null
  const typed = text.slice(ADDRESS_PREFIX.length)
  // A space means the name is finished and the task has begun — the person is
  // past choosing, so the menu gets out of the way.
  if (/\s/.test(typed)) return null
  const q = typed.toLowerCase()
  // POSITION IS THE FILTER. This composer STARTS a conversation, so it offers
  // what can start one. The composer attached to a conversation offers what
  // acts on one. Offering the wrong half would put a command in front of
  // somebody at the one place it does nothing.
  const hits = entries.filter(
    (e) => e.position === position && e.name.toLowerCase().startsWith(q),
  )
  return hits.length > 0 ? hits : null
}
