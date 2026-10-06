import type { ReactNode } from 'react'
import { Button, Tooltip } from '@patternfly/react-core'
import { ArchiveIcon, SignOutAltIcon } from '@patternfly/react-icons'
import { useVocabulary } from '../../api/hooks'
import { PlainText } from '../../components/Text'
import type { Choice } from '../../api/types'

// Ported from `prototype/States.html`'s chip row, with ONE explicit
// deviation from it: the user instructed directly, more than once, that
// thread commands are icons the user RUNS, never raw "/name" text the user
// has to insert and then send — overriding the prototype's own mono-text
// rendering of `/exit`/`/close`. Pipeline/coordinator "starter" chips are
// REMOVED from inside an open conversation entirely (also by direct
// instruction): offering to start a redundant new conversation with a
// pipeline/coordinator you are already talking to was confusing, not useful.
// Starting a new conversation is the "New conversation" modal's job now.

const COMMAND_HINT: Record<string, string> = {
  exit: 'release this conversation’s runtime, keep the conversation',
  close: 'end this conversation and archive its thread',
}

const COMMAND_ICON: Record<string, ReactNode> = {
  exit: <SignOutAltIcon />,
  close: <ArchiveIcon />,
}

export interface QuickChipsProps {
  /** Writes are off entirely — console-quick-actions: "no start chip is shown". */
  canWrite: boolean
  /** The latest message's offered choices, if any. */
  choices?: Choice[]
  /** Runs a thread command (`/exit`, `/close`) immediately — never inserted
   * into the composer for the user to send themselves. `/close`'s own
   * confirmation gate lives in the sender, not here. */
  onRunCommand?: (text: string) => void
  /** Inserts text into the OPEN conversation's composer, for an offered choice
   * — absent when none is open. */
  onInsertCommand?: (text: string) => void
}

export function QuickChips({ canWrite, choices, onRunCommand, onInsertCommand }: Readonly<QuickChipsProps>) {
  const vocabulary = useVocabulary()
  if (!canWrite) return null
  const entries = vocabulary.data?.entries ?? []
  const threadCommands = onRunCommand
    ? entries.filter((e) => e.kind === 'builtin' && e.position === 'thread')
    : []
  const offered = choices ?? []

  if (threadCommands.length === 0 && offered.length === 0) return null

  return (
    <div
      data-testid="quick-chips"
      style={{ display: 'flex', gap: 6, flexWrap: 'wrap', alignItems: 'center' }}
    >
      {threadCommands.map((e) => (
        <Tooltip key={`cmd-${e.name}`} content={e.description || COMMAND_HINT[e.name] || e.name}>
          <Button
            variant="plain"
            size="sm"
            aria-label={`/${e.name}`}
            icon={COMMAND_ICON[e.name]}
            onClick={() => onRunCommand?.(`/${e.name}`)}
          />
        </Tooltip>
      ))}
      {offered.map((c) => (
        <Button key={`choice-${c.command}`} variant="secondary" size="sm" onClick={() => onInsertCommand?.(`${c.command} `)}>
          <PlainText>{c.label || c.command}</PlainText>
        </Button>
      ))}
    </div>
  )
}
