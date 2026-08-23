import { test, type Page } from '@playwright/test'
import { createServer, type Server, type ServerResponse } from 'node:http'
import { execFile } from 'node:child_process'
import { link, mkdir, readFile, rm, stat, writeFile } from 'node:fs/promises'
import { existsSync } from 'node:fs'
import { extname, join, normalize } from 'node:path'
import { promisify } from 'node:util'
import type { AddressInfo } from 'node:net'
import { NOW, responder, type Install } from '../screenshots/fixture'
import { REPLY, beats, opening } from './story'

// The landing page's recording, produced rather than captured.
//
// It drives the SAME bundle the console image embeds, against the SAME invented
// install the screenshots are taken of, and walks the story next door beat by
// beat. Every repaint comes from the console's own data path: the server's
// answers change, one `resync` goes down the open stream, and the app refetches.
// Nothing here paints a row or fakes a state.
//
// Run it deliberately, never as part of `npm test`:
//
//   npx playwright install chromium && npm run demo
//
// THE FRAMES ARE THE REPRODUCIBLE ARTIFACT, NOT THE MP4. Two runs produce the
// same frames — the clock is pinned per beat, animation is dead and SMIL is
// rewound, exactly as in the screenshot capture. The encoder is not asked to be
// deterministic, and nobody should try to make it so: review the story and the
// frames, never the container's bytes.

// ONE FIXED VIEWPORT, unlike the screenshots, which grow to their view. A video
// has one frame size, so the height is a compromise rather than a measurement:
// tall enough that the story's beats are legible, short enough that the beats
// which do not fill it are not mostly empty ground.
//
// 1200 was tried and is too tall. The conversation beats — where the story
// actually happens — ended two thirds of the way down, and on the landing page
// that reads as a bug in the recording rather than as a short view. 900 was
// still loose once the transcript began rendering markdown at full width.
//
// Re-measure this when the console's layout changes. It is a fit to the view
// the story spends most of its time in, not a constant.
const WIDTH = 1920
const HEIGHT = 800

/** Output frame rate. Held frames are repeated to fill their time. */
const FPS = 30

// Beats CROSS-FADE into each other. Hard cuts between six stills read as a
// slideshow with a stuck projector, and half a second is enough to say "this
// followed that" without anyone waiting for it.
//
// It is BETWEEN beats only. The frames inside the typing run are one continuous
// action, and fading them would smear the characters as they appear.
const FADE_SECONDS = 0.5

// The budgets. A recording that breaks one is shortened or re-encoded — the
// budget is not raised. A committed binary regenerated on every UI change needs
// a ceiling or it becomes repository weight nobody notices.
const MAX_SECONDS = 75
const MAX_BYTES = 4 * 1024 * 1024
const MAX_POSTER_BYTES = 400 * 1024

// No ffmpeg on a workstation that has no toolchains at all, so it runs in a
// container — PINNED BY DIGEST, because "latest" would re-encode differently on
// a machine that pulled it a month later.
//
// A STATIC BUILD, deliberately. An image that inits itself (linuxserver's does,
// through s6) insists on remapping its own user and dies under `--user`, which
// is not optional here: the container writes into the repository, and a
// root-owned MP4 is a mess to undo.
const FFMPEG_IMAGE = process.env.AGENTOPS_FFMPEG_IMAGE ??
  'mwader/static-ffmpeg@sha256:a8090df5f5608daef387e1b2e93b98aaacb4d92153ad904e7d715c725724fca4'

// THREE LEVELS UP — see the same note in screenshots/capture.spec.ts. The
// restructure added the `platform/` level and this path was not moved with it.
const OUT = join(process.cwd(), '..', '..', '..', 'docs', 'assets', 'video')

