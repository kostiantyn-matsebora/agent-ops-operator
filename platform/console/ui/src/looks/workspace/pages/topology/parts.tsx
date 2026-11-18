import { useId, type ReactElement, type ReactNode } from 'react'
import type { ActivityEvent } from '../../../../api/types'
import { hopSummary } from '../../../../graph/content'
import { clock, hopPath } from '../../../../graph/format'
import { resolveIcon } from '../../../../model/icon'
import { plain } from '../../../../model/text'
import { Chevron } from '../../icons'

// The small pieces the topology's panels share. Everything cluster- or
// wire-sourced is a React text node with tags stripped (`plain`), never markup.

export function Plain({ children, multiline }: Readonly<{ children?: string | null; multiline?: boolean }>) {
  if (!children) return null
  return <span className={multiline ? 'wk-tp-multi' : undefined}>{plain(children)}</span>
}

/** A Pipeline's declared icon reference, or nothing. */
export function PipelineIcon({ icon }: Readonly<{ icon?: string }>): ReactElement | null {
  const r = resolveIcon(icon)
  if (!r) return null
  if (r.kind === 'path') {
    return (
      <svg viewBox="0 0 24 24" width="1em" height="1em" fill="currentColor" aria-hidden focusable="false" className="wk-tp-icon">
        <path d={r.d} />
      </svg>
    )
  }
  if (r.kind === 'image') return <img src={r.src} alt="" width="1em" height="1em" className="wk-tp-icon" />
  return <span aria-hidden>{r.text}</span>
}

export function PipelineName({ name, icon }: Readonly<{ name: string; icon?: string }>) {
  return (
    <span className="wk-tp-name">
      <PipelineIcon icon={icon} />
      <Plain>{name}</Plain>
    </span>
  )
}

/** A card that folds to its title; the chevron is the one control either way. */
export function Fold({
  title, expanded, onToggle, label, testId, children,
}: Readonly<{ title: string; expanded: boolean; onToggle: () => void; label: string; testId?: string; children: ReactNode }>) {
  const id = useId()
  return (
    <section className="wk-card wk-tp-card" data-testid={testId} aria-labelledby={`${id}-t`}>
      <button type="button" className="wk-tp-fold" aria-expanded={expanded} aria-controls={`${id}-b`} aria-label={label} onClick={onToggle}>
        <Chevron dir={expanded ? 'down' : 'right'} />
        <span id={`${id}-t`}>{title}</span>
      </button>
      {expanded && <div id={`${id}-b`} className="wk-tp-body">{children}</div>}
    </section>
  )
}

/** A card with a small uppercase kicker over its title: the selection's panel. */
export function Card({
  kicker, title, testId, children,
}: Readonly<{ kicker: string; title: ReactNode; testId: string; children: ReactNode }>) {
  return (
    <section className="wk-card wk-tp-card" data-testid={testId}>
      <header className="wk-tp-head">
        <small>{kicker}</small>
        <h3>{title}</h3>
      </header>
      <div className="wk-tp-body">{children}</div>
    </section>
  )
}

export function Facts({ rows }: Readonly<{ rows: [string, ReactNode][] }>) {
  return (
    <dl className="wk-tp-facts">
      {rows.map(([k, v], i) => (
        <div key={`${k}-${i}`}>
          <dt>{k}</dt>
          <dd>{v}</dd>
        </div>
      ))}
    </dl>
  )
}

const seconds = (ms?: number) => (ms ? `${(ms / 1000).toFixed(1)}s` : '')
export const latency = (ms?: number) => (ms ? (ms >= 1000 ? `${(ms / 1000).toFixed(1)}s` : `${ms}ms`) : '—')

export function HopRow({
  ev, lead, now, onOpen,
}: Readonly<{ ev: ActivityEvent; lead?: string; now?: boolean; onOpen: () => void }>) {
  const summary = hopSummary(ev)
  return (
    <button type="button" data-testid="hop-row" className={`wk-tp-hop${now ? ' now' : ''}`} aria-current={now || undefined} onClick={onOpen}>
      <span className="wk-tp-t wk-num">{lead ?? clock(ev.ts)}</span>
      <span className="wk-tp-what">
        <b>{ev.kind}</b>
        {ev.status === 'error' && <span className="wk-tp-err"> error{ev.code ? ` · ${ev.code}` : ''}</span>}
        <span className="wk-tp-sub">
          <Plain>{hopPath(ev) + (summary ? ` · ${summary}` : '')}</Plain>
        </span>
      </span>
      <span className="wk-tp-t wk-num">{seconds(ev.latencyMs)}</span>
    </button>
  )
}

export function HopList({ children }: Readonly<{ children: ReactNode }>) {
  return <div className="wk-tp-hops">{children}</div>
}
