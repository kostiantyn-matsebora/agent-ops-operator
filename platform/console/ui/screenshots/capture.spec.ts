import { test, type Page } from '@playwright/test'
import { createServer, type Server, type ServerResponse } from 'node:http'
import { mkdir, readFile, writeFile } from 'node:fs/promises'
import { extname, join, normalize } from 'node:path'
import type { AddressInfo } from 'node:net'
import { NOW, answer } from './fixture'

// The site's console screenshots, produced rather than captured.
//
// It drives the SAME bundle the image embeds — served from `dist/` over a local
// http server, exactly as e2e/stream-chip.spec.ts does — against the curated
// fixture next door, and writes both theme variants of each view into the
// site's asset directory.
//
// Run it deliberately, never as part of `npm test`:
//
//   npx playwright install chromium && npm run screenshots
//
// THE OUTPUT MUST BE BYTE-STABLE between runs, or every future diff is
// unreadable. Three things buy that, and all three are load-bearing:
//
//   - the clock is PINNED, so every "3m ago" is the same string;
//   - animations are disabled AND SMIL is paused, because the topology graph
//     animates its edges in SVG, which the Playwright option does not cover;
//   - the viewport is fixed and the capture is of the viewport, not the page,
//     so a row more or less never changes the image's shape.

// ONE fixed WIDTH, and a height that follows the view.
//
// The width is fixed because it decides the layout: at 1920 the Overview's card
// row sits on one line and no table column is squeezed, so every screenshot is
// the same product at the same size.
//
// The height is measured, and that is a deliberate departure from a single
// fixed viewport. The console scrolls INSIDE its main region, so a fixed height
// either crops a view or pads it: at 1400 the Overview's Problems table — the
// whole point of that view — was only just in frame, while Queues carried 700px
// of empty background. Bounded, from content the fixture pins, so two runs
// still produce the same file.
const WIDTH = 1920
const MIN_HEIGHT = 420
const MAX_HEIGHT = 1600

/** Page ground below the last control, so a view does not end flush. */
const GUTTER = 24

const OUT = join(process.cwd(), '..', '..', 'docs', 'assets', 'img', 'console')

const MIME: Record<string, string> = {
  '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css',
  '.svg': 'image/svg+xml', '.woff2': 'font/woff2', '.json': 'application/json',
  '.png': 'image/png', '.ico': 'image/x-icon',
}

/** The six views, in the order the page tours them. */
const VIEWS = [
  { file: 'overview', path: '/overview', ready: 'Installation' },
  { file: 'topology', path: '/topology', ready: 'k8s-observe' },
  { file: 'conversations', path: '/conversations', ready: 'checkout-api is restarting' },
  { file: 'conversation', path: '/conversations/cluster-events-7c1d4e', ready: 'OOM-killed' },
  { file: 'queues', path: '/queues', ready: 'Queues and capacity' },
  // The Pipelines inventory rather than the kind index: the tour asks "what is
  // wired to what", and a grid of object counts does not answer it.
  { file: 'configuration', path: '/config/pipelines', ready: 'alert-triage' },
]

/**
 * Screenshots the page once it has stopped changing, then writes the file.
 *
 * Pausing SMIL and disabling CSS animation is not quite enough on its own: the
 * topology view lays itself out over a frame or two, and a capture taken during
 * that produced a file that differed between runs by a few bytes — which is
 * exactly the unreadable diff this whole approach exists to avoid. So the same
 * frame has to be produced TWICE before it is believed.
 */
async function writeWhenStable(page: Page, path: string): Promise<void> {
  let previous: Buffer | null = null
  for (let attempt = 0; attempt < 12; attempt++) {
    const shot = await page.screenshot({ animations: 'disabled', caret: 'hide', scale: 'css' })
    if (previous && previous.equals(shot)) {
      await writeFile(path, shot)
      return
    }
    previous = shot
    await page.waitForTimeout(250)
  }
  throw new Error(`${path}: the view never stopped changing`)
}

