import { useState } from 'react'
import { Button } from '@patternfly/react-core'
import { useCloseConversations, useDeleteConversations, useMarkRead, useMarkUnread } from '../../../../api/hooks'
import { errorMessage } from '../../../../model/commands'
import { TESTID } from '../../../../model/testids'
import { CloseSelectedModal, workingCount, workingMemberCount } from '../CloseConversations'
import { DeleteSelectedModal, deletableNames } from '../DeleteConversations'
import { descendantsOf, partitionSelection } from '../../../../model/tree'
import type { ConversationSummary } from '../../../../api/types'

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

  const names = [...selected]
  const deletable = deletableNames(items)
  // Every count below is over ROOTS: the selection never holds a member
  // (console-conversation-tree: "A member SHALL NOT be selectable in this
  // view"), so `partitionSelection` is now the selection itself, unchanged.
  const send = partitionSelection(selected)
  // The root's own cascade reaches every descendant whether or not it was
  // selected (console-conversation-tree), so the confirmation counts them
  // from the snapshot rather than from the selection.
  const memberCount = send.reduce((n, name) => n + descendantsOf(items, name).length, 0)
  const selectedAllClosed = send.length > 0 && send.every((n) => deletable.includes(n))

  function runClose(includeWorking: boolean) {
    close.mutate({ names: send, includeWorking }, { onSuccess: onDone })
  }

  function runDelete() {
    del.mutate({ names: send }, { onSuccess: onDone })
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
      data-testid={TESTID.selectionBar}
      style={{
        display: 'flex', alignItems: 'center', gap: 8, padding: '8px 12px', flexWrap: 'wrap',
        // `--ao-brand-strong` INVERTS lightness between the two themes (a dark
        // teal in light mode, a light teal in dark mode), which is exactly
        // what broke PatternFly's own `Button` text colour here: those
        // buttons read their colour from the ordinary page tokens
        // (`--ao-text` on `--ao-surface`), never from this div's `color`, so
        // they kept assuming a normal-surface background underneath them and
        // went unreadable the moment the real background inverted instead.
        // `--ao-brand-soft` is a TINT of the surface in EITHER theme — close
        // in lightness to `--ao-surface`/`--ao-canvas` on both sides — so the
        // buttons' own text colour stays legible with no per-button override.
        background: 'var(--ao-brand-soft)', color: 'var(--ao-text)',
        borderBottom: '1px solid var(--ao-border)',
      }}
    >
      <strong>{names.length} selected</strong>
      <Button
        variant="secondary"
        size="sm"
        isDisabled={names.length === 0 || markRead.isPending}
        onClick={() => markRead.mutate({ names }, { onSuccess: onDone })}
        data-testid={TESTID.markRead}
      >
        Mark read
      </Button>
      {hasReader && (
        <Button
          variant="secondary"
          size="sm"
          isDisabled={names.length === 0 || markUnread.isPending}
          onClick={() => markUnread.mutate({ names }, { onSuccess: onDone })}
          data-testid={TESTID.markUnread}
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
          data-testid={TESTID.closeSelected}
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
          data-testid={TESTID.deleteSelected}
        >
          {`Delete${countSuffix(names.length)}`}
        </Button>
      )}
      <Button variant="link" size="sm" onClick={onClear} style={{ marginLeft: 'auto' }}>
        Clear
      </Button>
      {closeOpen && (
        <CloseSelectedModal
          isOpen
          names={send}
          memberCount={memberCount}
          working={workingCount(items, new Set(send))}
          workingMembers={workingMemberCount(items, send)}
          result={close.data}
          error={close.error ? errorMessage(close.error) : undefined}
          busy={close.isPending}
          onConfirm={runClose}
          onClose={dismissClose}
        />
      )}
      {deleteOpen && (
        <DeleteSelectedModal
          isOpen
          names={send}
          memberCount={memberCount}
          result={del.data}
          error={del.error ? errorMessage(del.error) : undefined}
          busy={del.isPending}
          onConfirm={runDelete}
          onClose={dismissDelete}
        />
      )}
    </div>
  )
}
