import { type KeyboardEvent, type ReactElement, useEffect, useId, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import type { VocabularyEntry } from '../../../api/types'
import { resolveIcon } from '../../../model/icon'
import { filterEntries, useNewConversation } from '../../../model/newConversation'
import { TESTID, testid } from '../../../model/testids'
import './NewConversation.css'

// "New conversation", as a dialog over the workspace.
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

export function NewConversationDialog({ open, onClose }: Readonly<{ open: boolean; onClose: () => void }>): ReactElement {
  const ref = useRef<HTMLDialogElement>(null)
  const taskRef = useRef<HTMLTextAreaElement>(null)
  const cardsRef = useRef<HTMLDivElement>(null)
  const titleId = useId()
  const navigate = useNavigate()
  const nc = useNewConversation()
  const [task, setTask] = useState('')
  const [selected, setSelected] = useState<string | null>(null)
  const [filter, setFilter] = useState('')
  const [error, setError] = useState<string>()

  useEffect(() => {
    const el = ref.current
    if (!el) return
    if (open && !el.open) {
      setError(undefined)
      el.showModal?.()
      taskRef.current?.focus()
    }
    if (!open && el.open) el.close?.()
  }, [open])

  const filtered = filterEntries(nc.startable, filter)

  function onCardKeyDown(e: KeyboardEvent<HTMLButtonElement>) {
    if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp') return
    e.preventDefault()
    if (filtered.length === 0) return
    const current = Math.max(0, filtered.findIndex((x) => x.name === selected))
    const next = e.key === 'ArrowDown' ? (current + 1) % filtered.length : (current - 1 + filtered.length) % filtered.length
    setSelected(filtered[next].name)
    requestAnimationFrame(() => {
      cardsRef.current?.querySelectorAll<HTMLButtonElement>('button[role="radio"]')[next]?.focus()
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
    setTask('')
    setSelected(null)
    setFilter('')
    onClose()
    // The response names the source, not the conversation — it does not exist
    // until the manager opens it — so land on the list, where it appears first.
    navigate('/conversations')
  }

  function onTaskKeyDown(e: KeyboardEvent<HTMLTextAreaElement>) {
    // SHIFT+ENTER SENDS — the one keystroke that means "I am done".
    if (e.key === 'Enter' && e.shiftKey) {
      e.preventDefault()
      void start()
    }
  }

  const ready = nc.availability === 'ready'

  return (
    <dialog
      ref={ref}
      className="wk-nc"
      aria-labelledby={titleId}
      onClose={onClose}
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose()
      }}
      
    >
      <div className="wk-nc-head">
        <h2 id={titleId}>Start a conversation</h2>
        <button type="button" className="wk-btn wk-nc-x" aria-label="Close" onClick={onClose}>
          ✕
        </button>
      </div>
      {!ready && (
        <>
          <Unavailable availability={nc.availability} externalAuthenticator={nc.externalAuthenticator} unwired={nc.unwired} />
          <div className="wk-nc-foot">
            <button type="button" className="wk-btn" onClick={onClose}>
              Close
            </button>
          </div>
        </>
      )}
      {ready && (
        <form
          onSubmit={(e) => {
            e.preventDefault()
            void start()
          }}
        >
          <div className="wk-nc-body">
            <section className="wk-nc-fg" aria-label="Answered by">
              <b>Answered by</b>
              {nc.wired.map((s) => (
                <div key={s.name}>
                  <span className="wk-nc-chip">{s.name}</span> → pipeline <b>{s.pipeline}</b>
                  {s.profile && (
                    <>
                      , profile <b>{s.profile}</b>
                    </>
                  )}
                </div>
              ))}
              <small className="wk-nc-note">
                {nc.ambiguous
                  ? 'Several Pipelines serve this source — pick a destination below. An unaddressed task is refused rather than guessed.'
                  : 'The Pipeline serving this source answers — the console cannot reach a pipeline no wiring points at.'}
              </small>
            </section>
            <section className="wk-nc-fg" aria-label="Destination">
              <b>Destination</b>
              {nc.vocabularyLoading && <small className="wk-nc-note">Loading what can be addressed…</small>}
              {!nc.vocabularyLoading && nc.startable.length === 0 && (
                <small className="wk-nc-note" data-testid="destination-empty">
                  Nothing is directly addressable yet — the task below is sent unaddressed.
                </small>
              )}
              {!nc.vocabularyLoading && nc.startable.length > 0 && (
                <>
                  <div
                    ref={cardsRef}
                    className="wk-nc-cards"
                    role="radiogroup"
                    aria-label="Pick a destination"
                    data-testid={TESTID.destinationCards}
                  >
                    {filtered.map((entry, i) => {
                      const isSelected = selected === entry.name
                      return (
                        <button
                          key={entry.name}
                          type="button"
                          role="radio"
                          className="wk-nc-card"
                          aria-checked={isSelected}
                          tabIndex={(selected ? isSelected : i === 0) ? 0 : -1}
                          data-testid={testid.destination(entry.name)}
                          data-destination={entry.name}
                          onClick={() => setSelected(entry.name)}
                          onKeyDown={onCardKeyDown}
                        >
                          <DestinationIcon entry={entry} />
                          <span className="wk-nc-t">
                            <b>{entry.name}</b>
                            {(entry.description || entry.profile) && <span>{entry.description || entry.profile}</span>}
                          </span>
                          <span className={`wk-nc-tag ${entry.kind === 'coordinator' ? 'co' : ''}`}>{entry.kind}</span>
                        </button>
                      )
                    })}
                    {filtered.length === 0 && (
                      <small className="wk-nc-note" data-testid="destination-no-matches">
                        No match for “{filter}”.
                      </small>
                    )}
                  </div>
                  <input
                    type="text"
                    className="wk-nc-input"
                    aria-label="Filter destinations by name"
                    placeholder="Filter by name…"
                    value={filter}
                    onChange={(e) => setFilter(e.target.value)}
                    data-testid={TESTID.destinationFilter}
                  />
                </>
              )}
            </section>
            <section className="wk-nc-fg">
              <label htmlFor={`${titleId}-task`}>
                <b>Task</b> <span className="wk-nc-req" aria-hidden="true">*</span>
              </label>
              <textarea
                ref={taskRef}
                id={`${titleId}-task`}
                className="wk-nc-input"
                rows={5}
                required
                aria-label="task"
                placeholder="What should the agent do?"
                value={task}
                onChange={(e) => setTask(e.target.value)}
                onKeyDown={onTaskKeyDown}
              />
              <small className="wk-nc-note">
                <kbd>Shift</kbd> + <kbd>Enter</kbd> send
              </small>
            </section>
            {error && (
              <div className="wk-nc-error" role="alert">
                {error}
              </div>
            )}
          </div>
          <div className="wk-nc-foot">
            <button type="submit" className="wk-btn primary" disabled={nc.busy || !task.trim()} data-testid={TESTID.startConversation}>
              {nc.busy ? 'Starting…' : selected ? `Start with ${selected}` : 'Start'}
            </button>
            <button type="button" className="wk-btn" onClick={onClose}>
              Cancel
            </button>
          </div>
        </form>
      )}
    </dialog>
  )
}
