import { useState } from 'react'
import {
  Alert, Button, ClipboardCopy, Form, FormGroup, Label, Modal, ModalBody,
  ModalFooter, ModalHeader, Popover, TextArea,
} from '@patternfly/react-core'
import { useSession, useSources } from '../api/hooks'
import { api, ApiError } from '../api/client'
import { PlainText } from '../components/Text'

// "New conversation".
//
// The picker lists console SignalSources that are Wired=True, each labelled by
// its claiming pipeline and that pipeline's profile — a rendering of the
// topology, not a free-text pipeline field that could name something no wiring
// supports. Who answers is DECLARED by the claim, which is the origination
// invariant's actual point.
//
// It renders in three states rather than disappearing, because "there is no
// button" and "the button is unavailable, here is why" are different messages
// and only one of them is actionable:
//
//   wired          → the button
//   claimable      → disabled, with the exact patch that claims the source
//   not originating→ disabled, with the CR that grants the signal identity

export function NewConversation({ onStarted }: { onStarted?: () => void }) {
  const session = useSession()
  const sources = useSources()
  const [open, setOpen] = useState(false)
  const [task, setTask] = useState('')
  const [error, setError] = useState<string>()
  const [busy, setBusy] = useState(false)
  const [started, setStarted] = useState<string>()

  const writeEnabled = session.data?.writeEnabled ?? false
  const canOriginate = sources.data?.canOriginate ?? false
  const all = sources.data?.sources ?? []
  const wired = all.filter((s) => s.wired)
  const unwired = all.filter((s) => !s.wired)

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
                  → pipeline <PlainText>{s.pipeline}</PlainText>, profile{' '}
                  <PlainText>{s.profile}</PlainText>
                </div>
              ))}
              <small>
                The Pipeline claiming this source decides who answers — the console cannot reach an
                agent no wiring points at.
              </small>
            </FormGroup>
            <FormGroup label="Task" fieldId="task" isRequired>
              <TextArea
                id="task"
                value={task}
                onChange={(_e, v) => setTask(v)}
                rows={6}
                aria-label="task"
                placeholder="What should the agent do?"
              />
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
