import { Card, CardBody, CardTitle, Grid, GridItem, Label } from '@patternfly/react-core'
import { Chart, ChartAxis, ChartGroup, ChartLine, ChartThemeColor } from '@patternfly/react-charts/victory'
import { Empty } from '../App'
import { useChart } from '../api/hooks'
import { useDisplay } from '../graph/display'

// Historical charts, from the metrics backend.
//
// PRESENT ONLY WHEN A BACKEND IS CONFIGURED. With none, this renders the reason
// rather than an empty chart — an empty chart says "there was no traffic", which
// is a claim the console cannot make about a window it never had data for.
//
// Everything here is an AGGREGATE and is labelled as one: the cardinality rule
// keeps ids out of metric labels, so no point can identify a conversation. Live,
// per-item detail lives in the activity stream and /status.

const CHARTS: { key: string; title: string; unit: string }[] = [
  { key: 'throughput', title: 'Conversations started', unit: '/min' },
  { key: 'runDurationP95', title: 'Run duration (p95)', unit: 's' },
  { key: 'queueDepth', title: 'Channel ops queued', unit: '' },
]

export function HistoryCharts({ available }: { available: boolean }) {
  if (!available) {
    return (
      <Card>
        <CardTitle>History</CardTitle>
        <CardBody>
          <Empty title="No metrics backend is configured">
            Windows beyond the manager's activity buffer are unavailable. Set{' '}
            <code>console.metrics.url</code> to a Prometheus or VictoriaMetrics query endpoint to
            enable throughput, run-duration percentiles and queue depth over long ranges.
          </Empty>
        </CardBody>
      </Card>
    )
  }
  return (
    <Grid hasGutter>
      {CHARTS.map((c) => (
        <GridItem key={c.key} md={4}>
          <HistoryChart chartKey={c.key} title={c.title} unit={c.unit} />
        </GridItem>
      ))}
    </Grid>
  )
}

function HistoryChart({
  chartKey,
  title,
  unit,
}: {
  chartKey: string
  title: string
  unit: string
}) {
  const windowSeconds = useDisplay((s) => s.windowSeconds)
  // Charts deliberately ask for a LONGER range than the graph's window: their
  // whole purpose is the horizon the buffer cannot reach.
  const range = Math.max(windowSeconds, 6 * 3600)
  const { data, error, isLoading } = useChart(chartKey, range, true)

  return (
    <Card>
      <CardTitle>
        {title} <Label color="grey">aggregate</Label>
      </CardTitle>
      <CardBody>
        {isLoading && <Empty title="Loading…" />}
        {error && <Empty title="Unavailable">{String(error)}</Empty>}
        {data && data.series.length === 0 && (
          <Empty title="No data in this range">The backend returned no samples.</Empty>
        )}
        {data && data.series.length > 0 && (
          <Chart
            ariaTitle={title}
            height={200}
            padding={{ top: 12, bottom: 40, left: 56, right: 12 }}
            themeColor={ChartThemeColor.multiUnordered}
          >
            <ChartAxis tickFormat={(t: number) => new Date(t * 1000).toLocaleTimeString()} tickCount={4} />
            <ChartAxis dependentAxis showGrid tickFormat={(v: number) => `${v.toFixed(1)}${unit}`} />
            <ChartGroup>
              {data.series.map((s, i) => (
                <ChartLine
                  key={i}
                  data={s.points.map((p) => ({ x: p.ts, y: p.value }))}
                  name={Object.values(s.labels).join('/') || String(i)}
                />
              ))}
            </ChartGroup>
          </Chart>
        )}
      </CardBody>
    </Card>
  )
}
