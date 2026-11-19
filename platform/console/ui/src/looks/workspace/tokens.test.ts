import { describe, expect, it } from 'vitest'
import workspace from './workspace.css?raw'
import theme from '../../theme/theme.css?raw'

// Task 3.9 and design D6: every `--wk-*` token lives in the workspace's own
// stylesheet. The shared theme holds none, so the docs site's copy of the
// shared palette (palette-and-mark.md) needs none either.

const declared = (css: string) => new Set([...css.matchAll(/(--wk-[a-z0-9-]+)\s*:/g)].map((m) => m[1]))
const used = (css: string) => new Set([...css.matchAll(/var\((--wk-[a-z0-9-]+)/g)].map((m) => m[1]))

describe('the workspace tokens', () => {
  it('are absent from the shared theme', () => {
    expect(theme).not.toMatch(/--wk-/)
  })

  it('are each declared in workspace.css, and every one used there is declared', () => {
    const d = declared(workspace)
    expect(d.size).toBeGreaterThan(10)
    expect([...used(workspace)].filter((t) => !d.has(t))).toEqual([])
  })

  it('set a dark value for every colour they declare', () => {
    const dark = workspace.slice(workspace.indexOf("[data-theme='dark'] .wk {"))
    const colours = [...workspace.matchAll(/(--wk-[a-z-]+):\s*#[0-9a-f]{3,6};/gi)].map((m) => m[1])
    for (const c of new Set(colours)) expect(dark, c).toContain(`${c}:`)
  })
})
