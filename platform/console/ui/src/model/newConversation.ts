import { useSession, useSources, useVocabulary } from '../api/hooks'
import type { OriginationSource, VocabularyEntry } from '../api/types'
import { useCommands } from './commands'
import { NEW_CONVERSATION, startConversation } from './verbs'

// STARTING A CONVERSATION: whether this console can, why not when it cannot,
// who answers, and what can be addressed. A look draws the dialog; these facts
// are the same in every look.
//
// "There is no button" and "the button is unavailable, here is why" are
// different messages, and only one is actionable — so an unavailable start is
// a state with a reason, never an absence:
//
//   read-only       console.write.enabled is false
//   no-identity     authentication in front forwarded nobody to record it against
//   not-originating the console holds no signal identity
//   unwired         no Ready Pipeline claims the console's source
//   ready           the dialog

export type StartAvailability = 'loading' | 'read-only' | 'no-identity' | 'not-originating' | 'unwired' | 'ready'

/** The addressed form's prefix — the one place the UI spells it. */
export const ADDRESS_PREFIX = '/'

/** The task as sent: addressed to the chosen destination, or bare. */
export function addressedTask(task: string, destination: string | null): string {
  return destination ? `${ADDRESS_PREFIX}${destination} ${task}` : task
}

/**
 * Only what a start can address: Pipelines and Coordinators. `builtin`
 * entries are listing commands whose whole result is a reply on a general
 * surface — the destination list already IS that listing.
 */
export function startableEntries(entries: VocabularyEntry[] | undefined): VocabularyEntry[] {
  return (entries ?? []).filter((e) => e.kind === 'pipeline' || e.kind === 'coordinator')
}

/** A plain substring filter by name — never a slash-addressed command parser. */
export function filterEntries(entries: VocabularyEntry[], filter: string): VocabularyEntry[] {
  const q = filter.trim().toLowerCase()
  return q ? entries.filter((e) => e.name.toLowerCase().includes(q)) : entries
}

/**
 * The server fills `profile` only when ONE Pipeline serves the source; blank
 * with a named pipeline therefore means several do, which is exactly when an
 * unaddressed task gets refused.
 */
export function isAmbiguous(wired: OriginationSource[]): boolean {
  return wired.some((s) => !s.profile && !!s.pipeline)
}

export function useNewConversation() {
  const session = useSession()
  const sources = useSources()
  const vocabulary = useVocabulary()
  const busy = useCommands().sending(NEW_CONVERSATION)

  const all = sources.data?.sources ?? []
  const wired = all.filter((s) => s.wired)
  const unwired = all.filter((s) => !s.wired)

  let availability: StartAvailability = 'ready'
  if (session.isLoading || sources.isLoading) availability = 'loading'
  else if (!session.data?.writeEnabled) availability = 'read-only'
  else if (!session.data?.canWrite) availability = 'no-identity'
  else if (!sources.data?.canOriginate) availability = 'not-originating'
  else if (wired.length === 0) availability = 'unwired'

  /** Starts the task on the first wired source. Resolves to the error, or to who answers. */
  async function start(task: string, destination: string | null): Promise<{ ok: true; answering?: string } | { ok: false; error: string }> {
    const outcome = await startConversation(addressedTask(task, destination), wired[0]?.name)
    if (!outcome.ok) return { ok: false, error: outcome.error }
    return { ok: true, answering: destination ?? wired[0]?.pipeline }
  }

  return {
    availability,
    externalAuthenticator: session.data?.externalAuthenticator ?? '',
    wired,
    unwired,
    ambiguous: isAmbiguous(wired),
    vocabularyLoading: vocabulary.isLoading,
    startable: startableEntries(vocabulary.data?.entries),
    busy,
    start,
  }
}
