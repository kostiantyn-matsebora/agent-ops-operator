// The six viewports `console-looks` names once (spec.md, "Every look is
// responsive and tested at set viewports") — every spec that checks a look
// across sizes imports this instead of repeating the numbers, so a drift
// against the openspec table fails loudly (viewport-matrix.spec.ts pins the
// two against each other).

export interface NamedViewport {
  name: string
  width: number
  height: number
  /** The spec's "Pointer" column: touch viewports also carry the 44px control-size rule. */
  touch: boolean
  /**
   * Playwright's own device-emulation flag. Only the phone sets it — the
   * same way the ad-hoc viewports in workspace.spec.ts already do (a tablet
   * is touch but not a mobile device emulation).
   */
  isMobile: boolean
}

export const VIEWPORTS: readonly NamedViewport[] = [
  { name: 'phone', width: 390, height: 844, touch: true, isMobile: true },
  { name: 'tablet portrait', width: 768, height: 1024, touch: true, isMobile: false },
  { name: 'tablet landscape', width: 1024, height: 768, touch: true, isMobile: false },
  { name: 'laptop', width: 1280, height: 800, touch: false, isMobile: false },
  { name: 'desktop', width: 1440, height: 900, touch: false, isMobile: false },
  { name: 'wide', width: 2560, height: 1300, touch: false, isMobile: false },
]

/** The Playwright context options one of these viewports asks for. */
export function viewportUse(v: NamedViewport): { viewport: { width: number; height: number }; hasTouch: boolean; isMobile: boolean } {
  return { viewport: { width: v.width, height: v.height }, hasTouch: v.touch, isMobile: v.isMobile }
}
