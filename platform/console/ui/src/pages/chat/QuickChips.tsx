import type { ReactNode } from 'react'
import { Button, Tooltip } from '@patternfly/react-core'
import { ArchiveIcon, SignOutAltIcon } from '@patternfly/react-icons'
import { useVocabulary } from '../../api/hooks'

// Ported from `prototype/States.html`'s chip row, with ONE explicit
// deviation from it: the user instructed directly, more than once, that
// thread commands are icons the user RUNS, never raw "/name" text the user
// has to insert and then send — overriding the prototype's own mono-text
// rendering of `/exit`/`/close`. Pipeline/coordinator "starter" chips are
// REMOVED from inside an open conversation entirely (also by direct
// instruction): offering to start a redundant new conversation with a
// pipeline/coordinator you are already talking to was confusing, not useful.
// Starting a new conversation is the "New conversation" modal's job now.
//
// An offered CHOICE chip used to render here too, for the latest message
// only. It moved into `Timeline.tsx`, WITH the message that offers it (B2:
// "the console shows each message's choices with that message") — this
// component now carries only the thread's own built-in commands, which have
// no message to render beside.

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
  /** Runs a thread command (`/exit`, `/close`) immediately — never inserted
   * into the composer for the user to send themselves. `/close`'s own
   * confirmation gate lives in the sender, not here. */
  onRunCommand?: (text: string) => void
}

export function QuickChips({ canWrite, onRunCommand }: Readonly<QuickChipsProps>) {
  const vocabulary = useVocabulary()
  if (!canWrite) return null
  const entries = vocabulary.data?.entries ?? []
  const threadCommands = onRunCommand
    ? entries.filter((e) => e.kind === 'builtin' && e.position === 'thread')
    : []

  if (threadCommands.length === 0) return null

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
    </div>
  )
}
