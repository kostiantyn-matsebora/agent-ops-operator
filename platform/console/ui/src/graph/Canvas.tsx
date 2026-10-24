import { useContext, useEffect, useRef, useState, type PointerEvent } from 'react'
import type { ActivityEvent, EdgeTraffic } from '../api/types'
import type { EdgeTone } from './hops'
import type { Box } from './layout'
import type { Curve, Pt } from './layout/curve'
import { resolveIcon } from '../components/Icon'
import { CAPTION_Y, ICON_HALF, LABEL_Y, MARK_SCALE, ROLE_GLYPH, shapePath, styleFor } from './shapes'
import { markPath, pulseAt, pulseLife, streamMarks, type Pulse } from './traffic'
import type { ViewEdge, ViewNode } from './types'
import { ViewportScale } from './Viewport'

// The drawing. Colours come from the app's own tokens (theme/theme.css), which
// BOTH themes define, so the graph follows light and dark without a palette of
// its own.

const TONE_COLOR: Record<EdgeTone, string> = {
  idle: 'var(--ao-edge-idle)',
  ok: 'var(--ao-success)',
  error: 'var(--ao-danger)',
  unconfirmed: 'var(--ao-warning)',
}

const HEALTH_COLOR: Record<string, string> = {
  ok: 'var(--ao-success)',
  bad: 'var(--ao-danger)',
  unknown: 'var(--ao-warning)',
  none: 'var(--ao-border-strong)',
}

export function ArrowDefs() {
  return (
    <defs>
      <marker id="ao-arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse">
        <path d="M0 0 L10 5 L0 10 z" fill="var(--ao-border-strong)" />
      </marker>
    </defs>
  )
}

export function BoxRect({ box, onClick }: { box: Box; onClick?: () => void }) {
  const max = Math.max(6, Math.floor((box.w - 14) / 7.4))
  const title = box.title.length > max ? `${box.title.slice(0, max - 1)}…` : box.title
  return (
    <g data-testid={box.id} onClick={onClick} style={{ cursor: onClick ? 'pointer' : undefined }}>
      <rect x={box.x0} y={box.y0} width={box.w} height={box.h} rx={10} fill="var(--ao-lane-fill)" stroke="var(--ao-lane-border)" strokeDasharray="5 4" />
      <text x={box.x0 + 8} y={box.y0 + 16} fontSize={13.5} fontWeight={700} fill="var(--ao-text)">{title}</text>
      <text x={box.x0 + 8} y={box.y0 + 27} fontSize={10.5} fill="var(--ao-text-subtle)">{box.hint}</text>
    </g>
  )
}

export interface EdgeLineProps {
  edge: ViewEdge
  curve: Curve
  tone: EdgeTone
  label: string
  streaming: boolean
  selected: boolean
  dim: boolean
  onSelect: () => void
}

export function EdgeLine({ edge, curve, tone, label, streaming, selected, dim, onSelect }: EdgeLineProps) {
  return (
    <g
      data-testid={`edge-${edge.id}`}
      data-tone={tone}
      data-stream={streaming ? 'on' : 'off'}
      data-dim={dim ? 'true' : 'false'}
      opacity={dim ? 0.18 : 1}
    >
      <path
        d={curve.d}
        fill="none"
        stroke={selected ? 'var(--ao-brand)' : TONE_COLOR[tone]}
        strokeWidth={selected ? 3 : tone === 'error' ? 2.2 : 2}
        strokeDasharray={edge.dangling ? '2 4' : tone === 'unconfirmed' ? '6 4' : edge.dashed ? '3 4' : undefined}
        markerEnd="url(#ao-arrow)"
      />
      <path
        d={curve.d}
        fill="none"
        stroke="transparent"
        strokeWidth={14}
        style={{ cursor: 'pointer' }}
        onClick={(e) => {
          e.stopPropagation()
          onSelect()
        }}
      >
        <title>{`${edge.kind}${edge.dangling ? ' (unresolved reference)' : ''}`}</title>
      </path>
      {label && (
        <text x={curve.mid.x} y={curve.mid.y - 5} fontSize={13} textAnchor="middle" fill="var(--ao-text-subtle)"
          stroke="var(--ao-surface)" strokeWidth={3} paintOrder="stroke" pointerEvents="none">
          {label}
        </text>
      )}
    </g>
  )
}

