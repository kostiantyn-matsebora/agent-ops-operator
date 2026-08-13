import {
  Alert, Button, DescriptionList, DescriptionListDescription, DescriptionListGroup,
  DescriptionListTerm, Modal, ModalBody, ModalFooter, ModalHeader,
} from '@patternfly/react-core'
import { PlainText } from '../components/Text'
import type { DeleteResponse, ConversationSummary } from '../api/types'

// Deleting a selected batch.
//
// This is the irreversible half of the two-stage lifecycle, and everything here
// exists to make that fact hard to miss:
//
//   - only CLOSED conversations are selectable, and a non-closed name sent
//     anyway is skipped server-side with "close it first" — never closed on the
//     way through, because one call doing the irreversible thing to a
//     conversation that was still working, behind a confirmation naming only
//     the delete, is exactly what refusing prevents;
//   - the confirmation names what goes: the recorded answers, which are the only
//     durable copy of what the agent said, AND the workspace on disk.
//
// The console still performs no Kubernetes write. Deleting is a manager verb
// this page CALLS, reached over the same authenticated path as everything else.

/**
 * The rows on the CURRENT PAGE that may be deleted: closed, and not already on
 * their way out.
 *
 * Deliberately narrower than the close page's rule. There, anything not already
 * closing is fair game; here, a conversation that has not been closed first is
 * not a candidate at all — the two-step is the safety property, not a nag.
 */
export function deletableNames(items: ConversationSummary[]): string[] {
  return items.filter((c) => !c.closing && c.phase === 'Closed').map((c) => c.name)
}

/** The conversations a batch did not delete, with the server's own reason. */
export function notDeleted(result: DeleteResponse): DeleteResponse['results'] {
  return result.results.filter((r) => r.outcome !== 'deleted')
}

export function DeleteSelectedModal({
  isOpen,
  names,
  result,
  error,
  busy,
  onConfirm,
  onClose,
}: {
  isOpen: boolean
  names: string[]
  result?: DeleteResponse
  error?: string
  busy: boolean
  onConfirm: () => void
  onClose: () => void
}) {
  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      variant="medium"
      aria-label="delete conversations"
      data-testid="delete-modal"
    >
      <ModalHeader title={result ? 'Delete finished' : `Delete ${names.length} conversation(s)?`} />
      <ModalBody>
        {result ? (
          <>
            <Alert
              variant={result.deleted > 0 && result.failed === 0 ? 'success' : 'warning'}
              isInline
              title={`${result.deleted} deleted, ${result.skipped} skipped, ${result.failed} failed`}
            />
            {notDeleted(result).length > 0 && (
              <DescriptionList isCompact data-testid="delete-reasons">
                {notDeleted(result).map((r) => (
                  <DescriptionListGroup key={r.name}>
                    <DescriptionListTerm>
                      <PlainText>{r.name}</PlainText>
                    </DescriptionListTerm>
                    <DescriptionListDescription>
                      {r.outcome} — <PlainText>{r.reason ?? ''}</PlainText>
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                ))}
              </DescriptionList>
            )}
          </>
        ) : (
          <>
            <p data-testid="delete-consequences">
              {names.length} closed conversation(s) will be deleted. This removes{' '}
              <b>the recorded answers</b> — the only durable copy of what the agent said — and{' '}
              <b>the workspace on disk</b>. <b>This cannot be undone.</b>
            </p>
            <p>
              To keep the record and just tidy the list, close them instead: a closed conversation
              costs no runtime pod and no capacity, and can be reopened.
            </p>
            {error && <Alert variant="danger" isInline title={error} />}
          </>
        )}
      </ModalBody>
      <ModalFooter>
        {result ? (
          <Button onClick={onClose}>Done</Button>
        ) : (
          <>
            <Button
              variant="danger"
              onClick={() => onConfirm()}
              isDisabled={busy || names.length === 0}
              isLoading={busy}
              data-testid="delete-confirm"
            >
              Delete {names.length}
            </Button>
            <Button variant="link" onClick={onClose}>
              Cancel
            </Button>
          </>
        )}
      </ModalFooter>
    </Modal>
  )
}
