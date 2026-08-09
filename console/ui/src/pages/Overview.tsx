import {
  Alert, Card, CardBody, CardTitle, DescriptionList, DescriptionListDescription,
  DescriptionListGroup, DescriptionListTerm, Gallery, Label, PageSection, Progress,
  ProgressMeasureLocation, Stack, StackItem, Title,
} from '@patternfly/react-core'
import { Table, Tbody, Td, Th, Thead, Tr } from '@patternfly/react-table'
import { Link } from 'react-router-dom'
import { Empty, ErrorState, Loading } from '../App'
import { Crumbs } from '../components/Crumbs'
import { healthVariant, useCharts, useOverview } from '../api/hooks'
import { PlainText } from '../components/Text'
import type { Overview, Problem, ProblemSource } from '../api/types'

const SOURCE_LABEL: Record<ProblemSource, { text: string; color: 'blue' | 'grey' | 'purple' }> = {
  // A condition a reconciler wrote and a cross-reference the console derived
  // carry different authority. Labelling them apart is what stops the console
  // appearing to speak for the cluster.
  reported: { text: 'reported', color: 'blue' },
  pod: { text: 'pod', color: 'purple' },
  derived: { text: 'console check', color: 'grey' },
}

export function OverviewPage() {
  const { data, isLoading, error } = useOverview()
  if (isLoading) return <Loading />
  if (error || !data) return <ErrorState title="Could not load the overview">{String(error)}</ErrorState>

  const slots = data.manager?.runtimeSlots
  const problems = data.problems ?? []

  return (
    <>
      <Crumbs items={[{ label: 'Overview' }]} />
      <PageSection>
      <Stack hasGutter>
        <StackItem>
          <Title headingLevel="h1">Installation</Title>
        </StackItem>

        {data.managerError && (
          <StackItem>
            <Alert variant="warning" isInline title="The manager's status surface is unreachable">
              <PlainText>{data.managerError}</PlainText> — capacity and queue depth come only from
              the manager's memory, so they are unavailable rather than zero.
            </Alert>
          </StackItem>
        )}
        {!data.stream.connected && (
          <StackItem>
            <Alert variant="warning" isInline title="The activity stream is disconnected">
              Graphs will not animate. <PlainText>{data.stream.error}</PlainText>
            </Alert>
          </StackItem>
        )}

        <StackItem>
          <Gallery hasGutter minWidths={{ default: '320px' }}>
            <Card>
              <CardTitle>Manager</CardTitle>
              <CardBody>
                <DescriptionList isCompact>
                  <DescriptionListGroup>
                    <DescriptionListTerm>Version</DescriptionListTerm>
                    <DescriptionListDescription>
                      <PlainText>{data.manager?.version ?? 'unknown'}</PlainText>
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                  <DescriptionListGroup>
                    <DescriptionListTerm>Leader</DescriptionListTerm>
                    <DescriptionListDescription>
                      <PlainText>{data.manager?.leader || 'unknown'}</PlainText>
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                  <DescriptionListGroup>
                    <DescriptionListTerm>Namespace</DescriptionListTerm>
                    <DescriptionListDescription>{data.namespace}</DescriptionListDescription>
                  </DescriptionListGroup>
                </DescriptionList>
              </CardBody>
            </Card>

            <Card>
              <CardTitle>Capacity</CardTitle>
              <CardBody>
                {slots ? (
                  <>
                    <Progress
                      value={slots.max ? (slots.inUse / slots.max) * 100 : 0}
                      title={`${slots.inUse} of ${slots.max} runtime slots`}
                      measureLocation={ProgressMeasureLocation.top}
                      variant={slots.inUse >= slots.max ? 'warning' : undefined}
                    />
                    <p>
                      {slots.waiting} conversation(s) waiting for a slot.{' '}
                      <Link to="/queues">Queues</Link>
                    </p>
                  </>
                ) : (
                  <Empty title="Unavailable">
                    Capacity is counted in the manager's memory and cannot be read right now.
                  </Empty>
                )}
              </CardBody>
            </Card>

            <Card>
              <CardTitle>Runtimes</CardTitle>
              <CardBody>
                <DescriptionList isCompact>
                  {data.runtimes.map((rt) => (
                    <DescriptionListGroup key={rt.name}>
                      <DescriptionListTerm>{rt.name}</DescriptionListTerm>
                      <DescriptionListDescription>
                        <PlainText>{rt.image}</PlainText>
                      </DescriptionListDescription>
                    </DescriptionListGroup>
                  ))}
                </DescriptionList>
              </CardBody>
            </Card>

            <TelemetryCard stream={data.stream} />
          </Gallery>
        </StackItem>

        <StackItem>
          <Card>
            <CardTitle>Workloads</CardTitle>
            <CardBody>
              <Table variant="compact" aria-label="workloads">
                <Thead>
                  <Tr>
                    <Th>Name</Th>
                    <Th>Image</Th>
                    <Th>Ready</Th>
                    <Th>Restarts</Th>
                    <Th>Problem</Th>
                  </Tr>
                </Thead>
                <Tbody>
                  {data.workloads.map((w) => (
                    <Tr key={w.name}>
                      <Td dataLabel="Name">
                        <PlainText>{w.name}</PlainText>
                      </Td>
                      <Td dataLabel="Image">
                        <PlainText>{w.image}</PlainText>
                        {w.digest && (
                          <div>
                            <small>
                              <PlainText>{w.digest}</PlainText>
                            </small>
                          </div>
                        )}
                      </Td>
                      <Td dataLabel="Ready">
                        {w.ready}/{w.desired}
                      </Td>
                      <Td dataLabel="Restarts">{w.restarts}</Td>
                      <Td dataLabel="Problem">
                        {w.problem ? (
                          <Label status="danger">
                            <PlainText>{w.problem}</PlainText>
                          </Label>
                        ) : (
                          ''
                        )}
                      </Td>
                    </Tr>
                  ))}
                </Tbody>
              </Table>
            </CardBody>
          </Card>
        </StackItem>

        <StackItem>
          <Card>
            <CardTitle>Adapters</CardTitle>
            <CardBody>
              <Table variant="compact" aria-label="adapters">
                <Thead>
                  <Tr>
                    <Th>Kind</Th>
                    <Th>Name</Th>
                    <Th>Image</Th>
                    <Th>Serves</Th>
                    <Th>Health</Th>
                  </Tr>
                </Thead>
                <Tbody>
                  {data.adapters.map((a) => (
                    <Tr key={`${a.kind}/${a.name}`}>
                      <Td dataLabel="Kind">{a.kind}</Td>
                      <Td dataLabel="Name">
                        <Link to={`/config/${a.kind}/${a.name}`}>
                          <PlainText>{a.name}</PlainText>
                        </Link>
                      </Td>
                      <Td dataLabel="Image">
                        {/* An externally-served adapter owns no workload. That is
                            a configuration, not a missing field. */}
                        {a.servedBy ? (
                          <Label color="grey">served by {a.servedBy}</Label>
                        ) : (
                          <PlainText>{a.image}</PlainText>
                        )}
                      </Td>
                      <Td dataLabel="Serves">{a.serves}</Td>
                      <Td dataLabel="Health">
                        <Label status={healthVariant(a.health)}>
                          <PlainText>{a.reason || a.health}</PlainText>
                        </Label>
                      </Td>
                    </Tr>
                  ))}
                </Tbody>
              </Table>
            </CardBody>
          </Card>
        </StackItem>

        <StackItem>
          <ProblemsCard problems={problems} />
        </StackItem>
      </Stack>
      </PageSection>
    </>
  )
}

