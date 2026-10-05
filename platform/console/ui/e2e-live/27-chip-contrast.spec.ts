import { expect, test, type Page } from '@playwright/test'
import { loadFixtures } from './support/fixtures'

// Item #27 — the purple coordinator-name chip (`ConversationRow.tsx`'s
// `<Label color="purple">`) must stay legible in BOTH colour themes, now that
// `theme.css` wires `--ao-on-brand`/`--ao-on-accent` onto PatternFly's
// on-purple/on-blue text tokens instead of leaving the chip to inherit the
// page's own text colour.
//
// This is fundamentally a VISUAL fix. The screenshots below are the real
// proof, for a human to read — legible light/near-black text against the
// purple chip, not grey-on-grey. The computed-contrast check is a cheap,
// deliberately loose tripwire on top of that, not a substitute for it.

const f = loadFixtures()

function relativeLuminance([r, g, b]: [number, number, number]): number {
  const [rs, gs, bs] = [r, g, b].map((c) => {
    const s = c / 255
    return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4
  })
  return 0.2126 * rs + 0.7152 * gs + 0.0722 * bs
}

function parseRgb(css: string): [number, number, number] {
  const m = css.match(/rgba?\(([\d.]+),\s*([\d.]+),\s*([\d.]+)/)
  if (!m) throw new Error(`could not parse colour: ${css}`)
  return [Number(m[1]), Number(m[2]), Number(m[3])]
}

function contrastRatio(a: string, b: string): number {
  const la = relativeLuminance(parseRgb(a))
  const lb = relativeLuminance(parseRgb(b))
  const [lighter, darker] = la >= lb ? [la, lb] : [lb, la]
  return (lighter + 0.05) / (darker + 0.05)
}

async function chipColors(page: Page, chipText: string): Promise<{ color: string; bg: string }> {
  const chip = page.getByTestId(`row-${f.conversations.coordOpenRoot}`).getByText(chipText, { exact: true }).first()
  // Measured live (not reasoned): every real ancestor's `backgroundColor` is
  // `rgba(0, 0, 0, 0)` — PatternFly's Label paints its fill on the `::before`
  // PSEUDO-ELEMENT of `.pf-v6-c-label__content` (`label.css`:
  // `.pf-v6-c-label__content::before { background-color:
  // var(--pf-v6-c-label--BackgroundColor) }`), which no DOM element's own
  // `getComputedStyle` ever exposes — it has to be read from that specific
  // pseudo-element.
  return chip.evaluate((el) => {
    let node: HTMLElement | null = el as HTMLElement
    while (node && !node.className?.toString().includes('c-label__content')) {
      node = node.parentElement
    }
    const bg = node ? getComputedStyle(node, '::before').backgroundColor : getComputedStyle(el).backgroundColor
    return { color: getComputedStyle(el).color, bg }
  })
}

test('#27 the purple coordinator chip stays legible in both themes', async ({ page }) => {
  await page.goto('/conversations')
  await expect(page.getByTestId('inbox')).toBeVisible()
  const row = page.getByTestId(`row-${f.conversations.coordOpenRoot}`)
  await expect(row).toBeVisible({ timeout: 20_000 })

  for (const theme of ['light', 'dark'] as const) {
    await page.locator(`#theme-${theme}`).dispatchEvent('click')
    await expect(page.getByTestId('theme-switcher')).toHaveAttribute('data-applied', theme)

    const { color, bg } = await chipColors(page, f.coordinators.open)
    const ratio = contrastRatio(color, bg)
    expect(ratio, `${theme} theme: chip text ${color} on background ${bg}, contrast ${ratio.toFixed(2)}`).toBeGreaterThan(2.5)
    // The list keeps growing from OTHER tests' fixtures arriving live, which
    // can push this row out of the visible scroll position between runs —
    // bring it back into view so the screenshot actually SHOWS the chip,
    // rather than merely proving its colours by computation alone.
    await row.scrollIntoViewIfNeeded()
    await page.screenshot({ path: `e2e-live/proofs/27-chip-contrast-${theme}.png`, fullPage: true })
  }
})
