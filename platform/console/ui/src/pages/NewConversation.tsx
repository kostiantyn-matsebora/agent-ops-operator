import { useEffect, useRef, useState } from 'react'
import {
  Alert, Button, ClipboardCopy, Form, FormGroup, Label, Modal, ModalBody, ModalFooter,
  ModalHeader, Popover, TextArea, TextInput,
} from '@patternfly/react-core'
import { useSession, useSources, useVocabulary } from '../api/hooks'
import { api, ApiError } from '../api/client'
import { PlainText } from '../components/Text'
import { ComposerHint } from '../components/ComposerHint'
import { Icon } from '../components/Icon'
import { useComposerIntent } from './chat/composerIntent'
import type { VocabularyEntry } from '../api/types'

// "New conversation".
//
// The picker lists every Ready pipeline and coordinator as its own card,
// visible the moment the modal opens — discovering what can be addressed
// costs no typing. A filter box BELOW the card list narrows it by a plain
// name; it is a substring filter, never a slash-addressed command parser,
// because this modal starts conversations rather than acting on one.
//
// Selecting a card never sends anything by itself. It only prepares the
// ADDRESSED form (`/<name> `) that Start prepends to the task — the exact
// text a person used to have to type by hand. Leaving nothing selected keeps
// the old unaddressed behaviour: the task is posted bare, and the server
// resolves it when exactly one Ready Pipeline serves the chosen source
// (`wiring.md`) and refuses it otherwise.
//
// The "Answered by" section below is a DIFFERENT concept — which
// SignalSource this console originates from, never which Pipeline answers. A
// source is shareable, so the same source can still need an explicit
// destination even with several Pipelines willing to serve it.
//
// It renders in four states rather than disappearing, because "there is no
// button" and "the button is unavailable, here is why" are different messages
// and only one of them is actionable:
//
//   wired          → the button
//   claimable      → disabled, with the exact patch that claims the source
//   not originating→ disabled, with the CR that grants the signal identity
//   no identity    → disabled: authentication happens in front of this console
//                    and the proxy forwarded nobody to record the start against

// ADDRESS_PREFIX is the one place the addressed form is spelled in the UI.
const ADDRESS_PREFIX = '/'

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

/** A destination card's icon: the Pipeline's own, or a shape naming its KIND. */
function DestinationIcon({ entry }: Readonly<{ entry: VocabularyEntry }>) {
  const isCoordinator = entry.kind === 'coordinator'
  const fallbackGlyph = isCoordinator ? '◆' : '⚙'
  return (
    <span
      aria-hidden
      style={{
        width: '1.75rem',
        height: '1.75rem',
        borderRadius: '50%',
        flex: 'none',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        fontSize: '0.8rem',
        background: isCoordinator ? 'var(--ao-accent-soft)' : 'var(--ao-surface-alt)',
        border: `1px solid ${isCoordinator ? 'var(--ao-accent)' : 'var(--ao-border)'}`,
        color: isCoordinator ? 'var(--ao-accent)' : 'var(--ao-brand-strong)',
      }}
    >
      {entry.icon ? <Icon icon={entry.icon} /> : fallbackGlyph}
    </span>
  )
}

/** The `pipeline` / `coordinator` tag, styled distinctly for a coordinator. */
function DestinationKindTag({ kind }: Readonly<{ kind: VocabularyEntry['kind'] }>) {
  const isCoordinator = kind === 'coordinator'
  return (
    <span
      style={{
        flex: 'none',
        fontSize: '0.65rem',
        padding: '2px 7px',
        borderRadius: 9,
        border: `1px solid ${isCoordinator ? 'var(--ao-accent)' : 'var(--ao-border)'}`,
        color: isCoordinator ? 'var(--ao-accent)' : 'var(--ao-text-subtle)',
      }}
    >
      {kind}
    </span>
  )
}

