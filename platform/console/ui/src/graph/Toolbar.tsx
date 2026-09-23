import { useState } from 'react'
import {
  Button, Card, CardBody, CardTitle, Checkbox, Divider, FormGroup, FormSelect, FormSelectOption, Label,
  LabelGroup, MenuToggle, Select, SelectList, SelectOption, Stack, StackItem, Switch, TextInput,
  ToggleGroup, ToggleGroupItem, Toolbar, ToolbarContent, ToolbarItem,
} from '@patternfly/react-core'
import { useDisplay } from './display'
import type { HiddenSummary } from './filter'
import type { EdgeLabel } from './hops'
import { BOX_OPTIONS, type BoxBy } from './layout/boxes'
import { LAYOUTS, type LayoutId } from './layout'
import { plural, styleFor } from './shapes'
import { VIEWS, type ViewId } from './types'
import { SPINE_CLASS, VIEW_CLASSES } from './views/classes'
import type { Detail } from './views'

// The graph's toolbar, in PatternFly controls rather than the mockup's own.
// Everything here but find, hide and the scope is persisted per view.

export const WINDOWS = [
  { seconds: 60, label: 'Last 1m' },
  { seconds: 300, label: 'Last 5m' },
  { seconds: 600, label: 'Last 10m' },
  { seconds: 1800, label: 'Last 30m' },
]

export type Mode = 'live' | 'replay' | 'conversation'

export interface ToolbarProps {
  pipelines: string[]
  health: { ok: number; bad: number; unknown: number }
  find: string
  hide: string
  onFind: (v: string) => void
  onHide: (v: string) => void
  mode: Mode
  onMode: (m: 'live' | 'replay') => void
}

export function GraphToolbar({ pipelines, health, find, hide, onFind, onHide, mode, onMode }: ToolbarProps) {
  const d = useDisplay()
  const view = d.views[d.view]
  const [open, setOpen] = useState(false)
  const [hideDraft, setHideDraft] = useState(hide)
  const chosen = d.pipelines ?? pipelines
  const togglePipeline = (p: string) => {
    const next = chosen.includes(p) ? chosen.filter((x) => x !== p) : [...chosen, p]
    d.setPipelines(next.length === pipelines.length && pipelines.every((x) => next.includes(x)) ? null : next)
  }
  const boxes = BOX_OPTIONS[d.view]
  return (
    <Toolbar>
      <ToolbarContent>
        <ToolbarItem>
          <ToggleGroup aria-label="view">
            {VIEWS.map((v) => (
              <ToggleGroupItem key={v.id} text={v.label} isSelected={d.view === v.id} onChange={() => d.setView(v.id)} />
            ))}
          </ToggleGroup>
        </ToolbarItem>
        <ToolbarItem>
          <Select
            isOpen={open}
            onOpenChange={setOpen}
            onSelect={(_e, v) => togglePipeline(String(v))}
            toggle={(ref) => (
              <MenuToggle ref={ref} onClick={() => setOpen(!open)} isExpanded={open} aria-label="pipelines">
                Pipelines: {d.pipelines === null ? 'all' : `${chosen.length} of ${pipelines.length}`}
              </MenuToggle>
            )}
          >
            <SelectList>
              {pipelines.map((p) => (
                <SelectOption key={p} value={p} hasCheckbox isSelected={chosen.includes(p)}>{p}</SelectOption>
              ))}
            </SelectList>
            <Divider />
            <Button variant="link" isInline style={{ padding: '6px 16px' }} onClick={() => d.setPipelines(null)}>Select all</Button>
          </Select>
        </ToolbarItem>
        {d.view === 'model' && (
          <ToolbarItem>
            <FormSelect aria-label="detail" value={d.detail} onChange={(_e, v) => d.setDetail(v as Detail)} style={{ width: 160 }}>
              <FormSelectOption value="routes" label="Routes only" />
              <FormSelectOption value="full" label="Full model" />
            </FormSelect>
          </ToolbarItem>
        )}
        {boxes.length > 0 && (
          <ToolbarItem>
            <FormSelect aria-label="box by" value={view.box} onChange={(_e, v) => d.setBox(v as BoxBy)} style={{ width: 170 }}>
              {boxes.map((b) => <FormSelectOption key={b.value} value={b.value} label={`Box by ${b.label.toLowerCase()}`} />)}
            </FormSelect>
          </ToolbarItem>
        )}
        <ToolbarItem>
          <FormSelect aria-label="layout" value={view.layout} onChange={(_e, v) => d.setLayout(v as LayoutId)} style={{ width: 140 }}>
            {LAYOUTS.map((l) => <FormSelectOption key={l.value} value={l.value} label={l.label} />)}
          </FormSelect>
        </ToolbarItem>
        <ToolbarItem>
          <Button variant="secondary" onClick={() => d.setPanelOpen(!d.panelOpen)} aria-expanded={d.panelOpen}>
            Display {d.panelOpen ? '▴' : '▾'}
          </Button>
        </ToolbarItem>
        <ToolbarItem>
          <TextInput aria-label="find expression" placeholder="Find… e.g. name~k8s" value={find} onChange={(_e, v) => onFind(v)} style={{ width: 170 }} />
        </ToolbarItem>
        <ToolbarItem>
          <TextInput
            aria-label="hide expression"
            placeholder="Hide… e.g. idle"
            value={hideDraft}
            onChange={(_e, v) => setHideDraft(v)}
            onBlur={() => onHide(hideDraft)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') onHide(hideDraft)
            }}
            style={{ width: 150 }}
          />
        </ToolbarItem>
        <ToolbarItem>
          <FormSelect aria-label="window" value={String(d.windowSeconds)} onChange={(_e, v) => d.setWindow(Number(v))} style={{ width: 120 }}>
            {WINDOWS.map((w) => <FormSelectOption key={w.seconds} value={String(w.seconds)} label={w.label} />)}
          </FormSelect>
        </ToolbarItem>
        <ToolbarItem>
          <ToggleGroup aria-label="live or replay">
            <ToggleGroupItem text="Live" isSelected={mode !== 'replay'} onChange={() => onMode('live')} />
            <ToggleGroupItem text="Replay" isSelected={mode === 'replay'} onChange={() => onMode('replay')} />
          </ToggleGroup>
        </ToolbarItem>
        <ToolbarItem>
          <LabelGroup categoryName="Health">
            <Label isCompact color="green">{health.ok} ok</Label>
            <Label isCompact color="red">{health.bad} failing</Label>
            <Label isCompact color="orange">{health.unknown} unknown</Label>
          </LabelGroup>
        </ToolbarItem>
      </ToolbarContent>
    </Toolbar>
  )
}

