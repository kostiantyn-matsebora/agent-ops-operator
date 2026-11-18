import type { ReactElement } from 'react'
import type { Severity } from '../../model/message'
import { resolveIcon } from '../../model/icon'

// The workspace's icons: line drawings on a 24-unit grid, stroked in the
// current colour, the prototype's set. Decorative unless given a label — the
// control or text beside each one says what it means.

const PATHS = {
  overview: (
    <>
      <rect x="4" y="4" width="7" height="7" rx="1.5" />
      <rect x="13" y="4" width="7" height="7" rx="1.5" />
      <rect x="4" y="13" width="7" height="7" rx="1.5" />
      <rect x="13" y="13" width="7" height="7" rx="1.5" />
    </>
  ),
  queues: <path d="M8 6h12M8 12h12M8 18h12M4 6h.01M4 12h.01M4 18h.01" />,
  config: (
    <>
      <path d="M5 7h9M18 7h1M5 17h1M10 17h9" />
      <circle cx="16" cy="7" r="2" />
      <circle cx="8" cy="17" r="2" />
    </>
  ),
  topology: (
    <>
      <circle cx="12" cy="5" r="2" />
      <circle cx="5" cy="19" r="2" />
      <circle cx="19" cy="19" r="2" />
      <path d="M12 7v4M12 11L6 17M12 11l6 6" />
    </>
  ),
  chat: <path d="M5 5h14v10H10l-5 4z" />,
  all: <path d="M8 6h12M8 12h12M8 18h12M4 6h.01M4 12h.01M4 18h.01" />,
  working: <path d="M12 4a8 8 0 1 0 8 8" />,
  mine: (
    <>
      <circle cx="12" cy="8" r="3.5" />
      <path d="M5 20c1-4 4-6 7-6s6 2 7 6" />
    </>
  ),
  errored: (
    <>
      <circle cx="12" cy="12" r="8" />
      <path d="M12 8v5M12 16h.01" />
    </>
  ),
  hash: <path d="M5 9h14M5 15h14M10 4L8 20M16 4l-2 16" />,
  archive: (
    <>
      <rect x="4" y="5" width="16" height="4" rx="1" />
      <path d="M6 9v9h12V9M10 13h4" />
    </>
  ),
  pipeline: (
    <>
      <circle cx="6" cy="6" r="2" />
      <circle cx="18" cy="18" r="2" />
      <path d="M6 8v4a4 4 0 0 0 4 4h6" />
    </>
  ),
  bot: (
    <>
      <rect x="4" y="8" width="16" height="11" rx="3" />
      <path d="M12 4v4M9 13v1M15 13v1" />
    </>
  ),
  member: (
    <>
      <circle cx="6" cy="6" r="2" />
      <circle cx="18" cy="18" r="2" />
      <path d="M6 8v4a4 4 0 0 0 4 4h6" />
    </>
  ),
  bell: <path d="M6 16V11a6 6 0 0 1 12 0v5l1.5 2h-15zM10 21h4" />,
  menu: <path d="M4 7h16M4 12h16M4 17h16" />,
  more: <path d="M12 6h.01M12 12h.01M12 18h.01" />,
  critical: (
    <>
      <path d="M8 3h8l5 5v8l-5 5H8l-5-5V8z" />
      <path d="M12 8v5M12 16v.01" />
    </>
  ),
  error: (
    <>
      <circle cx="12" cy="12" r="9" />
      <path d="m9 9 6 6M15 9l-6 6" />
    </>
  ),
  warning: (
    <>
      <path d="M12 4 3 20h18L12 4z" />
      <path d="M12 10v4M12 17v.01" />
    </>
  ),
  info: (
    <>
      <circle cx="12" cy="12" r="9" />
      <path d="M12 11v5M12 8v.01" />
    </>
  ),
} as const

export type IconName = keyof typeof PATHS

export function Icon({ name, label, icon }: Readonly<{ name?: IconName; label?: string; icon?: string }>): ReactElement | null {
  // Custom icon reference (pipeline icons)
  if (icon) {
    const r = resolveIcon(icon)
    if (!r) return null
    if (r.kind === 'path') {
      return (
        <svg
          className="wk-i"
          viewBox="0 0 24 24"
          role={label ? 'img' : undefined}
          aria-label={label}
          aria-hidden={label ? undefined : true}
        >
          <path d={r.d} fill="currentColor" />
        </svg>
      )
    }
    if (r.kind === 'image') {
      return <img src={r.src} alt={label ?? ''} className="wk-i" style={{ objectFit: 'contain' }} />
    }
    return <span aria-hidden>{r.text}</span>
  }
  // Built-in icon
  if (!name) return null
  return (
    <svg
      className="wk-i"
      viewBox="0 0 24 24"
      data-icon={name}
      role={label ? 'img' : undefined}
      aria-label={label}
      aria-hidden={label ? undefined : true}
    >
      {PATHS[name]}
    </svg>
  )
}

const CHEVRONS = {
  right: 'M9 6l6 6-6 6',
  down: 'M6 9l6 6 6-6',
  left: 'M15 6l-6 6 6 6',
  doubleLeft: 'M11 6l-6 6 6 6M19 6l-6 6 6 6',
  doubleRight: 'M5 6l6 6-6 6M13 6l6 6-6 6',
} as const

export function Chevron({ dir }: Readonly<{ dir: keyof typeof CHEVRONS }>): ReactElement {
  return (
    <svg className="wk-chev" viewBox="0 0 24 24" aria-hidden="true">
      <path d={CHEVRONS[dir]} />
    </svg>
  )
}

/** The icon for each contract severity. An unrated signal has none. */
export const SEVERITY_ICON: Readonly<Record<Severity, IconName>> = {
  critical: 'critical',
  error: 'error',
  warning: 'warning',
  info: 'info',
}

export function CopyGlyph(): ReactElement {
  return (
    <svg viewBox="0 0 16 16" width="14" height="14" fill="none" stroke="currentColor" strokeWidth="1.5" aria-hidden="true">
      <rect x="5" y="5" width="8" height="8" rx="1.5" />
      <path d="M3 10.5V4a1 1 0 0 1 1-1h6.5" />
    </svg>
  )
}

export function CheckGlyph(): ReactElement {
  return (
    <svg viewBox="0 0 16 16" width="14" height="14" fill="none" stroke="currentColor" strokeWidth="1.75" aria-hidden="true">
      <path d="M3 8.5l3 3 7-7" />
    </svg>
  )
}
