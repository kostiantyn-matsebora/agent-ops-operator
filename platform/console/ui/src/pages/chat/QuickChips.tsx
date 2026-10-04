import { Button, Tooltip } from '@patternfly/react-core'
import { useVocabulary } from '../../api/hooks'
import { Icon } from '../../components/Icon'
import { PlainText } from '../../components/Text'
import { useComposerIntent } from './composerIntent'
import type { Choice } from '../../api/types'

// Ported from `prototype/States.html` ("Quick chips: Ready pipelines to
// start with, thread commands, and the last message's choices[]") and
// `D-incident.html`'s chip row — one row, three sources, read from the
// `console-quick-actions` spec.

const COMMAND_HINT: Record<string, string> = {
  exit: 'release this conversation’s runtime, keep the conversation',
  close: 'end this conversation and archive its thread',
}

export interface QuickChipsProps {
  /** Writes are off entirely — console-quick-actions: "no start chip is shown". */
  canWrite: boolean
  /** A Ready Pipeline or Coordinator claims a source this console can originate from. */
  canStart: boolean
  /** The latest message's offered choices, if any. */
  choices?: Choice[]
  /** Inserts text into the OPEN conversation's composer — absent when none is open. */
  onInsertCommand?: (text: string) => void
}

export function QuickChips({ canWrite, canStart, choices, onInsertCommand }: QuickChipsProps) {
  const vocabulary = useVocabulary()
  if (!canWrite) return null
  const entries = vocabulary.data?.entries ?? []
  const starters = canStart
    ? entries.filter((e) => (e.kind === 'pipeline' || e.kind === 'coordinator') && e.position === 'general')
    : []
  const threadCommands = onInsertCommand
    ? entries.filter((e) => e.kind === 'builtin' && e.position === 'thread')
    : []
  const offered = choices ?? []

  if (starters.length === 0 && threadCommands.length === 0 && offered.length === 0) return null

  return (
    <div
      data-testid="quick-chips"
      style={{ display: 'flex', gap: 6, flexWrap: 'wrap', alignItems: 'center' }}
    >
      {starters.map((e) => (
        <Button
          key={`start-${e.name}`}
          variant="secondary"
          size="sm"
          icon={e.icon ? <Icon icon={e.icon} /> : undefined}
          onClick={() => useComposerIntent.getState().openWith(`/${e.name} `)}
        >
          <PlainText>{e.name}</PlainText>
        </Button>
      ))}
      {threadCommands.map((e) => (
        <Tooltip key={`cmd-${e.name}`} content={e.description || COMMAND_HINT[e.name] || e.name}>
          <Button
            variant="secondary"
            size="sm"
            style={{ fontFamily: 'var(--pf-t--global--font--family--mono)' }}
            onClick={() => onInsertCommand?.(`/${e.name}`)}
          >
            {`/${e.name}`}
          </Button>
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
