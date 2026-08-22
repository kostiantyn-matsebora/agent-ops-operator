// The masthead mark.
//
// Previously a PatternFly <Brand src="">, which renders an <img> with an empty
// source — every browser draws its broken-image glyph, which is what the
// masthead was showing. Inline SVG instead: nothing to fetch, nothing to 404,
// no asset to keep in sync with the embed, and it inherits the theme's colours.
//
// The mark is the system in miniature: a signal entering on the left, a pipeline
// claiming it, an answer leaving on the right.
export function Logo({ size = 26 }: { size?: number }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 32 32"
      role="img"
      aria-label="agent-ops"
      style={{ flex: '0 0 auto' }}
    >
      <circle cx="16" cy="16" r="15" fill="var(--ao-brand)" />
      <path
        d="M7 16 h5 M20 16 h5"
        stroke="var(--ao-surface)"
        strokeWidth="2"
        strokeLinecap="round"
        opacity="0.9"
      />
      <path
        d="M16 9 l4 3.5 v7 L16 23 l-4-3.5 v-7 z"
        fill="none"
        stroke="var(--ao-surface)"
        strokeWidth="2"
        strokeLinejoin="round"
      />
      <circle cx="16" cy="16" r="2" fill="var(--ao-surface)" />
    </svg>
  )
}
