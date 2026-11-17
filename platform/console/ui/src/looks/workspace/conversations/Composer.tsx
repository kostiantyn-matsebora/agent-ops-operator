import { type FormEvent, type KeyboardEvent, type ReactElement, useRef, useState } from 'react'
import type { VocabularyEntry } from '../../../api/types'
import { matchEntries } from '../../../model/slash'
import { TESTID } from '../../../model/testids'

// THE REPLY BOX: a textarea sent with Shift+Enter, the slash-typed command
// listing above it, and the thread's own commands as chips beneath. What the
// listing shows for the text typed is the model's (model/slash.ts).

const CHIP_HINT: Record<string, string> = {
  exit: 'Release this conversation’s runtime, keep the conversation',
  close: 'End this conversation and archive its thread',
}

export function Composer({
  onSend, onCommand, vocabulary = [], disabled,
}: Readonly<{
  onSend: (text: string) => void
  /** Runs a thread command at once, never by writing it into the box. */
  onCommand?: (command: '/exit' | '/close') => void
  vocabulary?: VocabularyEntry[]
  disabled?: boolean
}>): ReactElement {
  const [text, setText] = useState('')
  const [dismissed, setDismissed] = useState(false)
  const [cursor, setCursor] = useState(0)
  const box = useRef<HTMLTextAreaElement>(null)
  const options = dismissed ? null : matchEntries(text, vocabulary, 'thread')
  const active = options ? Math.min(cursor, options.length - 1) : 0
  const chips = onCommand ? vocabulary.filter((e) => e.kind === 'builtin' && e.position === 'thread') : []

  function submit(e?: FormEvent) {
    e?.preventDefault()
    const t = text.trim()
    if (!t || disabled) return
    onSend(t)
    setText('')
  }
  function choose(entry: VocabularyEntry) {
    setText(`/${entry.name}`)
    setDismissed(true)
    setCursor(0)
    box.current?.focus()
  }
  function onKeyDown(e: KeyboardEvent<HTMLTextAreaElement>) {
    if (e.key === 'Enter' && e.shiftKey) {
      e.preventDefault()
      submit()
      return
    }
    if (!options) return
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault()
      setCursor((c) => (c + (e.key === 'ArrowDown' ? 1 : options.length - 1)) % options.length)
    } else if (e.key === 'Enter' || e.key === 'Tab') {
      e.preventDefault()
      choose(options[active])
    } else if (e.key === 'Escape') {
      e.preventDefault()
      setDismissed(true)
    }
  }

  return (
    <form className="wk-compose" onSubmit={submit}>
      {chips.length > 0 && (
        <div className="wk-chips" data-testid={TESTID.quickChips}>
          {chips.map((e) => (
            <button
              key={e.name}
              type="button"
              className="wk-btn"
              disabled={disabled}
              title={e.description || CHIP_HINT[e.name] || e.name}
              aria-label={`/${e.name}`}
              onClick={() => onCommand?.(`/${e.name}` as '/exit' | '/close')}
            >
              {`/${e.name}`}
            </button>
          ))}
        </div>
      )}
      <div className="wk-compose-wrap">
        {options && (
          <ul className="wk-typeahead" role="listbox" aria-label="Commands" data-testid={TESTID.commandTypeahead}>
            {options.map((o, i) => (
              <li key={o.name} role="none">
                <button type="button" role="option" aria-selected={i === active} onMouseDown={(ev) => ev.preventDefault()} onClick={() => choose(o)}>
                  <code>/{o.name}</code>
                  {o.description && <span className="d">{o.description}</span>}
                </button>
              </li>
            ))}
          </ul>
        )}
        <textarea
          ref={box}
          aria-label="message"
          placeholder="Reply to the agent…"
          value={text}
          onChange={(e) => {
            setText(e.target.value)
            setDismissed(false)
            setCursor(0)
          }}
          onKeyDown={onKeyDown}
        />
      </div>
      <span className="h">
        <span><kbd>/</kbd> commands</span>
        <span><kbd>Shift</kbd> + <kbd>Enter</kbd> send</span>
      </span>
      <button className="wk-btn primary" type="submit" disabled={disabled || !text.trim()}>
        Send
      </button>
    </form>
  )
}
