## Context

See `proposal.md` — Why. The constraints that shape the approach are already in
the repository:

- **The screenshots are produced, not captured.** `console/ui/screenshots/`
  serves the built bundle from `dist/` over a local HTTP server, answers
  `/api/*` from a curated fixture, pins the clock, kills animation, and writes
  byte-stable PNGs. The recording has to be the same kind of artifact.
- **The console repaints from a `resync` event.** `api/hooks.ts` wires the
  stream's `resync` to `client.invalidateQueries()`, so every query refetches.
  A harness that changes what it serves and pushes one `resync` gets a real
  repaint through the app's own data path.
- **A page carries no markup.** `{: .ao-tabs}`, `{: .ao-diagram}`,
  `{: .ao-cards}` — the page names a class, the theme draws the form.
- **The theme resolves themed assets** in `tabs.js`, matching `-light`/`-dark`
  before `.png` or `.svg` only, on panel images and panel links.
- **There is no ffmpeg on the workstation**, and no Node or Go either. Every
  toolchain here runs in a container.

## Goals / Non-Goals

**Goals:**

- One recording, reproducible from a committed command, that carries a single
  piece of work from signal to answered reply.
- The console reaching every beat the way it does in an install — served data
  plus stream events.
- A landing panel that costs a visitor nothing until they press play.
- No new rule for a page author: the recording is named exactly like a
  screenshot.

**Non-Goals:**

- **A narrated or captioned film.** No audio track, no title cards, no burned-in
  text. The page states the beats.
- **A byte-stable video.** The FRAMES are the reproducible artifact. An encoder
  is not required to produce identical bytes twice.
- **A second demo for the console page.** Its six screenshots stay as they are
  and are not re-cut into anything.
- **Recording a real cluster.** Same rule as the screenshots.

## Decisions

### Held frames plus a manifest, encoded by ffmpeg — not Playwright video

The recorder captures **one PNG per distinct state**, with a duration for each,
and a short run of frames only where something genuinely moves (text appearing
in the composer). An `ffmpeg` concat manifest turns that list into a
constant-rate MP4.

- *Alternative — Playwright `recordVideo`:* records the viewport as WebM with no
  control over pacing, no way to hold a frame, and a variable frame rate that
  changes with machine load. The result differs run to run in ways nobody can
  review, and the pacing of a demo is most of its quality.
- *Alternative — capture every frame at 12 fps:* ~900 screenshots per theme for
  a page that is static between beats. Slow, large, and no better.

The frames also give the poster for free: the poster is a named beat's frame.

### MP4/H.264 only, no WebM

H.264 in MP4 plays in every browser that reaches this site. Shipping a second
codec doubles the committed bytes to serve a case that does not exist.

### Two theme variants, named like every other themed asset

The recording and its poster ship `-light` and `-dark`. The page names the light
poster and the light recording, exactly as it names a light screenshot, and the
theme resolves both.

`tabs.js`'s variant matcher currently ends at `\.(png|svg)`. It gains the video
extension. **The suffix is still matched, never assumed**, so an ordinary link in
a panel is untouched.

### The themed painter is extracted, and the player is its own module

The player is not a tab, so it does not belong in `tabs.js`. But it needs the
same theme resolution, and two independent painters would disagree on a toggle.

So the resolution moves to a small `assets/js/themed.js` that both use: register
an element and an attribute, and it repaints them on `data-theme`. Registration
is by element REFERENCE, so tabs.js moving a node into a panel afterwards changes
nothing.

- *Alternative — put the player in `tabs.js`:* a file whose own opening line is
  "two tab components, one file" gains a third thing that is not a tab.
- *Alternative — a second painter in `player.js`:* two observers, one
  `data-theme` attribute, and a toggle that resolves one asset and not the other.

### The page names a link wrapping a poster, and the theme makes it a player

```markdown
[![<alt text, the page's words>](…/console-demo-poster-light.png)](…/console-demo-light.mp4){: .ao-demo}
```

`player.js` replaces it with a `<video controls preload="none" poster>` carrying
the image's alt text as its accessible name.

**With no script it is a poster image linking to the file** — which is a working
panel, not a fallback, and is the same bargain `{: .ao-tabs}` already makes.
`preload="none"` is what keeps the megabytes off a visitor who never presses
play.

