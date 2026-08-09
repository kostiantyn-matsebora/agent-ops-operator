import { useCallback, useEffect, useRef, useState, type ReactNode, type WheelEvent } from 'react'
import { Button, Tooltip } from '@patternfly/react-core'

// Pan and zoom.
//
// Implemented on the SVG transform rather than by swapping the viewBox, because
// a transform leaves stroke widths and font sizes in SCREEN units — text that
// shrank with the zoom would make zooming out useless, which is the one thing
// you zoom out to do.

const MIN_SCALE = 0.25
const MAX_SCALE = 3
const STEP = 1.25

export interface ViewportProps {
  /** Content size in graph units, used by "fit". */
  contentWidth: number
  contentHeight: number
  children: ReactNode
  ariaLabel?: string
  /** Re-fit whenever this changes (the graph was rebuilt). */
  fitKey?: string
}

interface Transform {
  x: number
  y: number
  k: number
}

function clamp(k: number): number {
  return Math.min(Math.max(k, MIN_SCALE), MAX_SCALE)
}

export function Viewport({ contentWidth, contentHeight, children, ariaLabel, fitKey }: ViewportProps) {
  const host = useRef<HTMLDivElement>(null)
  const [t, setT] = useState<Transform>({ x: 0, y: 0, k: 1 })
  const drag = useRef<{ x: number; y: number; tx: number; ty: number } | null>(null)

  const fit = useCallback(() => {
    const el = host.current
    if (!el || contentWidth <= 0 || contentHeight <= 0) return
    const { width, height } = el.getBoundingClientRect()
    const k = clamp(Math.min((width - 24) / contentWidth, (height - 24) / contentHeight, 1))
    setT({ k, x: (width - contentWidth * k) / 2, y: (height - contentHeight * k) / 2 })
  }, [contentWidth, contentHeight])

  // Fit on mount and whenever the graph is rebuilt — a re-render that left the
  // viewport where it was would strand the user off-canvas after a filter change.
  useEffect(() => {
    fit()
  }, [fit, fitKey])

  const zoomAt = useCallback((factor: number, cx?: number, cy?: number) => {
    setT((prev) => {
      const k = clamp(prev.k * factor)
      if (k === prev.k) return prev
      const el = host.current
      const rect = el?.getBoundingClientRect()
      // zoom toward the pointer when there is one, else the centre
      const px = cx ?? (rect ? rect.width / 2 : 0)
      const py = cy ?? (rect ? rect.height / 2 : 0)
      const ratio = k / prev.k
      return { k, x: px - (px - prev.x) * ratio, y: py - (py - prev.y) * ratio }
    })
  }, [])

  const onWheel = (e: WheelEvent<HTMLDivElement>) => {
    // Only claim the wheel when the pointer is over the canvas AND the user is
    // zooming; a bare scroll should still move the page.
    if (!e.ctrlKey && !e.metaKey && Math.abs(e.deltaY) < 40) return
    const rect = host.current?.getBoundingClientRect()
    zoomAt(e.deltaY < 0 ? STEP : 1 / STEP, e.clientX - (rect?.left ?? 0), e.clientY - (rect?.top ?? 0))
  }

  return (
    <div style={{ position: 'relative' }}>
      <div
        ref={host}
        data-testid="graph-viewport"
        onWheel={onWheel}
        onPointerDown={(e) => {
          if (e.button !== 0) return
          drag.current = { x: e.clientX, y: e.clientY, tx: t.x, ty: t.y }
          ;(e.target as Element).setPointerCapture?.(e.pointerId)
        }}
        onPointerMove={(e) => {
          const d = drag.current
          if (!d) return
          setT((prev) => ({ ...prev, x: d.tx + (e.clientX - d.x), y: d.ty + (e.clientY - d.y) }))
        }}
        onPointerUp={() => {
          drag.current = null
        }}
        onPointerLeave={() => {
          drag.current = null
        }}
        style={{
          height: '68vh',
          minHeight: 420,
          overflow: 'hidden',
          cursor: drag.current ? 'grabbing' : 'grab',
          background: 'var(--ao-canvas)',
          border: '1px solid var(--ao-border)',
          borderRadius: 6,
          touchAction: 'none',
        }}
      >
        <svg role="img" aria-label={ariaLabel ?? 'graph'} width="100%" height="100%">
          <g transform={`translate(${t.x},${t.y}) scale(${t.k})`} data-testid="graph-canvas">
            {children}
          </g>
        </svg>
      </div>

      <div
        style={{ position: 'absolute', right: 12, bottom: 12, display: 'flex', gap: 4 }}
        data-testid="zoom-controls"
      >
        <Tooltip content="Zoom in">
          <Button variant="control" aria-label="zoom in" onClick={() => zoomAt(STEP)}>
            +
          </Button>
        </Tooltip>
        <Tooltip content="Zoom out">
          <Button variant="control" aria-label="zoom out" onClick={() => zoomAt(1 / STEP)}>
            −
          </Button>
        </Tooltip>
        <Tooltip content="Fit to view">
          <Button variant="control" aria-label="fit to view" onClick={fit}>
            ⤢
          </Button>
        </Tooltip>
        <span
          aria-label="zoom level"
          style={{
            alignSelf: 'center',
            padding: '0 8px',
            fontSize: 12,
            color: 'var(--ao-text-subtle)',
          }}
        >
          {Math.round(t.k * 100)}%
        </span>
      </div>
    </div>
  )
}
