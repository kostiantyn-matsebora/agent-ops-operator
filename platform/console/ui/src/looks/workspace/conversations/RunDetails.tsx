import { type ReactElement, useState } from 'react'
import type { RunDetails as RunDetailsFact } from '../../../model/run'
import { runDuration, runEventRows } from '../../../model/run'
import { clock, duration, toolDuration } from '../format'

// EVERYTHING ABOUT A FINISHED RUN, BEHIND ONE COLLAPSED LINE
// (console-workspace-look: "Run detail is folded").
//
// The line states the run's duration. Opened, it shows the tool calls as one
// expandable row, then the run's lifecycle events — the checkpoint among them.
// No row repeats a fact another row already states: the duration is said once,
// on the line itself, so the tools row carries only the count.

export const RUN_DETAILS_TESTID = 'run-details'

export function RunDetails({ details }: Readonly<{ details: RunDetailsFact }>): ReactElement | null {
  const [open, setOpen] = useState(false)
  const [toolsOpen, setToolsOpen] = useState(false)
  const took = duration(runDuration(details))
  const events = runEventRows(details)
  const tools = details.tools
  const openable = tools.length > 0 || events.length > 0
  // Nothing to say at all: no line, rather than an empty one.
  if (!openable && !took) return null

  return (
    <div data-testid={RUN_DETAILS_TESTID} data-expanded={open}>
      <div className="wk-rm">
        <span>
          Run details{took && <> · <span className="wk-num">{took}</span></>}
        </span>
        {openable && (
          <button type="button" className="wk-lnk" aria-expanded={open} onClick={() => setOpen((v) => !v)}>
            {open ? 'collapse' : 'show'}
          </button>
        )}
      </div>
      {open && (
        <>
          {tools.length > 0 && (
            <>
              <div className="wk-rm sub">
                <span>
                  Ran {tools.length} tool{tools.length === 1 ? '' : 's'}
                </span>
                <button
                  type="button"
                  className="wk-lnk"
                  aria-expanded={toolsOpen}
                  onClick={() => setToolsOpen((v) => !v)}
                >
                  {toolsOpen ? 'collapse' : 'show'}
                </button>
              </div>
              {toolsOpen && (
                <dl className="wk-tools-list" aria-label="tool calls">
                  {tools.map((t, i) => (
                    // A tool may be called twice in one run, so its position keys it.
                    <div key={i} style={{ display: 'contents' }}>
                      <dt>{t.tool || 'unnamed tool'}</dt>
                      <dd className="wk-num">{toolDuration(t.durationMs)}</dd>
                    </div>
                  ))}
                </dl>
              )}
            </>
          )}
          {events.map((e, i) => (
            <div key={`${e.kind}-${i}`} className="wk-rm sub" data-event={e.kind}>
              <span>{e.label}</span>
              <span className="wk-num">{clock(e.at)}</span>
            </div>
          ))}
        </>
      )}
    </div>
  )
}
