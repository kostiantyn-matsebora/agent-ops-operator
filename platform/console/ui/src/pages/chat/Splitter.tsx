import { useEffect, useRef, useState } from 'react'

// Two pointer listeners on a 6px handle (design D-B) — PatternFly 6 ships a
// resizable Drawer and no three-pane splitter, and a Drawer is the wrong
// primitive for two independent handles in a row.
//
// The handle is a focusable, keyboard-operable value control, so it carries
// the interactive `slider` role (a bare `separator` is non-interactive and may
// not take listeners or a tabIndex). `aria-orientation="vertical"` is the
// design's spelled-out value for the handle's own orientation.

function clamp(v: number, min: number, max: number): number {
  return Math.min(Math.max(v, min), max)
}

export interface SplitterProps {
  width: number
  min: number
  /** Unbounded above when absent — the thread pane's own flex basis is the real ceiling. */
  max?: number
  defaultWidth: number
  onChange: (width: number) => void
  ariaLabel: string
  /** Keyboard step, 16px per design D-B. */
  step?: number
}

export function Splitter({ width, min, max, defaultWidth, onChange, ariaLabel, step = 16 }: Readonly<SplitterProps>) {
  const [dragging, setDragging] = useState(false)
  const start = useRef<{ x: number; width: number } | null>(null)
  const ceiling = max ?? Infinity

  function onPointerDown(e: React.PointerEvent) {
    // Primary button only — a touch or a secondary-button chord must not
    // start a drag that a later pointerup on a DIFFERENT element never ends.
    if (e.button !== 0) return
    start.current = { x: e.clientX, width }
    setDragging(true)
    e.preventDefault()
  }

  useEffect(() => {
    if (!dragging) return
    const onMove = (e: PointerEvent) => {
      if (!start.current) return
      onChange(clamp(start.current.width + (e.clientX - start.current.x), min, ceiling))
    }
    const onUp = () => {
      start.current = null
      setDragging(false)
    }
    window.addEventListener('pointermove', onMove)
    window.addEventListener('pointerup', onUp)
    return () => {
      window.removeEventListener('pointermove', onMove)
      window.removeEventListener('pointerup', onUp)
    }
  }, [dragging, min, ceiling, onChange])

  function onKeyDown(e: React.KeyboardEvent) {
    if (e.key === 'ArrowLeft') {
      e.preventDefault()
      onChange(clamp(width - step, min, ceiling))
    } else if (e.key === 'ArrowRight') {
      e.preventDefault()
      onChange(clamp(width + step, min, ceiling))
    } else if (e.key === 'Enter') {
      e.preventDefault()
      onChange(clamp(defaultWidth, min, ceiling))
    }
  }

  return (
    <div
      role="slider"
      aria-orientation="vertical"
      aria-label={ariaLabel}
      aria-valuenow={Math.round(width)}
      aria-valuemin={min}
      aria-valuemax={Number.isFinite(ceiling) ? ceiling : undefined}
      tabIndex={0}
      data-testid="splitter"
      title="drag to resize · double-click to reset"
      onPointerDown={onPointerDown}
      onDoubleClick={() => onChange(clamp(defaultWidth, min, ceiling))}
      onKeyDown={onKeyDown}
      style={{
        width: 6,
        flex: 'none',
        cursor: 'col-resize',
        background: dragging ? 'var(--ao-brand-soft)' : 'transparent',
        position: 'relative',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
      }}
    >
      <span
        aria-hidden
        style={{
          width: 2,
          height: 28,
          borderRadius: 1,
          background: dragging ? 'var(--ao-brand)' : 'var(--ao-border-strong)',
        }}
      />
      {/* The width readout shows only WHILE dragging (design D-J) — the
          prototype's always-visible label is for the board, not the view. */}
      {dragging && (
        <span
          aria-hidden
          data-testid="splitter-readout"
          style={{
            position: 'absolute',
            left: 10,
            top: '50%',
            transform: 'translateY(-50%)',
            background: 'var(--ao-text)',
            color: 'var(--ao-surface)',
            fontSize: 11,
            padding: '2px 6px',
            borderRadius: 4,
            whiteSpace: 'nowrap',
            zIndex: 1,
          }}
        >
          {`${Math.round(width)} px`}
        </span>
      )}
    </div>
  )
}
