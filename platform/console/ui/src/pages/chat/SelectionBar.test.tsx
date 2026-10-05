import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { SelectionBar } from './SelectionBar'
import type { ConversationSummary } from '../../api/types'

const closeMutate = vi.fn()
const deleteMutate = vi.fn()
const markReadMutate = vi.fn()
const markUnreadMutate = vi.fn()

vi.mock('../../api/hooks', () => ({
  useCloseConversations: () => ({ mutate: closeMutate, data: undefined, error: null, isPending: false, reset: vi.fn() }),
  useDeleteConversations: () => ({ mutate: deleteMutate, data: undefined, error: null, isPending: false, reset: vi.fn() }),
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
  closeMutate.mockClear()
  deleteMutate.mockClear()
  markReadMutate.mockClear()
  markUnreadMutate.mockClear()
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

describe('a root in the selection', () => {
  const items = [
    conv('root-1', { coordinator: 'root-1', phase: 'Working' }),
    conv('member-1', { causedBy: { parent: 'root-1', entry: 'diagnose' } }),
    conv('member-2', { causedBy: { parent: 'root-1', entry: 'logs' } }),
  ]

  it('counts its two members in the close confirmation', async () => {
    renderBar(items, new Set(['root-1']))
    await userEvent.click(screen.getByTestId('close-selected'))
    expect(screen.getByText(/3 conversation\(s\) will be closed/)).toBeInTheDocument()
    expect(screen.getByTestId('close-member-count')).toHaveTextContent('1 selected, reaching 2 member')
  })

  it('counts its two members in the delete confirmation', async () => {
    const closedItems = items.map((c) => ({ ...c, phase: 'Closed' }))
    renderBar(closedItems, new Set(['root-1']))
    await userEvent.click(screen.getByTestId('delete-selected'))
    expect(screen.getByText(/3 conversation\(s\)\?/)).toBeInTheDocument()
    expect(screen.getByTestId('delete-member-count')).toHaveTextContent('1 selected, reaching 2')
  })
})

describe('a member reached directly, with no ancestor also selected', () => {
  const items = [
    conv('root-1', { coordinator: 'root-1', phase: 'Closed' }),
    conv('member-1', { causedBy: { parent: 'root-1', entry: 'diagnose' }, phase: 'Closed' }),
  ]

  it('close has nothing left to send, and never reaches the server', async () => {
    renderBar(items, new Set(['member-1']))
    await userEvent.click(screen.getByTestId('close-selected'))
    expect(screen.getByTestId('close-confirm')).toBeDisabled()
    expect(closeMutate).not.toHaveBeenCalled()
  })

  it('delete is refused outright — the selection offers nothing to act on', () => {
    renderBar(items, new Set(['member-1']))
    expect(screen.getByTestId('delete-selected')).toBeDisabled()
    expect(deleteMutate).not.toHaveBeenCalled()
  })

  it('a selected root still reaches the server normally, the member dropped from the request', async () => {
    renderBar(items, new Set(['root-1', 'member-1']))
    await userEvent.click(screen.getByTestId('close-selected'))
    await userEvent.click(screen.getByTestId('close-confirm'))
    expect(closeMutate).toHaveBeenCalledWith({ names: ['root-1'], includeWorking: false }, expect.anything())
  })
})
