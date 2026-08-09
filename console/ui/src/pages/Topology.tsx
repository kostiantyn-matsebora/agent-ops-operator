import { useMemo } from 'react'
import {
  Alert, Card, CardBody, CardTitle, Label, PageSection, Stack, StackItem, Title,
} from '@patternfly/react-core'
import { ErrorState, Loading } from '../App'
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
            <Alert variant="info" isInline title="This window is longer than the recorded buffer">
              Rates cover only what the manager still holds. Longer windows come from a metrics
              backend when one is configured{data.metricsAvailable ? '' : ' — none is'}.
            </Alert>
          </StackItem>
        )}
        {!data.stream.connected && (
          <StackItem>
            <Alert variant="warning" isInline title="The activity stream is disconnected">
              Edges will not animate — this graph shows wiring, not current traffic.
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
