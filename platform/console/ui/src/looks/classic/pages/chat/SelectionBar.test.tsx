import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { SelectionBar } from './SelectionBar'
import type { ConversationSummary } from '../../../../api/types'

const closeMutate = vi.fn()
const deleteMutate = vi.fn()
const markReadMutate = vi.fn()
const markUnreadMutate = vi.fn()
const closeReset = vi.fn()
const deleteReset = vi.fn()
// Overridable per test — the default mirrors "nothing has completed yet",
// which is every test below except the ones that exercise the merged result
// (`close.data`/`del.data` truthy only once a batch has actually finished).
let closeState: { data?: unknown; error?: unknown; isPending?: boolean } = {}
let deleteState: { data?: unknown; error?: unknown; isPending?: boolean } = {}

vi.mock('../../../../api/hooks', () => ({
  useCloseConversations: () => ({ mutate: closeMutate, data: undefined, error: null, isPending: false, reset: closeReset, ...closeState }),
  useDeleteConversations: () => ({ mutate: deleteMutate, data: undefined, error: null, isPending: false, reset: deleteReset, ...deleteState }),
  useMarkRead: () => ({ mutate: markReadMutate, data: undefined, error: null, isPending: false, reset: vi.fn() }),
  useMarkUnread: () => ({ mutate: markUnreadMutate, data: undefined, error: null, isPending: false, reset: vi.fn() }),
}))

function conv(name: string, over: Partial<ConversationSummary> = {}): ConversationSummary {
  return {
    name, runCount: 0, queued: 0, joined: true, errored: false, unread: false,
    ageSeconds: 1, deleting: false, phase: 'Idle', ...over,
  }
}

beforeEach(() => {
  closeMutate.mockReset()
  deleteMutate.mockReset()
  markReadMutate.mockClear()
  markUnreadMutate.mockClear()
  closeReset.mockClear()
  deleteReset.mockClear()
  closeState = {}
  deleteState = {}
})

function renderBar(items: ConversationSummary[], selected: Set<string>, over: Partial<React.ComponentProps<typeof SelectionBar>> = {}) {
  return render(
    <SelectionBar items={items} selected={selected} canWrite hasReader onClear={vi.fn()} onDone={vi.fn()} {...over} />,
  )
}

describe('contrast (item 10)', () => {
  it('uses a surface tint that does not invert between themes, never --ao-brand-strong', () => {
    const items = [conv('a')]
    renderBar(items, new Set(['a']))
    const bar = screen.getByTestId('selection-bar')
    expect(bar.style.background).toContain('--ao-brand-soft')
    expect(bar.style.background).not.toContain('--ao-brand-strong')
  })
})

describe('ordinary close and delete', () => {
  it('sends the selected names, exactly as the old toolbar did', async () => {
    const items = [conv('a'), conv('b')]
    renderBar(items, new Set(['a']))
    await userEvent.click(screen.getByTestId('close-selected'))
    await userEvent.click(screen.getByTestId('close-confirm'))
    expect(closeMutate).toHaveBeenCalledWith({ names: ['a'], includeWorking: false }, expect.anything())
  })

  it('offers delete only when the whole selection is already closed', () => {
    const items = [conv('a', { phase: 'Working' }), conv('b', { phase: 'Closed' })]
    renderBar(items, new Set(['a']))
    expect(screen.getByTestId('delete-selected')).toBeDisabled()
  })

  it('sends a delete once every selected name is closed', async () => {
    const items = [conv('a', { phase: 'Closed' })]
    renderBar(items, new Set(['a']))
    await userEvent.click(screen.getByTestId('delete-selected'))
    await userEvent.click(screen.getByTestId('delete-confirm'))
    expect(deleteMutate).toHaveBeenCalledWith({ names: ['a'] }, expect.anything())
  })
})

describe('mark unread', () => {
  it('posts the selection, bounded and attributed exactly as mark read', async () => {
    const items = [conv('a')]
    renderBar(items, new Set(['a']))
    await userEvent.click(screen.getByTestId('mark-unread'))
    expect(markUnreadMutate).toHaveBeenCalledWith({ names: ['a'] }, expect.anything())
  })

  it('is absent with no reader resolved', () => {
    const items = [conv('a')]
    renderBar(items, new Set(['a']), { hasReader: false })
    expect(screen.queryByTestId('mark-unread')).toBeNull()
  })
})

describe('the selection label and buttons count roots', () => {
  it('two roots with five members between them read 2', () => {
    // console-bulk-close: "The selection label counts roots" scenario.
    const items = [
      conv('root-1', { coordinator: 'root-1' }),
      conv('root-1-a', { causedBy: { parent: 'root-1', entry: 'diagnose' } }),
      conv('root-1-b', { causedBy: { parent: 'root-1', entry: 'logs' } }),
      conv('root-1-c', { causedBy: { parent: 'root-1', entry: 'metrics' } }),
      conv('root-2', { coordinator: 'root-2' }),
      conv('root-2-a', { causedBy: { parent: 'root-2', entry: 'diagnose' } }),
      conv('root-2-b', { causedBy: { parent: 'root-2', entry: 'logs' } }),
    ]
    renderBar(items, new Set(['root-1', 'root-2']))
    expect(screen.getByText('2 selected')).toBeInTheDocument()
    expect(screen.getByTestId('close-selected')).toHaveTextContent('Close… (2)')
    expect(screen.getByTestId('delete-selected')).toHaveTextContent('Delete (2)')
  })
})

