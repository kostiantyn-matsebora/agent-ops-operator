## 1. Publish the console's identity

- [x] 1.1 Add `global.agentops.console` (`signalSource`, `channel`, both
      defaulting to `console`) to `chart/values.yaml`, beside
      `global.agentops.runtime.*` and documented as the same kind of fact: the
      one scope a subchart can read.
- [x] 1.2 State in the comment that these MIRROR `console.enabled` /
      `console.signalSourceName` / `console.channelName`, that the duplication
      is unavoidable (values cannot derive from values), and that the render
      fails if they disagree.

## 2. Guard the duplication

- [x] 2.1 In `chart/templates/console.yaml`, read the published names BEFORE the
      `if .Values.console.enabled` gate — the disabled case is the one the guard
      most needs to catch, and the rest of that template does not render there.
- [x] 2.2 Fail when the console is enabled and the published source or channel
      names something else, with the exact value to set in the message.
- [x] 2.3 Fail when the console is disabled and either is still published,
      naming both values to clear.

## 3. Claim it from the route

- [x] 3.1 In `chart/charts/k8s-bundle/templates/pipelines.yaml`, append the
      published signal source to `$sources`, on the same terms `channels`
      already uses: a values-supplied name for a foreign object, omitted when
      unset.
- [x] 3.2 Append the published channel to the route's channels, merging with any
      the operator named rather than replacing them.
- [x] 3.3 Comment why: without the claim the console installs inert — source
      `Wired=False`, composer unavailable — and that this rides the EXISTING
      route because a second claimant would make every unaddressed console
      message ambiguous.

## 4. Verify by render

- [x] 4.1 Turnkey install: the route claims `cluster-events` AND `console`, and
      binds `channelRefs: [console]`.
- [x] 4.2 Console disabled with the globals left in place: render FAILS, message
      names both values to clear.
- [x] 4.3 Console disabled with the globals cleared: renders, route claims only
      `cluster-events`, no console channel.
- [x] 4.4 Console source renamed without updating the global: render FAILS,
      message names the value to set.
- [x] 4.5 Operator-named channel plus the console: both bound, neither replaced.
- [x] 4.6 Bundle wiring off (no route at all): nothing references the console,
      and the guard still passes.
- [x] 4.7 `go test ./internal/integration/...` — the chart render tests pin
      bundle behavior; update the ones that assert the route's refs.

## 5. Ship it

- [x] 5.1 Bump `chart/Chart.yaml` (minor — new default wiring, and an upgrade
      that can fail the render).
      **Not bumped here: 5.18.0 is already unreleased** and this change rides
      that same train. The bump from 5.17.0 belongs to the in-flight unread
      change, and a second bump would claim a release neither has cut. The
      CHANGELOG entry is keyed to 5.18.0 alongside it.
- [x] 5.2 `CHANGELOG.md`, newest first: an install that sets
      `console.enabled: false` MUST also clear
      `global.agentops.console.signalSource` and `.channel`, or the upgrade fails
      the render with a message saying so. Say why the failure is deliberate.
- [x] 5.3 `docs/k8s-bundle.md`: the route claims the console source and binds the
      console channel, and where those names come from.
- [x] 5.4 `docs/console.md`: the console is wired by the shipped route out of the
      box, and what disabling it requires.
- [x] 5.5 `docs/getting-started.md`: drop the `--set` and the `kubectl patch`
      the walkthrough needed while this was broken.
- [x] 5.6 `CLAUDE.md`: record that the console's identity is a `global.` fact
      with a render guard, and why the claim rides the existing route.
