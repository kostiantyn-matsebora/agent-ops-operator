import { usePipelineIcon } from '../api/hooks'

// A PIPELINE'S DECLARED ICON, looked up by name, for a look that draws a
// Pipeline's name beside it. One lookup for every look, so no mention draws a
// different icon from the next (see model/icon.ts for the resolution itself).

/** Returns a lookup from a Pipeline's name to the icon reference it declares. */
export function usePipelineIconLookup(): (name?: string) => string | undefined {
  return usePipelineIcon()
}
