import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ThemeSwitcher } from '../components/ThemeSwitcher'
import { applyTheme, resolve, startThemeSync, useThemeStore } from './useTheme'

// The theme is applied to <html>, because that is where PatternFly's tokens and
// our own `--ao-*` palette are both defined.

/** matchMedia stub: jsdom has none, and "system" is the default choice. */
function stubSystem(dark: boolean) {
  const listeners: ((e: MediaQueryListEvent) => void)[] = []
  vi.stubGlobal(
    'matchMedia',
    vi.fn().mockImplementation((query: string) => ({
      matches: dark && query.includes('dark'),
      media: query,
      addEventListener: (_: string, fn: (e: MediaQueryListEvent) => void) => listeners.push(fn),
      removeEventListener: () => undefined,
      addListener: () => undefined,
      removeListener: () => undefined,
      dispatchEvent: () => false,
    })),
  )
  return {
    flip: (nowDark: boolean) => {
      vi.stubGlobal(
        'matchMedia',
        vi.fn().mockImplementation((query: string) => ({
          matches: nowDark && query.includes('dark'),
          media: query,
          addEventListener: () => undefined,
          removeEventListener: () => undefined,
          addListener: () => undefined,
          removeListener: () => undefined,
          dispatchEvent: () => false,
        })),
      )
      listeners.forEach((fn) => fn({} as MediaQueryListEvent))
    },
  }
}

beforeEach(() => {
  document.documentElement.className = ''
  document.documentElement.removeAttribute('data-theme')
  useThemeStore.setState({ choice: 'system' })
  stubSystem(false)
})

describe('applyTheme', () => {
  it('puts exactly one PatternFly theme class on the root', () => {
    applyTheme('dark')
    expect(document.documentElement).toHaveClass('pf-v6-theme-dark')
    expect(document.documentElement).not.toHaveClass('pf-v6-theme-light')

    applyTheme('light')
    expect(document.documentElement).toHaveClass('pf-v6-theme-light')
    expect(document.documentElement).not.toHaveClass('pf-v6-theme-dark')
  })

  it('exposes the theme as an attribute and to the browser chrome', () => {
    applyTheme('dark')
    expect(document.documentElement.getAttribute('data-theme')).toBe('dark')
    expect(document.documentElement.style.colorScheme).toBe('dark')
  })
})

describe('resolve', () => {
  it('follows the system only for the system choice', () => {
    stubSystem(true)
    expect(resolve('system')).toBe('dark')
    expect(resolve('light')).toBe('light')
    expect(resolve('dark')).toBe('dark')
  })
})

describe('startThemeSync', () => {
  it('keeps following the OS while the choice is system', () => {
    const sys = stubSystem(false)
    const stop = startThemeSync()
    expect(document.documentElement.getAttribute('data-theme')).toBe('light')

    // an operator who leaves the console open overnight should not have to
    // reload when the OS flips
    sys.flip(true)
    expect(document.documentElement.getAttribute('data-theme')).toBe('dark')
    stop()
  })

  it('ignores the OS once a theme is chosen explicitly', () => {
    const sys = stubSystem(false)
    const stop = startThemeSync()
    useThemeStore.getState().setChoice('light')

    sys.flip(true)
    expect(document.documentElement.getAttribute('data-theme')).toBe('light')
    stop()
  })
})

// Item 27: a `<Label color="purple">` (the root/coordinator chip,
// ConversationRow's `RowBadges`) read near-black text on the medium-purple
// `--ao-accent` background in light mode. PatternFly pairs each nonstatus
// background with its own matched foreground token, and overriding only the
// background orphaned that pairing — this asserts the fix stays wired rather
// than re-drifting, since jsdom does not compute real cascaded contrast from
// an external stylesheet the way a screenshot would.
describe('Label contrast tokens (item 27)', () => {
  // Vite reads the source at build time, so this needs no Node `fs` —
  // the same `?raw` idiom `Text.test.tsx` uses to scan every `.ts`/`.tsx`
  // file, here pointed at the one stylesheet.
  const css = Object.values(
    import.meta.glob('./theme.css', { eager: true, query: '?raw', import: 'default' }),
  )[0] as string
  const lightBlock = css.slice(css.indexOf(':root,\n.pf-v6-theme-light {'), css.indexOf('.pf-v6-theme-dark {'))
  const darkBlock = css.slice(css.indexOf('.pf-v6-theme-dark {'), css.indexOf('/*\n * Map PatternFly'))

  it('pairs the light theme\'s saturated brand/accent backgrounds with light text', () => {
    expect(lightBlock).toContain('--ao-on-brand: var(--ao-surface);')
    expect(lightBlock).toContain('--ao-on-accent: var(--ao-surface);')
  })

  it('pairs the dark theme\'s pale, inverted brand/accent backgrounds with dark text', () => {
    expect(darkBlock).toContain('--ao-on-brand: var(--ao-canvas);')
    expect(darkBlock).toContain('--ao-on-accent: var(--ao-canvas);')
  })

  it('wires PatternFly\'s on-blue/on-purple text AND icon tokens to the matched pairing, not left on its own stock default', () => {
    for (const nonstatus of ['blue', 'purple']) {
      const own = nonstatus === 'blue' ? '--ao-on-brand' : '--ao-on-accent'
      for (const state of ['default', 'hover', 'clicked']) {
        expect(css).toContain(`--pf-t--global--text--color--nonstatus--on-${nonstatus}--${state}: var(${own});`)
        expect(css).toContain(`--pf-t--global--icon--color--nonstatus--on-${nonstatus}--${state}: var(${own});`)
      }
    }
  })

  it('leaves grey Labels untouched — PatternFly\'s own stock gray pairing is never overridden', () => {
    expect(css).not.toMatch(/nonstatus--gray/)
  })
})

describe('ThemeSwitcher', () => {
  it('offers three states, because "follow the system" is a real choice', () => {
    render(<ThemeSwitcher />)
    expect(screen.getByLabelText('Light theme')).toBeInTheDocument()
    expect(screen.getByLabelText('Dark theme')).toBeInTheDocument()
    expect(screen.getByLabelText('Auto theme')).toBeInTheDocument()
  })

  it('applies the chosen theme and remembers it', async () => {
    render(<ThemeSwitcher />)
    await userEvent.click(screen.getByLabelText('Dark theme'))

    expect(useThemeStore.getState().choice).toBe('dark')
    expect(document.documentElement).toHaveClass('pf-v6-theme-dark')
    expect(screen.getByTestId('theme-switcher')).toHaveAttribute('data-applied', 'dark')
  })
})
