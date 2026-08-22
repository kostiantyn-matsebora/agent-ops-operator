/**
 * The keys a composer answers to, stated where they can be READ.
 *
 * It used to live in the field's placeholder, which is the worst of both
 * worlds: rendered faint by design, at the smallest size on the page, and gone
 * the moment somebody types the thing it was explaining.
 *
 * So it sits UNDER the field, at body size, in the regular text colour, and the
 * keys are drawn as keys — a reader recognises a keycap before they read the
 * sentence around it.
 */

/** Key renders one keycap. */
function Key({ children }: { children: React.ReactNode }) {
  return (
    <kbd
      style={{
        display: 'inline-block',
        minWidth: '1.5em',
        padding: '0.1em 0.4em',
        border: '1px solid var(--ao-border-strong)',
        borderBottomWidth: 2,
        borderRadius: '0.25em',
        background: 'var(--ao-surface-alt)',
        color: 'var(--ao-text)',
        font: 'inherit',
        fontWeight: 600,
        lineHeight: 1.4,
        textAlign: 'center',
      }}
    >
      {children}
    </kbd>
  )
}

export interface Shortcut {
  /** The keys, in the order they are pressed. */
  keys: string[]
  /** What pressing them does. */
  does: string
}

export function ComposerHint({ shortcuts }: { shortcuts: Shortcut[] }) {
  return (
    <div
      style={{
        display: 'flex',
        flexWrap: 'wrap',
        alignItems: 'center',
        // Everything here is relative to the TEXT it sits beside: no fixed
        // sizes, so it stays proportionate at any viewport and any user font
        // size. A hard rem would be right at one width and wrong at the next.
        gap: '0.35em 1.25em',
        // No outer margin: the CALLER decides the spacing, because one puts
        // this in a row beside a button and another stands it alone.
        marginTop: '0.5rem',
        // INHERITS the body size, and takes the regular text colour. A hint
        // nobody can read is a hint nobody follows, and the smallest type on
        // the page is how it got unreadable in the first place.
        color: 'var(--ao-text)',
      }}
    >
      {shortcuts.map((s) => (
        <span key={s.does} style={{ display: 'inline-flex', alignItems: 'center', gap: '0.3rem' }}>
          {s.keys.map((k, i) => (
            <span key={k} style={{ display: 'inline-flex', alignItems: 'center', gap: '0.3rem' }}>
              {i > 0 && <span aria-hidden style={{ color: 'var(--ao-text-subtle)' }}>+</span>}
              <Key>{k}</Key>
            </span>
          ))}
          <span>{s.does}</span>
        </span>
      ))}
    </div>
  )
}
