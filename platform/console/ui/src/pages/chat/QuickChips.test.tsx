import { afterEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { QuickChips } from './QuickChips'
import { useComposerIntent } from './composerIntent'

vi.mock('../../api/hooks', () => ({
  useVocabulary: () => ({
    data: {
      entries: [
        { kind: 'pipeline', name: 'k8s-observe', position: 'general', icon: 'aops:kubernetes' },
        { kind: 'coordinator', name: 'rollout-coordinator', position: 'general' },
        { kind: 'builtin', name: 'exit', position: 'thread', description: 'release the runtime' },
        { kind: 'builtin', name: 'close', position: 'thread', description: 'end the conversation' },
      ],
    },
  }),
}))

afterEach(() => {
  useComposerIntent.getState().clear()
})

describe('start chips', () => {
  it('opens the composer addressed to the chosen pipeline', () => {
    render(<QuickChips canWrite canStart onInsertCommand={vi.fn()} />)
    screen.getByText('k8s-observe').click()
    expect(useComposerIntent.getState().requestedTask).toBe('/k8s-observe ')
  })

  it('offers a coordinator exactly like a pipeline', () => {
    render(<QuickChips canWrite canStart onInsertCommand={vi.fn()} />)
    expect(screen.getByText('rollout-coordinator')).toBeInTheDocument()
  })

  it('marks every starter with a "+" — it starts something new (item 20)', () => {
    render(<QuickChips canWrite canStart onInsertCommand={vi.fn()} />)
    const starter = screen.getByText('k8s-observe').closest('button')
    expect(starter).toHaveTextContent('+k8s-observe')
  })

  it('is absent when origination is unavailable', () => {
    render(<QuickChips canWrite canStart={false} onInsertCommand={vi.fn()} />)
    expect(screen.queryByText('k8s-observe')).toBeNull()
  })
})

describe('thread command chips', () => {
  it('inserts the addressed command', () => {
    const onInsertCommand = vi.fn()
    render(<QuickChips canWrite canStart={false} onInsertCommand={onInsertCommand} />)
    screen.getByText('/exit').click()
    expect(onInsertCommand).toHaveBeenCalledWith('/exit')
  })
})

describe('choice chips', () => {
  it('prefills the choice’s command, ready to send', () => {
    const onInsertCommand = vi.fn()
    render(
      <QuickChips
        canWrite
        canStart={false}
        choices={[{ label: 'Roll back', command: '/exit roll back' }]}
        onInsertCommand={onInsertCommand}
      />,
    )
    screen.getByText('Roll back').click()
    expect(onInsertCommand).toHaveBeenCalledWith('/exit roll back ')
  })
})

describe('the read-only case', () => {
  it('renders nothing at all', () => {
    const { container } = render(
      <QuickChips
        canWrite={false}
        canStart
        choices={[{ label: 'Roll back', command: '/exit' }]}
        onInsertCommand={vi.fn()}
      />,
    )
    expect(container).toBeEmptyDOMElement()
  })
})
