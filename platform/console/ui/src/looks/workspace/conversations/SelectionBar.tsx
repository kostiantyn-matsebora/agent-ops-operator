import { type ReactElement, useMemo, useState } from 'react'
import type { CloseResponse, ConversationSummary, DeleteResponse } from '../../../api/types'
import { deletableNames, workingCount } from '../../../model/bulk'
import {
  errorMessage, useCloseBatch, useDeleteBatch, useMarkRead, useMarkUnread,
} from '../../../model/actions'
import { TESTID } from '../../../model/testids'
import { descendantsOf, partitionSelection } from '../../../model/tree'
import { CloseBatchDialog, DeleteBatchDialog } from './BatchDialogs'

// THE SELECTION BAR: the actions that reach a selection, and nothing beyond
// this page's rows (the selection is built from them). The rules — a member
// is skipped and names its parent, delete takes only closed conversations,
// the cascade is counted — are the model's; this draws them.

interface Skipped {
  name: string
  parentName: string
}

const skippedResults = (skipped: Skipped[]) =>
  skipped.map((s) => ({
    name: s.name,
    outcome: 'skipped' as const,
    reason: `member of ${s.parentName} — act on ${s.parentName} instead`,
  }))

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
  const [skipped, setSkipped] = useState<Skipped[]>([])

  const names = [...selected]
  const { send, skipped: willSkip } = partitionSelection(items, selected)
  const deletable = deletableNames(items)
  const memberCount = send.reduce((n, name) => n + descendantsOf(items, name).length, 0)
  const allClosed = send.length > 0 && send.every((n) => deletable.includes(n))

  const closeResult: CloseResponse | undefined = useMemo(() => {
    if (!close.data) return undefined
    const extra = skippedResults(skipped)
    return { ...close.data, results: [...close.data.results, ...extra], skipped: close.data.skipped + extra.length }
  }, [close.data, skipped])
  const deleteResult: DeleteResponse | undefined = useMemo(() => {
    if (!del.data) return undefined
    const extra = skippedResults(skipped)
    return { ...del.data, results: [...del.data.results, ...extra], skipped: del.data.skipped + extra.length }
  }, [del.data, skipped])

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
          onClick={() => {
            setSkipped(willSkip)
            setDialog('close')
          }}
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
          onClick={() => {
            setSkipped(willSkip)
            setDialog('delete')
          }}
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
          result={closeResult}
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
          result={deleteResult}
          error={del.error ? errorMessage(del.error) : undefined}
          busy={del.isPending}
          onConfirm={() => del.mutate({ names: send }, { onSuccess: onDone })}
          onClose={dismiss}
        />
      )}
    </div>
  )
}