/**
 * Where the two telemetry paths are answered for.
 *
 * "Is this wired to my monitoring?" was previously only answerable by reading a
 * banner on another page whose wording implied the opposite. The two paths are
 * separate and fail separately, so they are stated separately:
 *
 *   LIVE       the manager's activity stream — exact per-item detail, bounded
 *   HISTORICAL a metrics backend — aggregates over long windows, optional
 */
function TelemetryCard({ stream }: { stream: Overview['stream'] }) {
  const charts = useCharts()
  const backend = charts.data?.available ?? false

  return (
    <Card>
      <CardTitle>Telemetry</CardTitle>
      <CardBody>
        <DescriptionList isCompact>
          <DescriptionListGroup>
            <DescriptionListTerm>Live activity</DescriptionListTerm>
            <DescriptionListDescription>
              <Label isCompact status={stream.connected ? 'success' : 'warning'}>
                {stream.connected ? 'connected' : 'disconnected'}
              </Label>{' '}
              <small>{stream.events} event(s) held</small>
              {stream.resyncs > 0 && <small> · {stream.resyncs} resync(s)</small>}
            </DescriptionListDescription>
          </DescriptionListGroup>
          <DescriptionListGroup>
            <DescriptionListTerm>Metrics backend</DescriptionListTerm>
            <DescriptionListDescription>
              <Label isCompact status={backend ? 'success' : 'warning'}>
                {backend ? 'connected' : 'not configured'}
              </Label>
              <div>
                <small>
                  {backend ? (
                    <>
                      Historical aggregates available. <Link to="/topology">See the charts</Link>.
                    </>
                  ) : (
                    <>
                      Set <code>console.metrics.url</code> for windows beyond the activity buffer.
                    </>
                  )}
                </small>
              </div>
            </DescriptionListDescription>
          </DescriptionListGroup>
          <DescriptionListGroup>
            <DescriptionListTerm>Scraped by</DescriptionListTerm>
            <DescriptionListDescription>
              {/* The manager always exposes :9090; whether anything scrapes it is
                  the cluster's business, so this states the fact rather than
                  claiming a connection the console cannot see. */}
              <small>
                the manager exposes <code>agentops_*</code> on <code>:9090/metrics</code>
              </small>
            </DescriptionListDescription>
          </DescriptionListGroup>
        </DescriptionList>
      </CardBody>
    </Card>
  )
}

