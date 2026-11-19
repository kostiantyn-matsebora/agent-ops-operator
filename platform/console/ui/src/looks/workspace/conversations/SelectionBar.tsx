import { type ReactElement, useState } from 'react'
import type { ConversationSummary } from '../../../api/types'
import { deletableNames, workingCount, workingMemberCount } from '../../../model/bulk'
import {
  errorMessage, useCloseBatch, useDeleteBatch, useMarkRead, useMarkUnread,
} from '../../../model/actions'
import { TESTID } from '../../../model/testids'
import { descendantsOf, partitionSelection } from '../../../model/tree'
import { CloseBatchDialog, DeleteBatchDialog } from './BatchDialogs'

// THE SELECTION BAR: the actions that reach a selection, and nothing beyond
// this page's rows (the selection is built from them). The rules — delete
// takes only closed conversations, the cascade is counted — are the
// model's; this draws them. A member is never IN the selection to begin
// with (console-conversation-tree: "A member SHALL NOT be selectable in
// this view"), so there is no local skip to compute any more.

const suffix = (n: number) => (n > 0 ? ` (${n})` : '')

export function SelectionBar({
  items, selected, canWrite, hasReader, onClear, onDone,
}: Readonly<{
  /** This page's rows — the only scope a selection ever reaches. */
  items: ConversationSummary[]
  selected: Set<string>
  canWrite: boolean
  hasReader: boolean
  onClear: () => void
  onDone: () => void
}>): ReactElement {
  const close = useCloseBatch()
  const del = useDeleteBatch()
  const markRead = useMarkRead()
  const markUnread = useMarkUnread()
  const [dialog, setDialog] = useState<'close' | 'delete' | null>(null)

  const names = [...selected]
  const send = partitionSelection(selected)
  const deletable = deletableNames(items)
  const memberCount = send.reduce((n, name) => n + descendantsOf(items, name).length, 0)
  const allClosed = send.length > 0 && send.every((n) => deletable.includes(n))

  function dismiss() {
    setDialog(null)
    close.reset()
    del.reset()
  }

  return (
    <div className="wk-selbar" data-testid={TESTID.selectionBar} role="toolbar" aria-label="Selected conversations">
      <strong>{names.length} selected</strong>
      <button
        type="button"
        className="wk-btn"
        data-testid={TESTID.markRead}
        disabled={names.length === 0 || markRead.isPending}
        onClick={() => markRead.mutate({ names }, { onSuccess: onDone })}
      >
        Mark read
      </button>
      {hasReader && (
        <button
          type="button"
          className="wk-btn"
          data-testid={TESTID.markUnread}
          disabled={names.length === 0 || markUnread.isPending}
          onClick={() => markUnread.mutate({ names }, { onSuccess: onDone })}
        >
          Mark unread
        </button>
      )}
      {canWrite && (
        <button
          type="button"
          className="wk-btn danger"
          data-testid={TESTID.closeSelected}
          disabled={names.length === 0}
          onClick={() => setDialog('close')}
        >
          {`Close…${suffix(names.length)}`}
        </button>
      )}
      {canWrite && (
        <button
          type="button"
          className="wk-btn danger"
          data-testid={TESTID.deleteSelected}
          disabled={!allClosed}
          onClick={() => setDialog('delete')}
        >
          {`Delete${suffix(names.length)}`}
        </button>
      )}
      <button type="button" className="wk-btn link end" onClick={onClear}>Clear</button>
      {dialog === 'close' && (
        <CloseBatchDialog
          names={send}
          memberCount={memberCount}
          working={workingCount(items, new Set(send))}
          workingMembers={workingMemberCount(items, send)}
          result={close.data}
          error={close.error ? errorMessage(close.error) : undefined}
          busy={close.isPending}
          onConfirm={(includeWorking) => close.mutate({ names: send, includeWorking }, { onSuccess: onDone })}
          onClose={dismiss}
        />
      )}
      {dialog === 'delete' && (
        <DeleteBatchDialog
          names={send}
          memberCount={memberCount}
          result={del.data}
          error={del.error ? errorMessage(del.error) : undefined}
          busy={del.isPending}
          onConfirm={() => del.mutate({ names: send }, { onSuccess: onDone })}
          onClose={dismiss}
        />
      )}
    </div>
  )
}
