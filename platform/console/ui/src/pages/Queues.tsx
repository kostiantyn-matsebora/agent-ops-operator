import {
  Alert, Card, CardBody, CardTitle, Grid, GridItem, Label, PageSection,
  Stack, StackItem, Title, Tooltip,
} from '@patternfly/react-core'
import { Table, Tbody, Td, Th, Thead, Tr } from '@patternfly/react-table'
import { Link } from 'react-router-dom'
import { Empty, ErrorState, Loading } from '../App'
import { Crumbs } from '../components/Crumbs'
import { useQueues } from '../api/hooks'
import { PlainText } from '../components/Text'
import type { StuckReason } from '../api/types'

// The view that separates "queued" from "stalled".
//
// The question it answers is asked under pressure and has two answers that look
// identical from outside: an agent has not replied — is it queued, or is it
// stuck? Every row carries an age, and the flags name the CAUSE, because each
// cause has a different fix.

const STUCK: Record<StuckReason, { title: string; detail: string }> = {
  'nothing-claiming': {
    title: 'nothing claiming',
    detail: 'ops are queued and no adapter is polling — the adapter is down or not running',
  },
  'adapter-wedged': {
    title: 'adapter wedged',
    detail: 'an adapter claimed these ops and never completed them — it is stuck mid-delivery',
  },
  'at-runtime-ceiling': {
    title: 'at runtime ceiling',
    detail: 'capacity-bound, not broken — raise MAX_ACTIVE_CONVERSATIONS or shed load',
  },
  'runtime-hung': {
    title: 'runtime hung',
    detail: 'inflight far beyond a typical run — the runtime is hung, not queued',
  },
}

function age(seconds?: number): string {
  if (!seconds) return '—'
  if (seconds < 60) return `${Math.round(seconds)}s`
  if (seconds < 3600) return `${Math.round(seconds / 60)}m`
  return `${(seconds / 3600).toFixed(1)}h`
}

function StuckLabel({ reason }: { reason?: StuckReason }) {
  if (!reason) return null
  const s = STUCK[reason]
  return (
    <Tooltip content={s.detail}>
      <Label status="danger">{s.title}</Label>
    </Tooltip>
  )
}

