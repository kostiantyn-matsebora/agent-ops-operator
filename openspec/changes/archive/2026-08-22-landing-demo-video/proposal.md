# Landing demo video

## Why

The landing page shows the console as **six still images**, one per view, and a
still cannot show the one thing the product claims: that something happens and
an agent takes care of it. A reader sees six screens of a dashboard and has to
assemble the story themselves.

The console page shows those same six images at full length, with the question
each view answers. That is the right home for them. On the landing page they are
a second tour of the same screens, one altitude up, and they crowd out the pitch
they were meant to support.

A short recording of one incident — the signal arriving, the conversation
opening, the agent answering, a person replying — states the claim in the time a
visitor actually spends on a landing page.

## What Changes

- **The landing strip becomes three panels**, in order: the demo recording, the
  product diagram, the `Pipeline` manifest. The six console panels **leave the
  landing page**.

- **The console page is untouched.** Its tour keeps all six screenshots, and it
  becomes the only place they are published.

- **A demo recording is a new build output**, produced the same way the
  screenshots are: a committed command driving the console's own built bundle
  against a fixture it owns, writing committed files. Never a hand capture, and
  nothing named after a real installation.

- **The fixture gains a TIMELINE.** The screenshot fixture answers one frozen
  state. A story needs ordered beats, so the recorder layers scripted state
  changes and stream events over that same baseline install.

- **The recording ships one variant per theme**, like every other themed asset,
  and the page names one file.

- **It does not autoplay.** The panel is a poster frame the reader clicks. With
  no JavaScript it stays a poster image linking to the file, which is the same
  bargain the tab strip already makes.

- **The video element is drawn by the theme, not written into the page.** The
  page names the poster, the file and its words. Pages carry no markup.

- **`docs/CLAUDE.md` loses the rule that the landing strip and the console tour
  show the same six images.** It is no longer true, and it is stated in exactly
  one place.

- **The site's theme-variant rewrite covers the recording**, which today matches
  `.png` and `.svg` only.

## Capabilities

### New Capabilities

*(none — the site's existing capability owns both the landing page and how a
published product asset is produced)*

### Modified Capabilities

- `docs-site`: the landing strip's panel list changes from diagram + manifest +
  six console views to recording + diagram + manifest. A new requirement covers
  the recording as a generated asset — how it is produced, what it may show, its
  budget, and that it is delivered without autoplay. The themed-asset
  requirement extends past images to a recording.
- `documentation-structure`: the routing rule names the command that regenerates
  the recording, alongside the one that regenerates the screenshots, so a change
  to the console's UI has one stated destination for both.

## Impact

**The site.**

- `docs/index.md` — the strip loses six panels and gains one.
- `docs/assets/js/tabs.js` — the theme-variant rewrite learns the video
  extension, and upgrades a poster link into a player.
- `docs/assets/css/agentops.css` — the player's frame.
- `docs/assets/video/` — new directory, two recordings and two poster frames.
- `docs/CLAUDE.md` — the same-six-images rule and the build-output rule.

**The console module.**

- `console/ui/demo/` — new recorder beside `console/ui/screenshots/`, sharing
  its fixture and its harness.
- `console/ui/package.json` — one script.

**Not affected.**

- `docs/console-guide.md` and every screenshot it shows.
- `console/ui/screenshots/` output. The six PNGs stay exactly as they are.
- The chart, the operator, and every contract. No chart version, so no
  `CHANGELOG.md` entry.

**A tool dependency is added to the contributor path.** Encoding needs `ffmpeg`,
which this workstation does not have, so the recorder runs it in a container —
beside the script, never through `/tmp`, for the reason `docs/diagrams/export.py`
already documents.
