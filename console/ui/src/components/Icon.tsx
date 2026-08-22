import { BUILTIN_ICONS } from '../icons/builtin'

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

/**
 * Where a named icon from a public set is fetched from, as a template.
 *
 * Defaults to Iconify's public API, which serves `mdi:`, `si:`, `logos:` and
 * every other set it carries with no bundle cost. It is a BROWSER fetch to a
 * third party, so an install with no egress — or one that would rather not —
 * points this at a mirror or empties it. Emptied, only `aops:`, emoji and
 * explicit URLs draw, which is why the chart ships `aops:`.
 */
const ICON_SERVICE =
  (globalThis as { AGENTOPS_ICON_SERVICE?: string }).AGENTOPS_ICON_SERVICE ??
  'https://api.iconify.design/{prefix}/{name}.svg'

/** A reference is a URL when it names a scheme a browser can fetch. */
function isUrl(ref: string) {
  return /^https?:\/\//i.test(ref)
}

/** `prefix:name`, where the prefix is a set rather than a URL scheme. */
function named(ref: string): { prefix: string; name: string } | null {
  const m = /^([a-z0-9-]+):([a-z0-9-]+)$/i.exec(ref)
  return m ? { prefix: m[1].toLowerCase(), name: m[2].toLowerCase() } : null
}

export function Icon({ icon, size = '1em' }: { icon?: string; size?: string }) {
  const ref = icon?.trim()
  if (!ref) return null

  const set = named(ref)

  // The built-in set. Bundled, so it needs nothing and cannot 404.
  if (set?.prefix === 'aops') {
    const path = BUILTIN_ICONS[set.name]
    if (!path) return null
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
        <path d={path} />
      </svg>
    )
  }

  if (isUrl(ref)) {
    return <img src={ref} alt="" width={size} height={size} style={{ verticalAlign: '-0.125em', flex: 'none' }} />
  }

  if (set) {
    if (!ICON_SERVICE) return null
    const url = ICON_SERVICE.replace('{prefix}', encodeURIComponent(set.prefix))
      .replace('{name}', encodeURIComponent(set.name))
    return <img src={url} alt="" width={size} height={size} style={{ verticalAlign: '-0.125em', flex: 'none' }} />
  }

  // Anything else is text — which is what an emoji is.
  return (
    <span aria-hidden style={{ flex: 'none' }}>
      {ref}
    </span>
  )
}

/**
 * Drops a lane emoji the manager wrote into a conversation TITLE.
 *
 * Titles are composed with a leading 🤖 or 🛠 so a chat surface — which cannot
 * draw an SVG next to a thread name — still says something at a glance. A
 * surface that CAN draw the Pipeline's own icon shows that instead, and would
 * otherwise show both.
 *
 * Deliberately narrow: only the two the manager writes, only at the very
 * start. Somebody's own emoji in the middle of a title is their text.
 */
const LANE_ICONS = ['🤖', '🛠', '💬']

export function stripLeadingIcon(title: string): string {
  const t = title.trimStart()
  for (const icon of LANE_ICONS) {
    if (t.startsWith(icon)) return t.slice(icon.length).trimStart()
  }
  return title
}
