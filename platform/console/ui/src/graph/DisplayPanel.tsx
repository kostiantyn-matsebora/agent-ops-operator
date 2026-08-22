import {
  Button, Card, CardBody, CardTitle, Checkbox, Divider, FormGroup,
  FormSelect, FormSelectOption, Label, LabelGroup, Stack, StackItem, Switch,
} from '@patternfly/react-core'
import { NODE_CLASSES, useDisplay, type EdgeLabel, type NodeClass } from './display'
import type { HiddenSummary } from './model'

const WINDOWS: { label: string; seconds: number }[] = [
  { label: 'Live (1m)', seconds: 60 },
  { label: '5 minutes', seconds: 300 },
  { label: '15 minutes', seconds: 900 },
  { label: '1 hour', seconds: 3600 },
]

export interface DisplayPanelProps {
  health: { ok: number; bad: number; unknown: number }
  hiddenSummary: HiddenSummary
  /** Set when a longer window needs a metrics backend the install may not have. */
  metricsAvailable?: boolean
}

/**
 * The Display panel: per-class show/hide, traffic animation, idle elements and
 * edge labels, plus the time window.
 *
 * The health summary here counts the WHOLE graph, hidden classes included. That
 * is deliberate and it is the panel's most important property: you can simplify
 * the picture, but the counts never move because you hid something.
 */
export function DisplayPanel({ health, hiddenSummary, metricsAvailable }: DisplayPanelProps) {
  const {
    hidden, animate, showIdle, edgeLabels, windowSeconds,
    toggleClass, setAnimate, setShowIdle, setEdgeLabels, setWindow, reset,
  } = useDisplay()

  return (
    <Card isCompact>
      <CardTitle>Display</CardTitle>
      <CardBody>
        <Stack hasGutter>
          <StackItem>
            <LabelGroup categoryName="Health">
              <Label color="green">{health.ok} ok</Label>
              <Label color="red">{health.bad} failing</Label>
              <Label color="orange">{health.unknown} unknown</Label>
            </LabelGroup>
            <small>
              Counted across every element, including {hiddenSummary.count} hidden.
            </small>
          </StackItem>
          <StackItem>
            <Divider />
          </StackItem>
          <StackItem>
            <FormGroup label="Element classes" fieldId="display-classes">
              {(Object.keys(NODE_CLASSES) as NodeClass[]).map((c) => (
                <Checkbox
                  key={c}
                  id={`display-class-${c}`}
                  label={NODE_CLASSES[c]}
                  isChecked={!hidden[c]}
                  onChange={() => toggleClass(c)}
                />
              ))}
            </FormGroup>
          </StackItem>
          <StackItem>
            <Divider />
          </StackItem>
          <StackItem>
            <Switch
              id="display-animate"
              label="Animate traffic"
              isChecked={animate}
              onChange={(_e, v) => setAnimate(v)}
            />
            <Switch
              id="display-idle"
              label="Show idle elements"
              isChecked={showIdle}
              onChange={(_e, v) => setShowIdle(v)}
            />
          </StackItem>
          <StackItem>
            <FormGroup label="Edge labels" fieldId="display-edge-labels">
              <FormSelect
                id="display-edge-labels"
                value={edgeLabels}
                onChange={(_e, v) => setEdgeLabels(v as EdgeLabel)}
              >
                <FormSelectOption value="none" label="None" />
                <FormSelectOption value="rate" label="Rate" />
                <FormSelectOption value="latency" label="Latency" />
              </FormSelect>
            </FormGroup>
          </StackItem>
          <StackItem>
            <FormGroup label="Time window" fieldId="display-window">
              <FormSelect
                id="display-window"
                value={String(windowSeconds)}
                onChange={(_e, v) => setWindow(Number(v))}
              >
                {WINDOWS.map((w) => (
                  <FormSelectOption key={w.seconds} value={String(w.seconds)} label={w.label} />
                ))}
              </FormSelect>
              {/* Windows are bounded by the manager's replay buffer. Longer ones
                  come from a metrics backend when one is configured, and are
                  reported unavailable rather than rendered empty when not. */}
              {metricsAvailable === false && (
                <small>
                  Windows longer than the activity buffer need a metrics backend
                  (<code>console.metrics.url</code>).
                </small>
              )}
            </FormGroup>
          </StackItem>
          <StackItem>
            <Button variant="link" isInline onClick={reset}>
              Reset display
            </Button>
          </StackItem>
        </Stack>
      </CardBody>
    </Card>
  )
}
