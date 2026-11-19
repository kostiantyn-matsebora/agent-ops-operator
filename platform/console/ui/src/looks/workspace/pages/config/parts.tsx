import { useMemo, useState, type ReactElement, type ReactNode } from 'react'
import { Link } from 'react-router-dom'
import type { Condition, Health, ObjectMeta } from '../../../../api/types'
import { styleFor } from '../../../../graph/shapes'
import { conditionTone, objectAge } from '../../../../model/config'
import { resolveIcon } from '../../../../model/icon'
import { healthTone } from '../../../../model/overview'
import { plain } from '../../../../model/text'
import { tokenizeLine, type TokenKind } from './yaml'
import './config.css'

// The pieces the three Configuration pages share. Everything cluster- or
// wire-sourced is rendered as a React text node, with tags stripped (`plain`),
// never as markup.

export const age = objectAge

type Tone = 'success' | 'danger' | 'warning' | 'info' | 'custom' | undefined
const PILL: Record<string, string> = { success: 'ok', danger: 'bad', warning: 'warn' }

/** A status pill; `tone` is the model's status vocabulary. */
export function Pill({ tone, title, children }: Readonly<{ tone?: Tone; title?: string; children: ReactNode }>) {
  return (
    <span className={`wk-pill ${(tone && PILL[tone]) || ''}`.trim()} title={title}>
      {children}
    </span>
  )
}

export function Plain({ children, multiline }: Readonly<{ children?: string | null; multiline?: boolean }>) {
  if (!children) return null
  return <span className={multiline ? 'wk-cfg-multi' : undefined}>{plain(children)}</span>
}

export function Dash() {
  return <span className="wk-cfg-dim">—</span>
}

export function PageState({ kind, title, children }: Readonly<{ kind: 'loading' | 'error' | 'empty'; title: string; children?: ReactNode }>) {
  return (
    <main className="wk-pane wk-page wk-cfg" aria-label="Configuration" aria-busy={kind === 'loading' || undefined}>
      <div className="wk-card wk-cfg-state" role={kind === 'error' ? 'alert' : 'status'} data-state={kind}>
        <b>{title}</b>
        {children && <div className="wk-note">{children}</div>}
      </div>
    </main>
  )
}

/** An empty state inside a card. */
export function Empty({ title, children }: Readonly<{ title: string; children?: ReactNode }>) {
  return (
    <div className="wk-cfg-empty">
      <b>{title}</b>
      {children && <div className="wk-note">{children}</div>}
    </div>
  )
}

export function Crumbs({ items }: Readonly<{ items: { label: string; to?: string; icon?: string }[] }>) {
  return (
    <nav className="wk-cfg-crumb" aria-label="Breadcrumb">
      {items.map((c, i) => {
        const last = i === items.length - 1
        return (
          <span key={`${c.label}-${i}`} className="wk-cfg-crumb-item">
            {c.to && !last ? <Link to={c.to}>{c.label}</Link> : <span aria-current={last ? 'page' : undefined}>{c.icon ? <><Icon icon={c.icon} /> </> : null}{c.label}</span>}
            {!last && <i aria-hidden="true">›</i>}
          </span>
        )
      })}
    </nav>
  )
}

/** A Pipeline's declared icon reference, or nothing. */
export function Icon({ icon, size = '1em' }: Readonly<{ icon?: string; size?: string }>): ReactElement | null {
  const r = resolveIcon(icon)
  if (!r) return null
  if (r.kind === 'path') {
    return (
      <svg viewBox="0 0 24 24" width={size} height={size} fill="currentColor" aria-hidden focusable="false" className="wk-cfg-icon">
        <path d={r.d} />
      </svg>
    )
  }
  if (r.kind === 'image') return <img src={r.src} alt="" width={size} height={size} className="wk-cfg-icon" />
  return <span aria-hidden>{r.text}</span>
}

export function PipelineName({ name, icon }: Readonly<{ name: string; icon?: string }>) {
  return (
    <span className="wk-cfg-name">
      {icon && <Icon icon={icon} />}
      <Plain>{name}</Plain>
    </span>
  )
}