/**
 * The page's real job: "is my install healthy" answered on one page.
 *
 * Counted from the CACHE, never from whatever the topology view is currently
 * displaying — a Display filter can simplify a graph but must never remove a
 * failure from this list.
 */
function ProblemsCard({ problems }: { problems: Problem[] }) {
  return (
    <Card>
      <CardTitle>Problems ({problems.length})</CardTitle>
      <CardBody>
        {problems.length === 0 ? (
          <Empty title="Nothing is reporting a problem">
            Every condition across every kind is True, and no pod is failing.
          </Empty>
        ) : (
          <Table variant="compact" aria-label="problems">
            <Thead>
              <Tr>
                <Th>Object</Th>
                <Th>Condition</Th>
                <Th>Reason</Th>
                <Th>Message</Th>
                <Th>Source</Th>
              </Tr>
            </Thead>
            <Tbody>
              {problems.map((p, i) => (
                <Tr key={`${p.kind}/${p.name}/${p.type}/${i}`}>
                  <Td dataLabel="Object">
                    {p.kind === 'pods' ? (
                      <PlainText>{p.name}</PlainText>
                    ) : (
                      <Link to={`/config/${p.kind}/${p.name}`}>
                        <PlainText>{`${p.kind}/${p.name}`}</PlainText>
                      </Link>
                    )}
                  </Td>
                  <Td dataLabel="Condition">{p.type}</Td>
                  <Td dataLabel="Reason">
                    <PlainText>{p.reason}</PlainText>
                  </Td>
                  <Td dataLabel="Message">
                    <PlainText multiline>{p.message}</PlainText>
                  </Td>
                  <Td dataLabel="Source">
                    <Label color={SOURCE_LABEL[p.source].color}>{SOURCE_LABEL[p.source].text}</Label>
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
