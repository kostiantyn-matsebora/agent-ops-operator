import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { RowMenu } from './RowMenu'
import type { ConversationSummary } from '../../api/types'

function conv(name: string, over: Partial<ConversationSummary> = {}): ConversationSummary {
  return {
    name, runCount: 0, queued: 0, joined: true, errored: false, unread: false,
    ageSeconds: 1, deleting: false, phase: 'Working', ...over,
  }
}

function renderMenu(row: ConversationSummary, over: Partial<React.ComponentProps<typeof RowMenu>> = {}) {
  render(
    <RowMenu
      row={row}
      canWrite
      hasReader
      onMarkRead={vi.fn()}
      onMarkUnread={vi.fn()}
      onOpenNewTab={vi.fn()}
      onCopyLink={vi.fn()}
      onOpenIncident={vi.fn()}
      onReopen={vi.fn()}
      onExitRuntime={vi.fn()}
      onClose={vi.fn()}
      onDelete={vi.fn()}
      {...over}
    />,
  )
}

async function open(name: string) {
  await userEvent.click(screen.getByLabelText(`actions for ${name}`))
}

describe('a working row', () => {
  it('offers mark unread, navigation, exit runtime and close — never reopen or delete', async () => {
    renderMenu(conv('a', { unreadCount: 0 }))
    await open('a')
    expect(screen.getByText('Mark unread')).toBeInTheDocument()
    expect(screen.getByText('Open in new tab')).toBeInTheDocument()
    expect(screen.getByText('Copy link')).toBeInTheDocument()
    expect(screen.getByText('Exit runtime')).toBeInTheDocument()
    expect(screen.getByText('Close')).toBeInTheDocument()
    expect(screen.queryByText('Reopen')).toBeNull()
    expect(screen.queryByText('Delete')).toBeNull()
  })

  it('offers mark read instead, once the row is unread', async () => {
    renderMenu(conv('a', { unreadCount: 3, unread: true }))
    await open('a')
    expect(screen.getByText('Mark read')).toBeInTheDocument()
    expect(screen.queryByText('Mark unread')).toBeNull()
  })

  it('offers no mark-unread item with no reader resolved', async () => {
    renderMenu(conv('a', { unreadCount: 0 }), { hasReader: false })
    await open('a')
    expect(screen.queryByText('Mark unread')).toBeNull()
  })
})

describe('a closed row', () => {
  it('offers reopen and delete, never close or exit runtime', async () => {
    renderMenu(conv('b', { phase: 'Closed' }))
    await open('b')
    expect(screen.getByText('Reopen')).toBeInTheDocument()
    expect(screen.getByText('Delete')).toBeInTheDocument()
    expect(screen.queryByText('Close')).toBeNull()
    expect(screen.queryByText('Exit runtime')).toBeNull()
  })
})

describe('a member row', () => {
  it('offers only navigation and the incident — none of the conversation-scoped actions', async () => {
    renderMenu(conv('member-1', { causedBy: { parent: 'root-1', entry: 'diagnose' } }))
    await open('member-1')
    expect(screen.getByText('Open in new tab')).toBeInTheDocument()
    expect(screen.getByText('Copy link')).toBeInTheDocument()
    expect(screen.getByText('Open incident')).toBeInTheDocument()
    expect(screen.queryByText('Mark unread')).toBeNull()
    expect(screen.queryByText('Mark read')).toBeNull()
    expect(screen.queryByText('Reopen')).toBeNull()
    expect(screen.queryByText('Exit runtime')).toBeNull()
    expect(screen.queryByText('Close')).toBeNull()
    expect(screen.queryByText('Delete')).toBeNull()
  })
})

it('a read-only console offers no close, exit, reopen or delete', async () => {
  renderMenu(conv('c', { phase: 'Closed' }), { canWrite: false })
  await open('c')
  expect(screen.queryByText('Reopen')).toBeNull()
  expect(screen.queryByText('Delete')).toBeNull()
})
