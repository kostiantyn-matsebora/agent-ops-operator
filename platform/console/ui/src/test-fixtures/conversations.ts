import type { ConversationDetail, ConversationPage } from '../api/types'

// The conversations of the curated install the site's screenshots and demo
// are taken of (`screenshots/fixture.ts`), re-exported so the model's unit
// tests pin their output against the same picture a published frame shows.
//
// Loaded through the bundler rather than a static import: the screenshot
// harness is type-checked by its own run, not by this one, and a static import
// would pull it into the app's type check.
//
// Every timestamp there is relative to `NOW`, which is fixed, so a test that
// passes `curatedNow` gets the same answer on every run.

interface CuratedInstall {
  NOW: Date
  install: { conversations: ConversationPage; conversationDetail: ConversationDetail }
}

const loaded = import.meta.glob('../../screenshots/fixture.ts', { eager: true })
const fixture = Object.values(loaded)[0] as CuratedInstall | undefined
if (!fixture) throw new Error('test-fixtures: screenshots/fixture.ts did not load')

export const curatedNow = fixture.NOW.getTime()
export const curatedConversations = fixture.install.conversations
/** The one conversation with a full transcript: a signal-started run that replied, and a second run working. */
export const curatedDetail = fixture.install.conversationDetail
