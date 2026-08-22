import {
  Card, CardBody, CardTitle, DescriptionList, DescriptionListDescription,
  DescriptionListGroup, DescriptionListTerm, Label, LabelGroup, Tooltip,
} from '@patternfly/react-core'
import type { Condition, Health, ObjectMeta } from '../api/types'
import { PlainText } from './Text'
import { healthVariant } from '../api/hooks'

// Object metadata, shown rather than summarized.
//
// A detail page that hides uid, resourceVersion, labels and annotations makes
// you leave for kubectl to answer ordinary questions — which is the failure mode
// this console exists to remove. Everything the watch cache holds is shown; the
// cache holds no more than the read-only Role can see.

export function age(iso?: string): string {
  if (!iso) return '—'
  const then = new Date(iso).getTime()
  if (Number.isNaN(then)) return '—'
  const s = Math.max(0, (Date.now() - then) / 1000)
  if (s < 60) return `${Math.round(s)}s`
  if (s < 3600) return `${Math.round(s / 60)}m`
  if (s < 86400) return `${Math.round(s / 3600)}h`
  return `${Math.round(s / 86400)}d`
}

/** Conditions as chips — the shape an operator scans rather than reads. */
export function ConditionChips({ conditions }: { conditions?: Condition[] | null }) {
  if (!conditions || conditions.length === 0) {
    return <small>reports none</small>
  }
  return (
    <LabelGroup numLabels={6}>
      {conditions.map((c) => (
        <Tooltip
          key={c.type}
          content={
            <>
              <PlainText>{c.reason}</PlainText>
              {c.message ? <div><PlainText multiline>{c.message}</PlainText></div> : null}
              {c.lastTransitionTime ? <div>since {age(c.lastTransitionTime)} ago</div> : null}
            </>
          }
        >
          <Label
            isCompact
            status={c.status === 'True' ? 'success' : c.status === 'False' ? 'danger' : 'warning'}
          >
            {c.type}
          </Label>
        </Tooltip>
      ))}
    </LabelGroup>
  )
}

export function HealthChip({ health }: { health: Health }) {
  if (health === 'none') return <small>reports none</small>
  return (
    <Label isCompact status={healthVariant(health)}>
      {health}
    </Label>
  )
}

/** Key/value chips for labels and annotations. */
export function KeyValueChips({ values, max = 12 }: { values?: Record<string, string>; max?: number }) {
  const entries = Object.entries(values ?? {})
  if (entries.length === 0) return <small>none</small>
  return (
    <LabelGroup numLabels={max}>
      {entries.map(([k, v]) => (
        <Tooltip key={k} content={<PlainText>{`${k}=${v}`}</PlainText>}>
          <Label isCompact color="grey" textMaxWidth="18ch">
            <PlainText>{`${k}=${v}`}</PlainText>
          </Label>
        </Tooltip>
      ))}
    </LabelGroup>
  )
}

export function MetadataCard({
  meta,
  health,
  conditions,
}: {
  meta: ObjectMeta
  health?: Health
  conditions?: Condition[] | null
}) {
  return (
    <Card isCompact>
      <CardTitle>Metadata</CardTitle>
      <CardBody>
        <DescriptionList isCompact isHorizontal>
          <DescriptionListGroup>
            <DescriptionListTerm>Name</DescriptionListTerm>
            <DescriptionListDescription>
              <PlainText>{meta.name}</PlainText>
            </DescriptionListDescription>
          </DescriptionListGroup>
          <DescriptionListGroup>
            <DescriptionListTerm>Namespace</DescriptionListTerm>
            <DescriptionListDescription>
              <PlainText>{meta.namespace}</PlainText>
            </DescriptionListDescription>
          </DescriptionListGroup>
          {health && (
            <DescriptionListGroup>
              <DescriptionListTerm>Health</DescriptionListTerm>
              <DescriptionListDescription>
                <HealthChip health={health} />
              </DescriptionListDescription>
            </DescriptionListGroup>
          )}
          <DescriptionListGroup>
            <DescriptionListTerm>Conditions</DescriptionListTerm>
            <DescriptionListDescription>
              <ConditionChips conditions={conditions} />
            </DescriptionListDescription>
          </DescriptionListGroup>
          <DescriptionListGroup>
            <DescriptionListTerm>Created</DescriptionListTerm>
            <DescriptionListDescription>
              {meta.creationTimestamp
                ? `${new Date(meta.creationTimestamp).toLocaleString()} (${age(meta.creationTimestamp)} ago)`
                : '—'}
            </DescriptionListDescription>
          </DescriptionListGroup>
          <DescriptionListGroup>
            <DescriptionListTerm>Labels</DescriptionListTerm>
            <DescriptionListDescription>
              <KeyValueChips values={meta.labels} />
            </DescriptionListDescription>
          </DescriptionListGroup>
          <DescriptionListGroup>
            <DescriptionListTerm>Annotations</DescriptionListTerm>
            <DescriptionListDescription>
              <KeyValueChips values={meta.annotations} max={6} />
            </DescriptionListDescription>
          </DescriptionListGroup>
          <DescriptionListGroup>
            <DescriptionListTerm>UID</DescriptionListTerm>
            <DescriptionListDescription>
              <small>
                <PlainText>{meta.uid}</PlainText>
              </small>
            </DescriptionListDescription>
          </DescriptionListGroup>
          <DescriptionListGroup>
            <DescriptionListTerm>Resource version</DescriptionListTerm>
            <DescriptionListDescription>
              <small>
                <PlainText>{meta.resourceVersion}</PlainText>
              </small>
            </DescriptionListDescription>
          </DescriptionListGroup>
        </DescriptionList>
      </CardBody>
    </Card>
  )
}