### The demo fixture is a TIMELINE layered over the screenshot fixture

`console/ui/demo/story.ts` imports the screenshot fixture's install and defines
ordered **beats**. Each beat may change what `/api/*` answers and then push one
`resync` down the open stream. The console refetches and repaints itself.

`fixture.ts` therefore exports its state objects alongside today's frozen
`answer()`. **The screenshots' output must not move by one pixel** — the refactor
is exports only, and the six PNGs are re-generated and diffed to prove it.

- *Alternative — a second, independent fixture:* two invented installs that
  drift apart, and a landing page whose product does not match the console
  page's.
- *Alternative — the recorder paints the beats* (injecting DOM, faking a row):
  then the recording no longer shows the console working, which is the only
  thing it is for.

### The clock advances between beats, and stays pinned within one

Each beat sets a fixed time via `page.clock.setFixedTime`, later than the last.
Relative ages progress plausibly across the story and are identical on every run.

### The story is six beats, ending on the wiring

1. **Overview** — a healthy install, nothing happening.
2. **A cluster event is admitted** — the conversation list gains a row, unread.
3. **Inside the conversation** — the signal that opened it, a run in flight.
4. **The answer lands** — the run completes and the explanation is on screen.
5. **A person replies** — typed into the composer, relayed.
6. **Topology** — where that signal entered, which pipeline claimed it, where
   the answer went.

Budget: **≤ 75 seconds**, **≤ 4 MB per variant**, poster **≤ 400 KB**. The
numbers live beside the recorder, and a recording that exceeds one is shortened
or re-encoded rather than granted a bigger budget.

### No synthetic cursor

Beats are state changes the console itself makes visible — a new row, an unread
mark, a run's result, text in the composer. A drawn pointer is chrome the product
does not have, and it is one more thing to keep consistent between themes.

*Reconsider only if a beat reads as a jump cut*, and then as a theme-neutral
pointer element, never as a captioned callout.

### Capture geometry: 1920 × 1200, one to one

1920 is the width the screenshots already establish as the one where no column is
squeezed. Height is fixed at 1200 because a video cannot grow to its content, and
16:10 holds more of a view than 16:9. Encoded 1:1, so nothing is scaled.

### ffmpeg runs in a pinned container, beside the script

No local ffmpeg, and this workstation's daemon runs in a VM: a bind mount of
`/tmp` is an EMPTY directory in the container. So frames are written **beside the
recorder**, exactly as `docs/diagrams/export.py` does, and the container mounts
that directory at its real path.

`docker pull` from a non-interactive session fails on this workstation — the
`pass` credential helper wants an unlocked gpg agent — so the image is pulled
once, interactively. The recorder says so when the image is missing rather than
failing with a registry error.

## Risks / Trade-offs

- **A committed binary that changes with the UI** → one recording, one codec, a
  stated byte budget, and re-record only when a change makes the recording wrong
  — the same test the screenshots already carry.
- **The encoder is not deterministic, so a diff is unreviewable** → the review
  object is the beat script and the frames, not the container's output. Stated in
  the recorder, so nobody tries to make the MP4 reproducible.
- **A visitor never presses play** → the poster is a real beat of the product,
  the panel's prose states what the recording shows, and the diagram and manifest
  panels are one click away. Nothing on the page depends on the video being
  watched.
- **The demo fixture drifts from the screenshot fixture** → the demo layers over
  it rather than copying it, so a change to the install is made once.
- **The screenshots move while the fixture is refactored** → they are
  regenerated and diffed in the same task. A non-empty diff fails the task.
- **The story goes stale when the console changes** → it is a Playwright script
  waiting on the console's own text, so a renamed view or a moved control fails
  the recorder rather than producing a wrong recording.
- **A page author copies the video markup and gets it wrong** → `docs/CLAUDE.md`
  gains the component with its one-line form, next to `{: .ao-diagram}`.

## Migration Plan

Site-only. No chart version, no CRD, no image, therefore no `CHANGELOG.md`
entry and nothing for an operator to do.

- **Deploy** is the merge: GitHub Pages rebuilds `docs/` from `master`.
- **Rollback** is reverting the commit. The six console screenshots are untouched
  and stay referenced by `docs/console-guide.md`, so nothing else depends on the
  landing panels that leave.
