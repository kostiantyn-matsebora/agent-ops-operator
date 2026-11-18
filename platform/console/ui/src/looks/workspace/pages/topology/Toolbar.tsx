import { useEffect, useId, useRef, useState, type ReactNode } from 'react'
import { WINDOWS, type ToolbarProps } from '../../../../graph/chrome'
import { useDisplay } from '../../../../graph/display'
import type { HiddenSummary } from '../../../../graph/filter'
import type { EdgeLabel } from '../../../../graph/hops'
import { LAYOUTS, type LayoutId } from '../../../../graph/layout'
import { BOX_OPTIONS, type BoxBy } from '../../../../graph/layout/boxes'
import { plural, styleFor } from '../../../../graph/shapes'
import { VIEWS, type ViewId } from '../../../../graph/types'
import type { Detail } from '../../../../graph/views'
import { SPINE_CLASS, VIEW_CLASSES } from '../../../../graph/views/classes'
import { Chevron } from '../../icons'
import { Fold, PipelineIcon, Plain } from './parts'

// The graph's toolbar in the workspace's own controls: two rows, as the
// prototype draws them — the view, the pipelines, the detail, the grouping and
// the layout; then find, hide, the window and live or replay. Everything here
// but find, hide and the scope is persisted per view by the display store.

function Seg({ label, children }: Readonly<{ label: string; children: ReactNode }>) {
  return <div className="wk-seg" role="group" aria-label={label}>{children}</div>
}

/** The pipelines selector: a button over a small checkbox popover, closed by Escape or a click away. */
function Pipelines({ pipelines, iconFor }: Readonly<Pick<ToolbarProps, 'pipelines' | 'iconFor'>>) {
  const d = useDisplay()
  const [open, setOpen] = useState(false)
  const root = useRef<HTMLDivElement>(null)
  const id = useId()
  const chosen = d.pipelines ?? pipelines
  useEffect(() => {
    if (!open) return
    const away = (e: PointerEvent) => !root.current?.contains(e.target as Node) && setOpen(false)
    const esc = (e: KeyboardEvent) => e.key === 'Escape' && setOpen(false)
    document.addEventListener('pointerdown', away)
    document.addEventListener('keydown', esc)
    return () => {
      document.removeEventListener('pointerdown', away)
      document.removeEventListener('keydown', esc)
    }
  }, [open])
  const toggle = (p: string) => {
    const next = chosen.includes(p) ? chosen.filter((x) => x !== p) : [...chosen, p]
    d.setPipelines(next.length === pipelines.length && pipelines.every((x) => next.includes(x)) ? null : next)
  }
  return (
    <div className="wk-tp-pop" ref={root}>
      <button type="button" className="wk-btn" aria-label="pipelines" aria-expanded={open} aria-controls={id} onClick={() => setOpen(!open)}>
        Pipelines: {d.pipelines === null ? 'all' : `${chosen.length} of ${pipelines.length}`}
        <Chevron dir="down" />
      </button>
      {open && (
        <div id={id} className="wk-tp-menu" role="group" aria-label="pipelines to show">
          {pipelines.length === 0 && <p className="wk-note">No pipelines.</p>}
          {pipelines.map((p) => (
            <label key={p} className="wk-tp-check">
              <input type="checkbox" checked={chosen.includes(p)} onChange={() => toggle(p)} />
              <PipelineIcon icon={iconFor(p)} />
              <Plain>{p}</Plain>
            </label>
          ))}
          <button type="button" className="wk-btn link" onClick={() => d.setPipelines(null)}>Select all</button>
        </div>
      )}
    </div>
  )
}

