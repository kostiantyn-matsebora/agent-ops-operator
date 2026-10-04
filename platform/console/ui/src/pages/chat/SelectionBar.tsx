import { useMemo, useState } from 'react'
import { Button } from '@patternfly/react-core'
import { useCloseConversations, useDeleteConversations, useMarkRead, useMarkUnread } from '../../api/hooks'
import { ApiError } from '../../api/client'
import { CloseSelectedModal, workingCount } from '../CloseConversations'
import { DeleteSelectedModal, deletableNames } from '../DeleteConversations'
import { descendantsOf, partitionSelection } from './tree'
import type { CloseResponse, ConversationSummary, DeleteResponse } from '../../api/types'

// `console-chat-layout` D-I and `console-quick-actions`: the SAME actions
// and rules the old toolbar carried, now in the selection bar. The two
// modals it opens are reused as-is (CloseConversations.tsx,
// DeleteConversations.tsx) — this file is the glue, not a second
// implementation of what they already do.

export interface SelectionBarProps {
  /** This page's rows — the ONLY scope a selection ever reaches. */
  items: ConversationSummary[]
  selected: Set<string>
  canWrite: boolean
  hasReader: boolean
  onClear: () => void
  onDone: () => void
}

function skippedResults(skipped: { name: string; parentName: string }[]) {
  return skipped.map((s) => ({
    name: s.name,
    outcome: 'skipped' as const,
    reason: `member of ${s.parentName} — act on ${s.parentName} instead`,
  }))
}

function countSuffix(n: number): string {
  return n > 0 ? ` (${n})` : ''
}

export function SelectionBar({ items, selected, canWrite, hasReader, onClear, onDone }: Readonly<SelectionBarProps>) {
  const close = useCloseConversations()
  const del = useDeleteConversations()
  const markRead = useMarkRead()
  const markUnread = useMarkUnread()
  const [closeOpen, setCloseOpen] = useState(false)
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [closeSkipped, setCloseSkipped] = useState<{ name: string; parentName: string }[]>([])
  const [deleteSkipped, setDeleteSkipped] = useState<{ name: string; parentName: string }[]>([])

  const names = [...selected]
  const deletable = deletableNames(items)
  const { send: closeSend, skipped: closeWillSkip } = partitionSelection(items, selected)
  const { send: deleteSend, skipped: deleteWillSkip } = partitionSelection(items, selected)
  // The root's own cascade reaches every descendant whether or not it was
  // selected (console-conversation-tree), so the confirmation counts them
  // from the snapshot rather than from the selection.
  const closeMemberCount = closeSend.reduce((n, name) => n + descendantsOf(items, name).length, 0)
  const deleteMemberCount = deleteSend.reduce((n, name) => n + descendantsOf(items, name).length, 0)
  // A member skipped entirely never needs to be CLOSED first — it is not
  // being deleted, its root's own closedness is what matters.
  const selectedAllClosed = deleteSend.length > 0 && deleteSend.every((n) => deletable.includes(n))

  const mergedCloseResult: CloseResponse | undefined = useMemo(() => {
    if (!close.data) return undefined
    const extra = skippedResults(closeSkipped)
    return {
      results: [...close.data.results, ...extra],
      closed: close.data.closed,
      skipped: close.data.skipped + extra.length,
      failed: close.data.failed,
    }
  }, [close.data, closeSkipped])

  const mergedDeleteResult: DeleteResponse | undefined = useMemo(() => {
    if (!del.data) return undefined
    const extra = skippedResults(deleteSkipped)
    return {
      results: [...del.data.results, ...extra],
      deleted: del.data.deleted,
      skipped: del.data.skipped + extra.length,
      failed: del.data.failed,
    }
  }, [del.data, deleteSkipped])

  function runClose(includeWorking: boolean) {
    setCloseSkipped(closeWillSkip)
    close.mutate({ names: closeSend, includeWorking }, { onSuccess: onDone })
  }

  function runDelete() {
    setDeleteSkipped(deleteWillSkip)
    del.mutate({ names: deleteSend }, { onSuccess: onDone })
  }

  function dismissClose() {
    setCloseOpen(false)
    close.reset()
  }

  function dismissDelete() {
    setDeleteOpen(false)
    del.reset()
  }

  return (
    <div
      data-testid="selection-bar"
      style={{
        display: 'flex', alignItems: 'center', gap: 8, padding: '8px 12px', flexWrap: 'wrap',
        background: 'var(--ao-brand-strong)', color: 'var(--ao-surface)',
      }}
    >
      <strong>{names.length} selected</strong>
      <Button
        variant="secondary"
        size="sm"
        isDisabled={names.length === 0 || markRead.isPending}
        onClick={() => markRead.mutate({ names }, { onSuccess: onDone })}
        data-testid="mark-read"
      >
        Mark read
      </Button>
      {hasReader && (
        <Button
          variant="secondary"
          size="sm"
          isDisabled={names.length === 0 || markUnread.isPending}
          onClick={() => markUnread.mutate({ names }, { onSuccess: onDone })}
          data-testid="mark-unread"
        >
          Mark unread
        </Button>
      )}
      {canWrite && (
        <Button
          variant="secondary"
          size="sm"
          isDanger
          isDisabled={names.length === 0}
          onClick={() => setCloseOpen(true)}
          data-testid="close-selected"
        >
          {`Close…${countSuffix(names.length)}`}
        </Button>
      )}
      {canWrite && (
        <Button
          variant="secondary"
          size="sm"
          isDanger
          isDisabled={!selectedAllClosed}
          onClick={() => setDeleteOpen(true)}
          data-testid="delete-selected"
        >
          {`Delete${countSuffix(names.length)}`}
        </Button>
      )}
      <Button variant="link" size="sm" onClick={onClear} style={{ marginLeft: 'auto', color: 'var(--ao-surface)' }}>
        Clear
      </Button>
      {closeOpen && (
        <CloseSelectedModal
          isOpen
          names={closeSend}
          memberCount={closeMemberCount}
          working={workingCount(items, new Set(closeSend))}
          result={mergedCloseResult}
          error={close.error ? (close.error as ApiError).message : undefined}
          busy={close.isPending}
          onConfirm={runClose}
          onClose={dismissClose}
        />
      )}
      {deleteOpen && (
        <DeleteSelectedModal
          isOpen
          names={deleteSend}
          memberCount={deleteMemberCount}
          result={mergedDeleteResult}
          error={del.error ? (del.error as ApiError).message : undefined}
          busy={del.isPending}
          onConfirm={runDelete}
          onClose={dismissDelete}
        />
      )}
    </div>
  )
}
