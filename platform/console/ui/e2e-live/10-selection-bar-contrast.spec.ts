import { expect, test } from '@playwright/test'
import { loadFixtures } from './support/fixtures'

// Table item #10 — the selection bar's text must actually be legible
// against its own background, in BOTH colour themes. The regression: the
// bar inverted its background's lightness between light and dark mode while
// its buttons kept reading colour from the PAGE's own tokens, so one theme
// went unreadable. Measured here, live, as a contrast ratio — never
// inferred from source.

const f = loadFixtures()

function relativeLuminance([r, g, b]: [number, number, number]): number {
  const [rs, gs, bs] = [r, g, b].map((c) => {
    const s = c / 255
    return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4
  })
  return 0.2126 * rs + 0.7152 * gs + 0.0722 * bs
}

function parseRgb(css: string): [number, number, number] {
  const m = /rgba?\(([\d.]+),\s*([\d.]+),\s*([\d.]+)/.exec(css)
  if (!m) throw new Error(`could not parse colour: ${css}`)
  return [Number(m[1]), Number(m[2]), Number(m[3])]
}

/** WCAG contrast ratio, (L1+0.05)/(L2+0.05) with the lighter one first. */
function contrastRatio(a: string, b: string): number {
  const la = relativeLuminance(parseRgb(a))
  const lb = relativeLuminance(parseRgb(b))
  const [lighter, darker] = la >= lb ? [la, lb] : [lb, la]
  return (lighter + 0.05) / (darker + 0.05)
}

async function selectionBarContrast(page: import('@playwright/test').Page): Promise<{ text: string; bg: string; ratio: number }> {
  const bar = page.getByTestId('selection-bar')
  const [text, bg] = await bar.evaluate((el) => {
    const s = getComputedStyle(el)
    return [s.color, s.backgroundColor]
  })
  return { text, bg, ratio: contrastRatio(text, bg) }
}

test('#10 the selection bar text is legible against its background in both themes', async ({ page }) => {
  const name = f.conversations.prefixTask.name
  await page.goto('/conversations')
  await expect(page.getByTestId('inbox')).toBeVisible()
  await expect(page.getByTestId(`row-${name}`)).toBeVisible({ timeout: 20_000 })
  await page.getByRole('button', { name: 'Select', exact: true }).click()
  await page.locator(`#select-${name}`).check()
  await expect(page.getByTestId('selection-bar')).toBeVisible()

  for (const theme of ['light', 'dark'] as const) {
    // Each button carries a hover Tooltip, and the PREVIOUS button's own
    // tooltip can still be overlaying this one's target at the moment of a
    // real pointer click — `force: true` alone was tried and measured NOT
    // to fix it: Playwright's own actionability check is bypassed, but the
    // browser's real hit-testing still delivers the click to whatever
    // element is actually on top at those coordinates, which was the
    // lingering tooltip. `dispatchEvent` fires the click on the TARGET
    // NODE directly, regardless of what visually overlaps it.
    await page.locator(`#theme-${theme}`).dispatchEvent('click')
    await expect(page.getByTestId('theme-switcher')).toHaveAttribute('data-applied', theme)

    const { text, bg, ratio } = await selectionBarContrast(page)
    // A minimum bar, well short of strict WCAG AA (4.5) to avoid pinning a
    // design choice this role does not own — but the regression measured
    // under 1.2 (near-identical colours), so this is a real, wide margin
    // against it recurring, not a rubber stamp.
    expect(ratio, `${theme} theme: text ${text} on background ${bg} contrast ${ratio.toFixed(2)}`).toBeGreaterThan(2.5)
    await page.screenshot({ path: `e2e-live/proofs/10-selection-bar-${theme}.png`, fullPage: true })
  }
})
