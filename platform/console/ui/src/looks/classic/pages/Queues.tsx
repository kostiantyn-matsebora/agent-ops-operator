import {
  Alert, Card, CardBody, CardTitle, Grid, GridItem, Label, PageSection,
  Stack, StackItem, Title, Tooltip,
} from '@patternfly/react-core'
import { Table, Tbody, Td, Th, Thead, Tr } from '@patternfly/react-table'
import { Link } from 'react-router-dom'
import { Empty, ErrorState, Loading } from '../components/States'
import { Crumbs } from '../components/Crumbs'
import { STUCK, queueAge, useQueuesPage, workQueueTitle } from '../../../model/queues'
import { PlainText } from '../components/Text'
import { PipelineName } from '../components/PipelineName'
import type { StuckReason } from '../../../api/types'

// The view that separates "queued" from "stalled".
//
// The question it answers is asked under pressure and has two answers that look
// identical from outside: an agent has not replied — is it queued, or is it
// stuck? Every row carries an age, and the flags name the CAUSE, because each
// cause has a different fix.

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
  const { data, isLoading, error } = useQueuesPage()
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
                <CardTitle>{workQueueTitle(data)}</CardTitle>
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
                                    <PipelineName name={row.pipeline} />
                                  </small>
                                </div>
                              )}
                            </Td>
                            <Td dataLabel="Phase">
                              <PlainText>{row.phase}</PlainText>
                            </Td>
                            <Td dataLabel="Queued">{row.queued}</Td>
                            <Td dataLabel="Age">
                              {row.inflight ? queueAge(row.inflightAgeSeconds) : queueAge(row.ageSeconds)}
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
                                  <small>claimed {queueAge(row.oldestClaimedAgeSeconds)}</small>
                                </>
                              ) : row.oldestQueuedOpId ? (
                                <>
                                  <PlainText>{row.oldestQueuedOpId}</PlainText>{' '}
                                  <small>queued {queueAge(row.oldestQueuedAgeSeconds)}</small>
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
                        <Td dataLabel="Window">{queueAge(c.windowSeconds)}</Td>
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
