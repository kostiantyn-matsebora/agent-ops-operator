import { ToggleGroup, ToggleGroupItem, Tooltip } from '@patternfly/react-core'
import { resolve, useThemeStore, type ThemeChoice } from '../theme/useTheme'

// The theme switcher.
//
// Three states, not two: "system" is a real choice and the default, so a toggle
// that only offered light/dark would silently opt everyone out of following
// their OS the first time they touched it.

const OPTIONS: { id: ThemeChoice; label: string; glyph: string; hint: string }[] = [
  { id: 'light', label: 'Light', glyph: '☀', hint: 'Always light' },
  { id: 'dark', label: 'Dark', glyph: '☾', hint: 'Always dark' },
  { id: 'system', label: 'Auto', glyph: '◐', hint: 'Follow the operating system' },
]

export function ThemeSwitcher() {
  const choice = useThemeStore((s) => s.choice)
  const setChoice = useThemeStore((s) => s.setChoice)
  const applied = resolve(choice)

  return (
    <ToggleGroup aria-label="colour theme" data-testid="theme-switcher" data-applied={applied}>
      {OPTIONS.map((o) => (
        <Tooltip
          key={o.id}
          content={o.id === 'system' ? `${o.hint} (currently ${applied})` : o.hint}
        >
          <ToggleGroupItem
            aria-label={`${o.label} theme`}
            buttonId={`theme-${o.id}`}
            isSelected={choice === o.id}
            onChange={() => setChoice(o.id)}
            text={
              <span aria-hidden="true" style={{ fontSize: 14, lineHeight: 1 }}>
                {o.glyph}
              </span>
            }
          />
        </Tooltip>
      ))}
    </ToggleGroup>
  )
}