describe('a root in the selection', () => {
  const items = [
    conv('root-1', { coordinator: 'root-1', phase: 'Working' }),
    conv('member-1', { causedBy: { parent: 'root-1', entry: 'diagnose' } }),
    conv('member-2', { causedBy: { parent: 'root-1', entry: 'logs' }, phase: 'Working' }),
  ]

  it('the title and body count the root alone, and the member line is separate', async () => {
    renderBar(items, new Set(['root-1']))
    await userEvent.click(screen.getByTestId('close-selected'))
    expect(screen.getByText('Close 1 conversation(s)?')).toBeInTheDocument()
    expect(screen.getByText(/1 conversation\(s\) will be closed/)).toBeInTheDocument()
    expect(screen.getByTestId('close-member-count')).toHaveTextContent('1 selected, reaching 2 member')
  })

  it('the member line also names how many of the members are working', async () => {
    // console-inbox-open-closed design: "The member line separately names
    // how many of the cascade's members are working" — member-2 is, root-1
    // itself is counted by the separate Working warning below.
    renderBar(items, new Set(['root-1']))
    await userEvent.click(screen.getByTestId('close-selected'))
    expect(screen.getByTestId('close-member-count')).toHaveTextContent('1 selected, reaching 2 member conversation(s) the manager\'s cascade closes too, 1 of them working')
  })

  it('the delete title counts the root alone too', async () => {
    const closedItems = items.map((c) => ({ ...c, phase: 'Closed' }))
    renderBar(closedItems, new Set(['root-1']))
    await userEvent.click(screen.getByTestId('delete-selected'))
    expect(screen.getByText('Delete 1 conversation(s)?')).toBeInTheDocument()
    expect(screen.getByTestId('delete-member-count')).toHaveTextContent('1 selected, reaching 2')
  })
})

describe('dismissing a dialog resets its mutation', () => {
  it('Cancel on the close dialog', async () => {
    const items = [conv('a')]
    renderBar(items, new Set(['a']))
    await userEvent.click(screen.getByTestId('close-selected'))
    await userEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(closeReset).toHaveBeenCalledTimes(1)
    expect(screen.queryByTestId('close-modal')).toBeNull()
  })

  it('Cancel on the delete dialog', async () => {
    const items = [conv('a', { phase: 'Closed' })]
    renderBar(items, new Set(['a']))
    await userEvent.click(screen.getByTestId('delete-selected'))
    await userEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(deleteReset).toHaveBeenCalledTimes(1)
    expect(screen.queryByTestId('delete-modal')).toBeNull()
  })
})

describe('the result banner reports the server’s per-root outcomes only', () => {
  // console-inbox-open-closed design: "The result banner reports the
  // server's per-root outcomes. The manager reports a close as accepted and
  // does not report cascaded members individually, so the banner does not
  // claim a member outcome." There is no local skip to fold in any more —
  // whatever the server returned is shown exactly as it came.
  const items = [
    conv('root-1', { coordinator: 'root-1' }),
    conv('root-2', { coordinator: 'root-2' }),
    conv('root-3', { coordinator: 'root-3' }),
  ]

  it('close: two closed and one server-skipped root, nothing added locally', async () => {
    // Preset as "already finished" — the batch itself is exercised by
    // 'ordinary close and delete' above; this is about what the banner
    // shows once the server has answered, with no local fold-in.
    closeState = {
      data: {
        results: [
          { name: 'root-1', outcome: 'closed' },
          { name: 'root-2', outcome: 'closed' },
          { name: 'root-3', outcome: 'skipped', reason: 'already closing' },
        ],
        closed: 2,
        skipped: 1,
        failed: 0,
      },
    }
    renderBar(items, new Set(['root-1', 'root-2', 'root-3']))
    await userEvent.click(screen.getByTestId('close-selected'))
    expect(screen.getByText('2 closed, 1 skipped, 0 failed')).toBeInTheDocument()
    expect(screen.getByTestId('close-reasons')).toHaveTextContent('root-3')
    expect(screen.getByTestId('close-reasons')).toHaveTextContent('already closing')
    // No "member of ..." reason ever appears: a member is never sent, so the
    // server never reports one back.
    expect(screen.queryByText(/member of/)).toBeNull()
  })

  it('delete: the same rule', async () => {
    const closedItems = items.map((c) => ({ ...c, phase: 'Closed' }))
    deleteState = {
      data: {
        results: [
          { name: 'root-1', outcome: 'deleted' },
          { name: 'root-2', outcome: 'deleted' },
          { name: 'root-3', outcome: 'skipped', reason: 'already being deleted' },
        ],
        deleted: 2,
        skipped: 1,
        failed: 0,
      },
    }
    renderBar(closedItems, new Set(['root-1', 'root-2', 'root-3']))
    await userEvent.click(screen.getByTestId('delete-selected'))
    expect(screen.getByText('2 deleted, 1 skipped, 0 failed')).toBeInTheDocument()
    expect(screen.queryByText(/member of/)).toBeNull()
  })
})
