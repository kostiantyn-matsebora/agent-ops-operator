import { type ReactElement, useRef, useState } from 'react'
import { MAX_PANE_WIDTH, SPLITTER_STEP, clampWidth } from './layout'

// The gutter between two panes, and the control that resizes the one on its
// left: dragged with a pointer, stepped with the arrow keys, reset with Home.
// A focusable separator, which is what WAI-ARIA calls a resizer.

export function Splitter({
  label, width, min, defaultWidth, disabled, onChange,
}: Readonly<{
  label: string
  width: number
  min: number
  defaultWidth: number
  disabled?: boolean
  onChange: (width: number) => void
}>): ReactElement {
  const [dragging, setDragging] = useState(false)
  const start = useRef<{ x: number; w: number } | null>(null)
  return (
    <div
      className={`wk-spl${dragging ? ' drag' : ''}${disabled ? ' off' : ''}`}
      role="separator"
      aria-orientation="vertical"
      aria-label={label}
      aria-valuenow={width}
      aria-valuemin={min}
      aria-valuemax={MAX_PANE_WIDTH}
      tabIndex={disabled ? -1 : 0}
      onKeyDown={(e) => {
        if (disabled) return
        if (e.key === 'ArrowLeft' || e.key === 'ArrowRight') {
          e.preventDefault()
          onChange(clampWidth(width + (e.key === 'ArrowRight' ? SPLITTER_STEP : -SPLITTER_STEP), min))
        } else if (e.key === 'Home') {
          e.preventDefault()
          onChange(defaultWidth)
        }
      }}
      onPointerDown={(e) => {
        if (disabled || e.button !== 0) return
        start.current = { x: e.clientX, w: width }
        setDragging(true)
        e.currentTarget.setPointerCapture?.(e.pointerId)
      }}
      onPointerMove={(e) => {
        if (!start.current) return
        onChange(clampWidth(start.current.w + e.clientX - start.current.x, min))
      }}
      onPointerUp={() => {
        start.current = null
        setDragging(false)
      }}
      onDoubleClick={() => !disabled && onChange(defaultWidth)}
    />
  )
}
