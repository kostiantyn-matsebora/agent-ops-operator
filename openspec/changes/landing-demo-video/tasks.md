## 1. The fixture becomes layerable

- [ ] 1.1 Export the screenshot fixture's install state from
      `console/ui/screenshots/fixture.ts` alongside today's `answer()`, changing
      no value — verify `npm run screenshots` in `console/ui` regenerates the six
      views and `git status docs/assets/img/console/` reports NO modified file.
- [ ] 1.2 Add `console/ui/demo/story.ts`: the six beats over that install, each
      one a patch to what `/api/*` answers plus the fixed time it happens at —
      verify `npx tsc -b --noEmit` passes and every name in it is invented.

## 2. The recorder

- [ ] 2.1 Add `console/ui/demo/record.spec.ts` and its own
      `playwright.config.ts`, reusing the screenshot harness's server, clock
      pinning and animation kill at 1920×1200 — verify a run writes the frame
      files for one theme with no timeout.
- [ ] 2.2 Drive each beat by changing the served state and pushing one `resync`
      down the open stream, waiting on the console's OWN text for each — verify
      every beat's frame shows the state that beat introduces, and that no frame
      is produced by injecting DOM.
- [ ] 2.3 Capture one frame per state with its hold duration, and a short run of
      frames only where the composer is typed into — verify the written manifest
      lists every frame with a duration and totals ≤ 75 seconds.
- [ ] 2.4 Write the frames and manifest BESIDE the recorder, never under `/tmp`
      — verify the scratch directory is inside `console/ui/demo/` and is
      git-ignored.
- [ ] 2.5 Encode the manifest to MP4/H.264 through a pinned ffmpeg container
      mounted at its real path, and write the poster frame as PNG — verify both
      themes produce `docs/assets/video/console-demo-{light,dark}.mp4` and
      `console-demo-poster-{light,dark}.png`.
- [ ] 2.6 Fail with an instruction, not a registry error, when the ffmpeg image
      is absent — verify the message names the one-off interactive
      `docker pull`.
- [ ] 2.7 State the duration, byte and poster budgets beside the recorder and
      check them after encoding — verify a deliberately over-budget encode fails
      the run.
- [ ] 2.8 Add `npm run demo` to `console/ui/package.json`, outside `npm test` —
      verify `npm test` does not invoke it.

## 3. The theme

- [ ] 3.1 Extract the themed-variant painter from `docs/assets/js/tabs.js` into
      `docs/assets/js/themed.js`, registering by element reference — verify the
      landing diagram and the console-guide screenshots still swap on a theme
      toggle.
- [ ] 3.2 Extend the variant matcher to the video extension, still matching the
      suffix rather than assuming it — verify a panel link with no
      `-light`/`-dark` stem is left exactly as the page wrote it.
- [ ] 3.3 Add `docs/assets/js/player.js`: `{: .ao-demo}` on a link wrapping a
      poster image becomes `<video controls preload="none" poster>` carrying the
      image's alt text as its accessible name — verify with scripting disabled
      the panel is the poster image linking to the MP4.
- [ ] 3.4 Register the player's `src` and `poster` with the shared painter —
      verify toggling the theme mid-page swaps both, and that the recording is
      not fetched by the swap.
- [ ] 3.5 Add the `.ao-demo` frame to `docs/assets/css/agentops.css`, stating no
      colour literally — verify `grep -n '#[0-9a-fA-F]\{3,6\}'` on that file
      still hits only the token blocks.
- [ ] 3.6 Load `themed.js` and `player.js` from the layout as deferred scripts —
      verify the landing page's first panel is a working player and the console
      guide is unchanged.

## 4. The landing page

- [ ] 4.1 Replace the six console panels in `docs/index.md` with one recording
      panel, first in the strip, keeping the diagram and manifest panels in
      order — verify the strip renders three panels and the fragment links still
      resolve.
- [ ] 4.2 Write the panel's own words: what the recording shows, as the beats,
      plus the poster's alt text — verify no beat is burned into a frame and
      every word is in the page.
- [ ] 4.3 Replace the trailing "the last six are the console" sentence with the
      link to the Console page — verify the landing page names no console view
      and the Console page still tours all six.
- [ ] 4.4 Run the site's pre-flight prose lint from `docs/CLAUDE.md` over
      `index.md` — verify it is silent.

## 5. The rules that were made untrue

- [ ] 5.1 Remove the "landing strip and the guide's tour show the SAME six
      images" rule from `docs/CLAUDE.md` and state where each generated asset is
      published — verify the file names both commands and no rule claims the
      strip shows screenshots.
- [ ] 5.2 Add the player to the components table in `docs/CLAUDE.md`, in one
      line, beside `{: .ao-diagram}` — verify the form given renders.
- [ ] 5.3 Update the root `CLAUDE.md`: the `docs/` map's asset list, the
      screenshots-are-build-output paragraph, and the "A change to the console's
      UI" routing row so it names both commands — verify no sentence there still
      says the landing page shows screenshots.

## 6. Verification

- [ ] 6.1 Build the site's markdown as GitHub Pages does and open the landing
      page in both themes — verify the poster resolves per theme, the player
      plays, and nothing 404s at the project sub-path.
- [ ] 6.2 Confirm the six console screenshots are byte-identical to before the
      change and `docs/console-guide.md` is untouched — verify with
      `git diff --stat docs/assets/img/console/ docs/console-guide.md`.
- [ ] 6.3 Read every published frame for a real name — verify no cluster,
      namespace, host, identity or digest from any real installation appears.
- [ ] 6.4 Run `openspec validate landing-demo-video --type change --strict` —
      verify it passes.
