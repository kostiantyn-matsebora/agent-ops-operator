import { type ReactElement, useState } from 'react'
import type { CloseResponse, DeleteResponse } from '../../../api/types'
import { notClosed, notDeleted } from '../../../model/bulk'
import { Dialog } from './Dialog'

// THE TWO CONFIRMATIONS A BATCH RUNS BEHIND: close and delete. What each may
// take is the model's (bulk.ts, tree.ts); these only state the consequence,
// run it, and report what did not land with the server's own reason.

function Reasons({ testid, rows }: Readonly<{ testid: string; rows: { name: string; outcome: string; reason?: string }[] }>) {
  if (rows.length === 0) return null
  return (
    <dl className="wk-kv wk-reasons" data-testid={testid}>
      {rows.map((r) => (
        <div key={r.name} className="wk-reason">
          <dt>{r.name}</dt>
          <dd>
            {r.outcome} — {r.reason ?? ''}
          </dd>
        </div>
      ))}
    </dl>
  )
}

export function CloseBatchDialog({
  names, memberCount, working, workingMembers = 0, result, error, busy, onConfirm, onClose,
}: Readonly<{
  /** The selected ROOTS — a member is never in here (console-conversation-tree). */
  names: string[]
  /** Live descendants the manager's cascade also closes. */
  memberCount: number
  working: number
  /** How many of the cascade's MEMBERS, not the roots above, are working. */
  workingMembers?: number
  result?: CloseResponse
  error?: string
  busy: boolean
  onConfirm: (includeWorking: boolean) => void
  onClose: () => void
}>): ReactElement {
  // Default OFF and re-defaulted at every open: an opt-in that remembers
  // itself is not one.
  const [includeWorking, setIncludeWorking] = useState(false)
  if (result) {
    return (
      <Dialog
        title="Close finished"
        testid="close-modal"
        onClose={onClose}
        footer={<button type="button" className="wk-btn primary" onClick={onClose}>Done</button>}
      >
        <p className={`wk-alert${result.failed > 0 ? ' bad' : ''}`}>
          {`${result.closed} closed, ${result.skipped} skipped, ${result.failed} failed`}
        </p>
        <Reasons testid="close-reasons" rows={notClosed(result)} />
      </Dialog>
    )
  }
  return (
    <Dialog
      title={`Close ${names.length} conversation(s)?`}
      label="Close conversations"
      testid="close-modal"
      onClose={onClose}
      footer={
        <>
          <button
            type="button"
            className="wk-btn danger"
            disabled={busy || names.length === 0}
            data-testid="close-confirm"
            onClick={() => onConfirm(includeWorking)}
          >
            {`Close ${names.length}`}
          </button>
          <button type="button" className="wk-btn link" onClick={onClose}>Cancel</button>
        </>
      }
    >
      <p>
        Each agent says goodbye and its thread is archived. The conversations stay: their answers and workspaces are
        kept, and they can be reopened.
      </p>
      {memberCount > 0 && (
        <p data-testid="close-member-count">
          {`${names.length} selected, reaching ${memberCount} live member conversation(s) the manager's cascade closes too`}
          {workingMembers > 0 ? `, ${workingMembers} of them working` : ''}.
        </p>
      )}
      {working > 0 && (
        <label className="wk-check" data-testid="close-working">
          <input type="checkbox" checked={includeWorking} onChange={(e) => setIncludeWorking(e.target.checked)} />
          {`Also close ${working} working now, abandoning the run in flight`}
        </label>
      )}
      {error && <p className="wk-alert bad">{error}</p>}
    </Dialog>
  )
}

export function DeleteBatchDialog({
  names, memberCount, result, error, busy, onConfirm, onClose,
}: Readonly<{
  names: string[]
  /** Already-closed descendants the manager's cascade also deletes. */
  memberCount: number
  result?: DeleteResponse
  error?: string
  busy: boolean
  onConfirm: () => void
  onClose: () => void
}>): ReactElement {
  if (result) {
    return (
      <Dialog
        title="Delete finished"
        testid="delete-modal"
        onClose={onClose}
        footer={<button type="button" className="wk-btn primary" onClick={onClose}>Done</button>}
      >
        <p className={`wk-alert${result.failed > 0 ? ' bad' : ''}`}>
          {`${result.deleted} deleted, ${result.skipped} skipped, ${result.failed} failed`}
        </p>
        <Reasons testid="delete-reasons" rows={notDeleted(result)} />
      </Dialog>
    )
  }
  return (
    <Dialog
      title={`Delete ${names.length} conversation(s)?`}
      label="Delete conversations"
      testid="delete-modal"
      onClose={onClose}
      footer={
        <>
          <button
            type="button"
            className="wk-btn danger"
            disabled={busy || names.length === 0}
            data-testid="delete-confirm"
            onClick={onConfirm}
          >
            {`Delete ${names.length}`}
          </button>
          <button type="button" className="wk-btn link" onClick={onClose}>Cancel</button>
        </>
      }
    >
      <p data-testid="delete-consequences">
        {`${names.length} closed conversation(s) will be deleted. This removes `}
        <b>the recorded answers</b>, the only durable copy of what the agent said, and <b>the workspace on disk</b>.{' '}
        <b>This cannot be undone.</b>
      </p>
      {memberCount > 0 && (
        <p data-testid="delete-member-count">
          {`${names.length} selected, reaching ${memberCount} already-closed member conversation(s) the manager's cascade deletes too.`}
        </p>
      )}
      <p>To keep the record and only tidy the list, close them instead: a closed conversation costs no pod and can be reopened.</p>
      {error && <p className="wk-alert bad">{error}</p>}
    </Dialog>
  )
}