// THE OUTPUT PATH IS ASSERTED, NOT ASSUMED.
//
// This wrote to a junk directory for a full day and reported SUCCESS every
// time, because a relative path that no longer resolves still names somewhere
// writable. The published assets simply stopped updating and nothing said so.
//
// THERE IS EXACTLY ONE `docs/` IN THIS REPOSITORY, at the root. If the resolved
// path is not an existing directory under it, the run FAILS rather than
// creating a second one.
if (!existsSync(OUT)) {
  throw new Error(
    `output directory does not exist: ${OUT}\n` +
      `There is one docs/ tree in this repo, at the root. A missing path here means ` +
      `this file moved and its relative path did not — do NOT let it create the directory.`,
  )
}

// BESIDE THE RECORDER, never under /tmp: a VM-backed daemon (Rancher Desktop)
// does not mount /tmp, so a bind mount there is an empty directory the
// container writes nothing into and says nothing about.
const FRAMES = join(process.cwd(), 'demo', 'frames')

const MIME: Record<string, string> = {
  '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css',
  '.svg': 'image/svg+xml', '.woff2': 'font/woff2', '.json': 'application/json',
  '.png': 'image/png', '.ico': 'image/x-icon',
}

const run = promisify(execFile)

interface Frame {
  file: string
  /** Seconds this frame is held. */
  seconds: number
}

/** One beat's frames, kept together because a beat is what fades into the next. */
interface Segment {
  label: string
  frames: Frame[]
  /** The story marks ONE beat as the poster. Matching on the label was fragile:
   *  rewording a caption silently stopped selecting a frame. */
  poster: boolean
}

interface Harness {
  url: string
  /** Applies a change to what the server answers, then tells the console to refetch. */
  advance(patch?: (state: Install) => void): void
  /** Puts the install back to the opening, so the next theme records the same story. */
  reset(): void
  stop(): Promise<void>
}

