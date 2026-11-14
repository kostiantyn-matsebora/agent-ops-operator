import { useContext, useId } from 'react'
import { useLookStore } from '../../theme/useLook'
import { LookContext } from './LookContext'
import { TESTID } from '../../model/testids'

// The look switcher, rendered by every look's header.
//
// A native select, not a component library's: it belongs to no look, so it
// carries none of one's styling, and the browser already makes it keyboard-
// and screen-reader-complete. `color-scheme` on <html> (useTheme.ts) gives
// its native chrome the colour scheme in force.
//
// Choosing stores the look and applies it at once — the root re-resolves and
// mounts the chosen look with no reload, and no request leaves the browser.

export function LookSwitcher({ className }: Readonly<{ className?: string }>) {
  const { active, looks } = useContext(LookContext)
  const choose = useLookStore((s) => s.choose)
  const id = useId()

  return (
    <span
      className={className}
      data-testid={TESTID.lookSwitcher}
      data-active={active}
      style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}
    >
      <label htmlFor={id}>Look</label>
      <select
        id={id}
        value={active}
        onChange={(e) => choose(e.target.value)}
        style={{ font: 'inherit', padding: '4px 6px', borderRadius: 4 }}
      >
        {looks.map((l) => (
          <option key={l.id} value={l.id}>
            {l.label}
          </option>
        ))}
      </select>
    </span>
  )
}
