import {
  Alert, Button, DescriptionList, DescriptionListDescription, DescriptionListGroup,
  DescriptionListTerm, Modal, ModalBody, ModalFooter, ModalHeader,
} from '@patternfly/react-core'
import { PlainText } from '../components/Text'
import { notDeleted } from '../../../model/bulk'
import type { DeleteResponse } from '../../../api/types'

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

// The selection rules are the model's (model/bulk.ts), shared by every look;
// re-exported so this look's callers keep one import.
export { deletableNames, notDeleted } from '../../../model/bulk'

export function DeleteSelectedModal({
  isOpen,
  names,
  result,
  error,
  busy,
  onConfirm,
  onClose,
  memberCount = 0,
}: {
  isOpen: boolean
  names: string[]
  result?: DeleteResponse
  error?: string
  busy: boolean
  onConfirm: () => void
  onClose: () => void
  /** Already-closed descendants the manager's cascade also deletes — console-conversation-tree. */
  memberCount?: number
}) {
  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      variant="medium"
      aria-label="delete conversations"
      data-testid="delete-modal"
    >
      <ModalHeader
        title={result ? 'Delete finished' : `Delete ${names.length + memberCount} conversation(s)?`}
      />
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
              {names.length + memberCount} closed conversation(s) will be deleted. This removes{' '}
              <b>the recorded answers</b> — the only durable copy of what the agent said — and{' '}
              <b>the workspace on disk</b>. <b>This cannot be undone.</b>
            </p>
            {memberCount > 0 && (
              <p data-testid="delete-member-count">
                {names.length} selected, reaching {memberCount} already-closed member
                conversation(s) the manager's cascade deletes too.
              </p>
            )}
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
