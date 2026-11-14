/** The viewer's reduced-motion preference — console-thread-live-cues: "Motion SHALL respect the viewer's reduced-motion preference." */
export function prefersReducedMotion(): boolean {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') return false
  return window.matchMedia('(prefers-reduced-motion: reduce)').matches
}