/** The same silhouette the graph uses, so the two views teach one vocabulary. */
export function KindGlyph({ kind, size = 18 }: Readonly<{ kind: string; size?: number }>) {
  const s = styleFor(kind)
  return (
    <svg width={size} height={size} viewBox="-12 -12 24 24" aria-hidden="true" className="wk-cfg-glyph">
      <path d={s.glyph} fill="none" stroke="currentColor" strokeWidth={1.4} strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}

export function HealthPill({ health }: Readonly<{ health: Health }>) {
  if (health === 'none') return <span className="wk-cfg-dim">reports none</span>
  const t = healthTone(health)
  return <Pill tone={t === 'custom' ? undefined : t}>{health}</Pill>
}

function conditionTitle(c: Condition): string {
  const parts = [plain(c.reason ?? '')]
  if (c.message) parts.push(plain(c.message))
  if (c.lastTransitionTime) parts.push(`since ${age(c.lastTransitionTime)} ago`)
  return parts.filter(Boolean).join('\n')
}

/** Conditions as pills, the reason and message in the title rather than a wall of text. */
export function ConditionChips({ conditions }: Readonly<{ conditions?: Condition[] | null }>) {
  if (!conditions || conditions.length === 0) return <span className="wk-cfg-dim">reports none</span>
  return (
    <span className="wk-cfg-chips">
      {conditions.map((c) => (
        <Pill key={c.type} tone={conditionTone(c.status)} title={conditionTitle(c)}>
          {c.type}
        </Pill>
      ))}
    </span>
  )
}

/** Key/value chips for labels and annotations, the rest behind a count. */
export function KeyValueChips({ values, max = 12 }: Readonly<{ values?: Record<string, string>; max?: number }>) {
  const entries = Object.entries(values ?? {})
  if (entries.length === 0) return <span className="wk-cfg-dim">none</span>
  const shown = entries.slice(0, max)
  const more = entries.length - shown.length
  return (
    <span className="wk-cfg-chips">
      {shown.map(([k, v]) => (
        <span key={k} className="wk-cfg-kvchip" title={plain(`${k}=${v}`)}>
          {plain(`${k}=${v}`)}
        </span>
      ))}
      {more > 0 && (
        <span className="wk-cfg-kvchip more" title={entries.slice(max).map(([k, v]) => plain(`${k}=${v}`)).join('\n')}>
          +{more} more
        </span>
      )}
    </span>
  )
}

/** Object metadata, shown whole: uid, resourceVersion, labels and annotations included. */
export function MetadataCard({ meta, health, conditions }: Readonly<{ meta: ObjectMeta; health?: Health; conditions?: Condition[] | null }>) {
  return (
    <section className="wk-card" aria-label="Metadata">
      <h3 className="wk-cfg-h">Metadata</h3>
      <dl className="wk-kv wk-cfg-kv">
        <dt>Name</dt>
        <dd><Plain>{meta.name}</Plain></dd>
        <dt>Namespace</dt>
        <dd><Plain>{meta.namespace}</Plain></dd>
        {health && (
          <>
            <dt>Health</dt>
            <dd><HealthPill health={health} /></dd>
          </>
        )}
        <dt>Conditions</dt>
        <dd><ConditionChips conditions={conditions} /></dd>
        <dt>Created</dt>
        <dd>
          {meta.creationTimestamp ? `${new Date(meta.creationTimestamp).toLocaleString()} (${age(meta.creationTimestamp)} ago)` : '—'}
        </dd>
        <dt>Labels</dt>
        <dd><KeyValueChips values={meta.labels} /></dd>
        <dt>Annotations</dt>
        <dd><KeyValueChips values={meta.annotations} max={6} /></dd>
        <dt>UID</dt>
        <dd className="wk-cfg-small"><Plain>{meta.uid}</Plain></dd>
        <dt>Resource version</dt>
        <dd className="wk-cfg-small"><Plain>{meta.resourceVersion}</Plain></dd>
      </dl>
    </section>
  )
}

/** A table that scrolls inside its own container, never the page. */
export function TableWrap({ children }: Readonly<{ children: ReactNode }>) {
  return <div className="wk-cfg-tw">{children}</div>
}

/** A YAML viewer that keeps the document: numbered lines, highlight, wrap and copy. */
export function Yaml({ value, title }: Readonly<{ value: string; title: string }>) {
  const [wrap, setWrap] = useState(false)
  const [copied, setCopied] = useState(false)
  const lines = useMemo(() => value.replace(/\n$/, '').split('\n'), [value])
  const gutter = String(lines.length).length

  async function copy() {
    try {
      await navigator.clipboard.writeText(value)
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    } catch {
      setCopied(false)
    }
  }

  return (
    <div className="wk-card">
      <div className="wk-cfg-yamlbar">
        <label className="wk-cfg-switch">
          <input type="checkbox" checked={wrap} onChange={(e) => setWrap(e.target.checked)} /> Wrap long lines
        </label>
        <button type="button" className="wk-btn" onClick={copy} aria-label={`copy ${title}`} title={copied ? 'Copied' : `Copy ${title}`}>
          {copied ? 'Copied' : 'Copy'}
        </button>
      </div>
      <pre
        className={`wk-yaml wk-cfg-yaml${wrap ? ' wrap' : ''}`}
        data-testid="yaml"
        aria-label={title}
        tabIndex={0}
      >
        {lines.map((line, i) => (
          <div key={i} className="wk-cfg-yl">
            <span aria-hidden="true" className="wk-cfg-yn" style={{ minWidth: `${gutter + 1}ch` }}>
              {i + 1}
            </span>
            <span className="wk-cfg-yt">
              {tokenizeLine(line).map((t, j) => (
                <span key={j} className={`wk-cfg-y-${t.kind satisfies TokenKind}`}>
                  {t.text}
                </span>
              ))}
            </span>
          </div>
        ))}
      </pre>
    </div>
  )
}
