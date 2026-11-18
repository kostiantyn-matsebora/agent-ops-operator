import { Link } from 'react-router-dom'
import { useConfigKinds } from '../../../../model/config'
import { Crumbs, KindGlyph, PageState, Pill } from './parts'

/** The Configuration index: every kind the console watches, with its count. */
export function ConfigPage() {
  const { data, isLoading, error } = useConfigKinds()
  if (isLoading) return <PageState kind="loading" title="Loading…" />
  if (error || !data) return <PageState kind="error" title="Could not load kinds">{String(error)}</PageState>
  return (
    <main className="wk-pane wk-page wk-cfg" aria-label="Configuration">
      <Crumbs items={[{ label: 'Configuration' }]} />
      <h1>Configuration</h1>
      <ul className="wk-cfg-kgrid">
        {data.map((k) => (
          <li key={k.kind}>
            <Link className="wk-cfg-kcard" to={`/config/${k.kind}`}>
              <b>
                <KindGlyph kind={k.kind} />
                {k.title}
              </b>
              <span>
                {k.count} object{k.count === 1 ? '' : 's'}
              </span>
              {!k.synced && <Pill tone="warning">not synced yet</Pill>}
            </Link>
          </li>
        ))}
      </ul>
    </main>
  )
}
