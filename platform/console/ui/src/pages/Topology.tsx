import { useMemo } from 'react'
import {
  Alert, Card, CardBody, CardTitle, Label, PageSection, Stack, StackItem, Title,
} from '@patternfly/react-core'
import { ErrorState, Loading } from '../components/States'
import { useTopology } from '../api/hooks'
import { useDisplay } from '../graph/display'
import { useStream } from '../api/stream'
import { Graph } from '../graph/Graph'
import { PlainText } from '../components/Text'
import { Crumbs } from '../components/Crumbs'
import { HistoryCharts } from './History'

export function TopologyPage() {
  const windowSeconds = useDisplay((s) => s.windowSeconds)
  const { data, isLoading, error } = useTopology(windowSeconds)
  const liveEvents = useStream((s) => s.events)

  // A window longer than the buffer holds is reported as such rather than
  // rendered as a quiet one — "we cannot see that far back" and "nothing
  // happened" are different claims.
  const bufferCovers = useMemo(() => {
    if (!data?.oldestEvent) return true
    const oldest = new Date(data.oldestEvent).getTime()
    return Date.now() - oldest >= windowSeconds * 1000
  }, [data?.oldestEvent, windowSeconds])

  if (isLoading && !data) return <Loading />
  if (error || !data) return <ErrorState title="Could not load the topology">{String(error)}</ErrorState>

  return (
    <>
      <Crumbs items={[{ label: 'Topology' }]} />
      <PageSection>
      <Stack hasGutter>
        <StackItem>
          <Title headingLevel="h1">Topology</Title>
        </StackItem>

        {!bufferCovers && (
          <StackItem>
            {/* Two DIFFERENT situations that the same sentence used to describe,
                which read as "no backend configured" even when one was. */}
            {data.metricsAvailable ? (
              <Alert
                variant="info"
                isInline
                title="Edge rates cover the manager's live buffer, not the whole window"
              >
                A metrics backend <strong>is</strong> connected, but it cannot extend these
                particular numbers: metric labels are bounded by CR count, so there is no per-edge
                series to read. Edge rates are therefore always live-buffer only. The aggregate
                charts below do cover this window.
              </Alert>
            ) : (
              <Alert variant="warning" isInline title="No metrics backend is configured">
                Edge rates cover only what the manager still holds in memory, and nothing covers
                longer windows. Set <code>console.metrics.url</code> to a Prometheus or
                VictoriaMetrics query endpoint for historical aggregates.
              </Alert>
            )}
          </StackItem>
        )}
        {!data.stream.connected && (
          <StackItem>
            <Alert variant="warning" isInline title="The activity stream is disconnected">
              Edges will not animate — this graph shows wiring, not current traffic.
            </Alert>
          </StackItem>
        )}
        {data.stream.lastGap && (
          <StackItem>
            {/* A quiet edge and an edge whose traffic was never recorded look
                identical. Only one of them means the system was idle. */}
            <Alert variant="info" isInline title="This window has a gap in recorded activity">
              Nothing was recorded before {new Date(data.stream.lastGap.ts).toLocaleTimeString()} — the
              manager's activity buffer is in memory, so a restart or an overflow ends its history.
              Edge rates covering that stretch under-report; the wiring itself is read from
              Kubernetes and is complete.
            </Alert>
          </StackItem>
        )}

        <StackItem>
          <Graph topology={data.topology} liveEvents={liveEvents} />
        </StackItem>

        {(data.unjoinedPipelines ?? []).length > 0 && (
          <StackItem>
            <Card>
              <CardTitle>Pipelines the console is not joined to</CardTitle>
              <CardBody>
                <p>
                  Their conversations are visible but read-only — no console thread means nowhere to
                  reply. Add <code>{data.consoleChannel}</code> to a pipeline's{' '}
                  <code>channelRefs</code> to join it. The console never edits a Pipeline.
                </p>
                {(data.unjoinedPipelines ?? []).map((p) => (
                  <Label key={p} style={{ marginRight: 8 }}>
                    <PlainText>{p}</PlainText>
                  </Label>
                ))}
              </CardBody>
            </Card>
          </StackItem>
        )}

        <StackItem>
          <HistoryCharts available={data.metricsAvailable} />
        </StackItem>
      </Stack>
      </PageSection>
    </>
  )
}