export function Toolbar({ pipelines, iconFor, find, hide, onFind, onHide, mode, onMode }: ToolbarProps) {
  const d = useDisplay()
  const view = d.views[d.view]
  const [hideDraft, setHideDraft] = useState(hide)
  const boxes = BOX_OPTIONS[d.view]
  return (
    <div className="wk-tp-tools" role="toolbar" aria-label="Graph controls">
      <div className="wk-tp-row">
        <Seg label="view">
          {VIEWS.map((v) => (
            <button key={v.id} type="button" aria-pressed={d.view === v.id} onClick={() => d.setView(v.id)}>{v.label}</button>
          ))}
        </Seg>
        <Pipelines pipelines={pipelines} iconFor={iconFor} />
        {d.view === 'model' && (
          <select aria-label="detail" value={d.detail} onChange={(e) => d.setDetail(e.target.value as Detail)}>
            <option value="routes">Routes only</option>
            <option value="full">Full model</option>
          </select>
        )}
        {boxes.length > 0 && (
          <select aria-label="box by" value={view.box} onChange={(e) => d.setBox(e.target.value as BoxBy)}>
            {boxes.map((b) => <option key={b.value} value={b.value}>{`Box by ${b.label.toLowerCase()}`}</option>)}
          </select>
        )}
        <select aria-label="layout" value={view.layout} onChange={(e) => d.setLayout(e.target.value as LayoutId)}>
          {LAYOUTS.map((l) => <option key={l.value} value={l.value}>{l.label}</option>)}
        </select>
      </div>
      <div className="wk-tp-row">
        <input aria-label="find expression" placeholder="Find… e.g. name~k8s" value={find} onChange={(e) => onFind(e.target.value)} />
        <input
          aria-label="hide expression"
          placeholder="Hide… e.g. idle"
          value={hideDraft}
          onChange={(e) => setHideDraft(e.target.value)}
          onBlur={() => onHide(hideDraft)}
          onKeyDown={(e) => e.key === 'Enter' && onHide(hideDraft)}
        />
        <select aria-label="window" value={String(d.windowSeconds)} onChange={(e) => d.setWindow(Number(e.target.value))}>
          {WINDOWS.map((w) => <option key={w.seconds} value={String(w.seconds)}>{w.label}</option>)}
        </select>
        <Seg label="live or replay">
          <button type="button" aria-pressed={mode !== 'replay'} onClick={() => onMode('live')}>Live</button>
          <button type="button" aria-pressed={mode === 'replay'} onClick={() => onMode('replay')}>Replay</button>
        </Seg>
      </div>
    </div>
  )
}

/**
 * The display control: the classes THIS view can draw, the one it cannot do
 * without listed and fixed, and the options every view shares. The health
 * summary counts every element, hidden ones included.
 */
export function Display({ hidden, viewLabel }: Readonly<{ hidden: HiddenSummary; viewLabel: string }>) {
  const d = useDisplay()
  const view: ViewId = d.view
  const hiddenSet = new Set(d.views[view].hidden)
  const spine = SPINE_CLASS[view]
  return (
    <Fold title="Display" expanded={d.panelOpen} onToggle={() => d.setPanelOpen(!d.panelOpen)} label="Toggle display panel" testId="display-panel">
      <fieldset className="wk-tp-set">
        <legend>Classes in the {viewLabel} view</legend>
        {VIEW_CLASSES[view].map((c) => {
          const fixed = c === spine && view !== 'infrastructure'
          return (
            <label key={c} className="wk-tp-check">
              <input type="checkbox" checked={fixed || !hiddenSet.has(c)} disabled={fixed} onChange={() => d.toggleClass(c)} />
              {`${plural(styleFor(c).label)}${c === spine && view === 'infrastructure' ? ' (the manager’s stays)' : ''}`}
            </label>
          )
        })}
      </fieldset>
      <p className="wk-note">
        {hidden.count} hidden
        {hidden.failing > 0 && `, including ${hidden.failing} failing (${hidden.classes.join(', ')})`}. Health counts every
        element, hidden or not.
      </p>
      <div className="wk-tp-set">
        <label className="wk-tp-check"><input type="checkbox" role="switch" checked={d.animate} onChange={(e) => d.setAnimate(e.target.checked)} />Traffic animation</label>
        <label className="wk-tp-check"><input type="checkbox" role="switch" checked={d.idleEdges} onChange={(e) => d.setIdleEdges(e.target.checked)} />Idle edges</label>
        <label className="wk-tp-check"><input type="checkbox" role="switch" checked={d.idleNodes} onChange={(e) => d.setIdleNodes(e.target.checked)} />Idle elements</label>
      </div>
      <label className="wk-tp-labelled">
        Edge labels
        <select value={d.edgeLabels} onChange={(e) => d.setEdgeLabels(e.target.value as EdgeLabel)}>
          <option value="none">None</option>
          <option value="rate">Rate</option>
          <option value="latency">Latency (p50)</option>
        </select>
      </label>
      <button type="button" className="wk-btn link" onClick={d.reset}>Reset display</button>
    </Fold>
  )
}
