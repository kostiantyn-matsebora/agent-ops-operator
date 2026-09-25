import { Alert, Button, FormSelect, FormSelectOption, Label, ToggleGroup, ToggleGroupItem } from '@patternfly/react-core'
import { PlainText } from '../components/Text'
import { clock } from './Panel'
import { INTERVALS, SPEEDS, describeNotHeld, frameHeld, frameTime, type ReplayWindow, type Step } from './replay'

// Kiali's replay toolbar, exactly: an interval, ten second frames, a slider,
// play and pause, three speeds — plus the one thing a mesh cannot do, one
// conversation hop by hop.

const bar = {
  display: 'flex', flexWrap: 'wrap' as const, gap: '8px 12px', alignItems: 'center',
  border: '1px solid var(--ao-brand)', borderRadius: 8, padding: '10px 12px', background: 'var(--ao-surface)',
}

export function WindowReplayBar({
  window: w, interval, frame, playing, speed, onInterval, onFrame, onPlay, onSpeed, onClose,
}: {
  window: ReplayWindow
  interval: number
  frame: number
  playing: boolean
  speed: number
  onInterval: (s: number) => void
  onFrame: (f: number) => void
  onPlay: (on: boolean) => void
  onSpeed: (ms: number) => void
  onClose: () => void
}) {
  const t = frameTime(w, frame)
  return (
    <div data-testid="replay-bar">
      <div style={bar}>
        <strong style={{ color: 'var(--ao-brand-strong)' }}>Replay</strong>
        <FormSelect aria-label="replay interval" value={String(interval)} onChange={(_e, v) => onInterval(Number(v))} style={{ width: 170 }}>
          {INTERVALS.map((s) => (
            <FormSelectOption key={s} value={String(s)} label={`Last ${s / 60} minute${s === 60 ? '' : 's'}`} />
          ))}
        </FormSelect>
        <span>from {clock(w.start)}</span>
        <Button size="sm" variant="secondary" onClick={() => onPlay(!playing)}>{playing ? '❚❚ Pause' : '▶ Play'}</Button>
        <input
          type="range"
          aria-label="replay frame"
          min={0}
          max={w.frames}
          step={1}
          value={frame}
          onChange={(e) => onFrame(Number(e.target.value))}
          style={{ flex: '1 1 220px', minWidth: 160, accentColor: 'var(--ao-brand)' }}
        />
        <span data-testid="replay-time" style={{ fontVariantNumeric: 'tabular-nums' }}>
          {clock(t)} [{frame}/{w.frames}]
        </span>
        <ToggleGroup aria-label="replay speed">
          {SPEEDS.map((s) => (
            <ToggleGroupItem key={s.label} text={s.label} isSelected={speed === s.ms} onChange={() => onSpeed(s.ms)} />
          ))}
        </ToggleGroup>
        <Button size="sm" variant="link" onClick={onClose} aria-label="close replay">✕ Close</Button>
      </div>
      {w.notHeldMs > 0 && (
        <Alert
          variant={frameHeld(w, frame) ? 'info' : 'warning'}
          isInline
          isPlain
          data-testid="replay-not-held"
          title={`The activity buffer does not hold ${describeNotHeld(w.notHeldMs)} of this window`}
        >
          {frameHeld(w, frame)
            ? 'Frames before it are reported as not held, never drawn as quiet.'
            : 'This frame is before what the buffer holds: its edges show nothing because nothing is known, not because nothing happened.'}
        </Alert>
      )}
    </div>
  )
}

export function ConversationReplayBar({
  name, list, current, playing, onPlay, onStep, onClose,
}: {
  name: string
  list: Step[]
  current: number
  playing: boolean
  onPlay: (on: boolean) => void
  onStep: (i: number) => void
  onClose: () => void
}) {
  const cur = list[current]
  return (
    <div style={bar} data-testid="conversation-bar">
      <strong style={{ color: 'var(--ao-brand-strong)' }}>Replay conversation</strong>
      <code><PlainText>{name}</PlainText></code>
      <Button size="sm" variant="secondary" onClick={() => onPlay(!playing)} isDisabled={!list.length}>
        {playing ? '❚❚ Pause' : '▶ Play'}
      </Button>
      <Button size="sm" variant="secondary" onClick={() => onStep(current + 1)} isDisabled={current >= list.length - 1}>Step ›</Button>
      <input
        type="range"
        aria-label="conversation hop"
        min={0}
        max={Math.max(0, list.length - 1)}
        step={1}
        value={Math.max(0, current)}
        onChange={(e) => onStep(Number(e.target.value))}
        style={{ flex: '1 1 220px', minWidth: 160, accentColor: 'var(--ao-brand)' }}
      />
      <span data-testid="conversation-time" style={{ fontVariantNumeric: 'tabular-nums' }}>
        {cur ? `+${(cur.offsetMs / 1000).toFixed(1)}s · ${cur.ev.kind}` : 'no hops'}
      </span>
      <Label isCompact>compressed · real gaps shown as +s</Label>
      <Button size="sm" variant="link" onClick={onClose} aria-label="leave conversation replay">✕ Back to live</Button>
    </div>
  )
}
