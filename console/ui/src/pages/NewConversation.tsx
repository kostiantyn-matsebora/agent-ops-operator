import { useRef, useState } from 'react'
import {
  Alert, Button, ClipboardCopy, Form, FormGroup, Label, Menu, MenuContent, MenuItem,
  MenuList, Modal, ModalBody, ModalFooter, ModalHeader, Popover, TextArea,
} from '@patternfly/react-core'
import { useAgents, useSession, useSources } from '../api/hooks'
import { api, ApiError } from '../api/client'
import { PlainText } from '../components/Text'
import type { Agent } from '../api/types'

// "New conversation".
//
// The picker lists console SignalSources that are Wired=True, each labelled by
// the pipelines serving it and — when exactly one does — that pipeline's
// profile. It is a rendering of the topology, not a free-text pipeline field
// that could name something no wiring supports.
//
// A source is SHAREABLE, so "who answers" is single-valued only while one
// Pipeline serves it. With several, an unaddressed task is REFUSED rather than
// handed to an arbitrary one, and the way to reach a specific agent is to
// address it. That is what the typeahead is for: typing the prefix lists the
// Ready pipelines and inserts the addressed form, so a person chooses from what
// exists instead of recalling a name. It is Ready-filtered for the same reason
// `/agents` is — offering an unready pipeline invites a request nothing can
// serve — and the two must never disagree.
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
 * matchAgents answers what the typeahead should show for the current text, and
 * `null` for "show nothing".
 *
 * The listing opens only on a prefix at the very START of the message, because
 * that is the only position that addresses anyone — a slash mid-sentence is a
 * path or a date, and popping a menu over it would fight the person typing.
 *
 * An empty result is `null` rather than an empty list: a popup saying nothing
 * is worse than no popup, and a surface with no Ready pipelines has nothing to
 * offer in the first place.
 */
export function matchAgents(text: string, agents: Agent[]): Agent[] | null {
  if (!text.startsWith(ADDRESS_PREFIX)) return null
  const typed = text.slice(ADDRESS_PREFIX.length)
  // A space means the name is finished and the task has begun — the person is
  // past choosing, so the menu gets out of the way.
  if (/\s/.test(typed)) return null
  const q = typed.toLowerCase()
  const hits = agents.filter((a) => a.name.toLowerCase().startsWith(q))
  return hits.length > 0 ? hits : null
}

export function NewConversation({ onStarted }: { onStarted?: () => void }) {
  const session = useSession()
  const sources = useSources()
  const agents = useAgents()
  const [open, setOpen] = useState(false)
  const [task, setTask] = useState('')
  const [error, setError] = useState<string>()
  const [busy, setBusy] = useState(false)
  const [started, setStarted] = useState<string>()
  // Dismissed on Escape: the menu must be closable without sending and without
  // deleting what was typed.
  const [dismissed, setDismissed] = useState(false)
  const [cursor, setCursor] = useState(0)
  const taskRef = useRef<HTMLTextAreaElement>(null)

  const matches = dismissed ? null : matchAgents(task, agents.data?.agents ?? [])
  const active = matches ? Math.min(cursor, matches.length - 1) : 0

  // Selecting inserts the addressed form and leaves the cursor after the space,
  // ready for the task — the point is to save typing a name, not to make the
  // person reposition afterwards.
  function choose(agent: Agent) {
    const next = ADDRESS_PREFIX + agent.name + ' '
    setTask(next)
    setDismissed(true)
    setCursor(0)
    requestAnimationFrame(() => {
      taskRef.current?.focus()
      taskRef.current?.setSelectionRange(next.length, next.length)
    })
  }

  function onTaskKeyDown(e: React.KeyboardEvent<HTMLTextAreaElement>) {
    if (!matches) return
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      setCursor((c) => (c + 1) % matches.length)
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      setCursor((c) => (c - 1 + matches.length) % matches.length)
    } else if (e.key === 'Enter' || e.key === 'Tab') {
      // Enter picks the highlighted agent rather than submitting: the menu is
      // open BECAUSE the message so far is only a half-typed name, which is
      // never a task worth starting.
      e.preventDefault()
      choose(matches[active])
    } else if (e.key === 'Escape') {
      e.preventDefault()
      setDismissed(true)
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
      await api.start(task, wired[0]?.name)
      setOpen(false)
      setTask('')
      setStarted(wired[0]?.pipeline)
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
        <ModalBody>
          <Form>
            <FormGroup label="Answered by" fieldId="destination">
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
                  ? `Several Pipelines serve this source, so start the task with ${ADDRESS_PREFIX} and pick one — an unaddressed task is refused rather than guessed.`
                  : 'The Pipeline serving this source answers — the console cannot reach an agent no wiring points at.'}
              </small>
            </FormGroup>
            <FormGroup label="Task" fieldId="task" isRequired>
              <TextArea
                id="task"
                ref={taskRef}
                value={task}
                onChange={(_e, v) => {
                  setTask(v)
                  // Typing re-opens the menu: dismissal applies to the text the
                  // person escaped out of, not to the field forever.
                  setDismissed(false)
                  setCursor(0)
                }}
                onKeyDown={onTaskKeyDown}
                rows={6}
                aria-label="task"
                placeholder={`What should the agent do? Start with ${ADDRESS_PREFIX} to address one.`}
              />
              {matches && (
                <Menu aria-label="agents" data-testid="agent-typeahead" isScrollable>
                  <MenuContent>
                    <MenuList>
                      {matches.map((a, i) => (
                        <MenuItem
                          key={a.name}
                          isFocused={i === active}
                          onClick={() => choose(a)}
                          description={a.profile}
                        >
                          {ADDRESS_PREFIX}
                          {a.name}
                        </MenuItem>
                      ))}
                    </MenuList>
                  </MenuContent>
                </Menu>
              )}
            </FormGroup>
            {error && <Alert variant="danger" isInline title={error} />}
          </Form>
        </ModalBody>
        <ModalFooter>
          <Button onClick={start} isDisabled={busy || !task.trim()} isLoading={busy}>
            Start
          </Button>
          <Button variant="link" onClick={() => setOpen(false)}>
            Cancel
          </Button>
        </ModalFooter>
      </Modal>
    </>
  )
}
