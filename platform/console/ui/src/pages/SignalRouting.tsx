import {
  Alert, Card, CardBody, CardTitle, FormSelect, FormSelectOption, Label, Toolbar, ToolbarContent, ToolbarItem,
} from '@patternfly/react-core'
import { Table, Tbody, Td, Th, Thead, Tr } from '@patternfly/react-table'
import { Link } from 'react-router-dom'
import { PlainText } from '../components/Text'
import { Empty } from '../components/States'
import type { SignalStatusWire } from '../api/types'
import { FILTER_ALL, isSignalTrouble, signalPhaseStatus } from './signals'

// The Signal kind's own additions to the generic Configuration pages — console-signals
// spec: filterable by source/phase on the list, and the full per-claimant
// routing record (plus the terminal reason, as loud as the phase) on the
// detail. No write control anywhere — this kind carries none, like every
// other Configuration page.

/** A phase rendered as a status-coloured chip, same vocabulary as HealthChip/ConditionChips. */
export function SignalPhaseLabel({ phase }: { phase?: string }) {
  if (!phase) return <small>—</small>
  const status = signalPhaseStatus(phase)
  return <Label isCompact status={status}>{phase}</Label>
}

export interface SignalFiltersProps {
  sources: string[]
  phases: string[]
  source: string
  phase: string
  onSourceChange: (v: string) => void
  onPhaseChange: (v: string) => void
}

/** The two filters console-signals requires: by source, and by phase. Local to this page — nothing here is shared state. */
export function SignalFilters({ sources, phases, source, phase, onSourceChange, onPhaseChange }: SignalFiltersProps) {
  return (
    <Toolbar>
      <ToolbarContent>
        <ToolbarItem>
          <FormSelect
            aria-label="Filter by source"
            value={source}
            onChange={(_e, v) => onSourceChange(v)}
          >
            <FormSelectOption value={FILTER_ALL} label="All sources" />
            {sources.map((s) => (
              <FormSelectOption key={s} value={s} label={s} />
            ))}
          </FormSelect>
        </ToolbarItem>
        <ToolbarItem>
          <FormSelect
            aria-label="Filter by phase"
            value={phase}
            onChange={(_e, v) => onPhaseChange(v)}
          >
            <FormSelectOption value={FILTER_ALL} label="All phases" />
            {phases.map((p) => (
              <FormSelectOption key={p} value={p} label={p} />
            ))}
          </FormSelect>
        </ToolbarItem>
      </ToolbarContent>
    </Toolbar>
  )
}

/**
 * The detail view's routing record — console-signals' second requirement
 * whole: every claimant's kind/name/state, a routed one linked to its
 * conversation, a failed/skipped one's reason VERBATIM, and the signal's own
 * terminal reason shown as loudly as its phase.
 */
export function SignalRoutingCard({ status }: { status: SignalStatusWire }) {
  const claimants = status.claimants ?? []
  return (
    <Card>
      <CardTitle>Routing</CardTitle>
      <CardBody>
        <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 12 }}>
          <SignalPhaseLabel phase={status.phase} />
          {status.reason && (
            <Alert
              isInline
              isPlain
              variant={isSignalTrouble(status.phase) ? 'danger' : 'warning'}
              title={<PlainText>{status.reason}</PlainText>}
            />
          )}
        </div>
        {claimants.length === 0 ? (
          <Empty title="No claimant was ever snapshotted">
            {status.phase === 'Dropped'
              ? 'This signal was dropped before any claimant saw it.'
              : 'No routing attempt has run for this signal yet.'}
          </Empty>
        ) : (
          <Table variant="compact" aria-label="claimants">
            <Thead>
              <Tr>
                <Th>Kind</Th>
                <Th>Name</Th>
                <Th>State</Th>
                <Th>Conversation</Th>
                <Th>Reason</Th>
              </Tr>
            </Thead>
            <Tbody>
              {claimants.map((c) => (
                <Tr key={`${c.ref.kind}/${c.ref.name}`}>
                  <Td dataLabel="Kind">{c.ref.kind}</Td>
                  <Td dataLabel="Name"><PlainText>{c.ref.name}</PlainText></Td>
                  <Td dataLabel="State">
                    <Label isCompact status={signalPhaseStatus(c.state === 'Routed' ? 'Routed' : undefined) ?? (c.state === 'Failed' ? 'danger' : c.state === 'Skipped' ? 'warning' : 'info')}>
                      {c.state}
                    </Label>
                  </Td>
                  <Td dataLabel="Conversation">
                    {c.conversationRef ? (
                      <Link to={`/conversations/${c.conversationRef.name}`}>{c.conversationRef.name}</Link>
                    ) : (
                      <small>—</small>
                    )}
                  </Td>
                  <Td dataLabel="Reason">
                    {c.reason ? <PlainText multiline>{c.reason}</PlainText> : <small>—</small>}
                  </Td>
                </Tr>
              ))}
            </Tbody>
          </Table>
        )}
      </CardBody>
    </Card>
  )
}
