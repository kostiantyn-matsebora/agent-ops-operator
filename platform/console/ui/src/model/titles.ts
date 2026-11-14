// How a conversation is named on screen — a derived fact, so every look names
// a conversation the same way.

/**
 * Drops a lane emoji the manager wrote into a conversation TITLE.
 *
 * Titles are composed with a leading 🤖, 🛠 or 💬 so a chat surface — which
 * cannot draw an SVG next to a thread name — still says something at a
 * glance. A surface that CAN draw the Pipeline's own icon shows that
 * instead, and would otherwise show both. 🤝 is the fourth: a Coordinator
 * member's title (`internal/chat/coordinate.go`'s `"🤝 " + entryName + ...`).
 *
 * Deliberately narrow: only what the manager writes, only at the very
 * start. Somebody's own emoji in the middle of a title is their text.
 */
const LANE_ICONS = ['🤖', '🛠', '💬', '🤝']

export function stripLeadingIcon(title: string): string {
  const t = title.trimStart()
  for (const icon of LANE_ICONS) {
    if (t.startsWith(icon)) return t.slice(icon.length).trimStart()
  }
  return title
}

/**
 * Strips a leading "`<name>`: " from a title, ONLY when `<name>` is exactly
 * the name the row's own chip already shows — the coordinator for a root, the
 * pipeline otherwise. Any other colon-prefixed text (an agent's own
 * "esphome: ...", "homeassistant: ...") is content and stays untouched: this
 * never strips a prefix it cannot attribute to the chip.
 */
export function stripNamePrefix(title: string, name: string | undefined): string {
  if (!name) return title
  const prefix = `${name}: `
  return title.startsWith(prefix) ? title.slice(prefix.length) : title
}
