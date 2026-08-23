/**
 * fence wraps a machine document in a code block, tagged json when it looks like
 * one so the highlighter colours it.
 *
 * A payload unfenced is reflowed as prose — every newline collapsed, every brace
 * run together with the sentence before it. That is what made an event card
 * unreadable.
 */
export function fence(body: string): string {
  const t = body.trim()
  const lang = t.startsWith('{') || t.startsWith('[') ? 'json' : ''
  // A payload containing its own fence would close ours early. Longer fences
  // nest, which is the markdown-defined way out.
  let ticks = '```'
  while (t.includes(ticks)) ticks += '`'
  return `${ticks}${lang}\n${t}\n${ticks}`
}
