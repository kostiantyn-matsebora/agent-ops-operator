import { describe, expect, it } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import type { ConversationSummary } from '../../../api/types'
import { testid } from '../../../model/testids'
import type { TreeRow } from '../../../model/tree'
import css from '../workspace.css?raw'
import { Row, lifecycle } from './Row'

const NOW = Date.parse('2025-06-11T09:40:00Z')

function conv(over: Partial<ConversationSummary> = {}): ConversationSummary {
  return {
    name: 'conv-1', title: 'checkout-api is restarting', pipeline: 'k8s-observe', phase: 'Idle',
    runCount: 1, queued: 0, joined: true, errored: false, unread: false, ageSeconds: 0, deleting: false,
    lastActivity: '2025-06-11T09:37:00Z', lastMessage: { kind: 'agent', text: 'diagnosed' },
    ...over,
  }
}

const tree = (row: ConversationSummary, over: Partial<TreeRow> = {}): TreeRow => ({
  row, depth: 0, rootName: row.name, parentMissing: false, memberCount: 0, membersActive: 0, activeEntries: [], ...over,
})

const mount = (row: ConversationSummary, over: Partial<TreeRow> = {}) =>
  render(
    <MemoryRouter>
      <ul>
        <Row tree={tree(row, over)} current={false} now={NOW} />
      </ul>
    </MemoryRouter>,
  )

/**
 * What decides a row's height: its columns and its text lines, by class. A
 * status, a badge or a lifecycle segment changes what is INSIDE these, never
 * which of them exist.
 */
const skeleton = (row: Element): string =>
  [
    ...[...row.children].map((c) => c.className.split(' ')[0] || c.tagName),
    ...[...(row.querySelector('.txt')?.children ?? [])].map((c) => c.className.split(' ')[0]),
    ...[...(row.querySelector('.st')?.children ?? [])].slice(0, 1).map((c) => c.className),
  ].join('|')