async function startHarness(): Promise<Harness> {
  const dist = join(process.cwd(), 'dist')
  const live = new Set<ServerResponse>()
  let state = opening()
  let answer = responder(state)

  const server: Server = createServer(async (req, res) => {
    const [path, rawQuery] = (req.url ?? '/').split('?')

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

    // The console WRITES on two paths in this story: it marks the thread read
    // when it is opened, and it posts the reply. Both are answered, because a
    // refused write would put an error banner in the recording.
    if (req.method === 'POST' && path.startsWith('/api/')) {
      res.writeHead(200, { 'content-type': 'application/json' })
      res.end('{}')
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
    advance(patch) {
      patch?.(state)
      answer = responder(state)
      for (const res of live) res.write('event: resync\ndata: {"reason":"beat"}\n\n')
    },
    reset() {
      state = opening()
      answer = responder(state)
    },
    async stop() {
      for (const res of live) res.end()
      live.clear()
      server.closeAllConnections()
      await new Promise<void>((resolve) => server.close(() => resolve()))
    },
  }
}

/**
 * Screenshots the page once it has stopped changing.
 *
 * Same reason as the screenshot capture: the topology view lays itself out over
 * a frame or two, and a frame taken during that differs between runs.
 */
async function stableShot(page: Page): Promise<Buffer> {
  let previous: Buffer | null = null
  for (let attempt = 0; attempt < 12; attempt++) {
    const shot = await page.screenshot({ animations: 'disabled', caret: 'hide', scale: 'css' })
    if (previous && previous.equals(shot)) return shot
    previous = shot
    await page.waitForTimeout(200)
  }
  return previous as Buffer
}

/** SMIL is not covered by `animations: 'disabled'`, and freezes wherever it lands. */
async function freezeSvg(page: Page): Promise<void> {
  await page.evaluate(() => {
    document.querySelectorAll('svg').forEach((svg) => {
      if (typeof svg.pauseAnimations !== 'function') return
      svg.pauseAnimations()
      svg.setCurrentTime(0)
    })
  })
}

async function record(page: Page, harness: Harness, theme: string): Promise<Segment[]> {
  const segments: Segment[] = []
  let index = 0
  let current: Frame[] = []

  const shoot = async (seconds: number) => {
    await freezeSvg(page)
    const file = join(FRAMES, `${theme}-${String(index++).padStart(4, '0')}.png`)
    await writeFile(file, await stableShot(page))
    current.push({ file, seconds })
    return file
  }

  for (const beat of beats) {
    current = []
    await page.clock.setFixedTime(new Date(NOW.getTime() + beat.clock * 1000))
    harness.advance(beat.patch)

    if (beat.path) {
      await page.goto(`${harness.url}${beat.path}`)
    }
    // A tab is part of getting to the beat, so it is opened BEFORE the wait:
    // what the beat waits for is on the tab, not on the view that holds it.
    if (beat.act === 'yaml') {
      await page.getByText('YAML', { exact: true }).first().click()
    }
    if (beat.ready) {
      await page.getByText(beat.ready, { exact: false }).first().waitFor({ timeout: 30_000 })
    }
    await page.waitForLoadState('networkidle')
    await page.evaluate(() => document.fonts.ready)

    // The manifest sits below the metadata card, and the beat is ABOUT the
    // manifest. Without this the frame is eight rows of metadata and the first
    // four lines of the object it was meant to show.
    if (beat.act === 'yaml') {
      await page.evaluate(() => {
        document.querySelector('pre')?.scrollIntoView({ block: 'center' })
      })
    }

    if (beat.act === 'reply') {
      // Typed in runs rather than all at once: a reply that appears whole reads
      // as another server change, and this beat is the one where a PERSON acts.
      const box = page.getByPlaceholder(/reply/i).first()
      await box.click()
      const chunks = REPLY.match(/.{1,20}(\s|$)/g) ?? [REPLY]
      let typed = ''
      for (const chunk of chunks) {
        typed += chunk
        await box.fill(typed)
        await shoot(0.4)
      }
      await page.getByRole('button', { name: 'Send' }).click()
      // The post is answered, and the story says what the thread then holds.
      harness.advance(beat.patch)
      await page.getByText(REPLY, { exact: false }).first().waitFor({ timeout: 30_000 })
    }

    await shoot(beat.hold)
    segments.push({ label: beat.label, frames: current, poster: beat.poster === true })
  }

  // The poster is a beat's LAST frame — the one the beat rests on.
  const posterSegment = segments.find((segment) => segment.poster)
  if (!posterSegment) throw new Error('no beat is marked as the poster')
  const poster = posterSegment.frames[posterSegment.frames.length - 1]
  await writeFile(join(OUT, `console-demo-poster-${theme}.png`), await readFile(poster.file))

  return segments
}

/**
 * Where each beat sits in the finished recording.
 *
 * A beat's last frame gives up FADE_SECONDS to the fade that follows it, and the
 * fade frames take exactly that long, so the total is unchanged and a cue never
 * overlaps the next one.
 */
function timeline(segments: Segment[]): { start: number; end: number }[] {
  const spans: { start: number; end: number }[] = []
  let at = 0
  segments.forEach((segment, i) => {
    const held = segment.frames.reduce((total, f) => total + f.seconds, 0)
    const fading = i < segments.length - 1 ? FADE_SECONDS : 0
    spans.push({ start: at, end: at + held - fading })
    at += held
  })
  return spans
}

/** The whole recording's length. Fades are taken OUT of the beats they join. */
function duration(segments: Segment[]): number {
  return segments.reduce(
    (total, segment) => total + segment.frames.reduce((sum, f) => sum + f.seconds, 0), 0,
  )
}

/**
 * The caption track: the beats, as text, timed to the frames they describe.
 *
 * NOT burned in. A cue is a separate file the browser renders as selectable
 * text, so it can be translated, read aloud and turned off — everything text
 * inside a frame cannot be. It carries the SAME words as the list the page
 * prints under the player, from the same source, so the two cannot drift.
 *
 * One file for both themes. The words do not change with the palette, and a
 * `-light`/`-dark` stem would make the resolver rewrite a file that has no
 * second variant.
 */
async function writeCaptions(segments: Segment[]): Promise<void> {
  const stamp = (seconds: number) => {
    const whole = Math.floor(seconds)
    const ms = Math.round((seconds - whole) * 1000)
    const hh = String(Math.floor(whole / 3600)).padStart(2, '0')
    const mm = String(Math.floor((whole % 3600) / 60)).padStart(2, '0')
    const ss = String(whole % 60).padStart(2, '0')
    return `${hh}:${mm}:${ss}.${String(ms).padStart(3, '0')}`
  }

  const spans = timeline(segments)
  const cues = segments.map((segment, i) =>
    `${i + 1}\n${stamp(spans[i].start)} --> ${stamp(spans[i].end)}\n${segment.label}`,
  )
  await writeFile(join(OUT, 'console-demo.vtt'), `WEBVTT\n\n${cues.join('\n\n')}\n`)
}

/** Runs the pinned ffmpeg container over the frame directory. */
async function ffmpeg(args: string[]): Promise<void> {
  try {
    await run('docker', [
      'run', '--rm',
      '--user', `${process.getuid?.() ?? 0}:${process.getgid?.() ?? 0}`,
      '-v', `${FRAMES}:${FRAMES}`, '-v', `${OUT}:${OUT}`,
      FFMPEG_IMAGE, '-y', ...args,
    ], { maxBuffer: 32 * 1024 * 1024 })
  } catch (error) {
    const text = String(error)
    if (/manifest unknown|No such image|pull access denied|not found/i.test(text)) {
      throw new Error(
        `the ffmpeg image is not present. Pull it once, from an interactive shell:\n` +
        `  docker pull ${FFMPEG_IMAGE}\n` +
        `(a non-interactive pull fails on this workstation before it reaches the ` +
        `registry — the pass credential helper wants an unlocked gpg agent)`,
      )
    }
    throw error
  }
}

/**
 * Blends one beat's last frame into the next beat's first, as real PNG frames.
 *
 * THE FADE IS FRAMES, not a filter graph over the whole recording. An `xfade`
 * chain across six concat inputs was tried first and is where this stopped being
 * obvious: each concat of stills carries one frame per beat, so there was
 * nothing to interpolate and the transitions silently did nothing, and adding
 * `fps` per input made the arithmetic wrong in the other direction. Two stills
 * and one 0.5s xfade is a self-contained sum nobody has to reason about.
 */
async function fadeFrames(from: Frame, to: Frame, theme: string, index: number): Promise<Frame[]> {
  const pattern = join(FRAMES, `${theme}-fade-${index}-%03d.png`)
  await ffmpeg([
    '-loop', '1', '-t', String(FADE_SECONDS), '-i', from.file,
    '-loop', '1', '-t', String(FADE_SECONDS), '-i', to.file,
    '-filter_complex',
    `[0:v][1:v]xfade=transition=fade:duration=${FADE_SECONDS}:offset=0,fps=${FPS}`,
    '-f', 'image2', pattern,
  ])
  // One frame per output frame, so the fade takes exactly the time it says.
  const count = Math.round(FADE_SECONDS * FPS)
  return Array.from({ length: count }, (_, i) => ({
    file: join(FRAMES, `${theme}-fade-${index}-${String(i + 1).padStart(3, '0')}.png`),
    seconds: 1 / FPS,
  }))
}

/**
 * Encodes the beats, and the fades between them, into one MP4.
 *
 * THE WHOLE RECORDING IS LAID OUT AS A FIXED-RATE FRAME SEQUENCE, and a held
 * frame is HARDLINKED once per output frame. That is not the obvious way to do
 * it — the concat demuxer holds a still for a stated duration and needs one file
 * per beat — but concat rounds a duration up to its own timebase, so the 1/30s
 * fade frames each became several times longer and 40.6 seconds of story encoded
 * as 47.6. A sequence has no such arithmetic: frame count IS time.
 *
 * The links cost nothing. A thousand of them point at the ninety-odd PNGs that
 * were actually captured.
 */
async function encode(segments: Segment[], theme: string): Promise<void> {
  const sequence = join(FRAMES, `${theme}-seq`)
  await rm(sequence, { recursive: true, force: true })
  await mkdir(sequence, { recursive: true })

  let count = 0
  const place = async (file: string, repeats: number) => {
    for (let i = 0; i < repeats; i++) {
      await link(file, join(sequence, `${String(++count).padStart(5, '0')}.png`))
    }
  }

  for (const [i, segment] of segments.entries()) {
    const fading = i < segments.length - 1
    const frames = segment.frames.map((f) => ({ ...f }))
    const last = frames[frames.length - 1]
    if (fading) {
      // The fade is taken OUT of the beat it leaves, so the recording is the
      // same length with fades as without and the captions stay in step.
      if (last.seconds <= FADE_SECONDS) {
        throw new Error(`${segment.label}: holds ${last.seconds}s, too short to fade out of`)
      }
      last.seconds -= FADE_SECONDS
    }
    for (const frame of frames) await place(frame.file, Math.round(frame.seconds * FPS))
    if (fading) {
      const blend = await fadeFrames(last, segments[i + 1].frames[0], theme, i)
      for (const frame of blend) await place(frame.file, 1)
    }
  }

  await ffmpeg([
    '-framerate', String(FPS), '-i', join(sequence, '%05d.png'),
    '-vf', 'format=yuv420p',
    '-c:v', 'libx264', '-crf', '28', '-preset', 'slow',
    '-movflags', '+faststart',
    join(OUT, `console-demo-${theme}.mp4`),
  ])
}

async function checkBudgets(segments: Segment[], theme: string): Promise<void> {
  const seconds = duration(segments)
  if (seconds > MAX_SECONDS) {
    throw new Error(`${theme}: ${seconds}s of recording, over the ${MAX_SECONDS}s budget — shorten the story`)
  }
  const video = await stat(join(OUT, `console-demo-${theme}.mp4`))
  if (video.size > MAX_BYTES) {
    throw new Error(`${theme}: ${video.size} bytes, over the ${MAX_BYTES} budget — re-encode, do not raise it`)
  }
  const poster = await stat(join(OUT, `console-demo-poster-${theme}.png`))
  if (poster.size > MAX_POSTER_BYTES) {
    throw new Error(`${theme}: poster ${poster.size} bytes, over the ${MAX_POSTER_BYTES} budget`)
  }
  console.log(`${theme}: ${seconds.toFixed(1)}s, ${(video.size / 1024).toFixed(0)}KB video, ${(poster.size / 1024).toFixed(0)}KB poster`)
}

test.describe.configure({ mode: 'serial' })

test('records the landing page demo in both themes', async ({ browser }) => {
  test.setTimeout(600_000)

  const harness = await startHarness()
  await rm(FRAMES, { recursive: true, force: true })
  await mkdir(FRAMES, { recursive: true })
  await mkdir(OUT, { recursive: true })

  try {
    for (const theme of ['light', 'dark'] as const) {
      const context = await browser.newContext({
        viewport: { width: WIDTH, height: HEIGHT },
        colorScheme: theme,
        // A relative age and a formatted time are both locale-dependent, and a
        // recording made on another machine must show the same words.
        locale: 'en-GB',
        timezoneId: 'UTC',
        reducedMotion: 'reduce',
      })
      const page = await context.newPage()
      await page.clock.setFixedTime(NOW)
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

      const segments = await record(page, harness, theme)
      await encode(segments, theme)
      await writeCaptions(segments)
      await checkBudgets(segments, theme)
      await context.close()

      // Each theme starts the story from its opening, or the second one would
      // record an install that has already had everything happen to it.
      harness.reset()
    }
  } finally {
    await harness.stop()
  }
})
