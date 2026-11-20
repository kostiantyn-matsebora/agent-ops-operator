import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { SelectionBar } from './SelectionBar'
import type { ConversationSummary } from '../../../api/types'

// The workspace look's own glue over the same model rules the classic look
// draws (model/bulk.ts, model/tree.ts) — selection is over roots, a member
// carries no local skip any more, and the result banner shows exactly what
// the server returned.

const closeMutate = vi.fn()
const deleteMutate = vi.fn()
const markReadMutate = vi.fn()
const markUnreadMutate = vi.fn()
const closeReset = vi.fn()
const deleteReset = vi.fn()
let closeState: { data?: unknown; error?: unknown; isPending?: boolean } = {}
let deleteState: { data?: unknown; error?: unknown; isPending?: boolean } = {}

vi.mock('../../../model/actions', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../../model/actions')>()),
  useCloseBatch: () => ({ mutate: closeMutate, data: undefined, error: null, isPending: false, reset: closeReset, ...closeState }),
  useDeleteBatch: () => ({ mutate: deleteMutate, data: undefined, error: null, isPending: false, reset: deleteReset, ...deleteState }),
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

describe('the selection label and buttons count roots', () => {
  it('two roots with five members between them read 2', async () => {
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

describe('the close and delete dialogs', () => {
  const items = [
    conv('root-1', { coordinator: 'root-1', phase: 'Working' }),
    conv('member-1', { causedBy: { parent: 'root-1', entry: 'diagnose' } }),
    conv('member-2', { causedBy: { parent: 'root-1', entry: 'logs' }, phase: 'Working' }),
    conv('member-3', { causedBy: { parent: 'root-1', entry: 'metrics' } }),
  ]

  it('the title counts the root alone, and the member line names three members with one working', async () => {
    renderBar(items, new Set(['root-1']))
    await userEvent.click(screen.getByTestId('close-selected'))
    expect(screen.getByText('Close 1 conversation(s)?')).toBeInTheDocument()
    expect(screen.getByTestId('close-member-count')).toHaveTextContent(
      '1 selected, reaching 3 live member conversation(s) the manager\'s cascade closes too, 1 of them working',
    )
  })

  it('the delete title counts the root alone too', async () => {
    const closedItems = items.map((c) => ({ ...c, phase: 'Closed' }))
    renderBar(closedItems, new Set(['root-1']))
    await userEvent.click(screen.getByTestId('delete-selected'))
    expect(screen.getByText('Delete 1 conversation(s)?')).toBeInTheDocument()
    expect(screen.getByTestId('delete-member-count')).toHaveTextContent('1 selected, reaching 3')
  })
})

describe('dismissing a dialog resets both mutations', () => {
  it('Cancel on the close dialog', async () => {
    const items = [conv('a')]
    renderBar(items, new Set(['a']))
    await userEvent.click(screen.getByTestId('close-selected'))
    await userEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(closeReset).toHaveBeenCalledTimes(1)
    expect(deleteReset).toHaveBeenCalledTimes(1)
    expect(screen.queryByTestId('close-modal')).toBeNull()
  })

  it('Cancel on the delete dialog', async () => {
    const items = [conv('a', { phase: 'Closed' })]
    renderBar(items, new Set(['a']))
    await userEvent.click(screen.getByTestId('delete-selected'))
    await userEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(closeReset).toHaveBeenCalledTimes(1)
    expect(deleteReset).toHaveBeenCalledTimes(1)
    expect(screen.queryByTestId('delete-modal')).toBeNull()
  })
})

describe('the result banner reports the server’s per-root outcomes only', () => {
  const items = [
    conv('root-1', { coordinator: 'root-1' }),
    conv('root-2', { coordinator: 'root-2' }),
    conv('root-3', { coordinator: 'root-3' }),
  ]

  it('close: two closed and one server-skipped root, nothing added locally', async () => {
    // Preset as "already finished" — this is about what the banner shows
    // once the server has answered, with no local fold-in.
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
