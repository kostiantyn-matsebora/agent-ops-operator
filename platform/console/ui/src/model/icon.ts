import { BUILTIN_ICONS } from '../icons/builtin'

// A PIPELINE'S ICON REFERENCE, resolved once for every look and the graph, so
// every surface draws the same thing or nothing.
//
// The manager publishes the string and interprets it no further:
//
//   aops:<name>   the built-in set, bundled — no network, works air-gapped
//   <emoji>       drawn as text
//   https://…     drawn as an image
//   <lib>:<name>  resolved through the icon service (see ICON_SERVICE)
//
// NOTHING FAILS OVER AN ICON. An unknown reference resolves to nothing.

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

/** What a reference resolves to, for whichever surface draws it: a bundled path, a fetched image, or text. */
export type ResolvedIcon = { kind: 'path'; d: string } | { kind: 'image'; src: string } | { kind: 'text'; text: string }

/** One resolution for the list and the graph, so both draw the same thing or nothing. */
export function resolveIcon(icon?: string): ResolvedIcon | null {
  const ref = icon?.trim()
  if (!ref) return null
  const set = named(ref)
  // The built-in set. Bundled, so it needs nothing and cannot 404.
  if (set?.prefix === 'aops') {
    const d = BUILTIN_ICONS[set.name]
    return d ? { kind: 'path', d } : null
  }
  if (isUrl(ref)) return { kind: 'image', src: ref }
  if (set) {
    if (!ICON_SERVICE) return null
    const src = ICON_SERVICE.replace('{prefix}', encodeURIComponent(set.prefix)).replace('{name}', encodeURIComponent(set.name))
    return { kind: 'image', src }
  }
  // Anything else is text — which is what an emoji is.
  return { kind: 'text', text: ref }
}

