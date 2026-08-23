import { useMemo, useState, type ReactNode } from 'react'
import {
  Alert, Button, Label, LabelGroup, Split, SplitItem, Stack, StackItem, ToggleGroup,
  ToggleGroupItem,
} from '@patternfly/react-core'
import type { ActivityEvent, GraphEdge, Topology } from '../api/types'
import { useDisplay } from './display'
import {
  animationDuration, edgeId, edgeLabel, edgeTone, liveEdgeIds, scopedGraph, visibleGraph,
  type ScopeDepth,
} from './model'
import { DisplayPanel } from './DisplayPanel'
import { NodeDetails } from './NodeDetails'
import { PlainText } from '../components/Text'
import { Viewport } from './Viewport'
import { anchors, layout, type PlacedNode } from './layout'
import { Glyph, NODE_H, NODE_W, shapeDecoration, shapePath, styleFor } from './shapes'

// Colours come from the app's own tokens (theme/theme.css), which BOTH themes
// define — so the graph follows light/dark without a second palette here, and
// cannot drift from what PatternFly's components render.
const TONE_COLOR: Record<string, string> = {
  idle: 'var(--ao-edge-idle)',
  ok: 'var(--ao-success)',
  error: 'var(--ao-danger)',
  unconfirmed: 'var(--ao-warning)',
}

const HEALTH_COLOR: Record<string, string> = {
  ok: 'var(--ao-success)',
  bad: 'var(--ao-danger)',
  unknown: 'var(--ao-warning)',
  none: 'var(--ao-neutral)',
}

export interface GraphProps {
  topology: Topology
  liveEvents: ActivityEvent[]
  renderNodeExtras?: (node: PlacedNode) => ReactNode
  emptyMessage?: string
}

