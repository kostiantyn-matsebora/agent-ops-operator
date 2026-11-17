import { resolveIcon } from '../../../model/icon'

/**
 * Draws a Pipeline's icon REFERENCE, or nothing.
 *
 * The manager publishes the string and interprets it no further. Resolving it
 * is this surface's job, and this surface can do more than a chat transport
 * can — so it handles every form and falls back rather than failing.
 *
 *   aops:<name>   the built-in set, bundled — no network, works air-gapped
 *   <emoji>       drawn as text
 *   https://…     drawn as an image
 *   <lib>:<name>  resolved through the icon service (see ICON_SERVICE)
 *
 * NOTHING FAILS OVER AN ICON. An unknown reference renders nothing at all,
 * because a broken image beside an agent's name is worse than no image.
 */

// The resolution is the model's (model/icon.ts), shared by every look and the
// graph; re-exported so this look's callers keep one import.
export { resolveIcon, type ResolvedIcon } from '../../../model/icon'

export function Icon({ icon, size = '1em' }: { icon?: string; size?: string }) {
  const r = resolveIcon(icon)
  if (!r) return null
  if (r.kind === 'path') {
    return (
      <svg
        viewBox="0 0 24 24"
        width={size}
        height={size}
        fill="currentColor"
        aria-hidden
        focusable="false"
        style={{ verticalAlign: '-0.125em', flex: 'none' }}
      >
        <path d={r.d} />
      </svg>
    )
  }
  if (r.kind === 'image') {
    return <img src={r.src} alt="" width={size} height={size} style={{ verticalAlign: '-0.125em', flex: 'none' }} />
  }
  return (
    <span aria-hidden style={{ flex: 'none' }}>
      {r.text}
    </span>
  )
}

// The lane-emoji strip is a naming fact, the model's (model/titles.ts).
export { stripLeadingIcon } from '../../../model/titles'
