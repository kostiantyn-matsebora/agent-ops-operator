// PLAIN TEXT, ALWAYS — the one rule every look renders cluster- and
// wire-sourced strings by.
//
// React already escapes, so this is not escaping. An agent's output, a
// condition message or a label can carry the chat HTML subset an adapter
// once spoke, and the console shows the text a person typed, never a tag.
// Each look draws the result its own way; none recomputes it.

const TAG = /<\/?[a-zA-Z][^>]*>/g
const ENTITIES: Record<string, string> = {
  '&amp;': '&',
  '&lt;': '<',
  '&gt;': '>',
  '&quot;': '"',
  '&#39;': "'",
  '&nbsp;': ' ',
}

/**
 * plain strips the chat HTML subset and decodes the entities the adapters
 * escape, so text arrives as the person actually typed it.
 */
export function plain(input: string): string {
  const withoutTags = input.replace(TAG, '')
  return withoutTags.replace(/&(amp|lt|gt|quot|#39|nbsp);/g, (m) => ENTITIES[m] ?? m)
}
