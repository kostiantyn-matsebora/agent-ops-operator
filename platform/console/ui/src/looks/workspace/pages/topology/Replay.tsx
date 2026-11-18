import type { ComponentProps } from 'react'
import { clock } from '../../../../graph/format'
import { INTERVALS, SPEEDS, describeNotHeld, frameHeld, frameTime } from '../../../../graph/replay'
import type { ConversationReplayBar, WindowReplayBar } from '../../../../graph/ReplayBars'
import { Plain } from './parts'

// Kiali's replay toolbar, in the workspace's controls: an interval, ten second
// frames, a slider, play and pause, three speeds — plus the one thing a mesh
// cannot do, one conversation hop by hop.

export function WindowReplay({
  window: w, interval, frame, playing, speed, onInterval, onFrame, onPlay, onSpeed, onClose,
}: ComponentProps<typeof WindowReplayBar>) {
  const t = frameTime(w, frame)
  const held = frameHeld(w, frame)
  return (
    <div data-testid="replay-bar">
      <div className="wk-tp-bar">
        <b>Replay</b>
        <select aria-label="replay interval" value={String(interval)} onChange={(e) => onInterval(Number(e.target.value))}>
          {INTERVALS.map((s) => <option key={s} value={String(s)}>{`Last ${s / 60} minute${s === 60 ? '' : 's'}`}</option>)}
        </select>
        <span>from {clock(w.start)}</span>
        <button type="button" className="wk-btn" onClick={() => onPlay(!playing)}>{playing ? '❚❚ Pause' : '▶ Play'}</button>
        <input type="range" aria-label="replay frame" min={0} max={w.frames} step={1} value={frame} onChange={(e) => onFrame(Number(e.target.value))} />
        <span data-testid="replay-time" className="wk-num">{clock(t)} [{frame}/{w.frames}]</span>
        <div className="wk-seg" role="group" aria-label="replay speed">
          {SPEEDS.map((s) => (
            <button key={s.label} type="button" aria-pressed={speed === s.ms} onClick={() => onSpeed(s.ms)}>{s.label}</button>
          ))}
        </div>
        <button type="button" className="wk-btn link" onClick={onClose} aria-label="close replay">✕ Close</button>
      </div>
      {w.notHeldMs > 0 && (
        <div className={`wk-tp-notice ${held ? 'info' : 'warn'} quiet`} role="status" data-testid="replay-not-held">
          <b>The activity buffer does not hold {describeNotHeld(w.notHeldMs)} of this window</b>
          <div>
            {held
              ? 'Frames before it are reported as not held, never drawn as quiet.'
              : 'This frame is before what the buffer holds: its edges show nothing because nothing is known, not because nothing happened.'}
          </div>
        </div>
      )}
    </div>
  )
}

export function ConversationReplay({
  name, list, current, playing, onPlay, onStep, onClose,
}: ComponentProps<typeof ConversationReplayBar>) {
  const cur = list[current]
  return (
    <div className="wk-tp-bar" data-testid="conversation-bar">
      <b>Replay conversation</b>
      <code><Plain>{name}</Plain></code>
      <button type="button" className="wk-btn" onClick={() => onPlay(!playing)} disabled={!list.length}>{playing ? '❚❚ Pause' : '▶ Play'}</button>
      <button type="button" className="wk-btn" onClick={() => onStep(current + 1)} disabled={current >= list.length - 1}>Step ›</button>
      <input
        type="range"
        aria-label="conversation hop"
        min={0}
        max={Math.max(0, list.length - 1)}
        step={1}
        value={Math.max(0, current)}
        onChange={(e) => onStep(Number(e.target.value))}
      />
      <span data-testid="conversation-time" className="wk-num">{cur ? `+${(cur.offsetMs / 1000).toFixed(1)}s · ${cur.ev.kind}` : 'no hops'}</span>
      <span className="wk-pill">compressed · real gaps shown as +s</span>
      <button type="button" className="wk-btn link" onClick={onClose} aria-label="leave conversation replay">✕ Back to live</button>
    </div>
  )
}