describe('a list row', () => {
  it('carries the title, the last message, the pipeline, a reserved lifecycle line, the time, the badge and the status', () => {
    mount(conv({ unread: true, unreadCount: 2, presence: true, phase: 'Working' }))
    const row = screen.getByTestId(testid.row('conv-1'))
    expect(within(row).getByText('checkout-api is restarting')).toBeInTheDocument()
    expect(within(row).getByText('working · diagnosed')).toBeInTheDocument()
    expect(within(row).getByText('k8s-observe')).toBeInTheDocument()
    expect(row.querySelector('.lc')).not.toBeNull()
    expect(within(row).getByText('3m')).toBeInTheDocument()
    expect(screen.getByTestId(testid.unread('conv-1'))).toHaveTextContent('2')
    expect(within(row).getByText('Working')).toBeInTheDocument()
    // The badge sits beside the time, the status below both.
    expect(screen.getByTestId(testid.unread('conv-1')).parentElement).toBe(row.querySelector('time')?.parentElement)
    expect(row.querySelector('.st > .wk-pill.working')).not.toBeNull()
    expect(screen.getByRole('link', { name: 'Open checkout-api is restarting, 2 unread' })).toHaveAttribute('href', '/conversations/conv-1')
  })

  it('keeps one shape whatever its status, so a status change moves no row', () => {
    const shapes = [
      conv(),
      conv({ presence: true, phase: 'Working', unread: true, unreadCount: 3 }),
      conv({ phase: 'Pending', lastMessage: undefined }),
      conv({ errored: true }),
      conv({ phase: 'Closed', closeReason: 'done' }),
    ].map((c) => {
      const { container, unmount } = mount(c)
      const row = container.querySelector('li') as HTMLElement
      // Every line is present, the lifecycle line reserved even while empty.
      for (const line of ['.name', '.l2', '.l3', '.lc', '.st', '.tl']) expect(row.querySelector(line), line).not.toBeNull()
      const shape = skeleton(row)
      unmount()
      return shape
    })
    expect(new Set(shapes).size).toBe(1)
    // And the height is fixed by the stylesheet, not grown by content.
    expect(css).toMatch(/\.wk-row \{[^}]*\n\s+height: var\(--wk-row-h\);/)
    expect(css).toMatch(/--wk-row-h: 84px;/)
  })

  it('fills the lifecycle line only while pending or working', () => {
    expect(lifecycle('pending')).toEqual(['cur', '', ''])
    expect(lifecycle('working')).toEqual(['on', 'cur', ''])
    expect(lifecycle('idle')).toBeUndefined()
    expect(lifecycle('none')).toBeUndefined()
  })

  it('shows no badge with nothing unread and no pill while idle', () => {
    mount(conv())
    expect(screen.queryByTestId(testid.unread('conv-1'))).toBeNull()
    expect(screen.getByTestId(testid.row('conv-1')).querySelector('.wk-pill')).toBeNull()
  })

  it('names a member by the entry it was invoked as and folds a root with members', () => {
    render(
      <MemoryRouter>
        <ul>
          <Row tree={tree(conv({ name: 'root', coordinator: 'incident' }), { memberCount: 2 })} current={false} folded onToggleFold={() => undefined} now={NOW} />
          <Row tree={tree(conv({ name: 'm', causedBy: { parent: 'root', entry: 'triage' } }), { depth: 1 })} current={false} now={NOW} />
        </ul>
      </MemoryRouter>,
    )
    expect(screen.getByText('incident · 2 members')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Expand checkout-api is restarting' })).toHaveAttribute('aria-expanded', 'false')
    expect(screen.getByText('via triage')).toBeInTheDocument()
    expect(screen.getByTestId(testid.row('m'))).toHaveClass('mem')
  })

  it('offers its actions from a menu that the keyboard reaches', async () => {
    const { default: userEvent } = await import('@testing-library/user-event')
    let ran = ''
    render(
      <MemoryRouter>
        <ul>
          <Row tree={tree(conv())} current={false} now={NOW} actions={[{ label: 'Exit runtime', run: () => { ran = 'exit' } }, { label: 'Close', run: () => { ran = 'close' } }]} />
        </ul>
      </MemoryRouter>,
    )
    const toggle = screen.getByRole('button', { name: 'Actions for checkout-api is restarting' })
    await userEvent.click(toggle)
    expect(screen.getByRole('menuitem', { name: 'Exit runtime' })).toHaveFocus()
    await userEvent.keyboard('{ArrowDown}{Enter}')
    expect(ran).toBe('close')
    expect(screen.queryByRole('menu')).toBeNull()
    expect(toggle).toHaveFocus()
  })

  it('folded to icons, keeps its unread count as a badge', () => {
    render(
      <MemoryRouter>
        <ul>
          <Row tree={tree(conv({ unread: true, unreadCount: 4 }))} current={false} iconOnly now={NOW} />
        </ul>
      </MemoryRouter>,
    )
    expect(screen.getByTestId(testid.row('conv-1')).querySelector('.wk-av .bd')).toHaveTextContent('4')
  })
})

describe('a root waiting on its members', () => {
  const idleRoot = () => conv({ phase: 'Idle', presence: false, coordinator: 'incident' })

  it('says Waiting, names who, animates the lifecycle bar and carries a testid', () => {
    mount(idleRoot(), { memberCount: 2, membersActive: 2, activeEntries: ['triage', 'ops'] })
    const row = screen.getByTestId(`row-${idleRoot().name}`)
    expect(row).toHaveAttribute('data-status', 'waiting')
    expect(screen.getByTestId(`waiting-${idleRoot().name}`)).toHaveTextContent('Waiting')
    expect(row).toHaveTextContent('waiting on triage, ops…')
    expect(row.querySelectorAll('.lc s.cur')).toHaveLength(1)
  })

  it('is plainly idle when no member is at work', () => {
    mount(idleRoot(), { memberCount: 2, membersActive: 0 })
    expect(screen.getByTestId(`row-${idleRoot().name}`)).toHaveAttribute('data-status', 'idle')
    expect(screen.queryByTestId(`waiting-${idleRoot().name}`)).toBeNull()
  })

  it('leaves its own working state alone', () => {
    mount(conv({ phase: 'Working', presence: true }), { memberCount: 1, membersActive: 1, activeEntries: ['triage'] })
    expect(screen.getByText('Working')).toBeInTheDocument()
  })
})
