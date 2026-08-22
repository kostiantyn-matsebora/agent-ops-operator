## 1. The fixture becomes layerable

- [x] 1.1 Export the screenshot fixture's install state from
      `console/ui/screenshots/fixture.ts` alongside today's `answer()`, changing
      no value — verify `npm run screenshots` in `console/ui` regenerates the six
      views and `git status docs/assets/img/console/` reports NO modified file.
- [x] 1.2 Add `console/ui/demo/story.ts`: the six beats over that install, each
      one a patch to what `/api/*` answers plus the fixed time it happens at —
      verify `npx tsc -b --noEmit` passes and every name in it is invented.

## 2. The capture shows the whole view

- [x] 2.1 Grow the capture window until nothing inside the view scrolls —
      main's OWN overflow included — so the Conversation screenshot carries its
      reply box and Send button, verified by re-reading the written PNG.
- [x] 2.2 Trim back to where the content ends after growing, reverting the trim
      the moment it would clip — verify no view loses a control and none gains
      a screenful of empty ground.
- [x] 2.3 Re-run the capture once the console's UI settles and commit all twelve
      images — verify each view's controls are in frame in both themes.

## 3. The recorder

- [x] 3.1 Add `console/ui/demo/record.spec.ts` and its own
      `playwright.config.ts`, reusing the screenshot harness's server, clock
      pinning and animation kill at 1920×1200 — verify a run writes the frame
      files for one theme with no timeout.
- [x] 3.2 Drive each beat by changing the served state and pushing one `resync`
      down the open stream, waiting on the console's OWN text for each — verify
      every beat's frame shows the state that beat introduces, and that no frame
      is produced by injecting DOM.
- [x] 3.3 Capture one frame per state with its hold duration, and a short run of
      frames only where the composer is typed into — verify the written manifest
      lists every frame with a duration and totals ≤ 75 seconds.
- [x] 3.4 Write the frames and manifest BESIDE the recorder, never under `/tmp`
      — verify the scratch directory is inside `console/ui/demo/` and is
      git-ignored.
- [x] 3.5 Encode the manifest to MP4/H.264 through a pinned ffmpeg container
      mounted at its real path, and write the poster frame as PNG — verify both
      themes produce `docs/assets/video/console-demo-{light,dark}.mp4` and
      `console-demo-poster-{light,dark}.png`.
- [x] 3.6 Fail with an instruction, not a registry error, when the ffmpeg image
      is absent — verify the message names the one-off interactive
      `docker pull`.
- [x] 3.7 State the duration, byte and poster budgets beside the recorder and
      check them after encoding — verify a deliberately over-budget encode fails
      the run.
- [x] 3.8 Add `npm run demo` to `console/ui/package.json`, outside `npm test` —
      verify `npm test` does not invoke it.

## 3b. Fades and captions

- [x] 3b.1 Cross-fade between beats, taken OUT of the beat that fades — verify
      the encoded duration equals the sum of the held frames and a frame pulled
      mid-transition shows both beats blended.
- [x] 3b.2 Lay the recording out as a fixed-rate frame sequence of hardlinks
      rather than a concat of durations — verify a 40.6s story encodes to 40.6s.
- [x] 3b.3 Emit `console-demo.vtt` from the beat labels, one file for both
      themes — verify no two cues overlap and the words match the page's list.
- [x] 3b.4 Give the player a caption track from a `data-captions` the PAGE names
      — verify the track is default-on and the page still names every file.

## 3c. The story earns its claim

- [x] 3c.1 Name the beats for the PRODUCT, not the lane — verify no caption reads
      as though the tool is only for cluster events.
- [x] 3c.2 Add the queue, the wiring and the resource beats, ending on the
      conversation's own manifest — verify the YAML beat shows the object rather
      than the metadata card above it.
- [x] 3c.3 Write the fixture's agent answer in the format `format.md` mandates —
      verify the transcript renders as sections, not a wall of text.
- [x] 3c.4 Correct every manifest the fixture publishes to field names that exist
      on the CRDs — verify against `api/v1alpha1/*_types.go`.
- [x] 3c.5 Select the poster by a declared flag rather than by matching a label —
      verify rewording a caption still produces a poster.

## 4. The theme

- [x] 4.1 Extract the themed-variant painter from `docs/assets/js/tabs.js` into
      `docs/assets/js/themed.js`, registering by element reference — verify the
      landing diagram and the console-guide screenshots still swap on a theme
      toggle.
- [x] 4.2 Extend the variant matcher to the video extension, still matching the
      suffix rather than assuming it — verify a panel link with no
      `-light`/`-dark` stem is left exactly as the page wrote it.
- [x] 4.3 Add `docs/assets/js/player.js`: `{: .ao-demo}` on a link wrapping a
      poster image becomes `<video controls preload="none" poster>` carrying the
      image's alt text as its accessible name — verify with scripting disabled
      the panel is the poster image linking to the MP4.
- [x] 4.4 Register the player's `src` and `poster` with the shared painter —
      verify toggling the theme mid-page swaps both, and that the recording is
      not fetched by the swap.
- [x] 4.5 Add the `.ao-demo` frame to `docs/assets/css/agentops.css`, stating no
      colour literally — verify `grep -n '#[0-9a-fA-F]\{3,6\}'` on that file
      still hits only the token blocks.
- [x] 4.6 Load `themed.js` and `player.js` from the layout as deferred scripts —
      verify the landing page's first panel is a working player and the console
      guide is unchanged.

## 5. The landing page

- [x] 5.1 Replace the six console panels in `docs/index.md` with one recording
      panel, first in the strip, keeping the diagram and manifest panels in
      order — verify the strip renders three panels and the fragment links still
      resolve.
- [x] 5.2 Write the panel's own words: what the recording shows, as the beats,
      plus the poster's alt text — verify no beat is burned into a frame and
      every word is in the page.
- [x] 5.3 Replace the trailing "the last six are the console" sentence with the
      link to the Console page — verify the landing page names no console view
      and the Console page still tours all six.
- [x] 5.4 Run the site's pre-flight prose lint from `docs/CLAUDE.md` over
      `index.md` — verify it is silent.

## 6. The rules that were made untrue

- [x] 6.1 Remove the "landing strip and the guide's tour show the SAME six
      images" rule from `docs/CLAUDE.md` and state where each generated asset is
      published — verify the file names both commands and no rule claims the
      strip shows screenshots.
- [x] 6.2 Add the player to the components table in `docs/CLAUDE.md`, in one
      line, beside `{: .ao-diagram}` — verify the form given renders.
- [x] 6.3 Update the root `CLAUDE.md`: the `docs/` map's asset list, the
      screenshots-are-build-output paragraph, and the "A change to the console's
      UI" routing row so it names both commands — verify no sentence there still
      says the landing page shows screenshots.

## 7. Verification

- [x] 7.1 Build the site's markdown as GitHub Pages does and open the landing
      page in both themes — verify the poster resolves per theme, the player
      plays, and nothing 404s at the project sub-path.
- [x] 7.2 Confirm the six console screenshots are byte-identical to before the
      change and `docs/console-guide.md` is untouched — verify with
      `git diff --stat docs/assets/img/console/ docs/console-guide.md`.
- [x] 7.3 Read every published frame for a real name — verify no cluster,
      namespace, host, identity or digest from any real installation appears.
- [x] 7.4 Run `openspec validate landing-demo-video --type change --strict` —
      verify it passes.
