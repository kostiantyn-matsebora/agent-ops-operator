// Arranges every fixture this suite's specs read, against the REAL backend
// already live in the cluster `E2E_LIVE_KUBE_CONTEXT` names (default
// `k3d-agentops-e2e`) — no mocks, no envtest. Setup only: every ASSERTION
// lives in the specs, reading the console's own rendering. Where setup needs
// a fact only Kubernetes or the manager's own HTTP surface can give it
// precisely (a deterministic close, a specific coordinator member, a run
// carrying turns/toolCalls), it goes straight at that interface, exactly as
// `platform/manager/test/e2e`'s Go harness does — never by clicking through
// the UI, which is what the specs are FOR.
//
// Writes `.generated/fixtures.json`, read by every spec via `./support/fixtures`.

import { mkdirSync, writeFileSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { request, type FullConfig } from '@playwright/test'
import {
  findConversationByCoordinatorRoot, findConversationByTitle, findMembersOf,
  getJSON, getSecretValue, kubectlApply, kubectlDelete, portForward, runCount, waitFor,
} from './support/kube'
import { ManagerClient } from './support/manager'
import { ConsoleApiClient } from './support/consoleApi'
import type { Fixtures } from './support/fixtures'

const here = path.dirname(fileURLToPath(import.meta.url))

const STUB_PROFILE = 'e2e-stub'
const SOURCE_TASKS = 'e2e-tasks' // console-bound task lane, claimed by PipelineConsole (e2e-console)
const SOURCE_CONSOLE = 'console' // the console's own chat-capable signal source
const SOURCE_TELEGRAM = 'tg-ops' // claimed only by PipelineChat (e2e-chat) -> never joins console

const COORD_OPEN = 'e2e-ui-coord'
const COORD_CLOSED = 'e2e-ui-coord-closed'
const COORD_DELETED = 'e2e-ui-coord-del'
// Item 19: a root the CONSOLE channel never bound — opened on the telegram
// source instead of the console one, so `joined` stays false and the
// join-hint fires for a Coordinator, never the generic Pipeline wording.
const COORD_UNJOINED = 'e2e-ui-coord-unjoined'
const CAPABILITY = 'e2e-ui-domain'

function fixtureYAML(): string {
  // One shared AgentCapability (profile-only, e2e-stub) referenced by every
  // coordinator's agents[] — capabilities are referenced, not owned, so
  // sharing one across coordinators costs nothing and keeps setup small.
  //
  // COORD_OPEN carries two DISTINCT agent entries: `invoke` re-attaches to
  // an already-open member for the SAME entry name rather than creating a
  // second one (verified live against this cluster), so a tree with two
  // members needs two entries, not one invoked twice.
  return `
apiVersion: agentops.dev/v1alpha1
kind: AgentCapability
metadata:
  name: ${CAPABILITY}
spec:
  profileRef: { name: ${STUB_PROFILE} }
---
apiVersion: agentops.dev/v1alpha1
kind: Coordinator
metadata:
  name: ${COORD_OPEN}
spec:
  profileRef: { name: ${STUB_PROFILE} }
  agents:
    - name: domain
      capabilityRef: { name: ${CAPABILITY} }
      description: handles domain tasks
    - name: support
      capabilityRef: { name: ${CAPABILITY} }
      description: handles support tasks
---
apiVersion: agentops.dev/v1alpha1
kind: Coordinator
metadata:
  name: ${COORD_CLOSED}
spec:
  profileRef: { name: ${STUB_PROFILE} }
---
apiVersion: agentops.dev/v1alpha1
kind: Coordinator
metadata:
  name: ${COORD_DELETED}
spec:
  profileRef: { name: ${STUB_PROFILE} }
---
apiVersion: agentops.dev/v1alpha1
kind: Coordinator
metadata:
  name: ${COORD_UNJOINED}
spec:
  profileRef: { name: ${STUB_PROFILE} }
`
}

interface ConditionedObject {
  status?: { conditions?: { type: string; status: string }[] }
}

async function waitCoordinatorsReady(names: string[]): Promise<void> {
  await Promise.all(
    names.map((name) =>
      waitFor(
        `coordinator ${name} Ready`,
        () => {
          const out = getJSON<ConditionedObject>('coordinator.agentops.dev', name)
          const ready = (out.status?.conditions ?? []).find((c) => c.type === 'Ready')
          return ready?.status === 'True' ? true : undefined
        },
        60_000,
      ),
    ),
  )
}

export default async function globalSetup(config: FullConfig): Promise<() => Promise<void>> {
  const project = config.projects[0]
  const consoleUrl = (project.use.baseURL as string | undefined) ?? process.env.E2E_LIVE_CONSOLE_URL ?? 'http://localhost:18099'
  const uiToken = getSecretValue('agentops-console-console', 'uiToken') ?? process.env.E2E_LIVE_UI_TOKEN ?? 'e2e-ui-token-not-a-secret'
  const adapterToken = getSecretValue('agentops-adapter-token', 'token') ?? process.env.E2E_LIVE_ADAPTER_TOKEN ?? 'e2e-adapter-token-not-a-secret'

  console.log(`[global-setup] console at ${consoleUrl}, applying wiring fixtures`)
  kubectlApply(fixtureYAML())
  await waitCoordinatorsReady([COORD_OPEN, COORD_CLOSED, COORD_DELETED, COORD_UNJOINED])

  console.log('[global-setup] port-forwarding the manager service')
  const managerForward = await portForward('agentops-manager', 8080)
  // Everything below can throw partway through (a slow admission, a bad
  // fixture) — without this try/catch an early throw would skip the
  // `return` below entirely, and with it the teardown Playwright calls to
  // stop this very port-forward, leaking the kubectl process. Measured
  // live: two earlier failed setup runs each left one behind.
  try {
    return await arrangeFixtures(managerForward.localPort, consoleUrl, uiToken, adapterToken, managerForward.stop)
  } catch (e) {
    managerForward.stop()
    throw e
  }
}

async function arrangeFixtures(
  managerLocalPort: number,
  consoleUrl: string,
  uiToken: string,
  adapterToken: string,
  stopManagerForward: () => void,
): Promise<() => Promise<void>> {
  const manager = new ManagerClient(`http://127.0.0.1:${managerLocalPort}`, adapterToken)
  const consoleApi = new ConsoleApiClient(consoleUrl, uiToken)

  const stamp = String(Date.now())
  const start = new Date()

  // ---- Coordinator fixtures -------------------------------------------------

  console.log('[global-setup] opening the coordinator fixtures')
  await manager.postChatCommand(SOURCE_CONSOLE, 'console', `e2e-ui-coord-open-${stamp}`, `/${COORD_OPEN} echo coordopen-${stamp}`)
  const coordOpenRoot = await waitFor('the open coordinator root', () => findConversationByCoordinatorRoot(COORD_OPEN, start))
  // Two DISTINCT members (see fixtureYAML's comment on why two entries).
  const member1 = await manager.invoke(COORD_OPEN, coordOpenRoot.metadata.name, 'domain', `echo member-domain-${stamp}`)
  const member2 = await manager.invoke(COORD_OPEN, coordOpenRoot.metadata.name, 'support', `echo member-support-${stamp}`)
  await waitFor('both coordinator members to finish their first run', () => {
    const members = findMembersOf(coordOpenRoot.metadata.name)
    const m1 = members.find((m) => m.metadata.name === member1)
    const m2 = members.find((m) => m.metadata.name === member2)
    return m1 && m2 && runCount(m1) >= 1 && runCount(m2) >= 1 ? true : undefined
  })

  await manager.postChatCommand(SOURCE_CONSOLE, 'console', `e2e-ui-coord-closed-${stamp}`, `/${COORD_CLOSED} echo coordclosed-${stamp}`)
  const coordClosedRoot = await waitFor('the to-be-closed coordinator root', () => findConversationByCoordinatorRoot(COORD_CLOSED, start))
  await waitFor('the to-be-closed coordinator root to finish its run', () => {
    const c = findConversationByCoordinatorRoot(COORD_CLOSED, start)
    return c && runCount(c) >= 1 ? c : undefined
  })
  await consoleApi.closeConversations([coordClosedRoot.metadata.name])

  await manager.postChatCommand(SOURCE_CONSOLE, 'console', `e2e-ui-coord-deleted-${stamp}`, `/${COORD_DELETED} echo coorddeleted-${stamp}`)
  const coordDeletedRoot = await waitFor('the to-be-deleted coordinator root', () => findConversationByCoordinatorRoot(COORD_DELETED, start))
  // The regression this reproduces: the Coordinator CR is gone, but
  // `spec.coordinatorRef` on the conversation is provenance and must survive.
  kubectlDelete('coordinator.agentops.dev', COORD_DELETED)

  // Item 19: addressed on the TELEGRAM source/channel, never the console one,
  // so the console's own channel never binds — `joined` stays false and the
  // join-hint has something real to explain.
  await manager.postChatCommand(SOURCE_TELEGRAM, 'tg-ops', `e2e-ui-coord-unjoined-${stamp}`, `/${COORD_UNJOINED} echo coordunjoined-${stamp}`)
  const coordUnjoinedRoot = await waitFor('the unjoined coordinator root', () => findConversationByCoordinatorRoot(COORD_UNJOINED, start))

  // ---- Plain-pipeline fixtures -----------------------------------------------

  console.log('[global-setup] opening the plain-pipeline fixtures')
  const prefixTitle = `e2e-console: handle the prefix thing ${stamp}`
  await manager.postTask(SOURCE_TASKS, `e2e-ui-prefix-${stamp}`, prefixTitle, `echo prefixed-${stamp}`)
  const prefixTask = await waitFor('the prefix-stripping task conversation', () => findConversationByTitle(prefixTitle))

  const colonTitle = `alert: disk is low on nodeA ${stamp}`
  await manager.postTask(SOURCE_TASKS, `e2e-ui-colon-${stamp}`, colonTitle, `echo coloned-${stamp}`)
  const colonTask = await waitFor('the unrelated-colon task conversation', () => findConversationByTitle(colonTitle))

  const observedFingerprint = `e2e-ui-tg-${stamp}`
  await manager.postChatCommand(SOURCE_TELEGRAM, 'tg-ops', observedFingerprint, `ping from telegram ${stamp}`)
  const observedTitle = `💬 ping from telegram ${stamp}`
  const observedChat = await waitFor('the observed (console-unjoined) chat conversation', () => findConversationByTitle(observedTitle))

  const markTitle = `e2e-ui-mark-${stamp}`
  await manager.postTask(SOURCE_TASKS, `e2e-ui-mark-${stamp}`, markTitle, `echo marktoggle-${stamp}`)
  const markTask = await waitFor('the mark-read/unread task conversation', () => findConversationByTitle(markTitle))
  await waitFor('the mark-read/unread task to finish its run', () => (runCount(findConversationByTitle(markTitle)!) >= 1 ? true : undefined))

  const runsTitle = `e2e-ui-runs-${stamp}`
  await manager.postTask(SOURCE_TASKS, `e2e-ui-runs-${stamp}`, runsTitle, 'calls')
  const runsTask = await waitFor('the runs-view task conversation', () => findConversationByTitle(runsTitle))
  await waitFor('the runs-view task\'s first (turns/toolCalls) run to finish', () =>
    (runCount(findConversationByTitle(runsTitle)!) >= 1 ? true : undefined))
  await consoleApi.sendMessage(runsTask.metadata.name, `echo plainrun-${stamp}`)
  await waitFor('the runs-view task\'s second (plain) run to finish', () =>
    (runCount(findConversationByTitle(runsTitle)!) >= 2 ? true : undefined))

  const closedTitle = `e2e-ui-closed-${stamp}`
  await manager.postTask(SOURCE_TASKS, `e2e-ui-closed-${stamp}`, closedTitle, `echo closeme-${stamp}`)
  const closedTask = await waitFor('the show-closed task conversation', () => findConversationByTitle(closedTitle))
  await waitFor('the show-closed task to finish its run', () => (runCount(findConversationByTitle(closedTitle)!) >= 1 ? true : undefined))
  await consoleApi.closeConversations([closedTask.metadata.name])

  // Item 22: four DEDICATED, never-reused conversations so the close-confirm
  // spec can actually send `/close` and `/exit` through the real composer —
  // every other task fixture above is read by a LATER spec file and must
  // stay open and untouched.
  console.log('[global-setup] opening the close-confirm fixtures (item 22)')
  const closeConfirmCancelTitle = `e2e-ui-closeconfirm-cancel-${stamp}`
  await manager.postTask(SOURCE_TASKS, `e2e-ui-closeconfirm-cancel-${stamp}`, closeConfirmCancelTitle, `echo closeconfirmcancel-${stamp}`)
  const closeConfirmCancel = await waitFor('the close-confirm cancel conversation', () => findConversationByTitle(closeConfirmCancelTitle))

  const closeConfirmSkipATitle = `e2e-ui-closeconfirm-skipa-${stamp}`
  await manager.postTask(SOURCE_TASKS, `e2e-ui-closeconfirm-skipa-${stamp}`, closeConfirmSkipATitle, `echo closeconfirmskipa-${stamp}`)
  const closeConfirmSkipA = await waitFor('the close-confirm skip(A) conversation', () => findConversationByTitle(closeConfirmSkipATitle))

  const closeConfirmSkipBTitle = `e2e-ui-closeconfirm-skipb-${stamp}`
  await manager.postTask(SOURCE_TASKS, `e2e-ui-closeconfirm-skipb-${stamp}`, closeConfirmSkipBTitle, `echo closeconfirmskipb-${stamp}`)
  const closeConfirmSkipB = await waitFor('the close-confirm skip(B) conversation', () => findConversationByTitle(closeConfirmSkipBTitle))

  const closeConfirmExitTitle = `e2e-ui-closeconfirm-exit-${stamp}`
  await manager.postTask(SOURCE_TASKS, `e2e-ui-closeconfirm-exit-${stamp}`, closeConfirmExitTitle, `echo closeconfirmexit-${stamp}`)
  const closeConfirmExit = await waitFor('the close-confirm exit conversation', () => findConversationByTitle(closeConfirmExitTitle))

  // ---- Session cookie, shared by every spec via `use.storageState` ----------

  console.log('[global-setup] signing in to mint the session cookie every spec reuses')
  const authDir = path.join(here, '.auth')
  mkdirSync(authDir, { recursive: true })
  const storageStatePath = path.join(authDir, 'state.json')
  const apiContext = await request.newContext({ baseURL: consoleUrl })
  const loginRes = await apiContext.post('/api/login', { data: { token: uiToken } })
  if (!loginRes.ok()) {
    throw new Error(`console login failed: ${loginRes.status()} ${await loginRes.text()}`)
  }
  await apiContext.storageState({ path: storageStatePath })
  await apiContext.dispose()

  // ---- Fixture manifest, read by every spec ----------------------------------

  const generatedDir = path.join(here, '.generated')
  mkdirSync(generatedDir, { recursive: true })
  const fixtures: Fixtures = {
    stamp,
    adapterToken,
    uiToken,
    storageStatePath,
    managerUrl: `http://127.0.0.1:${managerLocalPort}`,
    coordinators: { open: COORD_OPEN, closed: COORD_CLOSED, deleted: COORD_DELETED, unjoined: COORD_UNJOINED },
    conversations: {
      coordOpenRoot: coordOpenRoot.metadata.name,
      coordOpenMembers: [member1, member2],
      coordClosedRoot: coordClosedRoot.metadata.name,
      coordDeletedRoot: coordDeletedRoot.metadata.name,
      coordUnjoinedRoot: coordUnjoinedRoot.metadata.name,
      prefixTask: { name: prefixTask.metadata.name, title: prefixTitle, pipeline: 'e2e-console' },
      colonTask: { name: colonTask.metadata.name, title: colonTitle, pipeline: 'e2e-console' },
      observedChat: { name: observedChat.metadata.name, title: observedTitle },
      markTask: markTask.metadata.name,
      runsTask: runsTask.metadata.name,
      closedTask: closedTask.metadata.name,
      closeConfirmCancel: closeConfirmCancel.metadata.name,
      closeConfirmSkipA: closeConfirmSkipA.metadata.name,
      closeConfirmSkipB: closeConfirmSkipB.metadata.name,
      closeConfirmExit: closeConfirmExit.metadata.name,
    },
  }
  writeFileSync(path.join(generatedDir, 'fixtures.json'), JSON.stringify(fixtures, null, 2))
  console.log('[global-setup] fixtures ready:', JSON.stringify(fixtures.conversations, null, 2))

  return () => {
    stopManagerForward()
    return Promise.resolve()
  }
}
