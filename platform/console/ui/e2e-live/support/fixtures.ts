// The fixture manifest `global-setup.ts` writes and every spec reads. A
// plain JSON file rather than a shared module-level variable: Playwright may
// run spec files as separate workers/processes, and only a file on disk is
// guaranteed to cross that boundary.

import { readFileSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const here = path.dirname(fileURLToPath(import.meta.url))

export interface TitledConversation {
  name: string
  title: string
  pipeline?: string
}

export interface Fixtures {
  stamp: string
  adapterToken: string
  uiToken: string
  storageStatePath: string
  /** The manager's own base URL, reached through the port-forward
   * `global-setup.ts` opens and keeps alive for the whole run — specs that
   * need to post a signal AT A PRECISE MOMENT (an arrival while already
   * watching the list) use this directly, rather than pre-seeding it in
   * setup, which could never observe the live arrival. */
  managerUrl: string
  coordinators: { open: string; closed: string; deleted: string; unjoined: string }
  conversations: {
    coordOpenRoot: string
    coordOpenMembers: [string, string]
    coordClosedRoot: string
    coordDeletedRoot: string
    coordUnjoinedRoot: string
    prefixTask: TitledConversation
    colonTask: TitledConversation
    observedChat: TitledConversation
    markTask: string
    runsTask: string
    closedTask: string
    closeConfirmCancel: string
    closeConfirmSkipA: string
    closeConfirmSkipB: string
    closeConfirmExit: string
  }
}

let cached: Fixtures | undefined

function readFixturesFile(): Fixtures {
  if (cached) return cached
  const file = path.join(here, '..', '.generated', 'fixtures.json')
  try {
    cached = JSON.parse(readFileSync(file, 'utf8')) as Fixtures
    return cached
  } catch (e) {
    throw new Error(
      `could not read ${file} — run the full suite (globalSetup builds it) rather than a single spec in isolation, ` +
        `or a single file with --config e2e-live/playwright.config.ts, never 'npx playwright test' bare: ${String(e)}`,
    )
  }
}

/**
 * Every spec calls this once, at module scope, for a plain `f.whatever`
 * reading style — but `playwright test --list` imports every spec file
 * WITHOUT running `globalSetup` first, and a module-scope throw there would
 * break listing even though no test actually ran. The returned object is
 * therefore a Proxy that defers the real read to the first PROPERTY access
 * — which happens inside a test body, after `globalSetup` has already run —
 * rather than at import time.
 */
export function loadFixtures(): Fixtures {
  return new Proxy({} as Fixtures, {
    get(_target, prop) {
      return Reflect.get(readFixturesFile(), prop)
    },
  })
}