export function NewConversation({ onStarted }: { onStarted?: () => void }) {
  const session = useSession()
  const sources = useSources()
  const vocabulary = useVocabulary()
  const [open, setOpen] = useState(false)
  const [task, setTask] = useState('')
  // The chosen Pipeline or Coordinator's name, or none — leaving it unset
  // keeps the task unaddressed, which the server resolves on its own when
  // exactly one Ready Pipeline serves the source.
  const [selected, setSelected] = useState<string | null>(null)
  const [filter, setFilter] = useState('')
  const [error, setError] = useState<string>()
  const [busy, setBusy] = useState(false)
  const [started, setStarted] = useState<string>()
  const cardsRef = useRef<HTMLDivElement>(null)

  // A quick-start chip (design D-H) lives beside an open thread, not beside
  // this button, so it reaches this one composer through the shared intent
  // rather than mounting a second modal just to open it from there.
  //
  // The chip still hands over the OLD addressed shape (`/<name> <rest>`) —
  // it is the one remaining producer of that shape — so it is split back into
  // a destination and a task here rather than teaching the chip a second
  // contract.
  const requestedTask = useComposerIntent((s) => s.requestedTask)
  useEffect(() => {
    if (requestedTask === null) return
    const m = /^\/(\S+)/.exec(requestedTask)
    if (m) {
      setSelected(m[1])
      setTask(requestedTask.slice(m[0].length).trimStart())
    } else {
      setTask(requestedTask)
    }
    setOpen(true)
    useComposerIntent.getState().clear()
  }, [requestedTask])

  // ONLY WHAT THIS SURFACE CAN ACTUALLY DO. `kind: 'builtin'` entries are the
  // listing commands, whose whole result is a reply posted to a channel's
  // GENERAL surface — and this console has no general-surface view to put one
  // in. This card list already IS that listing, rendered directly instead of
  // typed.
  const startable = (vocabulary.data?.entries ?? []).filter(
    (e) => e.kind === 'pipeline' || e.kind === 'coordinator',
  )
  const q = filter.trim().toLowerCase()
  const filtered = q ? startable.filter((e) => e.name.toLowerCase().includes(q)) : startable

  // Arrow keys move the roving selection across the VISIBLE (filtered) list,
  // matching the typeahead menu's keyboard behaviour this replaces — a card
  // list a pointer can reach but a keyboard cannot is broken the same way a
  // missing button is. Indexed rather than name-selector-based, so it needs
  // nothing beyond what the list already is.
  function onCardKeyDown(e: React.KeyboardEvent<HTMLButtonElement>) {
    if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp') return
    e.preventDefault()
    if (filtered.length === 0) return
    const current = Math.max(0, filtered.findIndex((x) => x.name === selected))
    const next =
      e.key === 'ArrowDown'
        ? (current + 1) % filtered.length
        : (current - 1 + filtered.length) % filtered.length
    setSelected(filtered[next].name)
    requestAnimationFrame(() => {
      const buttons = cardsRef.current?.querySelectorAll<HTMLButtonElement>('button[role="radio"]')
      buttons?.[next]?.focus()
    })
  }

  function onTaskKeyDown(e: React.KeyboardEvent<HTMLTextAreaElement>) {
    // SHIFT+ENTER SENDS — the one keystroke that means "I am done".
    if (e.key === 'Enter' && e.shiftKey) {
      e.preventDefault()
      if (!busy && task.trim()) void start()
    }
  }

  const writeEnabled = session.data?.writeEnabled ?? false
  const canWrite = session.data?.canWrite ?? false
  const canOriginate = sources.data?.canOriginate ?? false
  const all = sources.data?.sources ?? []
  const wired = all.filter((s) => s.wired)
  const unwired = all.filter((s) => !s.wired)
  // The server fills `profile` only when ONE Pipeline serves the source; blank
  // with a named pipeline list therefore means several do, which is exactly
  // when an unaddressed task gets refused.
  const ambiguous = wired.some((s) => !s.profile && !!s.pipeline)

  if (!writeEnabled) {
    return (
      <Popover
        headerContent="This console is read-only"
        bodyContent="console.write.enabled is false, so both write paths are refused server-side."
      >
        <Button variant="secondary" isAriaDisabled>
          New conversation
        </Button>
      </Popover>
    )
  }

  // Writes are enabled and there is nobody to attribute them to: an install
  // with authentication in front that forwards no identity. Disabled WITH the
  // reason, like every other unavailable state here — the alternative is a
  // button that opens a modal and then fails on submit.
  if (!canWrite) {
    return (
      <Popover
        headerContent="Nobody said who you are"
        bodyContent={
          <>
            {session.data?.externalAuthenticator || 'The proxy'} in front of this console
            authenticated you but forwarded no identity header, and a conversation is recorded
            against the person who started it. Configure it to forward X-Forwarded-Email or
            X-Auth-Request-User.
          </>
        }
      >
        <Button variant="secondary" isAriaDisabled>
          New conversation
        </Button>
      </Popover>
    )
  }

  if (!canOriginate) {
    return (
      <Popover
        headerContent="This console holds no signal identity"
        bodyContent={
          <>
            It can carry and reply to conversations, but not start them. Declare a SignalAdapter
            served by this ChannelAdapter, plus a SignalSource.
          </>
        }
      >
        <Button variant="secondary" isAriaDisabled>
          New conversation
        </Button>
      </Popover>
    )
  }

  if (wired.length === 0) {
    return (
      <Popover
        headerContent="Nothing is wired to answer yet"
        bodyContent={
          <>
            {unwired.map((s) => (
              <div key={s.name}>
                <p>
                  <PlainText>{s.message || `no Ready Pipeline claims ${s.name}`}</PlainText>
                </p>
                {s.patch && <ClipboardCopy isReadOnly isCode>{s.patch}</ClipboardCopy>}
              </div>
            ))}
          </>
        }
      >
        <Button variant="secondary" isAriaDisabled data-testid="new-conversation-unavailable">
          New conversation
        </Button>
      </Popover>
    )
  }

  async function start() {
    setBusy(true)
    setError(undefined)
    try {
      const fullTask = selected ? `${ADDRESS_PREFIX}${selected} ${task}` : task
      await api.start(fullTask, wired[0]?.name)
      setOpen(false)
      setTask('')
      setSelected(null)
      setFilter('')
      setStarted(selected ?? wired[0]?.pipeline)
      onStarted?.()
    } catch (e) {
      // The server's reason is the useful one — it carries the Wired=False text
      // and the fix.
      const err = e as ApiError
      setError(
        [err.message, err.body.message as string, err.body.fix as string].filter(Boolean).join(' — '),
      )
    } finally {
      setBusy(false)
    }
  }

  return (
    <>
      <Button variant="primary" onClick={() => setOpen(true)} data-testid="new-conversation">
        New conversation
      </Button>
      {started && (
        <Alert
          variant="success"
          isInline
          isPlain
          title={`Started — ${started} is answering. It appears in the list once created.`}
        />
      )}
      <Modal isOpen={open} onClose={() => setOpen(false)} variant="medium">
        <ModalHeader title="Start a conversation" />
        {/* NOTHING SCROLLS SIDEWAYS. A dialog is as wide as the window gives
            it, so anything inside must wrap rather than widen — a long name, a
            long profile, a long line of prose. minWidth 0 lets flex children
            shrink below their content, which is what makes wrapping possible
            at all. */}
        <ModalBody style={{ minWidth: 0, overflowX: 'hidden', overflowWrap: 'anywhere' }}>
          <Form style={{ minWidth: 0 }}>
            <FormGroup label="Answered by" fieldId="answered-by">
              {wired.map((s) => (
                <div key={s.name}>
                  <Label color="blue" isCompact>
                    {s.name}
                  </Label>{' '}
                  → pipeline <PlainText>{s.pipeline}</PlainText>
                  {s.profile && (
                    <>
                      , profile <PlainText>{s.profile}</PlainText>
                    </>
                  )}
                </div>
              ))}
              <small>
                {/* A source is shareable, so "who answers" has one answer only
                    while one Pipeline serves it. With several, an unaddressed
                    task is refused rather than sent to an arbitrary one — so
                    say that HERE, before it is typed, not after it bounces. */}
                {ambiguous
                  ? 'Several Pipelines serve this source — pick a destination below. An unaddressed task is refused rather than guessed.'
                  : 'The Pipeline serving this source answers — the console cannot reach a pipeline no wiring points at.'}
              </small>
            </FormGroup>
            <FormGroup label="Destination" fieldId="new-conversation-destination">
              {vocabulary.isLoading && <small>Loading what can be addressed…</small>}
              {/* An empty state is a state: no Ready pipeline or coordinator
                  is directly addressable, so say so rather than showing an
                  empty box — the task below still goes out unaddressed. */}
              {!vocabulary.isLoading && startable.length === 0 && (
                <small data-testid="destination-empty">
                  Nothing is directly addressable yet — the task below is sent unaddressed.
                </small>
              )}
              {!vocabulary.isLoading && startable.length > 0 && (
                <>
                  <div
                    ref={cardsRef}
                    role="radiogroup"
                    aria-label="Pick a destination"
                    data-testid="destination-cards"
                    style={{
                      display: 'flex',
                      flexDirection: 'column',
                      gap: 6,
                      maxHeight: '220px',
                      overflowY: 'auto',
                    }}
                  >
                    {filtered.map((entry, i) => {
                      const isSelected = selected === entry.name
                      const isRovingTarget = selected ? isSelected : i === 0
                      return (
                        <button
                          key={entry.name}
                          type="button"
                          role="radio"
                          aria-checked={isSelected}
                          tabIndex={isRovingTarget ? 0 : -1}
                          data-testid={`destination-${entry.name}`}
                          data-destination={entry.name}
                          onClick={() => setSelected(entry.name)}
                          onKeyDown={onCardKeyDown}
                          style={{
                            display: 'flex',
                            alignItems: 'center',
                            gap: 10,
                            padding: '9px 10px',
                            border: `1px solid ${isSelected ? 'var(--ao-brand)' : 'var(--ao-border)'}`,
                            borderRadius: 8,
                            background: isSelected ? 'var(--ao-brand-soft)' : 'var(--ao-surface)',
                            cursor: 'pointer',
                            textAlign: 'left',
                            width: '100%',
                            minWidth: 0,
                            font: 'inherit',
                            color: 'inherit',
                          }}
                        >
                          <DestinationIcon entry={entry} />
                          <span style={{ minWidth: 0, flex: 1 }}>
                            <div style={{ fontSize: '0.85rem', fontWeight: 700 }}>
                              <PlainText>{entry.name}</PlainText>
                            </div>
                            {(entry.description || entry.profile) && (
                              <div
                                style={{
                                  fontSize: '0.72rem',
                                  color: 'var(--ao-text-subtle)',
                                  marginTop: 1,
                                  overflow: 'hidden',
                                  textOverflow: 'ellipsis',
                                  whiteSpace: 'nowrap',
                                }}
                              >
                                <PlainText>{entry.description || entry.profile}</PlainText>
                              </div>
                            )}
                          </span>
                          <DestinationKindTag kind={entry.kind} />
                        </button>
                      )
                    })}
                    {filtered.length === 0 && (
                      <small data-testid="destination-no-matches">No match for “{filter}”.</small>
                    )}
                  </div>
                  <TextInput
                    type="text"
                    aria-label="Filter destinations by name"
                    placeholder="Filter by name…"
                    value={filter}
                    onChange={(_e, v) => setFilter(v)}
                    style={{ marginTop: '0.5rem' }}
                    data-testid="destination-filter"
                  />
                </>
              )}
            </FormGroup>
            <FormGroup label="Task" fieldId="task" isRequired>
              <TextArea
                id="task"
                value={task}
                onChange={(_e, v) => setTask(v)}
                onKeyDown={onTaskKeyDown}
                rows={6}
                aria-label="task"
                placeholder="What should the agent do?"
              />
              <div style={{ marginTop: '0.75rem' }}>
                <ComposerHint shortcuts={[{ keys: ['Shift', 'Enter'], does: 'send' }]} />
              </div>
            </FormGroup>
            {error && (
              <div style={{ marginTop: '0.75rem' }}>
                <Alert variant="danger" isInline title={error} />
              </div>
            )}
          </Form>
        </ModalBody>
        <ModalFooter>
          <Button
            onClick={start}
            isDisabled={busy || !task.trim()}
            isLoading={busy}
            data-testid="start-conversation"
          >
            {selected ? `Start with ${selected}` : 'Start'}
          </Button>
          <Button variant="link" onClick={() => setOpen(false)}>
            Cancel
          </Button>
        </ModalFooter>
      </Modal>
    </>
  )
}
