import { type KeyboardEvent, type ReactElement, useId, useRef, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { Chevron, Icon } from '../icons'
import type { VocabularyEntry } from '../../../api/types'
import { resolveIcon } from '../../../model/icon'
import { filterEntries, useNewConversation } from '../../../model/newConversation'
import { TESTID, testid } from '../../../model/testids'
import './NewConversation.css'

// "New conversation", as the mockup composes it: a Draft row at the top of the
// list and, in the thread pane, the destinations as chips over a task box. No modal.
//
// Same facts as the classic look, drawn the workspace way: every Ready
// pipeline and coordinator is a destination card, visible the moment the
// dialog opens; a filter narrows them by name (a substring match, never a
// slash parser); choosing a card only PREPARES the addressed `/<name> ` form
// that Start prepends. "Answered by" names the source this console originates
// from and renders in four states rather than vanishing — wired, claimable
// (the exact patch), not originating (the CR that grants the identity), no
// identity — because "no button" and "unavailable, here is why" are different
// messages and only one is actionable.

function DestinationIcon({ entry }: Readonly<{ entry: VocabularyEntry }>): ReactElement {
  const kind = entry.kind === 'coordinator' ? 'co' : 'pl'
  const icon = resolveIcon(entry.icon)
  let glyph: ReactElement | string = entry.kind === 'coordinator' ? '◆' : '⚙'
  if (icon?.kind === 'path') {
    glyph = (
      <svg viewBox="0 0 24 24" className="wk-nc-svg">
        <path d={icon.d} />
      </svg>
    )
  } else if (icon?.kind === 'image') glyph = <img src={icon.src} alt="" className="wk-nc-img" />
  else if (icon?.kind === 'text') glyph = icon.text
  return (
    <span className={`wk-nc-av ${kind}`} aria-hidden="true">
      {glyph}
    </span>
  )
}

function Unavailable({
  availability, externalAuthenticator, unwired,
}: Readonly<{
  availability: string
  externalAuthenticator: string
  unwired: ReturnType<typeof useNewConversation>['unwired']
}>): ReactElement {
  const testId = availability === 'unwired' ? TESTID.newConversationUnavailable : undefined
  return (
    <div className="wk-nc-unavail" data-testid={testId} data-reason={availability} role="status">
      {availability === 'loading' && <p>Checking what this console can start…</p>}
      {availability === 'read-only' && (
        <>
          <b>This console is read-only</b>
          <p>console.write.enabled is false, so both write paths are refused server-side.</p>
        </>
      )}
      {availability === 'no-identity' && (
        <>
          <b>Nobody said who you are</b>
          <p>
            {externalAuthenticator || 'The proxy'} in front of this console authenticated you but forwarded no identity
            header, and a conversation is recorded against the person who started it. Configure it to forward
            X-Forwarded-Email or X-Auth-Request-User.
          </p>
        </>
      )}
      {availability === 'not-originating' && (
        <>
          <b>This console holds no signal identity</b>
          <p>
            It can carry and reply to conversations, but not start them. Declare a SignalAdapter served by this
            ChannelAdapter, plus a SignalSource.
          </p>
        </>
      )}
      {availability === 'unwired' && (
        <>
          <b>Nothing is wired to answer yet</b>
          {unwired.map((s) => (
            <div key={s.name}>
              <p>{s.message || `no Ready Pipeline claims ${s.name}`}</p>
              {s.patch && <pre className="wk-nc-patch">{s.patch}</pre>}
            </div>
          ))}
        </>
      )}
    </div>
  )
}

export interface Draft {
  task: string
  selected: string | null
}

export const EMPTY_DRAFT: Draft = { task: '', selected: null }

/** The list's Draft row: shown while a new conversation is being composed. */
export function DraftRow({ draft, current }: Readonly<{ draft: Draft; current: boolean }>): ReactElement {
  return (
    <li className={`wk-row wk-draft${current ? ' current' : ''}`} data-testid="draft-row" aria-current={current ? 'true' : undefined}>
      <Link className="open" to="/conversations?new=1" aria-label="New conversation, draft" />
      <span aria-hidden="true" />
      <span className="wk-av" aria-hidden="true">
        <Icon name="bot" />
      </span>
      <div className="txt">
        <span className="name"><i>New conversation</i></span>
        <span className="l2">{draft.task.trim() || 'Choose a destination and describe the task'}</span>
      </div>
      <div className="st">
        <span className="wk-pill">Draft</span>
      </div>
    </li>
  )
}

/** The thread pane while composing a new conversation. */
export function NewConversationPane({
  draft, onDraft, onBack,
}: Readonly<{ draft: Draft; onDraft: (d: Draft) => void; onBack?: () => void }>): ReactElement {
  const taskRef = useRef<HTMLTextAreaElement>(null)
  const chipsRef = useRef<HTMLDivElement>(null)
  const titleId = useId()
  const navigate = useNavigate()
  const nc = useNewConversation()
  const [filter, setFilter] = useState('')
  const [error, setError] = useState<string>()
  const { task, selected } = draft

  const filtered = filterEntries(nc.startable, filter)

  function onChipKeyDown(e: KeyboardEvent<HTMLButtonElement>) {
    if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp' && e.key !== 'ArrowRight' && e.key !== 'ArrowLeft') return
    e.preventDefault()
    if (filtered.length === 0) return
    const forward = e.key === 'ArrowDown' || e.key === 'ArrowRight'
    const current = Math.max(0, filtered.findIndex((x) => x.name === selected))
    const next = forward ? (current + 1) % filtered.length : (current - 1 + filtered.length) % filtered.length
    onDraft({ ...draft, selected: filtered[next].name })
    requestAnimationFrame(() => {
      chipsRef.current?.querySelectorAll<HTMLButtonElement>('button[role="radio"]')[next]?.focus()
    })
  }

  async function start() {
    if (nc.busy || !task.trim()) return
    setError(undefined)
    const outcome = await nc.start(task, selected)
    if (!outcome.ok) {
      setError(outcome.error)
      return
    }
    onDraft(EMPTY_DRAFT)
    // The response names the source, not the conversation — it does not exist
    // until the manager opens it — so land on the list, where it appears first.
    void navigate('/conversations')
  }

  function onTaskKeyDown(e: KeyboardEvent<HTMLTextAreaElement>) {
    // SHIFT+ENTER SENDS — the one keystroke that means "I am done".
    if (e.key === 'Enter' && e.shiftKey) {
      e.preventDefault()
      void start()
    }
  }

  const ready = nc.availability === 'ready'
  const chosen = nc.startable.find((x) => x.name === selected)

  return (
    <>
      <div className="wk-th">
        {onBack && (
          <button type="button" className="wk-icon-btn back" aria-label="Back to the list" onClick={onBack}>
            <Chevron dir="left" />
          </button>
        )}
        <h2 id={titleId}>New conversation</h2>
      </div>
      {!ready && <Unavailable availability={nc.availability} externalAuthenticator={nc.externalAuthenticator} unwired={nc.unwired} />}
      {ready && (
        <>
          <section className="wk-nc-dest" aria-label="Destination">
            {nc.vocabularyLoading && <small className="wk-nc-note">Loading what can be addressed…</small>}
            {!nc.vocabularyLoading && nc.startable.length === 0 && (
              <small className="wk-nc-note" data-testid="destination-empty">
                Nothing is directly addressable yet — the task below is sent unaddressed.
              </small>
            )}
            {!nc.vocabularyLoading && nc.startable.length > 0 && (
              <>
                <div ref={chipsRef} className="wk-dchips" role="radiogroup" aria-label="Pick a destination" data-testid={TESTID.destinationCards}>
                  {filtered.map((entry, i) => {
                    const isSelected = selected === entry.name
                    return (
                      <button
                        key={entry.name}
                        type="button"
                        role="radio"
                        className="wk-dchip"
                        aria-checked={isSelected}
                        tabIndex={(selected ? isSelected : i === 0) ? 0 : -1}
                        data-testid={testid.destination(entry.name)}
                        data-destination={entry.name}
                        title={entry.description || undefined}
                        onClick={() => onDraft({ ...draft, selected: entry.name })}
                        onKeyDown={onChipKeyDown}
                      >
                        <DestinationIcon entry={entry} />
                        <span className="wk-nc-t">
                          <b>{entry.name}</b>
                          <span>{entry.kind}</span>
                        </span>
                      </button>
                    )
                  })}
                  {filtered.length === 0 && (
                    <small className="wk-nc-note" data-testid="destination-no-matches">
                      No match for “{filter}”.
                    </small>
                  )}
                </div>
                {(
                  <input
                    type="text"
                    className="wk-nc-input"
                    aria-label="Filter destinations by name"
                    placeholder="Filter by name…"
                    value={filter}
                    onChange={(e) => setFilter(e.target.value)}
                    data-testid={TESTID.destinationFilter}
                  />
                )}
              </>
            )}
            <div className="wk-nc-meta" aria-label="Answered by">
              <span>
                Answered by{' '}
                {nc.wired.length === 0 ? <b>the wired pipeline</b> : nc.wired.map((s, i) => (
                  <span key={s.name}>
                    {i > 0 && '; '}
                    <b>{s.name}</b> → pipeline <b>{s.pipeline}</b>
                    {s.profile && <>, profile <b>{s.profile}</b></>}
                  </span>
                ))}
              </span>
              <small className="wk-nc-note">
                {nc.ambiguous
                  ? 'Several Pipelines serve this source — pick a destination above. An unaddressed task is refused rather than guessed.'
                  : 'The Pipeline serving this source answers — the console cannot reach a pipeline no wiring points at.'}
              </small>
            </div>
          </section>
          <div className="wk-nc-stage">
            <h3>{chosen ? `Start a conversation with ${chosen.name}` : 'Start a conversation'}</h3>
            <p>{chosen ? 'Describe the task below.' : 'Pick a destination above, then describe the task below.'}</p>
            {error && (
              <div className="wk-alert danger" role="alert">
                {error}
              </div>
            )}
          </div>
          <form
            className="wk-compose"
            onSubmit={(e) => {
              e.preventDefault()
              void start()
            }}
          >
            <div className="wk-compose-wrap">
              <textarea
                ref={taskRef}
                aria-label="task"
                placeholder="What should the agent do?"
                value={task}
                onChange={(e) => onDraft({ ...draft, task: e.target.value })}
                onKeyDown={onTaskKeyDown}
              />
            </div>
            <span className="h">
              <span><kbd>Shift</kbd> + <kbd>Enter</kbd> send</span>
            </span>
            <button type="submit" className="wk-btn primary" disabled={nc.busy || !task.trim()} data-testid={TESTID.startConversation}>
              {nc.busy ? 'Starting…' : chosen ? `Start with ${chosen.name}` : 'Start'}
            </button>
          </form>
        </>
      )}
    </>
  )
}
