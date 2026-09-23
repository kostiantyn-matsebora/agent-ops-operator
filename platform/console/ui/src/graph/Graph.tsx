import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  Alert, Button, Label, Split, SplitItem, Stack, StackItem, ToggleGroup, ToggleGroupItem,
} from '@patternfly/react-core'
import type { ActivityEvent, EdgeTraffic, Topology } from '../api/types'
import { PlainText } from '../components/Text'
import { ArrowDefs, BoxRect, EdgeLine, NodeMark, TrafficLayer } from './Canvas'
import { useDisplay } from './display'
import { compile, depthLevels, visible, type Scope, type ScopeDepth } from './filter'
import { crossing, edgeLabel, edgeTone, hopIndex, tsOf, windowStats } from './hops'
import { memberBoxes, pictureBounds, runLayout, type Canvas as CanvasSize } from './layout'
import { groupsOf } from './layout/boxes'
import { arcCurve, basisCurve, type Curve, type Pt } from './layout/curve'
import { ConversationPanel, EdgePanel, FeedPanel, HopPanel, NodePanel, clock } from './Panel'
import { ConversationReplayBar, WindowReplayBar } from './ReplayBars'
import {
  FRAME_MS, FRAME_WINDOW_MS, activeConversations, conversationRoute, frameHops, frameTime, latestRun, playDelay,
  replayWindow, steps,
} from './replay'
import { pipelinesOf, routeMembers, routeOwners } from './route'
import { DisplayPanel, GraphToolbar, type Mode } from './Toolbar'
import { pulseDuration, pulseLife, type Pulse } from './traffic'
import { isSpine, VIEWS, type ViewEdge } from './types'
import { Viewport } from './Viewport'
import { buildView } from './views'

// THE TOPOLOGY: three views of one activity feed, each a layer of the
// architecture with its own node identity. A hop maps onto every view's nodes,
// so the same event animates each picture; the feed, the hop panel and both
// replays are identical across views, and only the identity changes.

export interface GraphProps {
  topology: Topology
  /** Every hop held, the activity buffer and the live stream together. */
  events: ActivityEvent[]
  /** The oldest hop the buffer holds, in ms: how far back a replay can see. */
  bufferStart?: number
  /** Open on this conversation's replay. */
  conversation?: string
  emptyMessage?: string
}

const CALLS = new Set(['model.call', 'tool.call'])

