import { type ReactElement, type ReactNode, useEffect, useRef } from 'react'

// A MODAL DIALOG, the one this look uses: a scrim, a titled panel, Escape and
// the scrim's own click to dismiss, focus brought in on open and handed back
// on close. It carries no behaviour of its own beyond that.

export function Dialog({
  title, label, testid, onClose, children, footer,
}: Readonly<{
  title: string
  label?: string
  testid?: string
  onClose: () => void
  children: ReactNode
  footer: ReactNode
}>): ReactElement {
  const panel = useRef<HTMLDivElement>(null)
  useEffect(() => {
    const before = document.activeElement as HTMLElement | null
    panel.current?.querySelector<HTMLElement>('button, input')?.focus()
    return () => before?.focus?.()
  }, [])
  return (
    <div
      className="wk-dialog-scrim"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onClose()
      }}
    >
      <div
        ref={panel}
        className="wk-dialog"
        role="dialog"
        aria-modal="true"
        aria-label={label ?? title}
        data-testid={testid}
        onKeyDown={(e) => {
          if (e.key === 'Escape') onClose()
        }}
      >
        <h2>{title}</h2>
        <div className="wk-dialog-body">{children}</div>
        <div className="wk-dialog-foot">{footer}</div>
      </div>
    </div>
  )
}
