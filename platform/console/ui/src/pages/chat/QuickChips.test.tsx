import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { QuickChips } from './QuickChips'

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

// Pipeline/coordinator "starter" chips were REMOVED from inside an open
// conversation entirely, by direct instruction — offering to start a
// redundant new conversation with a pipeline/coordinator you are already
// talking to (or any other) was confusing, never useful. Starting a new
// conversation is the "New conversation" modal's job now. There is
// deliberately no "start chips" describe block any more: there is nothing
// left to test for a feature that no longer exists.

describe('thread command chips', () => {
  it('runs the command directly — never inserts it for the user to send themselves', () => {
    const onRunCommand = vi.fn()
    render(<QuickChips canWrite onRunCommand={onRunCommand} onInsertCommand={vi.fn()} />)
    screen.getByLabelText('/exit').click()
    expect(onRunCommand).toHaveBeenCalledWith('/exit')
  })

  it('renders as an icon, never raw "/name" text — by direct instruction, overriding the prototype', () => {
    render(<QuickChips canWrite onRunCommand={vi.fn()} onInsertCommand={vi.fn()} />)
    const exitButton = screen.getByLabelText('/exit')
    expect(exitButton).not.toHaveTextContent('/exit')
    expect(exitButton.querySelector('svg')).toBeInTheDocument()
  })

  it('is absent with no onRunCommand — nothing open to run a command against', () => {
    render(<QuickChips canWrite onInsertCommand={vi.fn()} />)
    expect(screen.queryByLabelText('/exit')).toBeNull()
  })
})

describe('choice chips', () => {
  it('prefills the choice’s command, ready to send', () => {
    const onInsertCommand = vi.fn()
    render(
      <QuickChips
        canWrite
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
        choices={[{ label: 'Roll back', command: '/exit' }]}
        onInsertCommand={vi.fn()}
        onRunCommand={vi.fn()}
      />,
    )
    expect(container).toBeEmptyDOMElement()
  })
})