export function Graph({ topology, events, bufferStart, conversation, emptyMessage }: GraphProps) {
  const d = useDisplay()
  const view = d.view
  const vd = d.views[view]
  const [expanded, setExpanded] = useState<ReadonlySet<string>>(new Set())
  const [scope, setScope] = useState<Scope>()
  const [selected, setSelected] = useState<string>()
  const [selectedEdge, setSelectedEdge] = useState<string>()
  const [hop, setHop] = useState<ActivityEvent>()
  const [find, setFind] = useState('')
  const [hide, setHide] = useState('')
  const [mode, setMode] = useState<Mode>(conversation ? 'conversation' : 'live')
  const [replay, setReplay] = useState({ interval: 300, frame: 0, playing: false, speed: 3000, anchor: Date.now() })
  const [conv, setConv] = useState<{ name: string; i: number; playing: boolean } | undefined>(
    conversation ? { name: conversation, i: 0, playing: false } : undefined,
  )
  const [pulses, setPulses] = useState<Pulse[]>([])
  const [dragged, setDragged] = useState<Map<string, Pt>>(new Map())
  const [canvas, setCanvas] = useState<CanvasSize>({ w: 1400, h: 700 })
  const host = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const el = host.current
    if (!el || typeof ResizeObserver === 'undefined') return
    const ro = new ResizeObserver(() => {
      const w = Math.round((el.clientWidth || 1400) / 100) * 100
      const h = Math.max(600, window.innerHeight - 160)
      setCanvas((c) => (c.w === w && c.h === h ? c : { w, h }))
    })
    ro.observe(el)
    return () => ro.disconnect()
  }, [])

  // A scope is not carried across views: it names an element of one of them.
  useEffect(() => {
    setScope(undefined)
    setSelected(undefined)
    setSelectedEdge(undefined)
  }, [view])

  // The live window ends now, and slides whenever a hop arrives or the
  // topology's own rate refresh lands — no second timer of its own.
  const tick = useMemo(() => Date.now(), [events, topology, mode])
  const pipelines = useMemo(() => pipelinesOf(topology), [topology])
  const g = useMemo(() => buildView(view, topology, { detail: d.detail, expanded }), [view, topology, d.detail, expanded])
  const index = useMemo(() => hopIndex(g), [g])
  const owners = useMemo(() => routeOwners(topology), [topology])
  const callEvents = useMemo(() => events.filter((e) => CALLS.has(e.kind)), [events])
  const routeSets = useMemo(
    () => new Map(pipelines.map((p) => [p, routeMembers(topology, g, p, callEvents)])),
    [pipelines, topology, g, callEvents],
  )

  // ---- the clock the stats are read at -----------------------------------------------
  const win = useMemo(() => replayWindow(replay.anchor, replay.interval, bufferStart), [replay.anchor, replay.interval, bufferStart])
  const convSteps = useMemo(() => (conv ? steps(latestRun(events, conv.name)) : []), [conv?.name, events])
  const frameT = frameTime(win, replay.frame)
  const stats = useMemo(() => {
    if (mode === 'conversation') {
      const upto = convSteps.slice(0, (conv?.i ?? 0) + 1).map((s) => s.ev)
      const end = upto.length ? tsOf(upto[upto.length - 1]) : 0
      return windowStats(g, index, upto, end, Math.max(1, end - (upto.length ? tsOf(upto[0]) : 0)) + 1)
    }
    if (mode === 'replay') return windowStats(g, index, events, frameT, Math.min(d.windowSeconds * 1000, FRAME_WINDOW_MS))
    return windowStats(g, index, events, tick, d.windowSeconds * 1000)
  }, [mode, g, index, events, frameT, tick, d.windowSeconds, convSteps, conv?.i])
  const trafficOf = useCallback(
    (e: ViewEdge): EdgeTraffic | undefined =>
      stats.edges.get(e.id) ?? (mode === 'live' && view === 'model' ? e.traffic : undefined),
    [stats, mode, view],
  )

  // ---- what is drawn -------------------------------------------------------------------
  const ctx = { counts: stats.nodes, windowSeconds: d.windowSeconds }
  const hideExpr = compile(hide, ctx)
  const findExpr = compile(find, ctx)
  const selectedRoutes = d.pipelines?.filter((p) => pipelines.includes(p))
  const routes = useMemo(() => {
    if (!selectedRoutes || selectedRoutes.length === pipelines.length) return undefined
    const out = new Set<string>()
    for (const p of selectedRoutes) for (const id of routeSets.get(p) ?? []) out.add(id)
    return out
  }, [selectedRoutes?.join('|'), pipelines, routeSets])
  const scopeMembers = useMemo(() => {
    if (!scope || view === 'model') return undefined
    const out = new Set<string>()
    for (const set of routeSets.values()) if (set.has(scope.id)) set.forEach((id) => out.add(id))
    return out.size ? out : undefined
  }, [scope, view, routeSets])
  const busyEdges = new Set(g.edges.filter((e) => (trafficOf(e)?.events ?? 0) > 0).map((e) => e.id))
  const busyNodes = new Set(stats.nodes.keys())
  const vis = visible(g, {
    hiddenClasses: new Set(vd.hidden), hide: hideExpr, routes, idleNodes: d.idleNodes, idleEdges: d.idleEdges,
    busyEdges, busyNodes, scope, scopeMembers,
  })
  const visIndex = useMemo(() => hopIndex(vis), [vis.nodes.map((n) => n.id).join('|'), vis.edges.map((e) => e.id).join('|')])
  const health = useMemo(() => {
    const h = { ok: 0, bad: 0, unknown: 0 }
    for (const n of g.nodes) if (n.health !== 'none') h[n.health]++
    return h
  }, [g])

  // ---- where it is drawn ---------------------------------------------------------------
  const groups = useMemo(() => groupsOf(vis.nodes, view, vd.box, owners), [vis.nodes, view, vd.box, owners])
  const layoutKey = [view, vd.layout, vd.box, canvas.w, canvas.h, vis.nodes.map((n) => n.id).join('|'), vis.edges.map((e) => e.id).join('|')].join('#')
  const placement = useMemo(
    () => runLayout({ nodes: vis.nodes, edges: vis.edges, groups, hub: g.hub, layout: vd.layout, canvas }),
    [layoutKey],
  )
  // A dragged node keeps its place until the next layout.
  useEffect(() => setDragged(new Map()), [placement])
  const pos = useMemo(() => {
    const m = new Map(placement.pos)
    for (const [id, p] of dragged) if (m.has(id)) m.set(id, p)
    return m
  }, [placement, dragged])
  const boxes = dragged.size ? memberBoxes(groups, pos) : placement.boxes
  const curves = useMemo(() => {
    const out = new Map<string, Curve>()
    for (const e of vis.edges) {
      const a = pos.get(e.from)
      const b = pos.get(e.to)
      if (!a || !b) continue
      const routed = placement.routes.get(e.id)
      out.set(e.id, routed && !dragged.has(e.from) && !dragged.has(e.to) ? basisCurve(a, b, routed) : arcCurve(a, b))
    }
    return out
  }, [vis.edges, pos, placement, dragged])
  const bounds = pictureBounds(pos, boxes)

  // ---- pulses --------------------------------------------------------------------------
  const pulseSeq = useRef(0)
  const spawn = useCallback(
    (items: { ev: ActivityEvent; delay?: number; dur?: number }[]) => {
      if (!d.animate || items.length === 0) return
      const now = Date.now()
      const fresh: Pulse[] = []
      for (const { ev, delay = 0, dur } of items) {
        const x = crossing(g, visIndex, ev)
        if (!x) continue
        fresh.push({ key: `p${pulseSeq.current++}`, ev, x, t0: now + delay, dur: dur ?? pulseDuration(ev) })
      }
      if (fresh.length) setPulses((prev) => [...prev.filter((p) => Date.now() - p.t0 <= pulseLife(p)), ...fresh])
    },
    [d.animate, g, visIndex],
  )
  // Live: a hop that arrives pulses at once. What was already held does not.
  const seen = useRef<string | undefined>(undefined)
  useEffect(() => {
    const latest = events.reduce((m, e) => (e.cursor > m ? e.cursor : m), '')
    const last = seen.current
    seen.current = latest
    if (last === undefined || mode !== 'live') return
    spawn(events.filter((e) => e.cursor > last).map((ev) => ({ ev })))
  }, [events])
  // Replay: every hop of the frame pulses, spread across the frame.
  useEffect(() => {
    if (mode !== 'replay') return
    setPulses([])
    const t = frameTime(win, replay.frame)
    spawn(frameHops(events, t).map((ev) => ({
      ev, dur: Math.max(600, replay.speed * 0.5), delay: ((tsOf(ev) - (t - FRAME_MS)) / FRAME_MS) * replay.speed * 0.3,
    })))
  }, [mode, replay.frame, win])
  // Conversation: the current hop pulses.
  useEffect(() => {
    if (mode !== 'conversation' || !conv) return
    const s = convSteps[conv.i]
    setPulses([])
    if (s) spawn([{ ev: s.ev, dur: 1100 }])
  }, [mode, conv?.i, conv?.name, convSteps.length])

  // ---- players -------------------------------------------------------------------------
  useEffect(() => {
    if (mode !== 'replay' || !replay.playing) return
    const id = setTimeout(() => {
      setReplay((r) => (r.frame >= win.frames ? { ...r, playing: false } : { ...r, frame: r.frame + 1 }))
    }, replay.speed)
    return () => clearTimeout(id)
  }, [mode, replay.playing, replay.speed, replay.frame, win.frames])
  useEffect(() => {
    if (mode !== 'conversation' || !conv?.playing) return
    if (conv.i >= convSteps.length - 1) {
      setConv((c) => (c ? { ...c, playing: false } : c))
      return
    }
    const gap = tsOf(convSteps[conv.i + 1].ev) - tsOf(convSteps[conv.i].ev)
    const id = setTimeout(() => setConv((c) => (c ? { ...c, i: c.i + 1 } : c)), playDelay(gap))
    return () => clearTimeout(id)
  }, [mode, conv?.playing, conv?.i, convSteps])

  const openConversation = (name: string) => {
    setMode('conversation')
    setConv({ name, i: 0, playing: false })
    setHop(undefined)
    setSelected(undefined)
    setSelectedEdge(undefined)
  }
  const toLive = () => {
    setMode('live')
    setConv(undefined)
    setPulses([])
    setReplay((r) => ({ ...r, playing: false }))
  }
  const toReplay = () => {
    setConv(undefined)
    setMode('replay')
    setReplay((r) => ({ ...r, frame: 0, playing: false, anchor: Date.now() }))
  }
  const toggleScope = (id: string) => setScope((s) => (s?.id === id ? undefined : { id, depth: 'all' }))
  const togglePod = (podId: string) =>
    setExpanded((prev) => {
      const next = new Set(prev)
      if (next.has(podId)) next.delete(podId)
      else next.add(podId)
      return next
    })

  // ---- what is lit -----------------------------------------------------------------------
  const route = useMemo(
    () => (mode === 'conversation' && conv ? conversationRoute(g, index, convSteps.map((s) => s.ev), conv.name) : undefined),
    [mode, conv?.name, g, index, convSteps],
  )
  const tEnd = mode === 'replay' ? frameT : tick
  const working = useMemo(() => {
    const open = new Map<string, ActivityEvent>()
    for (const e of events) {
      if (!e.conversation || tsOf(e) > tEnd) continue
      if (e.kind === 'run.dispatched') open.set(e.conversation, e)
      else if (e.kind === 'run.completed') open.delete(e.conversation)
    }
    const out = new Set<string>()
    for (const e of open.values()) {
      const x = crossing(g, index, e)
      const last = x?.edges[x.edges.length - 1]
      if (last) out.add(last.dir === 1 ? last.edge.to : last.edge.from)
    }
    return out
  }, [events, tEnd, g, index])
  const activeByPipeline = useMemo(() => new Map(topology.nodes.map((n) => [n.id, n.active])), [topology])

  // ---- the panel -------------------------------------------------------------------------
  const nodeById = useMemo(() => new Map(g.nodes.map((n) => [n.id, n])), [g])
  const panel = (() => {
    if (hop) {
      return (
        <HopPanel
          ev={hop}
          onBack={() => setHop(undefined)}
          onShow={() => spawn([{ ev: hop, dur: 1100 }])}
          onReplay={hop.conversation ? () => openConversation(hop.conversation!) : undefined}
        />
      )
    }
    if (mode === 'conversation' && conv) {
      return (
        <ConversationPanel
          name={conv.name}
          list={convSteps}
          current={conv.i}
          onStep={(i) => setConv({ ...conv, i, playing: false })}
          onOpen={(ev) => setHop(ev)}
        />
      )
    }
    const edge = selectedEdge ? g.edges.find((e) => e.id === selectedEdge) : undefined
    if (edge) {
      const windowMs = mode === 'replay' ? FRAME_WINDOW_MS : d.windowSeconds * 1000
      const history = events
        .filter((e) => {
          const t = tsOf(e)
          return t <= tEnd && t > tEnd - windowMs && crossing(g, index, e)?.edges.some((x) => x.edge.id === edge.id)
        })
        .slice(-8)
        .reverse()
      return (
        <EdgePanel edge={edge} from={nodeById.get(edge.from)} to={nodeById.get(edge.to)} tone={edgeTone(edge, trafficOf(edge))}
          traffic={trafficOf(edge)} history={history} onHop={setHop} />
      )
    }
    const node = selected ? nodeById.get(selected) : undefined
    if (node) {
      const neighbours = (pick: (e: ViewEdge) => string | undefined) =>
        g.edges.filter((e) => pick(e)).map((e) => ({ edge: e, other: nodeById.get(pick(e)!), traffic: trafficOf(e) }))
      const replayable = node.cls === 'conversations' ? node.name : node.conversation
      return (
        <NodePanel
          node={node}
          events={stats.nodes.get(node.id) ?? 0}
          scoped={scope?.id === node.id}
          spine={isSpine(g, node)}
          inbound={neighbours((e) => (e.to === node.id ? e.from : undefined))}
          outbound={neighbours((e) => (e.from === node.id ? e.to : undefined))}
          canReplay={replayable}
          podToggle={node.collapsed ? 'expand' : node.cls === 'container' ? 'collapse' : undefined}
          onScope={() => toggleScope(node.id)}
          onReplay={() => replayable && openConversation(replayable)}
          onTogglePod={() => {
            togglePod(node.pod ?? node.id)
            setSelected(undefined)
          }}
          onHide={() => {
            d.hideClass(node.cls)
            setSelected(undefined)
          }}
        />
      )
    }
    const feed =
      mode === 'replay'
        ? events.filter((e) => tsOf(e) > frameT - FRAME_WINDOW_MS && tsOf(e) <= frameT)
        : events.filter((e) => e.from || e.to)
    return (
      <FeedPanel
        title={mode === 'replay' ? 'Hops in this frame’s minute' : 'Hops, live'}
        feed={feed.slice(-14).reverse()}
        conversations={activeConversations(events, tEnd, d.windowSeconds * 1000).slice(0, 8)}
        onHop={(ev) => {
          setHop(ev)
          spawn([{ ev, dur: 1100 }])
        }}
        onConversation={openConversation}
      />
    )
  })()

  // ---- chips over the canvas ---------------------------------------------------------------
  const levels = depthLevels(vis.maxDepth)
  const scopedName = scope ? nodeById.get(scope.id)?.name ?? scope.id : undefined
  const overlay = (
    <>
      {scope && vis.nodes.some((n) => n.id === scope.id) && (
        <span data-testid="scope-bar" style={{ display: 'inline-flex', gap: 6, alignItems: 'center' }}>
          <Label color="blue" isCompact>Scoped to <PlainText>{scopedName}</PlainText></Label>
          {levels.length > 0 && (
            <ToggleGroup aria-label="scope depth" isCompact>
              {levels.map((l: ScopeDepth) => (
                <ToggleGroupItem
                  key={String(l)}
                  text={l === 'all' ? 'All' : String(l)}
                  aria-label={l === 'all' ? 'all connected' : `${l} hop`}
                  isSelected={l === 'all' ? scope.depth === 'all' || (typeof scope.depth === 'number' && scope.depth >= vis.maxDepth) : scope.depth === l}
                  onChange={() => setScope({ ...scope, depth: l })}
                />
              ))}
            </ToggleGroup>
          )}
          {vis.beyondDepth > 0 && <Label isCompact color="grey">{vis.beyondDepth} connected beyond this depth</Label>}
          <Button variant="link" isInline onClick={() => setScope(undefined)} aria-label="reset scope">Reset scope</Button>
        </span>
      )}
      {routes && <Label isCompact color="blue">routes: {(selectedRoutes ?? []).join(', ') || 'none'}</Label>}
      {vd.layout === 'dagre' && placement.orientation && (
        <Label isCompact>{placement.orientation === 'lr' ? 'left to right' : 'top down'} fits larger</Label>
      )}
      {vis.hidden.count > 0 && (
        <Label isCompact color={vis.hidden.failing ? 'red' : 'grey'} data-testid="hidden-chip">
          {vis.hidden.count} hidden{vis.hidden.failing ? ` · ${vis.hidden.failing} failing (${vis.hidden.classes.join(', ')})` : ''}
        </Label>
      )}
      {vis.outOfRoutes.count > 0 && (
        <Label isCompact color={vis.outOfRoutes.failing ? 'red' : 'grey'}>
          {vis.outOfRoutes.count} on other routes{vis.outOfRoutes.failing ? ` · ${vis.outOfRoutes.failing} failing (${vis.outOfRoutes.classes.join(', ')})` : ''}
        </Label>
      )}
      {mode === 'replay' && <Label isCompact color="blue">replay · {clock(frameT)}</Label>}
      {mode === 'conversation' && conv && <Label isCompact color="purple">conversation · <PlainText>{conv.name}</PlainText></Label>}
    </>
  )

  const viewLabel = VIEWS.find((v) => v.id === view)?.label ?? view
  const unknown = [...(hideExpr?.unknown ?? []), ...(findExpr?.unknown ?? [])]
  const failingOut = [
    ['hidden element(s) are failing', vis.hidden] as const,
    ['failing element(s) are on routes not selected', vis.outOfRoutes] as const,
    ['failing element(s) are outside this scope', vis.outOfScope] as const,
  ].filter(([, s]) => s.failing > 0)

  return (
    <Stack hasGutter>
      <StackItem>
        <GraphToolbar
          pipelines={pipelines}
          health={health}
          find={find}
          hide={hide}
          onFind={setFind}
          onHide={setHide}
          mode={mode}
          onMode={(m) => (m === 'replay' ? (mode === 'replay' ? toLive() : toReplay()) : toLive())}
        />
      </StackItem>
      {mode === 'replay' && (
        <StackItem>
          <WindowReplayBar
            window={win}
            interval={replay.interval}
            frame={replay.frame}
            playing={replay.playing}
            speed={replay.speed}
            onInterval={(interval) => setReplay({ ...replay, interval, frame: 0, playing: false, anchor: Date.now() })}
            onFrame={(frame) => setReplay({ ...replay, frame, playing: false })}
            onPlay={(playing) => setReplay({ ...replay, playing, frame: playing && replay.frame >= win.frames ? 0 : replay.frame })}
            onSpeed={(speed) => setReplay({ ...replay, speed })}
            onClose={toLive}
          />
        </StackItem>
      )}
      {mode === 'conversation' && conv && (
        <StackItem>
          <ConversationReplayBar
            name={conv.name}
            list={convSteps}
            current={conv.i}
            playing={conv.playing}
            onPlay={(playing) => setConv({ ...conv, playing, i: playing && conv.i >= convSteps.length - 1 ? 0 : conv.i })}
            onStep={(i) => setConv({ ...conv, i: Math.max(0, Math.min(convSteps.length - 1, i)), playing: false })}
            onClose={toLive}
          />
        </StackItem>
      )}
      {failingOut.map(([what, s]) => (
        <StackItem key={what}>
          {/* A cut that could conceal a broken component without saying so is
              the one way this view could mislead. */}
          <Alert variant="warning" isInline title={`${s.failing} ${what}`} data-testid="failing-hidden">
            Classes with failures: {s.classes.join(', ')}. They still count in the health summary and the overview rollup.
          </Alert>
        </StackItem>
      ))}
      {unknown.length > 0 && (
        <StackItem>
          <Alert variant="info" isInline isPlain title={`Not understood, matching nothing: ${unknown.join(', ')}`}>
            Terms: healthy, !healthy, idle, detached, kind=, name=, name~, bundle=, node=, pipeline=, rate&gt;, rate&lt;, joined with and.
          </Alert>
        </StackItem>
      )}
      <StackItem>
        <Split hasGutter>
          <SplitItem isFilled style={{ minWidth: 0 }}>
            <div ref={host}>
              {vis.nodes.length === 0 ? (
                <Alert variant="info" isInline title="Nothing to show" data-testid="graph-empty">
                  {emptyMessage ?? 'Every element class is hidden, or nothing is configured yet.'}
                </Alert>
              ) : (
                <Viewport
                  contentX={bounds.x0}
                  contentY={bounds.y0}
                  contentWidth={bounds.x1 - bounds.x0}
                  contentHeight={bounds.y1 - bounds.y0}
                  ariaLabel={`topology graph, ${viewLabel} view`}
                  fitKey={layoutKey}
                  aspect
                  overlay={overlay}
                  onBackgroundClick={() => {
                    setSelected(undefined)
                    setSelectedEdge(undefined)
                    setHop(undefined)
                  }}
                >
                  <ArrowDefs />
                  {[...boxes].sort((a, b) => b.w * b.h - a.w * a.h).map((b) => (
                    <BoxRect
                      key={b.id}
                      box={b}
                      onClick={b.id.startsWith('box:pod:') ? () => togglePod(`pods/${b.id.slice('box:pod:'.length)}`) : undefined}
                    />
                  ))}
                  {vis.edges.map((e) => {
                    const curve = curves.get(e.id)
                    if (!curve) return null
                    const t = trafficOf(e)
                    return (
                      <EdgeLine
                        key={e.id}
                        edge={e}
                        curve={curve}
                        tone={edgeTone(e, t)}
                        label={route && !route.edges.has(e.id) ? '' : edgeLabel(t, d.edgeLabels)}
                        streaming={d.animate && mode !== 'conversation' && (t?.events ?? 0) > 0}
                        selected={selectedEdge === e.id}
                        dim={Boolean(route && !route.edges.has(e.id))}
                        onSelect={() => {
                          setSelectedEdge(e.id)
                          setSelected(undefined)
                          setHop(undefined)
                        }}
                      />
                    )
                  })}
                  {vis.nodes.map((n) => {
                    const at = pos.get(n.id)
                    if (!at) return null
                    const active = activeByPipeline.get(n.id) ?? 0
                    const badge = n.count ? `×${n.count}` : n.collapsed ? '+' : n.cls === 'pipelines' && active > 0 ? String(active) : undefined
                    return (
                      <NodeMark
                        key={n.id}
                        node={n}
                        at={at}
                        role={n.cls === 'pod' ? topology.pods?.find((p) => p.id === n.id)?.component?.split('/')[0] : undefined}
                        selected={selected === n.id}
                        found={Boolean(findExpr?.test(n))}
                        working={working.has(n.id) && !isSpine(g, n)}
                        dim={Boolean(route && !route.nodes.has(n.id))}
                        badge={badge}
                        onSelect={() => {
                          setSelected(n.id)
                          setSelectedEdge(undefined)
                          setHop(undefined)
                        }}
                        onOpen={() => (n.collapsed ? togglePod(n.id) : toggleScope(n.id))}
                        onDrag={(p) => setDragged((m) => new Map(m).set(n.id, p))}
                      />
                    )
                  })}
                  <TrafficLayer
                    edges={vis.edges}
                    curves={curves}
                    pos={pos}
                    traffic={trafficOf}
                    pulses={pulses}
                    stream={d.animate && mode !== 'conversation'}
                    onHop={(ev) => {
                      setHop(ev)
                      setSelected(undefined)
                      setSelectedEdge(undefined)
                    }}
                  />
                </Viewport>
              )}
            </div>
          </SplitItem>
          <SplitItem style={{ width: 360, flex: '0 0 360px' }}>
            <Stack hasGutter>
              {d.panelOpen && (
                <StackItem>
                  <DisplayPanel hidden={vis.hidden} viewLabel={viewLabel} />
                </StackItem>
              )}
              <StackItem>{panel}</StackItem>
            </Stack>
          </SplitItem>
        </Split>
      </StackItem>
    </Stack>
  )
}
