import { useState, type Ref } from 'react'
import {
  Divider, Dropdown, DropdownItem, DropdownList, MenuToggle, type MenuToggleElement,
} from '@patternfly/react-core'
import { EllipsisVIcon } from '@patternfly/react-icons'
import type { ConversationSummary } from '../../api/types'

// `console-quick-actions` spec: "An action the conversation's state or the
// viewer's rights do not allow SHALL be absent, never disabled without a
// reason" — every branch below OMITS an item rather than greying it out.

export interface RowMenuProps {
  row: ConversationSummary
  canWrite: boolean
  /** A per-person watermark exists to rewind — absent under a shared token (design D-E). */
  hasReader: boolean
  onMarkRead: () => void
  onMarkUnread: () => void
  onOpenNewTab: () => void
  onCopyLink: () => void
  onOpenIncident: () => void
  onReopen: () => void
  onExitRuntime: () => void
  onClose: () => void
  onDelete: () => void
}

function renderToggle(title: string, open: boolean, onToggle: () => void) {
  return function MenuToggleFor(toggleRef: Ref<MenuToggleElement>) {
    return (
      <MenuToggle
        ref={toggleRef}
        variant="plain"
        aria-label={`actions for ${title}`}
        onClick={onToggle}
        isExpanded={open}
      >
        <EllipsisVIcon />
      </MenuToggle>
    )
  }
}

export function RowMenu(props: Readonly<RowMenuProps>) {
  const { row, canWrite, hasReader } = props
  const [open, setOpen] = useState(false)
  const title = row.title || row.name
  const isMember = Boolean(row.causedBy)
  const isClosed = row.phase === 'Closed'
  const isUnread = (row.unreadCount ?? 0) > 0

  function act(fn: () => void) {
    return () => {
      setOpen(false)
      fn()
    }
  }

  // A MEMBER holds no channel binding of its own, so none of mark unread/read,
  // reopen, exit runtime, close or delete ever apply to it — the row menu
  // offers only navigation (console-quick-actions, console-conversation-tree).
  const items = isMember
    ? [
        <DropdownItem key="new-tab" onClick={act(props.onOpenNewTab)}>Open in new tab</DropdownItem>,
        <DropdownItem key="copy-link" onClick={act(props.onCopyLink)}>Copy link</DropdownItem>,
        <DropdownItem key="incident" onClick={act(props.onOpenIncident)}>Open incident</DropdownItem>,
      ]
    : [
        isUnread ? (
          <DropdownItem key="mark-read" onClick={act(props.onMarkRead)}>Mark read</DropdownItem>
        ) : (
          hasReader && (
            <DropdownItem key="mark-unread" onClick={act(props.onMarkUnread)}>Mark unread</DropdownItem>
          )
        ),
        <Divider key="d1" />,
        <DropdownItem key="new-tab" onClick={act(props.onOpenNewTab)}>Open in new tab</DropdownItem>,
        <DropdownItem key="copy-link" onClick={act(props.onCopyLink)}>Copy link</DropdownItem>,
        <Divider key="d2" />,
        !isClosed && canWrite && (
          <DropdownItem key="exit" onClick={act(props.onExitRuntime)}>Exit runtime</DropdownItem>
        ),
        !isClosed && canWrite && (
          <DropdownItem key="close" isDanger onClick={act(props.onClose)}>Close</DropdownItem>
        ),
        isClosed && canWrite && (
          <DropdownItem key="reopen" onClick={act(props.onReopen)}>Reopen</DropdownItem>
        ),
        isClosed && canWrite && (
          <DropdownItem key="delete" isDanger onClick={act(props.onDelete)}>Delete</DropdownItem>
        ),
      ].filter((it): it is React.JSX.Element => Boolean(it))

  return (
    <Dropdown
      isOpen={open}
      onOpenChange={setOpen}
      toggle={renderToggle(title, open, () => setOpen((v) => !v))}
      popperProps={{ position: 'right' }}
    >
      <DropdownList>{items}</DropdownList>
    </Dropdown>
  )
}