/**
 * The display control: the classes THIS view can draw, the one it cannot do
 * without listed and fixed, and the options every view shares.
 *
 * The health summary counts every element, hidden ones included. You can
 * simplify the picture, but the counts never move because you hid something.
 */
export function DisplayPanel({ hidden, viewLabel }: { hidden: HiddenSummary; viewLabel: string }) {
  const d = useDisplay()
  const view: ViewId = d.view
  const hiddenSet = new Set(d.views[view].hidden)
  const spine = SPINE_CLASS[view]
  return (
    <Card isCompact data-testid="display-panel">
      <CardTitle>Display</CardTitle>
      <CardBody>
        <Stack hasGutter>
          <StackItem>
            <FormGroup label={`Classes in the ${viewLabel} view`} fieldId="display-classes">
              {VIEW_CLASSES[view].map((c) => {
                const fixed = c === spine && view !== 'infrastructure'
                return (
                  <Checkbox
                    key={c}
                    id={`display-class-${view}-${c}`}
                    label={`${plural(styleFor(c).label)}${c === spine && view === 'infrastructure' ? ' (the manager’s stays)' : ''}`}
                    isChecked={fixed || !hiddenSet.has(c)}
                    isDisabled={fixed}
                    onChange={() => d.toggleClass(c)}
                  />
                )
              })}
            </FormGroup>
            <small>
              {hidden.count} hidden
              {hidden.failing > 0 && `, including ${hidden.failing} failing (${hidden.classes.join(', ')})`}. Health counts
              every element, hidden or not.
            </small>
          </StackItem>
          <StackItem><Divider /></StackItem>
          <StackItem>
            <Switch id="display-animate" label="Traffic animation" isChecked={d.animate} onChange={(_e, v) => d.setAnimate(v)} />
            <Switch id="display-idle-edges" label="Idle edges" isChecked={d.idleEdges} onChange={(_e, v) => d.setIdleEdges(v)} />
            <Switch id="display-idle-nodes" label="Idle elements" isChecked={d.idleNodes} onChange={(_e, v) => d.setIdleNodes(v)} />
          </StackItem>
          <StackItem>
            <FormGroup label="Edge labels" fieldId="display-edge-labels">
              <FormSelect id="display-edge-labels" value={d.edgeLabels} onChange={(_e, v) => d.setEdgeLabels(v as EdgeLabel)}>
                <FormSelectOption value="none" label="None" />
                <FormSelectOption value="rate" label="Rate" />
                <FormSelectOption value="latency" label="Latency (p50)" />
              </FormSelect>
            </FormGroup>
          </StackItem>
          <StackItem>
            <Button variant="link" isInline onClick={d.reset}>Reset display</Button>
          </StackItem>
        </Stack>
      </CardBody>
    </Card>
  )
}