export interface NodeMarkProps {
  node: ViewNode
  at: Pt
  selected: boolean
  found: boolean
  working: boolean
  dim: boolean
  badge?: string
  role?: string
  onSelect: () => void
  onOpen: () => void
  onDrag: (p: Pt) => void
}

/**
 * A declared icon in a mark, in place of the class glyph: the built-in set as
 * a path, a fetched one as an image, an emoji as text — sized to the icon box
 * every shape's clearance leaves, so it never touches the outline either.
 */
function MarkIcon({ icon }: { icon: string }) {
  const r = resolveIcon(icon)
  if (!r) return null
  const size = ICON_HALF * 2
  if (r.kind === 'path') {
    return (
      <g transform={`translate(${-ICON_HALF} ${-ICON_HALF}) scale(${size / 24})`} data-testid="mark-icon">
        <path d={r.d} fill="var(--ao-text-subtle)" />
      </g>
    )
  }
  if (r.kind === 'image') {
    return <image href={r.src} x={-ICON_HALF} y={-ICON_HALF} width={size} height={size} data-testid="mark-icon" />
  }
  return (
    <text data-testid="mark-icon" fontSize={size * 0.8} textAnchor="middle" dominantBaseline="central">
      {r.text}
    </text>
  )
}

export function NodeMark({ node, at, selected, found, working, dim, badge, role, onSelect, onOpen, onDrag }: NodeMarkProps) {
  const style = styleFor(node.cls)
  const k = useContext(ViewportScale)
  const drag = useRef<{ x: number; y: number; from: Pt; moved: boolean } | null>(null)
  const text = node.label ?? node.name
  const label = text.length > 22 ? `${text.slice(0, 21)}…` : text
  const stroke = selected ? 'var(--ao-brand)' : HEALTH_COLOR[node.health] ?? HEALTH_COLOR.none

  const onPointerDown = (e: PointerEvent<SVGGElement>) => {
    if (e.button !== 0) return
    e.stopPropagation()
    drag.current = { x: e.clientX, y: e.clientY, from: at, moved: false }
    ;(e.currentTarget as Element).setPointerCapture?.(e.pointerId)
  }
  const onPointerMove = (e: PointerEvent<SVGGElement>) => {
    const d = drag.current
    if (!d) return
    const dx = (e.clientX - d.x) / k
    const dy = (e.clientY - d.y) / k
    if (!d.moved && Math.hypot(dx, dy) < 3) return
    d.moved = true
    onDrag({ x: d.from.x + dx, y: d.from.y + dy })
  }
  const onPointerUp = () => {
    const d = drag.current
    drag.current = null
    if (d && !d.moved) onSelect()
  }

  return (
    <g
      transform={`translate(${at.x},${at.y})`}
      data-testid={`node-${node.id}`}
      data-health={node.health}
      data-detached={node.detached ? 'true' : 'false'}
      data-dim={dim ? 'true' : 'false'}
      data-found={found ? 'true' : 'false'}
      opacity={dim ? 0.18 : 1}
      style={{ cursor: 'pointer' }}
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={onPointerUp}
      onClick={(e) => {
        e.stopPropagation()
        // keyboard and synthetic clicks arrive without a pointer
        if (!drag.current && e.detail === 0) onSelect()
      }}
      onDoubleClick={(e) => {
        e.stopPropagation()
        onOpen()
      }}
    >
      {found && <circle r={46} fill="none" stroke="var(--ao-accent)" strokeWidth={3} strokeDasharray="3 3" />}
      {working && (
        <circle r={47} fill="none" stroke="var(--ao-brand)" strokeWidth={2} strokeDasharray="6 5" opacity={0.9}>
          <animateTransform attributeName="transform" type="rotate" from="0" to="360" dur="2.4s" repeatCount="indefinite" />
        </circle>
      )}
      <g transform={`scale(${MARK_SCALE})`}>
        <path
          d={shapePath(style.shape)}
          fill="var(--ao-surface)"
          stroke={stroke}
          strokeWidth={selected ? 3.2 : node.health === 'bad' ? 2.6 : 1.3}
          strokeDasharray={node.detached ? '3 2' : undefined}
        />
        {!role && node.icon && resolveIcon(node.icon) ? (
          <MarkIcon icon={node.icon} />
        ) : (
          <path d={(role && ROLE_GLYPH[role]) || style.glyph} fill="none" stroke="var(--ao-text-subtle)" strokeWidth={0.9}
            strokeLinecap="round" strokeLinejoin="round" />
        )}
      </g>
      <text y={LABEL_Y} fontSize={15} textAnchor="middle" fill="var(--ao-text)" stroke="var(--ao-surface)" strokeWidth={3} paintOrder="stroke">
        {label}
      </text>
      {node.caption && (
        <text y={CAPTION_Y} fontSize={11.5} textAnchor="middle" fill="var(--ao-text-subtle)" stroke="var(--ao-surface)" strokeWidth={3} paintOrder="stroke">
          {node.caption}
        </text>
      )}
      {badge && (
        <g transform="translate(32,-32)" data-testid={`badge-${node.id}`}>
          <circle r={11} fill={node.collapsed ? 'var(--ao-neutral)' : 'var(--ao-brand)'} />
          <text y={4} fontSize={11} fill="#fff" textAnchor="middle" fontWeight={700}>{badge}</text>
        </g>
      )}
      <title>
        {`${style.label} ${node.name}${node.health !== 'none' ? ` — ${node.health}` : ''}${node.reason ? ` (${node.reason})` : ''}`}
      </title>
    </g>
  )
}

