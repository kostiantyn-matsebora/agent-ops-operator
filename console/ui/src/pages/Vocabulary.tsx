import { useState } from 'react'
import { Button, Label, Modal, ModalBody, ModalHeader } from '@patternfly/react-core'
import { Table, Tbody, Td, Th, Thead, Tr } from '@patternfly/react-table'
import { useVocabulary } from '../api/hooks'
import { PlainText } from '../components/Text'
import { Icon } from '../components/Icon'
import type { VocabularyEntry } from '../api/types'

/**
 * WHAT YOU CAN TYPE, on a page rather than in a reply.
 *
 * The manager answers `/pipelines` and `/help` with a message posted to a
 * channel's GENERAL surface — and this console has no general-surface view to
 * put one in, so asking it here produced an answer nobody ever saw.
 *
 * The console does not need to ask. It already holds the same vocabulary the
 * command would read, so it shows it directly: same list, same filter, no
 * round trip and nothing invisible.
 */

/** Rows for one position, sorted so the list does not reshuffle between opens. */
function rows(entries: VocabularyEntry[], kind: VocabularyEntry['kind'], position: VocabularyEntry['position']) {
  return entries
    .filter((e) => e.kind === kind && e.position === position)
    .sort((a, b) => a.name.localeCompare(b.name))
}

function EntryTable({ entries, caption }: { entries: VocabularyEntry[]; caption: string }) {
  if (entries.length === 0) return <p>{caption}</p>
  return (
    <Table variant="compact" aria-label={caption}>
      <Thead>
        <Tr>
          <Th>Type</Th>
          <Th>Does</Th>
        </Tr>
      </Thead>
      <Tbody>
        {entries.map((e) => (
          <Tr key={e.name}>
            <Td dataLabel="Type">
              <span style={{ display: 'inline-flex', alignItems: 'center', gap: '0.35rem' }}>
                <Icon icon={e.icon} />
                <code>/{e.name}</code>
              </span>
            </Td>
            <Td dataLabel="Does">
              <PlainText>{e.description ?? ''}</PlainText>
            </Td>
          </Tr>
        ))}
      </Tbody>
    </Table>
  )
}

/**
 * Pipelines — the agents a message can address, each with the profile
 * answering for it.
 */
export function PipelinesChip() {
  const [open, setOpen] = useState(false)
  const vocabulary = useVocabulary()
  const entries = vocabulary.data?.entries ?? []
  const pipelines = rows(entries, 'pipeline', 'general')

  return (
    <>
      <Button variant="secondary" isInline onClick={() => setOpen(true)} data-testid="pipelines-chip">
        Pipelines <Label isCompact>{pipelines.length}</Label>
      </Button>
      <Modal isOpen={open} onClose={() => setOpen(false)} variant="medium">
        <ModalHeader title="Pipelines you can address" />
        <ModalBody>
          <p>
            Start a task with <code>/</code> and the name. Only Ready pipelines are listed — an
            unready one names wiring that does not resolve.
          </p>
          <EntryTable entries={pipelines} caption="Nothing is wired to answer yet." />
        </ModalBody>
      </Modal>
    </>
  )
}

/** Help — every command, and where each one works. */
export function HelpChip() {
  const [open, setOpen] = useState(false)
  const vocabulary = useVocabulary()
  const entries = vocabulary.data?.entries ?? []

  return (
    <>
      <Button variant="secondary" isInline onClick={() => setOpen(true)} data-testid="help-chip">
        Help
      </Button>
      <Modal isOpen={open} onClose={() => setOpen(false)} variant="medium">
        <ModalHeader title="What you can type" />
        <ModalBody>
          <p>
            <b>Starting a conversation.</b> In the New conversation box, address a pipeline by
            name.
          </p>
          <EntryTable
            entries={rows(entries, 'pipeline', 'general')}
            caption="Nothing is wired to answer yet."
          />
          <p style={{ marginTop: '1rem' }}>
            <b>Inside a conversation.</b> These act on the conversation you are reading, and are
            offered by its reply box.
          </p>
          <EntryTable
            entries={rows(entries, 'builtin', 'thread')}
            caption="No thread commands are available."
          />
          <p style={{ marginTop: '1rem' }}>
            <code>Shift</code> + <code>Enter</code> sends, in both boxes.
          </p>
        </ModalBody>
      </Modal>
    </>
  )
}