export function Graph({ topology, liveEvents, renderNodeExtras, emptyMessage }: GraphProps) {
  const { hidden, animate, showIdle, edgeLabels, panelOpen, setPanelOpen } = useDisplay()
  // The selected node IS the scope. One click answers "what is this" and "what
  // is it connected to" together, and clicking it again already meant "never
  // mind" — so that gesture is also the way back to the whole picture.
  const [selected, setSelected] = useState<string | undefined>()
  // Kept across re-scoping: stepping from one element to its neighbour is
  // exploring at a chosen depth, not starting again.
  const [depth, setDepth] = useState<ScopeDepth>('all')

  const view = useMemo(() => visibleGraph(topology, hidden, showIdle), [topology, hidden, showIdle])
  // Class hiding first, scope second: reachability is computed over what the
  // operator can SEE, so a hidden class never joins two nodes as an invisible
  // stepping stone.
  const scoped = useMemo(
    () => scopedGraph(view.nodes, view.edges, selected ? { id: selected, depth } : undefined),
    [view.nodes, view.edges, selected, depth],
  )
  const placedLayout = useMemo(() => layout(scoped.nodes), [scoped.nodes])
  const byId = useMemo(
    () => new Map(placedLayout.nodes.map((n) => [n.id, n])),
    [placedLayout.nodes],
  )
  const live = useMemo(
    () => liveEdgeIds(liveEvents, topology.eventNodeKinds ?? {}, scoped.edges),
    [liveEvents, topology.eventNodeKinds, scoped.edges],
  )
  const selectedNode = placedLayout.nodes.find((n) => n.id === selected)
  const scopedName = view.nodes.find((n) => n.id === selected)?.name
  // Rings strictly INSIDE the route, then All. Empty when the route is a single
  // hop or less, which includes a detached element whose route is only itself.
  const levels = useMemo<ScopeDepth[]>(() => {
    const rings = Math.max(0, Math.min(scoped.maxDepth - 1, 3))
    return rings === 0 ? [] : [...Array(rings)].map((_, i) => i + 1 as ScopeDepth).concat('all')
  }, [scoped.maxDepth])
  // Re-fit when the SET of nodes changes, not on every traffic refresh — a
  // viewport that snapped back every ten seconds would be unusable.
  const fitKey = useMemo(() => placedLayout.nodes.map((n) => n.id).join('|'), [placedLayout.nodes])

  return (
    <Split hasGutter>
      <SplitItem isFilled>
        <Stack hasGutter>
          <StackItem>
            {/* Collapsing the panel hands its width to the graph. The toggle
                stays visible when collapsed, and it reports what is currently
                filtered — a hidden panel must not also hide the fact that a
                filter is on. */}
            <Split hasGutter style={{ alignItems: 'center' }}>
              <SplitItem>
                <Button
                  variant="secondary"
                  onClick={() => setPanelOpen(!panelOpen)}
                  aria-expanded={panelOpen}
                  aria-label={panelOpen ? 'hide display panel' : 'show display panel'}
                >
                  {panelOpen ? '◀ Hide display' : 'Display ▶'}
                </Button>
              </SplitItem>
              {!panelOpen && (
                <SplitItem>
                  <LabelGroup categoryName="Health">
                    <Label isCompact color="green">{view.health.ok} ok</Label>
                    <Label isCompact color="red">{view.health.bad} failing</Label>
                    {view.hiddenSummary.count > 0 && (
                      <Label isCompact color="grey">{view.hiddenSummary.count} hidden</Label>
                    )}
                  </LabelGroup>
                </SplitItem>
              )}
            </Split>
          </StackItem>
          {selected && (
            <StackItem>
              {/* A narrowed graph must never be mistaken for a small install,
                  so the bar names what it is scoped to and how far it reaches. */}
              <Split hasGutter style={{ alignItems: 'center' }} data-testid="scope-bar">
                <SplitItem>
                  <Label color="blue" isCompact>
                    Scoped to <PlainText>{scopedName ?? selected}</PlainText>
                  </Label>
                </SplitItem>
                <SplitItem>
                  {/* Only the levels that EXIST for this element. A level at or
                      past the end of the route is the same picture as All, and a
                      button that changes nothing reads as a broken control
                      rather than as a short route. A route one hop deep — or a
                      detached element, whose route is only itself — offers no
                      choice at all, so the control goes away. */}
                  {levels.length > 0 && (
                    <ToggleGroup aria-label="scope depth">
                      {levels.map((d) => (
                        <ToggleGroupItem
                          key={String(d)}
                          text={d === 'all' ? 'All' : String(d)}
                          aria-label={d === 'all' ? 'all connected' : `${d} hop`}
                          isSelected={
                            d === 'all'
                              ? depth === 'all'
                                || (typeof depth === 'number' && depth >= scoped.maxDepth)
                              : depth === d
                          }
                          onChange={() => setDepth(d)}
                        />
                      ))}
                    </ToggleGroup>
                  )}
                </SplitItem>
                {scoped.beyondDepth > 0 && (
                  <SplitItem>
                    <Label isCompact color="grey">
                      {scoped.beyondDepth} connected beyond this depth
                    </Label>
                  </SplitItem>
                )}
                <SplitItem>
                  {/* "Reset scope", not "Reset": the Display panel carries a
                      "Reset display" beside it, and two buttons a screen reader
                      announces as Reset on one screen name nothing. */}
                  <Button
                    variant="link"
                    isInline
                    onClick={() => setSelected(undefined)}
                    aria-label="reset scope"
                  >
                    Reset scope
                  </Button>
                </SplitItem>
              </Split>
            </StackItem>
          )}
          {scoped.outOfScope.failing > 0 && (
            <StackItem>
              {/* Reported SEPARATELY from the display panel's own hidden count.
                  "I hid that class" and "that is outside what I am looking at"
                  are different statements, and one number would answer neither. */}
              <Alert
                variant="warning"
                isInline
                title={`${scoped.outOfScope.failing} failing element(s) are outside this scope`}
                data-testid="scope-failing"
              >
                Out-of-scope classes with failures: {scoped.outOfScope.classes.join(', ')}. They
                still count in the health summary and the overview rollup.
              </Alert>
            </StackItem>
          )}
          {view.hiddenSummary.failing > 0 && (
            <StackItem>
              {/* A filter that could conceal a broken component without saying
                  so is the one way this view could mislead. */}
              <Alert
                variant="warning"
                isInline
                title={`${view.hiddenSummary.failing} hidden element(s) are failing`}
              >
                Hidden classes with failures: {view.hiddenSummary.classes.join(', ')}. They still
                count in the health summary and the overview rollup.
              </Alert>
            </StackItem>
          )}
          <StackItem>
            {placedLayout.nodes.length === 0 ? (
              <Alert
                variant="info"
                isInline
                title="Nothing to show"
                data-testid="graph-empty"
              >
                {emptyMessage ?? 'Every element class is hidden, or nothing is configured yet.'}
              </Alert>
            ) : (
              <Viewport
                contentWidth={placedLayout.width}
                contentHeight={placedLayout.height}
                ariaLabel="topology graph"
                fitKey={fitKey}
              >
                <defs>
                  <marker
                    id="arrow"
                    viewBox="0 0 10 10"
                    refX="9"
                    refY="5"
                    markerWidth="6"
                    markerHeight="6"
                    orient="auto-start-reverse"
                  >
                    <path d="M 0 0 L 10 5 L 0 10 z" fill="currentColor" />
                  </marker>
                </defs>

                {/* Boundaries first, so everything else sits inside them. */}
                {placedLayout.lanes.map((lane) => (
                  <g key={lane.id} data-testid={`lane-${lane.id}`}>
                    <rect
                      x={lane.x}
                      y={lane.y}
                      width={lane.width}
                      height={lane.height}
                      rx={10}
                      fill="var(--ao-lane-fill)"
                      fillOpacity={0.55}
                      stroke="var(--ao-lane-border)"
                      strokeDasharray="4 4"
                    />
                    <text
                      x={lane.x + 14}
                      y={lane.y + 22}
                      fontSize={12}
                      fontWeight={600}
                      fill="var(--ao-text)"
                    >
                      {lane.title}
                    </text>
                    <text
                      x={lane.x + 14}
                      y={lane.y + 34}
                      fontSize={10}
                      fill="var(--ao-text-subtle)"
                    >
                      {lane.hint}
                    </text>
                  </g>
                ))}

                {scoped.edges.map((e) => (
                  <EdgeShape
                    key={edgeId(e)}
                    edge={e}
                    from={byId.get(e.from)}
                    to={byId.get(e.to)}
                    animate={animate}
                    flashing={live.has(edgeId(e))}
                    label={edgeLabel(e, edgeLabels)}
                  />
                ))}
                {placedLayout.nodes.map((n) => (
                  <NodeShape
                    key={n.id}
                    node={n}
                    selected={n.id === selected}
                    onSelect={() => setSelected(n.id === selected ? undefined : n.id)}
                  />
                ))}
              </Viewport>
            )}
          </StackItem>
        </Stack>
      </SplitItem>
      {(panelOpen || selectedNode) && (
        <SplitItem style={{ minWidth: 320, maxWidth: 380 }}>
          <Stack hasGutter>
            {panelOpen && (
              <StackItem>
                <DisplayPanel health={view.health} hiddenSummary={view.hiddenSummary} />
              </StackItem>
            )}
            {selectedNode && (
              <StackItem>
                <NodeDetails node={selectedNode} extras={renderNodeExtras?.(selectedNode)} />
              </StackItem>
            )}
          </Stack>
        </SplitItem>
      )}
    </Split>
  )
}

