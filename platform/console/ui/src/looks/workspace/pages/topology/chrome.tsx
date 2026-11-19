import type { ChipProps, FrameProps, GraphChrome, NoticeProps, ScopeBarProps, ZoomProps } from '../../../../graph/chrome'
import { Chevron } from '../../icons'
import { Conversation, Edge, Feed, Hop, Node } from './panels'
import { PipelineName, Plain } from './parts'
import { ConversationReplay, WindowReplay } from './Replay'
import { Display, Toolbar } from './Toolbar'

// THE WORKSPACE LOOK'S CHROME around the shared canvas.

const CHIP = { ok: 'ok', bad: 'bad', warn: 'warn', info: 'working', accent: 'acc', quiet: '' } as const

function Frame({ toolbar, replay, notices, canvas, display, panel, sideShown, onToggleSide }: FrameProps) {
  return (
    <div className="wk-tp" data-side={sideShown ? 'shown' : 'hidden'}>
      {toolbar}
      {replay}
      {notices}
      <div className="wk-tp-split">
        <div className="wk-tp-canvas">{canvas}</div>
        {/* The column beside the canvas folds to a strip holding the chevron
            that brings it back: the fold and the unfold are one control in one
            place. Inside it each card folds to its title the same way. */}
        <aside className="wk-tp-side" aria-label="Graph details">
          <button
            type="button"
            className="wk-btn wk-tp-sidebtn"
            aria-label={sideShown ? 'Hide side panels' : 'Show side panels'}
            aria-expanded={sideShown}
            title={sideShown ? 'Hide side panels' : 'Show side panels'}
            onClick={onToggleSide}
          >
            <Chevron dir={sideShown ? 'doubleRight' : 'doubleLeft'} />
          </button>
          {sideShown && (
            <div className="wk-tp-stack">
              {display}
              {panel}
            </div>
          )}
        </aside>
      </div>
    </div>
  )
}

function Notice({ tone, title, testId, plain, children }: NoticeProps) {
  return (
    <div className={`wk-tp-notice ${tone === 'warning' ? 'warn' : 'info'}${plain ? ' quiet' : ''}`} role="status" data-testid={testId}>
      <b>{title}</b>
      {children && <div>{children}</div>}
    </div>
  )
}

function Chip({ tone = 'quiet', testId, children }: ChipProps) {
  return <span className={`wk-pill ${CHIP[tone]}`.trim()} data-testid={testId}>{children}</span>
}

function ScopeBar({ name, levels, depth, maxDepth, beyond, onDepth, onReset }: ScopeBarProps) {
  return (
    <span className="wk-tp-scope" data-testid="scope-bar">
      <span className="wk-pill working">Scoped to <Plain>{name}</Plain></span>
      {levels.length > 0 && (
        <div className="wk-seg" role="group" aria-label="scope depth">
          {levels.map((l) => (
            <button
              key={String(l)}
              type="button"
              aria-label={l === 'all' ? 'all connected' : `${l} hop`}
              aria-pressed={l === 'all' ? depth === 'all' || (typeof depth === 'number' && depth >= maxDepth) : depth === l}
              onClick={() => onDepth(l)}
            >
              {l === 'all' ? 'All' : String(l)}
            </button>
          ))}
        </div>
      )}
      {beyond > 0 && <span className="wk-pill">{beyond} connected beyond this depth</span>}
      <button type="button" className="wk-btn link" onClick={onReset} aria-label="reset scope">Reset scope</button>
    </span>
  )
}

function Zoom({ percent, onIn, onOut, onFit }: ZoomProps) {
  return (
    <div className="wk-tp-zoom">
      <button type="button" className="wk-btn" aria-label="zoom in" title="Zoom in" onClick={onIn}>+</button>
      <button type="button" className="wk-btn" aria-label="zoom out" title="Zoom out" onClick={onOut}>−</button>
      <button type="button" className="wk-btn" aria-label="fit to view" title="Fit to view" onClick={onFit}>⤢</button>
      <span aria-label="zoom level" className="wk-note wk-num">{percent}%</span>
    </div>
  )
}

export const workspaceChrome: GraphChrome = {
  Frame, Notice, Chip, ScopeBar, Zoom, PipelineName,
  Toolbar,
  Display,
  WindowReplay,
  ConversationReplay,
  Feed,
  Node,
  Edge,
  Hop,
  Conversation,
}