export function QueuesPage() {
  const { data, isLoading, error } = useQueues()
  if (isLoading && !data) return <Loading />
  if (error || !data) return <ErrorState title="Could not load queues">{String(error)}</ErrorState>

  return (
    <>
      <Crumbs items={[{ label: 'Queues' }]} />
      <PageSection>
      <Stack hasGutter>
        <StackItem>
          <Title headingLevel="h1">Queues and capacity</Title>
        </StackItem>

        {data.error && (
          <StackItem>
            {/* The delivery queue exists in NO Kubernetes object. Without the
                manager it is unavailable — not empty, which would read as "no
                ops pending". */}
            <Alert variant="warning" isInline title="Delivery queue unavailable">
              <PlainText>{data.error}</PlainText>
            </Alert>
          </StackItem>
        )}

        <StackItem>
          <Grid hasGutter>
            <GridItem md={6}>
              <Card>
                <CardTitle>
                  Work queue — {data.capacity.inUse}/{data.capacity.max} slots,{' '}
                  {data.capacity.waiting} waiting
                </CardTitle>
                <CardBody>
                  {data.work.length === 0 ? (
                    <Empty title="Nothing is waiting">
                      No conversation is queued and none has a run in flight.
                    </Empty>
                  ) : (
                    <Table variant="compact" aria-label="work queue">
                      <Thead>
                        <Tr>
                          <Th>Conversation</Th>
                          <Th>Phase</Th>
                          <Th>Queued</Th>
                          <Th>Age</Th>
                          <Th>Flag</Th>
                        </Tr>
                      </Thead>
                      <Tbody>
                        {data.work.map((row) => (
                          <Tr key={row.conversation}>
                            <Td dataLabel="Conversation">
                              <Link to={`/conversations/${row.conversation}`}>
                                <PlainText>{row.title || row.conversation}</PlainText>
                              </Link>
                              {row.pipeline && (
                                <div>
                                  <small>
                                    <PlainText>{row.pipeline}</PlainText>
                                  </small>
                                </div>
                              )}
                            </Td>
                            <Td dataLabel="Phase">
                              <PlainText>{row.phase}</PlainText>
                            </Td>
                            <Td dataLabel="Queued">{row.queued}</Td>
                            <Td dataLabel="Age">
                              {row.inflight ? age(row.inflightAgeSeconds) : age(row.ageSeconds)}
                            </Td>
                            <Td dataLabel="Flag">
                              <StuckLabel reason={row.stuck} />
                            </Td>
                          </Tr>
                        ))}
                      </Tbody>
                    </Table>
                  )}
                </CardBody>
              </Card>
            </GridItem>

            <GridItem md={6}>
              <Card>
                <CardTitle>Delivery queue</CardTitle>
                <CardBody>
                  {data.delivery.length === 0 ? (
                    <Empty title="No outstanding channel ops">
                      Every enqueued op has been claimed and completed.
                    </Empty>
                  ) : (
                    <Table variant="compact" aria-label="delivery queue">
                      <Thead>
                        <Tr>
                          <Th>Adapter</Th>
                          <Th>Queued</Th>
                          <Th>Claimed</Th>
                          <Th>Oldest</Th>
                          <Th>Flag</Th>
                        </Tr>
                      </Thead>
                      <Tbody>
                        {data.delivery.map((row) => (
                          <Tr key={row.adapter}>
                            <Td dataLabel="Adapter">
                              <Link to={`/config/channeladapters/${row.adapter}`}>
                                <PlainText>{row.adapter}</PlainText>
                              </Link>
                            </Td>
                            <Td dataLabel="Queued">{row.queued}</Td>
                            <Td dataLabel="Claimed">{row.claimed}</Td>
                            <Td dataLabel="Oldest">
                              {/* WHICH op, not just how many — that identity is
                                  exactly what a metric label may never carry. */}
                              {row.oldestClaimedOpId ? (
                                <>
                                  <PlainText>{row.oldestClaimedOpId}</PlainText>{' '}
                                  <small>claimed {age(row.oldestClaimedAgeSeconds)}</small>
                                </>
                              ) : row.oldestQueuedOpId ? (
                                <>
                                  <PlainText>{row.oldestQueuedOpId}</PlainText>{' '}
                                  <small>queued {age(row.oldestQueuedAgeSeconds)}</small>
                                </>
                              ) : (
                                '—'
                              )}
                            </Td>
                            <Td dataLabel="Flag">
                              <StuckLabel reason={row.stuck} />
                            </Td>
                          </Tr>
                        ))}
                      </Tbody>
                    </Table>
                  )}
                </CardBody>
              </Card>
            </GridItem>
          </Grid>
        </StackItem>

        <StackItem>
          <Card>
            <CardTitle>Cooldowns</CardTitle>
            <CardBody>
              {/* A suppressed signal lane looks exactly like an idle one on a
                  graph. This is where that distinction belongs. */}
              {data.cooldowns.length === 0 ? (
                <Empty title="No signals are being suppressed">
                  Nothing is inside a fingerprint cooldown window.
                </Empty>
              ) : (
                <Table variant="compact" aria-label="cooldowns">
                  <Thead>
                    <Tr>
                      <Th>Source</Th>
                      <Th>Suppressed</Th>
                      <Th>Window</Th>
                    </Tr>
                  </Thead>
                  <Tbody>
                    {data.cooldowns.map((c) => (
                      <Tr key={c.source}>
                        <Td dataLabel="Source">
                          <Link to={`/config/signalsources/${c.source}`}>
                            <PlainText>{c.source}</PlainText>
                          </Link>
                        </Td>
                        <Td dataLabel="Suppressed">{c.suppressed}</Td>
                        <Td dataLabel="Window">{age(c.windowSeconds)}</Td>
                      </Tr>
                    ))}
                  </Tbody>
                </Table>
              )}
            </CardBody>
          </Card>
        </StackItem>
      </Stack>
      </PageSection>
    </>
  )
}