function NodeShape({
  node,
  selected,
  onSelect,
}: {
  node: PlacedNode
  selected: boolean
  onSelect: () => void
}) {
  const style = styleFor(node.kind)
  const color = HEALTH_COLOR[node.health] ?? HEALTH_COLOR.none
  const name = node.name.length > 17 ? `${node.name.slice(0, 16)}…` : node.name

  return (
    <g
      transform={`translate(${node.x},${node.y})`}
      onClick={onSelect}
      style={{ cursor: 'pointer' }}
      data-testid={`node-${node.id}`}
      data-health={node.health}
      data-shape={style.shape}
      data-detached={node.detached ? 'true' : 'false'}
      color={color}
    >
      <path
        d={shapePath(style.shape)}
        fill="var(--ao-surface)"
        stroke={selected ? 'var(--ao-brand)' : color}
        strokeWidth={selected ? 3 : 2}
        // a detached node is drawn open: nothing in the wiring holds it
        strokeDasharray={node.detached ? '6 4' : undefined}
      />
      {shapeDecoration(style.shape)}
      <Glyph d={style.glyph} x={18} y={NODE_H / 2 - 8} />
      <text x={42} y={NODE_H / 2 - 4} fontSize={10} fill="var(--ao-text-subtle)">
        {style.label}
      </text>
      <text x={42} y={NODE_H / 2 + 12} fontSize={13} fill="var(--ao-text)">
        {name}
      </text>
      {node.active > 0 && (
        <g>
          <circle cx={NODE_W - 16} cy={16} r={10} fill="var(--ao-brand)" />
          <text x={NODE_W - 16} y={20} fontSize={11} fill="#fff" textAnchor="middle">
            {node.active}
          </text>
        </g>
      )}
      <title>
        {`${style.label} ${node.name}` +
          (node.health !== 'none' ? ` — ${node.health}` : '') +
          (node.reason ? ` (${node.reason})` : '')}
      </title>
    </g>
  )
}

function EdgeShape({
  edge,
  from,
  to,
  animate,
  flashing,
  label,
}: {
  edge: GraphEdge
  from?: PlacedNode
  to?: PlacedNode
  animate: boolean
  flashing: boolean
  label: string
}) {
  if (!from || !to) return null
  const tone = edgeTone(edge)
  const color = TONE_COLOR[tone]
  const { x1, y1, x2, y2 } = anchors(from, to)
  const mx = (x1 + x2) / 2
  const path = `M ${x1} ${y1} C ${mx} ${y1}, ${mx} ${y2}, ${x2} ${y2}`
  const duration = edge.traffic ? animationDuration(edge.traffic.ratePerMin) : 0

  return (
    <g
      color={color}
      data-testid={`edge-${edgeId(edge)}`}
      data-tone={tone}
      data-animated={animate && duration > 0 ? 'true' : 'false'}
      data-flashing={flashing ? 'true' : 'false'}
    >
      <path
        d={path}
        fill="none"
        stroke={color}
        strokeWidth={flashing ? 3 : 1.5}
        strokeDasharray={edge.dangling ? '2 4' : animate && duration > 0 ? '6 6' : undefined}
        markerEnd="url(#arrow)"
        opacity={tone === 'idle' ? 0.5 : 1}
      >
        {animate && duration > 0 && (
          <animate
            attributeName="stroke-dashoffset"
            from="12"
            to="0"
            dur={`${duration}s`}
            repeatCount="indefinite"
          />
        )}
      </path>
      {label && (
        <text
          x={mx}
          y={(y1 + y2) / 2 - 6}
          fontSize={10}
          textAnchor="middle"
          fill="var(--ao-text-subtle)"
        >
          {label}
        </text>
      )}
      <title>{`${edge.kind}${edge.dangling ? ' (unresolved reference)' : ''}`}</title>
    </g>
  )
}