/**
 * Sizes the window to the view: big enough that NOTHING inside it scrolls, and
 * no bigger than the content it holds.
 *
 * Both halves were paid for. The console scrolls inside its main region, so
 * what is off screen is off screen — and one measurement is not enough, because
 * the transcript list is sized in `vh`: growing the window grows the list,
 * which pushes the composer below the window that was just measured. The
 * Conversation screenshot published for months ended mid-thread with no reply
 * box in it, which is the one control that view exists for.
 *
 * MAIN ITSELF IS A SCROLLING REGION. Walking only its descendants left the Send
 * button clipped by main's own 32px of overflow, in a capture whose arithmetic
 * said everything fitted.
 *
 * A shrink pass used to follow, trimming back to the content. It is gone: once
 * the fit grows on BOTH measures it lands on the content already, and the trim
 * could never fire — the page body stretches to the window, so "where the
 * content ends" saturates at the window it was measured in.
 */
async function fitViewport(page: Page): Promise<void> {
  const measure = () =>
    page.evaluate(() => {
      const main = document.querySelector('main')
      if (!main) return { needed: 0, bottom: 0, hidden: 0 }
      const chrome = document.querySelector('header')?.getBoundingClientRect().height ?? 0

      let hidden = 0
      for (const el of [main, ...Array.from(main.querySelectorAll('*'))]) {
        const overflowY = getComputedStyle(el).overflowY
        if (overflowY !== 'auto' && overflowY !== 'scroll') continue
        hidden = Math.max(hidden, el.scrollHeight - el.clientHeight)
      }

      // Where the view actually ends, which `scrollHeight` does not report: a
      // child may sit past its parent's box, and the Send button does.
      //
      // Measured from the LEAVES. A layout's wrappers stretch to the window by
      // design, so the deepest element is always the window itself and the trim
      // never fires — 100px of empty ground under the last card is what that
      // looked like. What a leaf cannot see is the padding and border its
      // ancestors close with, so those are added back. Adding all of them
      // over-counts a little, which is the safe direction: this number may
      // leave slack, and may never clip.
      let bottom = 0
      main.querySelectorAll('*').forEach((el) => {
        if (el.children.length > 0) return
        const rect = el.getBoundingClientRect()
        if (rect.width <= 0 || rect.height <= 0) return
        let edge = rect.bottom
        for (let node = el.parentElement; node && node !== main.parentElement; node = node.parentElement) {
          const style = getComputedStyle(node)
          edge += (parseFloat(style.paddingBottom) || 0) +
            (parseFloat(style.borderBottomWidth) || 0) +
            (parseFloat(style.marginBottom) || 0)
        }
        bottom = Math.max(bottom, edge)
      })

      return {
        needed: Math.ceil(main.scrollHeight + chrome + hidden),
        bottom: Math.ceil(bottom),
        hidden,
      }
    })

  const resize = async (height: number) => {
    await page.setViewportSize({ width: WIDTH, height })
    await page.evaluate(() => document.fonts.ready)
  }

  const clamp = (h: number) => Math.min(MAX_HEIGHT, Math.max(MIN_HEIGHT, h))

  let height = MIN_HEIGHT
  for (let pass = 0; pass < 12; pass++) {
    const { needed, bottom } = await measure()
    // BOTH measures, whichever is larger. `scrollHeight` misses a child that
    // overflows its parent's box, and the composer is exactly that: it sits ~40
    // pixels below main's own bottom edge, so growing on `needed` alone stopped
    // one row short and cut the Send button off — twice, in two different ways.
    const next = clamp(Math.max(needed, bottom))
    if (next <= height) break
    height = next
    await resize(height)
  }

  // A page ends with its own ground, not flush against its last control. The
  // fit lands exactly on the content, which reads as a screenshot someone
  // cropped by hand and got wrong by a hair.
  height = clamp(height + GUTTER)
  await resize(height)
}

interface Harness {
  url: string
  stop(): Promise<void>
}

