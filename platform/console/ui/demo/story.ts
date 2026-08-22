// The story the landing page's recording tells.
//
// ONE piece of work, from the signal that starts it to the answer a person
// replies to.
//
// THE BEAT LABELS NAME THE PRODUCT, NOT THE LANE. What the frames happen to
// show is a Kubernetes event, because a story needs one concrete thing — but a
// label reading "a cluster event is admitted" taught a first-time reader that
// this is a Kubernetes-events tool, which is the one thing the landing page
// must not say. The signal kind is an example inside the sentence, never its
// subject. It is the same made-up install the screenshots are taken of —
// `screenshots/fixture.ts`, cloned and patched beat by beat — so the recording
// on the landing page and the screenshots on the console page can never drift
// into two different products.
//
// A beat says WHAT IS TRUE from here on. It does not say what the console
// should draw: the recorder changes what the server answers, pushes one
// `resync`, and the console repaints itself through its own data path. That is
// the whole point — a recording of the console working, rather than of a
// recorder painting.

import { NOW, install, type Install } from '../screenshots/fixture'

/** The conversation the story is about. */
export const CONVERSATION = 'cluster-events-7c1d4e'

/** What the person types back. */
export const REPLY = 'Does the same setting affect the other two services in that namespace?'

/** A moment, as seconds past the pinned clock. */
const at = (seconds: number) => new Date(NOW.getTime() + seconds * 1000).toISOString()

type Item = Install['conversations']['items'][number]

/** The conversation as the fixture has it, before the story takes it apart. */
const arrival = (): Item =>
  structuredClone(install.conversations.items.find((c) => c.name === CONVERSATION)!)

/** The agent's answer and the signal that woke it, from the same fixture. */
const message = (index: number) => structuredClone(install.conversationDetail.transcript![index])
const event = (index: number) => structuredClone(install.conversationDetail.events![index])

export interface Beat {
  /** What is happening. The PAGE says this in words — nothing is drawn on the frames. */
  label: string
  /** Where the console is while it holds. Omitted keeps the previous view. */
  path?: string
  /** Text the console must be showing before the frame is taken. */
  ready?: string
  /** Seconds past the pinned clock, so relative ages move as the story does. */
  clock: number
  /** Seconds the recording rests here. */
  hold: number
  /** A person doing something, rather than something arriving. */
  act?: 'reply' | 'yaml'
  /** This beat's last frame stands in for the recording before it is played. */
  poster?: true
  /** What the server answers from here on. */
  patch?: (state: Install) => void
}

/**
 * The install BEFORE the story starts: healthy, busy with other things, and
 * with no sign of the conversation the recording is about.
 */
export function opening(): Install {
  const state = structuredClone(install)

  state.conversations.items = state.conversations.items.filter((c) => c.name !== CONVERSATION)
  state.conversations.total -= 1
  state.conversations.unreadTotal -= 1
  state.overview.counts.conversations -= 1
  if (state.overview.manager) state.overview.manager.runtimeSlots.inUse -= 1
  const kind = state.kinds.find((k) => k.kind === 'conversations')
  if (kind) kind.count -= 1

  // The detail is what the thread will look like when it opens — the signal
  // alone. Everything after it arrives during the story.
  state.conversationDetail.transcript = []
  state.conversationDetail.events = []

  return state
}

export const beats: Beat[] = [
  {
    label: 'A healthy install, with nothing waiting',
    path: '/overview',
    ready: 'Installation',
    clock: 0,
    hold: 5,
  },
  {
    label: 'A signal arrives — alert, schedule, message, event — and a conversation opens',
    path: '/conversations',
    ready: 'checkout-api is restarting',
    clock: 20,
    hold: 5,
    patch(state) {
      const item = arrival()
      item.phase = 'Working'
      item.runCount = 0
      item.unread = true
      item.created = at(16)
      item.lastActivity = at(18)
      item.ageSeconds = 4
      item.inflight = { runId: 'run-1', dispatchedAt: at(18) }

      state.conversations.items.unshift(item)
      state.conversations.total += 1
      state.conversations.unreadTotal += 1
      state.overview.counts.conversations += 1
      if (state.overview.manager) state.overview.manager.runtimeSlots.inUse += 1
      const kind = state.kinds.find((k) => k.kind === 'conversations')
      if (kind) kind.count += 1

      state.conversationDetail.conversation = item
      const signal = message(0)
      signal.at = at(16)
      state.conversationDetail.transcript = [signal]
      const admitted = event(0)
      admitted.ts = at(16)
      const dispatched = event(1)
      dispatched.ts = at(18)
      state.conversationDetail.events = [admitted, dispatched]
    },
  },
  {
    label: 'The thread it opened, and the agent already working',
    path: `/conversations/${CONVERSATION}`,
    ready: 'Signal from cluster-events',
    clock: 26,
    hold: 6,
  },
  {
    label: 'The answer — what is wrong, what changed, and what it did not touch',
    poster: true,
    ready: 'OOM-killed',
    clock: 62,
    hold: 9,
    patch(state) {
      const answer = message(1)
      answer.at = at(58)
      state.conversationDetail.transcript?.push(answer)
      const answered = event(2)
      answered.ts = at(58)
      state.conversationDetail.events?.push(answered)

      const item = state.conversationDetail.conversation
      item.phase = 'Idle'
      item.runCount = 1
      item.lastActivity = at(58)
      delete item.inflight
      state.conversations.items[0] = item
    },
  },
  {
    label: 'You reply in the thread, and the conversation carries on',
    act: 'reply',
    clock: 80,
    hold: 7,
    patch(state) {
      state.conversationDetail.transcript?.push({
        id: 'm-reply',
        thread: `console/${CONVERSATION}`,
        kind: 'user',
        sender: install.session.identity,
        text: REPLY,
        at: at(78),
      })
      const inbound = event(3)
      inbound.ts = at(78)
      inbound.from = { kind: 'channel', name: 'console' }
      state.conversationDetail.events?.push(inbound)

      const item = state.conversationDetail.conversation
      item.phase = 'Working'
      item.lastActivity = at(78)
      item.inflight = { runId: 'run-2', dispatchedAt: at(79) }
      state.conversations.items[0] = item
    },
  },
  {
    label: 'What is waiting, and what is stuck — every stalled row names its cause',
    path: '/queues',
    ready: 'Queues and capacity',
    clock: 100,
    hold: 6,
  },
  {
    label: 'The wiring behind it: where the signal entered, who claimed it, where the answer went',
    path: '/topology',
    ready: 'k8s-observe',
    clock: 108,
    hold: 7,
  },
  {
    // NO COUNT. The chart ships eleven CRDs and this view lists the ten a
    // person configures, so a number here would contradict the stat tile on
    // the page it appears beside.
    label: 'Every part of it is a Kubernetes resource — one API, one RBAC model',
    path: '/config',
    ready: 'Pipelines',
    clock: 116,
    hold: 6,
  },
  {
    // The claim, made concrete. A grid of counts says "there are resources";
    // the object says "this incident IS one, and kubectl already knows it".
    label: 'The conversation itself is an object — kubectl, GitOps and RBAC already work',
    path: `/conversations/${CONVERSATION}`,
    act: 'yaml',
    ready: 'runtimeContextId',
    clock: 124,
    hold: 7,
  },
]