/** Wall-clock time, advanced every animation frame while something moves. */
export function useClock(running: boolean): number {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    if (!running || typeof requestAnimationFrame === 'undefined') return
    let id = requestAnimationFrame(function tick() {
      setNow(Date.now())
      id = requestAnimationFrame(tick)
    })
    return () => cancelAnimationFrame(id)
  }, [running])
  return running ? now : Date.now()
}

export interface TrafficProps {
  edges: ViewEdge[]
  curves: Map<string, Curve>
  pos: Map<string, Pt>
  traffic: (e: ViewEdge) => EdgeTraffic | undefined
  pulses: Pulse[]
  stream: boolean
  onHop: (ev: ActivityEvent) => void
}

/**
 * The moving layer. It owns the clock, so a frame re-renders the marks and
 * nothing under them.
 */
export function TrafficLayer({ edges, curves, pos, traffic, pulses, stream, onHop }: TrafficProps) {
  const live = pulses.some((p) => Date.now() - p.t0 <= pulseLife(p))
  const moving = stream && edges.some((e) => (traffic(e)?.events ?? 0) > 0)
  const now = useClock(moving || live)
  return (
    <g data-testid="traffic">
      {stream &&
        edges.map((e) => {
          const curve = curves.get(e.id)
          const t = traffic(e)
          if (!curve || !t) return null
          if (t.events === 0) return null
          // A slow stream has instants with no mark in flight; the edge is
          // still streaming through them.
          const marks = streamMarks(t, now, curve)
          return (
            <g key={e.id} data-testid={`stream-${e.id}`} pointerEvents="none">
              {marks.map((m, i) => (
                <path
                  key={i}
                  d={markPath(m.x, m.y, 4.2, m.cls === 'error')}
                  fill={m.cls === 'error' ? 'var(--ao-danger)' : m.cls === 'unconfirmed' ? 'none' : 'var(--ao-brand)'}
                  stroke={m.cls === 'unconfirmed' ? 'var(--ao-warning)' : 'none'}
                  strokeWidth={2}
                />
              ))}
            </g>
          )
        })}
      {pulses.map((p) => {
        // a pulse is drawn at its start until the clock moves, so an arriving
        // hop is on the graph from the frame it arrives in
        const m = pulseAt(p, Math.max(now, p.t0), curves, pos)
        if (!m) return null
        return (
          <path
            key={p.key}
            data-testid="pulse"
            data-hop={p.ev.kind}
            data-edge={p.x.edges[0]?.edge.id ?? p.x.pop}
            d={markPath(m.x, m.y, m.r, m.cls === 'error')}
            fill={m.pop ? 'none' : m.cls === 'error' ? 'var(--ao-danger)' : 'var(--ao-brand)'}
            stroke={m.pop ? (m.cls === 'error' ? 'var(--ao-danger)' : 'var(--ao-brand)') : 'none'}
            strokeWidth={2}
            opacity={m.opacity}
            style={{ cursor: 'pointer', filter: 'drop-shadow(0 0 3px var(--ao-brand))' }}
            onClick={(e) => {
              e.stopPropagation()
              onHop(p.ev)
            }}
          >
            <title>{p.ev.kind}</title>
          </path>
        )
      })}
    </g>
  )
}
