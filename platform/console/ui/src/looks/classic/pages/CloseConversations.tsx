import { useState } from 'react'
import {
  Alert, Button, DescriptionList, DescriptionListDescription, DescriptionListGroup,
  DescriptionListTerm, Modal, ModalBody, ModalFooter, ModalHeader, Switch,
} from '@patternfly/react-core'
import { PlainText } from '../components/Text'
import { notClosed } from '../../../model/bulk'
import type { CloseResponse } from '../../../api/types'

// Closing a selected batch.
//
// The action is a FAN-OUT OF `/close` — the same command a person types in a
// thread, posted on each selected conversation's console thread. Closing is a
// STATE now, not a deletion: the conversation stays, inert but intact, and can
// be reopened. Deleting is a separate batch (DeleteConversations) that refuses
// anything not already closed.
//
// Two properties are load-bearing and both live in this file so they can be
// tested without the whole page:
//
//   - the selection can never escape the current page (`selectableNames` takes
//     the rows the server returned, which IS one page), so a mis-set filter
//     cannot close hundreds across pages;
//   - including working conversations is OFF by default and says what it costs
//     in its own label, because `/close` abandoning an inflight run is right for
//     a deliberate single close and wrong as the silent default of a batch.

// The selection rules are the model's (model/bulk.ts), shared by every look;
// re-exported so this look's callers keep one import.
export { notClosed, selectableNames, workingCount, workingMemberCount } from '../../../model/bulk'

export function CloseSelectedModal({
  isOpen,
  names,
  working,
  workingMembers = 0,
  result,
  error,
  busy,
  onConfirm,
  onClose,
  memberCount = 0,
}: {
  isOpen: boolean
  /** The selected ROOTS — a member is never in here (console-conversation-tree). */
  names: string[]
  working: number
  /** How many of the cascade's MEMBERS, not the roots above, are working. */
  workingMembers?: number
  result?: CloseResponse
  error?: string
  busy: boolean
  onConfirm: (includeWorking: boolean) => void
  onClose: () => void
  /** Live descendants the manager's cascade also closes — console-conversation-tree: "the confirmation SHALL state how many descendants close with it". */
  memberCount?: number
}) {
  // Default OFF, and re-defaulted every time the dialog opens: an opt-in that
  // remembers itself is not one.
  const [includeWorking, setIncludeWorking] = useState(false)

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      variant="medium"
      aria-label="close conversations"
      data-testid="close-modal"
    >
      <ModalHeader
        title={result ? 'Close finished' : `Close ${names.length} conversation(s)?`}
      />
      <ModalBody>
        {result ? (
          <>
            <Alert
              variant={result.closed > 0 && result.failed === 0 ? 'success' : 'warning'}
              isInline
              title={`${result.closed} closed, ${result.skipped} skipped, ${result.failed} failed`}
            />
            {/* Per conversation, never one verdict: a partial batch is the
                normal outcome and "12 of 15 closed" is the honest summary. */}
            {notClosed(result).length > 0 && (
              <DescriptionList isCompact data-testid="close-reasons">
                {notClosed(result).map((r) => (
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
            <p>
              {names.length} conversation(s) will be closed. Each is sent{' '}
              <code>/close</code> on its console thread: the agent says goodbye and the threads
              are archived. The conversation itself stays — its answers and its workspace are kept
              — and <b>it can be reopened</b>.
            </p>
            {memberCount > 0 && (
              <p data-testid="close-member-count">
                {names.length} selected, reaching {memberCount} member conversation(s) the
                manager's cascade closes too
                {workingMembers > 0 ? `, ${workingMembers} of them working` : ''}.
              </p>
            )}
            {working > 0 && (
              <>
                <p data-testid="close-working-count">
                  {working} of them {working === 1 ? 'is' : 'are'} working. They are left alone
                  unless you say otherwise.
                </p>
                <Switch
                  id="close-include-working"
                  label="include working — abandons in-progress runs"
                  isChecked={includeWorking}
                  onChange={(_e, v) => setIncludeWorking(v)}
                />
              </>
            )}
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
              onClick={() => onConfirm(includeWorking)}
              isDisabled={busy || names.length === 0}
              isLoading={busy}
              data-testid="close-confirm"
            >
              Close {names.length}
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