async function startHarness(): Promise<Harness> {
  const dist = join(process.cwd(), 'dist')
  const live = new Set<ServerResponse>()

  const server: Server = createServer(async (req, res) => {
    const [path, rawQuery] = (req.url ?? '/').split('?')

    // A real stream, left open: the masthead chip settles on "live" instead of
    // painting "stream disconnected" into every screenshot.
    if (path === '/api/stream') {
      res.writeHead(200, {
        'content-type': 'text/event-stream',
        'cache-control': 'no-cache',
        connection: 'keep-alive',
      })
      res.write('event: resync\ndata: {"reason":"connected"}\n\n')
      live.add(res)
      req.on('close', () => live.delete(res))
      return
    }

    if (path.startsWith('/api/')) {
      const body = answer(path, new URLSearchParams(rawQuery ?? '')) ?? {}
      res.writeHead(200, { 'content-type': 'application/json' })
      res.end(JSON.stringify(body))
      return
    }

    const rel = normalize(path).replace(/^([/\\])+/, '')
    for (const file of [rel, 'index.html']) {
      try {
        const content = await readFile(join(dist, file))
        res.writeHead(200, { 'content-type': MIME[extname(file)] ?? 'application/octet-stream' })
        res.end(content)
        return
      } catch {
        /* SPA fallback */
      }
    }
    res.writeHead(404)
    res.end()
  })

  await new Promise<void>((resolve) => server.listen(0, '127.0.0.1', resolve))
  const { port } = server.address() as AddressInfo

  return {
    url: `http://127.0.0.1:${port}`,
    async stop() {
      for (const res of live) res.end()
      live.clear()
      server.closeAllConnections()
      await new Promise<void>((resolve) => server.close(() => resolve()))
    },
  }
}

test.describe.configure({ mode: 'serial' })

test('captures every console view in both themes', async ({ browser }) => {
  test.setTimeout(180_000)

  const harness = await startHarness()
  await mkdir(OUT, { recursive: true })

  try {
    for (const theme of ['light', 'dark'] as const) {
      // The console's own default is "follow the system", so the context's
      // colour scheme is all it takes — no click, no stored choice.
      const context = await browser.newContext({
        viewport: { width: WIDTH, height: MIN_HEIGHT },
        colorScheme: theme,
        // A relative age and a formatted time are both locale-dependent, and a
        // screenshot taken on another machine must be the same file.
        locale: 'en-GB',
        timezoneId: 'UTC',
        reducedMotion: 'reduce',
      })
      const page = await context.newPage()
      await page.clock.setFixedTime(NOW)
      // Belt and braces over Playwright's `animations: 'disabled'`, which ends
      // a transition rather than preventing it. One in-flight transition on the
      // masthead button was enough to move a single antialiased pixel between
      // runs — one pixel, and every future diff would have shown a change.
      await page.addInitScript(() => {
        const css = '*,*::before,*::after{transition:none!important;animation:none!important}'
        const apply = () => {
          const style = document.createElement('style')
          style.textContent = css
          document.head.appendChild(style)
        }
        if (document.head) apply()
        else document.addEventListener('DOMContentLoaded', apply)
      })

      for (const view of VIEWS) {
        await page.setViewportSize({ width: WIDTH, height: MIN_HEIGHT })
        await page.goto(`${harness.url}${view.path}`)
        await page.getByText(view.ready, { exact: false }).first().waitFor({ timeout: 30_000 })
        await page.waitForLoadState('networkidle')
        // Text metrics change when a web font lands, and the fonts are cold in
        // the first context — measuring the layout before they arrive is how a
        // row ends up one pixel taller in one run than the next.
        await page.evaluate(() => document.fonts.ready)

        await fitViewport(page)

        // SVG animation is SMIL, which `animations: 'disabled'` does not touch.
        // Pausing alone is not enough: it freezes at whatever moment the call
        // lands, so two runs stop the traffic dashes at different offsets.
        // Rewinding to zero makes the frozen frame the same frame every time.
        await page.evaluate(() => {
          document.querySelectorAll('svg').forEach((svg) => {
            if (typeof svg.pauseAnimations !== 'function') return
            svg.pauseAnimations()
            svg.setCurrentTime(0)
          })
        })

        await writeWhenStable(page, join(OUT, `${view.file}-${theme}.png`))
      }

      await context.close()
    }
  } finally {
    await harness.stop()
  }
})
